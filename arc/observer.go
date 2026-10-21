package arc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"go.uber.org/zap"

	"github.com/lasthyphen/dijetsnodesgo/chains"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
	"github.com/lasthyphen/dijetsnodesgo/utils/metric"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
	"github.com/lasthyphen/dijetsnodesgo/utils/wrappers"
)

const (
	defaultPollInterval   = 5 * time.Second
	defaultEventCapacity  = 1024
	healthLockAttempts    = 5
	healthLockRetryPeriod = 20 * time.Millisecond

	decisionAcceptorName  = "arc-observer-decisions"
	consensusAcceptorName = "arc-observer-containers"
)

var (
	_ chains.Registrant = (*Observer)(nil)
	_ snow.Acceptor     = (*decisionAcceptor)(nil)
	_ snow.Acceptor     = (*consensusAcceptor)(nil)
)

// ChainStats are statistics observed by this node since it started. They are
// not persisted and are not authoritative.
type ChainStats struct {
	AcceptedTxs             uint64
	AcceptedContainers      uint64
	LastAcceptedAt          time.Time
	LastAcceptedContainerID ids.ID
	RegisteredAt            time.Time
}

type observedChain struct {
	ctx          *snow.ConsensusContext
	engine       common.Engine
	stats        ChainStats
	health       EngineHealth
	healthAt     time.Time
	lifecycle    Lifecycle
	committeeKey string
}

// Evaluation is the derived state of one tracked Arc, produced by the
// service for the observer's poll loop.
type Evaluation struct {
	ChainID      ids.ID
	SubnetID     ids.ID
	Lifecycle    Lifecycle
	Reasons      []string
	CommitteeKey string
}

// Evaluator derives the state of tracked Arcs. Implemented by Service.
type Evaluator interface {
	EvaluateTracked(ctx context.Context) (registered int, evals []Evaluation, err error)
}

// ObserverConfig configures an Observer.
type ObserverConfig struct {
	Log                    logging.Logger
	NodeID                 ids.NodeID
	DecisionAcceptorGroup  snow.AcceptorGroup
	ConsensusAcceptorGroup snow.AcceptorGroup
	Registerer             prometheus.Registerer
	PollInterval           time.Duration
	EventCapacity          int
	Params                 PolicyParams
}

// Observer collects local, non-authoritative observations about Arcs
// running on this node: acceptance statistics, engine health and lifecycle
// transitions. It never writes consensus state.
type Observer struct {
	log       logging.Logger
	nodeID    ids.NodeID
	decisions snow.AcceptorGroup
	consensus snow.AcceptorGroup
	params    PolicyParams
	interval  time.Duration
	startedAt time.Time
	bootID    string

	lock         sync.RWMutex
	chains       map[ids.ID]*observedChain
	evaluator    Evaluator
	localStatus  func(ids.ID) (chains.LocalChainStatus, bool)
	shuttingDown bool
	lastPoll     time.Time
	registered   int
	events       *eventLog
	// Key: chain ID. Value: when this node first observed the chain waiting
	// for or performing initialisation. Removed once it becomes operational
	// or fails.
	initStartedAt map[ids.ID]time.Time
	// Chains whose initialisation failure was already counted.
	initFailed set.Set[ids.ID]

	closeOnce sync.Once
	closing   chan struct{}
	done      chan struct{}

	metrics        *observerMetrics
	apiInterceptor metric.APIInterceptor
}

// NewObserver returns a new observer. Start must be called to begin polling.
func NewObserver(config ObserverConfig) (*Observer, error) {
	if config.PollInterval <= 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.EventCapacity <= 0 {
		config.EventCapacity = defaultEventCapacity
	}
	if config.Params == (PolicyParams{}) {
		config.Params = DefaultPolicyParams()
	}
	if config.Registerer == nil {
		config.Registerer = prometheus.NewRegistry()
	}
	metrics, err := newObserverMetrics(config.Registerer)
	if err != nil {
		return nil, err
	}
	interceptor, err := metric.NewAPIInterceptor("arc_api", config.Registerer)
	if err != nil {
		return nil, err
	}
	bootID, err := newBootID()
	if err != nil {
		return nil, err
	}
	return &Observer{
		log:       config.Log,
		nodeID:    config.NodeID,
		decisions: config.DecisionAcceptorGroup,
		consensus: config.ConsensusAcceptorGroup,
		params:    config.Params,
		interval:  config.PollInterval,
		startedAt: time.Now().UTC(),
		bootID:    bootID,
		chains:    make(map[ids.ID]*observedChain),
		events:    newEventLog(config.EventCapacity),
		closing:   make(chan struct{}),
		done:      make(chan struct{}),

		initStartedAt: make(map[ids.ID]time.Time),

		metrics:        metrics,
		apiInterceptor: interceptor,
	}, nil
}

func newBootID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("couldn't generate boot ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Params returns the reporting thresholds.
func (o *Observer) Params() PolicyParams { return o.params }

// NodeID returns the ID of the observing node.
func (o *Observer) NodeID() ids.NodeID { return o.nodeID }

// StartedAt returns when observation started.
func (o *Observer) StartedAt() time.Time { return o.startedAt }

// BootID identifies this observer instance. It changes on every node start
// so that event consumers can detect restarts.
func (o *Observer) BootID() string { return o.bootID }

// SetLocalStatusSource sets the chain manager accessor used for lifecycle
// observations.
func (o *Observer) SetLocalStatusSource(f func(ids.ID) (chains.LocalChainStatus, bool)) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.localStatus = f
}

// LocalStatus returns the chain manager's status of [chainID].
func (o *Observer) LocalStatus(chainID ids.ID) (chains.LocalChainStatus, bool) {
	o.lock.RLock()
	f := o.localStatus
	o.lock.RUnlock()
	if f == nil {
		return chains.LocalChainStatus{}, false
	}
	return f(chainID)
}

// SetEvaluator sets the evaluator used by the poll loop.
func (o *Observer) SetEvaluator(e Evaluator) {
	o.lock.Lock()
	defer o.lock.Unlock()
	o.evaluator = e
}

// RegisterChain implements chains.Registrant. It is called before the chain
// starts processing messages.
func (o *Observer) RegisterChain(_ string, engine common.Engine) {
	ctx := engine.Context()
	if ctx.SubnetID == constants.PrimaryNetworkID {
		return
	}

	o.lock.Lock()
	o.chains[ctx.ChainID] = &observedChain{
		ctx:    ctx,
		engine: engine,
		stats:  ChainStats{RegisteredAt: time.Now().UTC()},
	}
	o.lock.Unlock()

	errs := wrappers.Errs{}
	errs.Add(
		o.decisions.RegisterAcceptor(ctx.ChainID, decisionAcceptorName, (*decisionAcceptor)(o), false),
		o.consensus.RegisterAcceptor(ctx.ChainID, consensusAcceptorName, (*consensusAcceptor)(o), false),
	)
	if errs.Errored() {
		o.log.Warn("couldn't register Arc acceptors",
			zap.Stringer("chainID", ctx.ChainID),
			zap.Error(errs.Err),
		)
	}
}

type decisionAcceptor Observer

func (a *decisionAcceptor) Accept(ctx *snow.ConsensusContext, _ ids.ID, _ []byte) error {
	o := (*Observer)(a)
	o.lock.Lock()
	if c, ok := o.chains[ctx.ChainID]; ok {
		c.stats.AcceptedTxs++
	}
	o.lock.Unlock()
	o.metrics.acceptedTxs.Inc()
	return nil
}

type consensusAcceptor Observer

func (a *consensusAcceptor) Accept(ctx *snow.ConsensusContext, containerID ids.ID, _ []byte) error {
	o := (*Observer)(a)
	o.lock.Lock()
	if c, ok := o.chains[ctx.ChainID]; ok {
		c.stats.AcceptedContainers++
		c.stats.LastAcceptedAt = time.Now().UTC()
		c.stats.LastAcceptedContainerID = containerID
	}
	o.lock.Unlock()
	o.metrics.acceptedContainers.Inc()
	return nil
}

// Stats returns the statistics of [chainID] if this node runs the chain.
func (o *Observer) Stats(chainID ids.ID) (ChainStats, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()
	c, ok := o.chains[chainID]
	if !ok {
		return ChainStats{}, false
	}
	return c.stats, true
}

// EngineHealth returns the latest engine health of [chainID], when it was
// checked, and whether the result is fresh. Results older than three poll
// intervals are reported as not checked.
func (o *Observer) EngineHealth(chainID ids.ID) (EngineHealth, time.Time) {
	o.lock.RLock()
	defer o.lock.RUnlock()
	c, ok := o.chains[chainID]
	if !ok || c.healthAt.IsZero() || time.Since(c.healthAt) > 3*o.interval {
		return EngineHealth{}, timeOrZero(c)
	}
	return c.health, c.healthAt
}

func timeOrZero(c *observedChain) time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.healthAt
}

// ShuttingDown reports whether the node started shutting down.
func (o *Observer) ShuttingDown() bool {
	o.lock.RLock()
	defer o.lock.RUnlock()
	return o.shuttingDown
}

// LastPoll returns the time of the last completed poll.
func (o *Observer) LastPoll() time.Time {
	o.lock.RLock()
	defer o.lock.RUnlock()
	return o.lastPoll
}

// Start begins polling in the background.
func (o *Observer) Start() {
	go o.run()
}

// MarkShuttingDown records that node shutdown started.
func (o *Observer) MarkShuttingDown() {
	o.lock.Lock()
	o.shuttingDown = true
	o.lock.Unlock()
	o.closeOnce.Do(func() { close(o.closing) })
}

func (o *Observer) run() {
	defer close(o.done)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()
	for {
		select {
		case <-o.closing:
			return
		case <-ticker.C:
			o.safePoll()
		}
	}
}

func (o *Observer) safePoll() {
	defer func() {
		if r := recover(); r != nil {
			o.metrics.pollPanics.Inc()
			o.log.Error("Arc observer poll panicked",
				zap.Any("panic", r),
			)
		}
	}()
	o.Poll(context.Background())
}

// Poll samples engine health and records lifecycle transitions. Exported for
// tests; normally invoked by the background loop.
func (o *Observer) Poll(ctx context.Context) {
	o.lock.RLock()
	targets := make([]*observedChain, 0, len(o.chains))
	for _, c := range o.chains {
		targets = append(targets, c)
	}
	evaluator := o.evaluator
	o.lock.RUnlock()

	for _, c := range targets {
		o.sampleHealth(ctx, c)
	}

	if evaluator == nil {
		return
	}
	registered, evals, err := evaluator.EvaluateTracked(ctx)
	if err != nil {
		o.log.Debug("Arc evaluation failed", zap.Error(err))
		return
	}
	o.record(registered, evals)
}

func (o *Observer) sampleHealth(ctx context.Context, c *observedChain) {
	// The consensus engine is only initialised once the chain reaches normal
	// operation; before that, its health check must not be called. Engine
	// health is only an input to the lifecycle in normal operation anyway.
	if c.ctx.GetState() != snow.NormalOp {
		return
	}
	locked := false
	for i := 0; i < healthLockAttempts; i++ {
		if c.ctx.Lock.TryLock() {
			locked = true
			break
		}
		time.Sleep(healthLockRetryPeriod)
	}
	if !locked {
		// The chain is busy (e.g. executing bootstrapping transitions). Keep
		// the previous result; it expires after three poll intervals.
		o.metrics.healthChecksSkipped.Inc()
		return
	}
	healthy := o.checkEngine(ctx, c)

	o.lock.Lock()
	c.health = EngineHealth{Checked: true, Healthy: healthy}
	c.healthAt = time.Now().UTC()
	o.lock.Unlock()
}

// checkEngine runs the engine health check with the chain lock held. A panic
// in the check is contained and reported as unhealthy: observing an Arc
// must never take down the node.
func (o *Observer) checkEngine(ctx context.Context, c *observedChain) (healthy bool) {
	defer c.ctx.Lock.Unlock()
	defer func() {
		if r := recover(); r != nil {
			o.metrics.healthCheckPanics.Inc()
			o.log.Error("Arc engine health check panicked",
				zap.Stringer("chainID", c.ctx.ChainID),
				zap.Any("panic", r),
			)
			healthy = false
		}
	}()
	_, err := c.engine.HealthCheck(ctx)
	if err != nil {
		o.log.Debug("Arc engine unhealthy",
			zap.Stringer("chainID", c.ctx.ChainID),
			zap.Error(err),
		)
	}
	return err == nil
}

func (o *Observer) record(registered int, evals []Evaluation) {
	now := time.Now().UTC()
	counts := make(map[Lifecycle]int, len(AllLifecycles))

	o.lock.Lock()
	o.registered = registered
	o.lastPoll = now
	for _, e := range evals {
		counts[e.Lifecycle]++
		prevLifecycle, prevCommittee, seen := o.events.last(e.ChainID)
		if !seen {
			o.appendEvent(now, EventRegisteredObserved, e, "", e.Lifecycle)
			// A chain first observed past PENDING (e.g. already failed, or
			// active after a restart) also gets the event for its state.
			if e.Lifecycle != Pending && e.Lifecycle != Unknown {
				o.appendEvent(now, eventTypeFor(Unknown, e.Lifecycle), e, Unknown, e.Lifecycle)
			}
		} else if prevLifecycle != e.Lifecycle {
			if !ValidTransition(prevLifecycle, e.Lifecycle) {
				o.metrics.invalidTransitions.Inc()
				o.log.Warn("unexpected Arc lifecycle transition",
					zap.Stringer("chainID", e.ChainID),
					zap.String("from", string(prevLifecycle)),
					zap.String("to", string(e.Lifecycle)),
				)
			}
			o.appendEvent(now, eventTypeFor(prevLifecycle, e.Lifecycle), e, prevLifecycle, e.Lifecycle)
			o.metrics.transitions.WithLabelValues(string(e.Lifecycle)).Inc()
		}
		if seen && prevCommittee != e.CommitteeKey {
			o.appendEvent(now, EventValidatorMembershipChanged, e, e.Lifecycle, e.Lifecycle)
		}
		o.events.remember(e.ChainID, e.Lifecycle, e.CommitteeKey)
		o.recordInitialisation(now, e)
	}
	o.lock.Unlock()

	o.metrics.registered.Set(float64(registered))
	for _, l := range AllLifecycles {
		o.metrics.tracked.WithLabelValues(string(l)).Set(float64(counts[l]))
	}
}

// recordInitialisation measures how long tracked chains take to become
// operational, from the first poll that observed them as PENDING or
// INITIALISING. The resolution is the poll interval. Chains first observed
// already past initialisation (e.g. after a restart that finished before the
// first poll) are not measured. Must be called with [o.lock] held.
func (o *Observer) recordInitialisation(now time.Time, e Evaluation) {
	startedAt, measuring := o.initStartedAt[e.ChainID]
	switch {
	case e.Lifecycle == Pending || e.Lifecycle == Initialising:
		if !measuring {
			o.initStartedAt[e.ChainID] = now
		}
	case e.Lifecycle.IsOperational():
		if measuring {
			o.metrics.initDuration.Observe(now.Sub(startedAt).Seconds())
			delete(o.initStartedAt, e.ChainID)
		}
	case e.Lifecycle == Failed:
		delete(o.initStartedAt, e.ChainID)
		if o.initFailed.Contains(e.ChainID) {
			return
		}
		for _, r := range e.Reasons {
			if r == ReasonInitialisationFailed {
				// A chain manager does not retry a failed build, so each
				// chain is counted at most once per node start.
				o.initFailed.Add(e.ChainID)
				o.metrics.initFailures.Inc()
				return
			}
		}
	}
}

// appendEvent must be called with [o.lock] held.
func (o *Observer) appendEvent(now time.Time, t EventType, e Evaluation, from, to Lifecycle) {
	reasons := e.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	o.events.append(Event{
		SchemaVersion: SchemaVersion,
		BootID:        o.bootID,
		Type:          t,
		ChainID:       e.ChainID,
		SubnetID:      e.SubnetID,
		From:          from,
		To:            to,
		Reasons:       reasons,
		ObservedAt:    now,
		ObserverNode:  o.nodeID,
		Source:        SourceLocalObservation,
		Authoritative: false,
	})
}

// Events returns events with a sequence number greater than [after].
func (o *Observer) Events(after uint64, limit int) EventPage {
	o.lock.RLock()
	defer o.lock.RUnlock()
	return o.events.page(o.bootID, after, limit)
}

// Registered returns the number of registered Arcs seen by the last
// poll.
func (o *Observer) Registered() int {
	o.lock.RLock()
	defer o.lock.RUnlock()
	return o.registered
}

type observerMetrics struct {
	registered          prometheus.Gauge
	tracked             *prometheus.GaugeVec
	transitions         *prometheus.CounterVec
	invalidTransitions  prometheus.Counter
	acceptedTxs         prometheus.Counter
	acceptedContainers  prometheus.Counter
	healthChecksSkipped prometheus.Counter
	healthCheckPanics   prometheus.Counter
	pollPanics          prometheus.Counter
	initDuration        prometheus.Histogram
	initFailures        prometheus.Counter
}

func newObserverMetrics(reg prometheus.Registerer) (*observerMetrics, error) {
	const namespace = "arc"
	m := &observerMetrics{
		registered: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "registered",
			Help:      "Number of Arcs registered on the M-chain, as of the last poll",
		}),
		tracked: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "tracked_by_lifecycle",
			Help:      "Number of Arcs tracked by this node, by locally observed lifecycle",
		}, []string{"lifecycle"}),
		transitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "lifecycle_transitions",
			Help:      "Number of observed lifecycle transitions, by target lifecycle",
		}, []string{"lifecycle"}),
		invalidTransitions: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "invalid_lifecycle_transitions",
			Help:      "Number of observed lifecycle transitions not allowed by the state machine",
		}),
		acceptedTxs: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "accepted_txs",
			Help:      "Number of transactions accepted across all Arcs run by this node",
		}),
		acceptedContainers: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "accepted_containers",
			Help:      "Number of vertices/blocks accepted across all Arcs run by this node",
		}),
		healthChecksSkipped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "health_checks_skipped",
			Help:      "Number of Arc engine health samples skipped because the chain was busy",
		}),
		healthCheckPanics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "health_check_panics",
			Help:      "Number of Arc engine health checks that panicked and were contained",
		}),
		pollPanics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "poll_panics",
			Help:      "Number of observer polls that panicked and were contained",
		}),
		initDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "initialisation_duration_seconds",
			Help:      "Time from first observing a tracked Arc as PENDING or INITIALISING to observing it operational, at poll resolution",
			Buckets:   []float64{5, 10, 30, 60, 120, 300, 600, 1800, 3600},
		}),
		initFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "initialisation_failures",
			Help:      "Number of tracked Arcs whose initialisation failed on this node",
		}),
	}
	errs := wrappers.Errs{}
	errs.Add(
		reg.Register(m.registered),
		reg.Register(m.tracked),
		reg.Register(m.transitions),
		reg.Register(m.invalidTransitions),
		reg.Register(m.acceptedTxs),
		reg.Register(m.acceptedContainers),
		reg.Register(m.healthChecksSkipped),
		reg.Register(m.healthCheckPanics),
		reg.Register(m.pollPanics),
		reg.Register(m.initDuration),
		reg.Register(m.initFailures),
	)
	return m, errs.Err
}
