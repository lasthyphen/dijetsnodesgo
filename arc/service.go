package arc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/rpc/v2/json2"

	"go.uber.org/zap"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
)

const (
	defaultPageSize   = 25
	maxPageSize       = 100
	maxEventPageSize  = 500
	requestTimeout    = 10 * time.Second
	maxConcurrentReqs = 16
	acquireTimeout    = time.Second

	// MembershipSource is the source of committee membership data.
	MembershipSource = RegistryAuthority
)

// JSON-RPC error codes returned by the arc service. They are stable
// across API versions.
const (
	CodeInternal      json2.ErrorCode = -32000
	CodeInvalidParams json2.ErrorCode = -32602
	CodeNotFound      json2.ErrorCode = -32004
	CodeNotArc        json2.ErrorCode = -32005
	CodeUnavailable   json2.ErrorCode = -32010
	CodeBusy          json2.ErrorCode = -32029
)

// ErrorData is attached to every error returned by the service.
type ErrorData struct {
	APIVersion string `json:"apiVersion"`
	Reason     string `json:"reason"`
}

func apiError(code json2.ErrorCode, reason, msg string) *json2.Error {
	return &json2.Error{
		Code:    code,
		Message: msg,
		Data:    ErrorData{APIVersion: APIVersion, Reason: reason},
	}
}

// Service is the arc.* JSON-RPC service. All methods are read-only.
type Service struct {
	log      logging.Logger
	registry Registry
	observer *Observer
	limiter  chan struct{}
}

// NewService returns the arc service and registers it as the
// observer's evaluator.
func NewService(log logging.Logger, registry Registry, observer *Observer) *Service {
	s := &Service{
		log:      log,
		registry: registry,
		observer: observer,
		limiter:  make(chan struct{}, maxConcurrentReqs),
	}
	observer.SetEvaluator(s)
	return s
}

func (s *Service) begin(r *http.Request) (context.Context, func(), error) {
	parent := context.Background()
	if r != nil {
		parent = r.Context()
	}
	timer := time.NewTimer(acquireTimeout)
	defer timer.Stop()
	select {
	case s.limiter <- struct{}{}:
	case <-timer.C:
		return nil, nil, apiError(CodeBusy, "too_many_concurrent_requests", "service busy, retry later")
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	return ctx, func() {
		cancel()
		<-s.limiter
	}, nil
}

func (s *Service) internal(method string, err error) error {
	s.log.Warn("arc API request failed",
		zap.String("method", method),
		zap.Error(err),
	)
	return apiError(CodeInternal, "internal_error", "internal error")
}

func (s *Service) mapRegistryErr(method string, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apiError(CodeNotFound, "not_found", "Arc not found")
	case errors.Is(err, ErrNotAnArc):
		return apiError(CodeNotArc, "not_an_arc", "chain is a primary network chain, not an Arc")
	default:
		return s.internal(method, err)
	}
}

func (s *Service) resolve(alias string) (ids.ID, error) {
	if alias == "" {
		return ids.Empty, apiError(CodeInvalidParams, "missing_chain_id", "chainID is required")
	}
	chainID, err := s.registry.LookupChain(alias)
	if err != nil {
		return ids.Empty, apiError(CodeInvalidParams, "invalid_chain_id", "chainID is not a valid chain ID or alias")
	}
	return chainID, nil
}

// Observation is this node's local, non-authoritative view of an Arc.
type Observation struct {
	Lifecycle Lifecycle `json:"lifecycle"`
	// Registered: authoritative, from M-chain state.
	Registered bool `json:"registered"`
	// Operational: this node runs the chain in normal operation.
	Operational bool `json:"operational"`
	// Healthy: every health input is satisfied on this node right now.
	Healthy       bool       `json:"healthy"`
	Reasons       []string   `json:"reasons"`
	TrackedByNode bool       `json:"trackedByNode"`
	ObservedAt    time.Time  `json:"observedAt"`
	ObserverNode  ids.NodeID `json:"observerNode"`
	Source        string     `json:"source"`
}

// ChainView combines authoritative and observed data of one Arc.
type ChainView struct {
	Descriptor  Descriptor  `json:"descriptor"`
	Observation Observation `json:"observation"`
}

type derived struct {
	view    ChainView
	eval    CommitteeEvaluation
	members []Member
	engine  EngineHealth
	engAt   time.Time
}

func (s *Service) tracked(subnetID ids.ID) bool {
	return !s.registry.StakingEnabled() || s.registry.TracksSubnet(subnetID)
}

func (s *Service) derive(rec Record, height uint64, members []Member, now time.Time) derived {
	params := s.observer.Params()
	eval := EvaluateCommittee(rec.Profile(), s.registry.StakingEnabled(), members, params)
	local, found := s.observer.LocalStatus(rec.ChainID)
	engine, engAt := s.observer.EngineHealth(rec.ChainID)
	tracked := s.tracked(rec.SubnetID)
	lifecycle, reasons := Derive(ObservationInput{
		Tracked:      tracked,
		LocalFound:   found,
		Local:        local,
		ShuttingDown: s.observer.ShuttingDown(),
		Engine:       engine,
		Committee:    eval,
	})
	if reasons == nil {
		reasons = []string{}
	}
	served := found && local.Created && !local.Stopped && local.CreationErr == nil
	return derived{
		view: ChainView{
			Descriptor: BuildDescriptor(rec, height, s.registry.NetworkID(), eval, params, served),
			Observation: Observation{
				Lifecycle:     lifecycle,
				Registered:    true,
				Operational:   lifecycle.IsOperational(),
				Healthy:       lifecycle.IsHealthy(),
				Reasons:       reasons,
				TrackedByNode: tracked,
				ObservedAt:    now,
				ObserverNode:  s.observer.NodeID(),
				Source:        SourceLocalObservation,
			},
		},
		eval:    eval,
		members: members,
		engine:  engine,
		engAt:   engAt,
	}
}

func (s *Service) getOne(ctx context.Context, method, alias string) (derived, uint64, error) {
	chainID, err := s.resolve(alias)
	if err != nil {
		return derived{}, 0, err
	}
	rec, height, err := s.registry.Chain(ctx, chainID)
	if err != nil {
		return derived{}, 0, s.mapRegistryErr(method, err)
	}
	members, err := s.registry.Committee(ctx, rec.SubnetID)
	if err != nil {
		return derived{}, 0, s.internal(method, err)
	}
	return s.derive(rec, height, members, time.Now().UTC()), height, nil
}

func (s *Service) deriveAll(ctx context.Context) ([]derived, uint64, error) {
	recs, height, err := s.registry.Chains(ctx)
	if err != nil {
		return nil, 0, err
	}
	committees := make(map[ids.ID][]Member)
	now := time.Now().UTC()
	out := make([]derived, 0, len(recs))
	for _, rec := range recs {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		members, ok := committees[rec.SubnetID]
		if !ok {
			members, err = s.registry.Committee(ctx, rec.SubnetID)
			if err != nil {
				return nil, 0, err
			}
			committees[rec.SubnetID] = members
		}
		out = append(out, s.derive(rec, height, members, now))
	}
	return out, height, nil
}

// CommitteeKey fingerprints the current committee membership.
func CommitteeKey(members []Member) string {
	entries := make([]string, 0, len(members))
	for _, m := range members {
		if m.Status != MemberCurrent {
			continue
		}
		entries = append(entries, fmt.Sprintf("%s:%d", m.NodeID, m.Weight))
	}
	sort.Strings(entries)
	h := sha256.Sum256([]byte(strings.Join(entries, ",")))
	return hex.EncodeToString(h[:8])
}

// EvaluateTracked implements Evaluator.
func (s *Service) EvaluateTracked(ctx context.Context) (int, []Evaluation, error) {
	all, _, err := s.deriveAll(ctx)
	if err != nil {
		return 0, nil, err
	}
	evals := make([]Evaluation, 0)
	for _, d := range all {
		if !d.view.Observation.TrackedByNode {
			continue
		}
		evals = append(evals, Evaluation{
			ChainID:      d.view.Descriptor.ID,
			SubnetID:     d.view.Descriptor.SubnetID,
			Lifecycle:    d.view.Observation.Lifecycle,
			Reasons:      d.view.Observation.Reasons,
			CommitteeKey: CommitteeKey(d.members),
		})
	}
	return len(all), evals, nil
}

// Envelope is embedded in every reply.
type Envelope struct {
	APIVersion    string `json:"apiVersion"`
	SchemaVersion uint32 `json:"schemaVersion"`
}

func envelope() Envelope {
	return Envelope{APIVersion: APIVersion, SchemaVersion: SchemaVersion}
}

// ListArgs are the arguments to arc.list.
type ListArgs struct {
	Lifecycle string `json:"lifecycle"`
	Owner     string `json:"owner"`
	VMID      string `json:"vmID"`
	Profile   string `json:"profile"`
	SubnetID  string `json:"subnetID"`
	Name      string `json:"name"`
	// Cursor is the nextCursor of the previous page.
	Cursor string      `json:"cursor"`
	Limit  json.Uint32 `json:"limit"`
}

// Counts have explicitly defined meanings, see each field.
type Counts struct {
	// Registered: Arcs registered in M-chain state (authoritative).
	Registered int `json:"registered"`
	// Tracked: registered Arcs this node is configured to run.
	Tracked int `json:"tracked"`
	// Operational: tracked Arcs in normal operation on this node.
	Operational int `json:"operational"`
	// Healthy: tracked Arcs whose health inputs are all satisfied.
	Healthy int `json:"healthy"`
	// ByLifecycle: local lifecycle of every registered Arc. Chains
	// this node does not run are counted as UNKNOWN.
	ByLifecycle map[Lifecycle]int `json:"byLifecycle"`
	// Matched: chains matching the request filters.
	Matched int `json:"matched"`
}

// ListReply is the reply of arc.list.
type ListReply struct {
	Envelope
	Chains       []ChainView `json:"chains"`
	NextCursor   string      `json:"nextCursor"`
	Counts       Counts      `json:"counts"`
	MChainHeight uint64      `json:"mChainHeight"`
	ObservedAt   time.Time   `json:"observedAt"`
	ObserverNode ids.NodeID  `json:"observerNode"`
}

func normaliseAddress(a string) string {
	if i := strings.IndexByte(a, '-'); i >= 0 {
		return a[i+1:]
	}
	return a
}

func ownerMatches(o Owner, filter string) bool {
	want := normaliseAddress(filter)
	for _, a := range o.Addresses {
		if normaliseAddress(a) == want {
			return true
		}
	}
	return false
}

func lifecycleValid(l Lifecycle) bool {
	for _, known := range AllLifecycles {
		if known == l {
			return true
		}
	}
	return false
}

// List returns a page of registered Arcs ordered by chain ID.
func (s *Service) List(r *http.Request, args *ListArgs, reply *ListReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	limit := int(args.Limit)
	switch {
	case limit == 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		return apiError(CodeInvalidParams, "limit_too_large", fmt.Sprintf("limit must be at most %d", maxPageSize))
	}
	var (
		cursor   ids.ID
		vmID     ids.ID
		subnetID ids.ID
	)
	if args.Cursor != "" {
		if cursor, err = ids.FromString(args.Cursor); err != nil {
			return apiError(CodeInvalidParams, "invalid_cursor", "cursor is invalid")
		}
	}
	if args.VMID != "" {
		if vmID, err = ids.FromString(args.VMID); err != nil {
			return apiError(CodeInvalidParams, "invalid_vm_id", "vmID is invalid")
		}
	}
	if args.SubnetID != "" {
		if subnetID, err = ids.FromString(args.SubnetID); err != nil {
			return apiError(CodeInvalidParams, "invalid_subnet_id", "subnetID is invalid")
		}
	}
	if args.Lifecycle != "" && !lifecycleValid(Lifecycle(args.Lifecycle)) {
		return apiError(CodeInvalidParams, "invalid_lifecycle", "unknown lifecycle")
	}
	if args.Profile != "" {
		if _, ok := ProfileInfoFor(Profile(args.Profile)); !ok {
			return apiError(CodeInvalidParams, "invalid_profile", "unknown security profile")
		}
	}

	all, height, err := s.deriveAll(ctx)
	if err != nil {
		return s.internal("list", err)
	}

	counts := Counts{ByLifecycle: make(map[Lifecycle]int, len(AllLifecycles))}
	for _, l := range AllLifecycles {
		counts.ByLifecycle[l] = 0
	}
	reply.Chains = []ChainView{}
	for _, d := range all {
		v := d.view
		counts.Registered++
		counts.ByLifecycle[v.Observation.Lifecycle]++
		if v.Observation.TrackedByNode {
			counts.Tracked++
		}
		if v.Observation.Operational {
			counts.Operational++
		}
		if v.Observation.Healthy {
			counts.Healthy++
		}

		switch {
		case args.Lifecycle != "" && string(v.Observation.Lifecycle) != args.Lifecycle,
			args.Owner != "" && !ownerMatches(v.Descriptor.Owner, args.Owner),
			args.VMID != "" && v.Descriptor.VMID != vmID,
			args.SubnetID != "" && v.Descriptor.SubnetID != subnetID,
			args.Profile != "" && string(v.Descriptor.ValidatorPolicy.Profile) != args.Profile,
			args.Name != "" && v.Descriptor.Name != args.Name:
			continue
		}
		counts.Matched++
		if args.Cursor != "" && bytes.Compare(v.Descriptor.ID[:], cursor[:]) <= 0 {
			continue
		}
		if len(reply.Chains) < limit {
			reply.Chains = append(reply.Chains, v)
			continue
		}
		if reply.NextCursor == "" {
			reply.NextCursor = reply.Chains[len(reply.Chains)-1].Descriptor.ID.String()
		}
	}

	reply.Envelope = envelope()
	reply.Counts = counts
	reply.MChainHeight = height
	reply.ObservedAt = time.Now().UTC()
	reply.ObserverNode = s.observer.NodeID()
	return nil
}

// ChainArgs identify an Arc by ID or alias.
type ChainArgs struct {
	ChainID string `json:"chainID"`
}

// GetReply is the reply of arc.get.
type GetReply struct {
	Envelope
	ChainView
}

// Get returns the descriptor and observation of one Arc.
func (s *Service) Get(r *http.Request, args *ChainArgs, reply *GetReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	d, _, err := s.getOne(ctx, "get", args.ChainID)
	if err != nil {
		return err
	}
	reply.Envelope = envelope()
	reply.ChainView = d.view
	return nil
}

// MemberView is the public representation of a committee member.
type MemberView struct {
	NodeID    ids.NodeID   `json:"nodeID"`
	TxID      ids.ID       `json:"txID"`
	Weight    json.Uint64  `json:"weight"`
	StartTime time.Time    `json:"startTime"`
	EndTime   time.Time    `json:"endTime"`
	Status    MemberStatus `json:"membershipStatus"`
	// Connected is a local observation; null when not observable.
	Connected *bool `json:"connected"`
	// UptimePercent is a local observation; null when not observable.
	UptimePercent *float64 `json:"uptimePercent"`
}

// CommitteeSummary summarises configured membership and observed
// connectivity.
type CommitteeSummary struct {
	CurrentCount  int         `json:"currentCount"`
	PendingCount  int         `json:"pendingCount"`
	CurrentWeight json.Uint64 `json:"currentWeight"`
	// The following are local observations, null when not observable.
	ConnectedCount          *int         `json:"connectedCount"`
	ConnectedWeight         *json.Uint64 `json:"connectedWeight"`
	ConnectedWeightFraction *float64     `json:"connectedWeightFraction"`
	ConnectivityObservable  bool         `json:"connectivityObservable"`
}

func summarise(eval CommitteeEvaluation) CommitteeSummary {
	sum := CommitteeSummary{
		CurrentCount:           eval.CurrentSize,
		PendingCount:           eval.PendingSize,
		CurrentWeight:          json.Uint64(eval.CurrentWeight),
		ConnectivityObservable: eval.ConnectivitySeen,
	}
	if eval.ConnectivitySeen {
		count := eval.ConnectedSize
		weight := json.Uint64(eval.ConnectedWeight)
		sum.ConnectedCount = &count
		sum.ConnectedWeight = &weight
		sum.ConnectedWeightFraction = eval.ConnectedFraction
	}
	return sum
}

// ValidatorsReply is the reply of arc.validators.
type ValidatorsReply struct {
	Envelope
	ChainID            ids.ID           `json:"chainID"`
	SubnetID           ids.ID           `json:"subnetID"`
	Policy             PolicyView       `json:"validatorPolicy"`
	Members            []MemberView     `json:"members"`
	Summary            CommitteeSummary `json:"summary"`
	Reasons            []string         `json:"reasons"`
	MembershipSource   string           `json:"membershipSource"`
	ConnectivitySource string           `json:"connectivitySource"`
	MChainHeight       uint64           `json:"mChainHeight"`
	ObservedAt         time.Time        `json:"observedAt"`
	ObserverNode       ids.NodeID       `json:"observerNode"`
}

// Validators returns the configured committee and observed connectivity.
func (s *Service) Validators(r *http.Request, args *ChainArgs, reply *ValidatorsReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	d, height, err := s.getOne(ctx, "validators", args.ChainID)
	if err != nil {
		return err
	}
	reply.Envelope = envelope()
	reply.ChainID = d.view.Descriptor.ID
	reply.SubnetID = d.view.Descriptor.SubnetID
	reply.Policy = d.view.Descriptor.ValidatorPolicy
	reply.Members = make([]MemberView, 0, len(d.members))
	for _, m := range d.members {
		reply.Members = append(reply.Members, MemberView{
			NodeID:        m.NodeID,
			TxID:          m.TxID,
			Weight:        json.Uint64(m.Weight),
			StartTime:     m.StartTime.UTC(),
			EndTime:       m.EndTime.UTC(),
			Status:        m.Status,
			Connected:     m.Connected,
			UptimePercent: m.UptimePercent,
		})
	}
	reply.Summary = summarise(d.eval)
	reply.Reasons = d.eval.Reasons
	if reply.Reasons == nil {
		reply.Reasons = []string{}
	}
	reply.MembershipSource = MembershipSource
	reply.ConnectivitySource = SourceLocalObservation
	reply.MChainHeight = height
	reply.ObservedAt = d.view.Observation.ObservedAt
	reply.ObserverNode = s.observer.NodeID()
	return nil
}

// StatsReply is the reply of arc.stats. Every value is a local
// observation of the answering node since [ObservedSince]; null values are
// unavailable, with the reason in [Unavailable].
type StatsReply struct {
	Envelope
	ChainID                 ids.ID            `json:"chainID"`
	Source                  string            `json:"source"`
	ObservedSince           *time.Time        `json:"observedSince"`
	ObservedAt              time.Time         `json:"observedAt"`
	ObserverNode            ids.NodeID        `json:"observerNode"`
	AcceptedTxs             *json.Uint64      `json:"acceptedTxs"`
	AcceptedContainers      *json.Uint64      `json:"acceptedContainers"`
	LastAcceptedAt          *time.Time        `json:"lastAcceptedAt"`
	LastAcceptedContainerID *ids.ID           `json:"lastAcceptedContainerID"`
	Height                  *json.Uint64      `json:"height"`
	CreatedAt               *time.Time        `json:"createdAt"`
	Unavailable             map[string]string `json:"unavailable"`
}

// Stats returns locally observed activity statistics.
func (s *Service) Stats(r *http.Request, args *ChainArgs, reply *StatsReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	d, _, err := s.getOne(ctx, "stats", args.ChainID)
	if err != nil {
		return err
	}
	chainID := d.view.Descriptor.ID
	reply.Envelope = envelope()
	reply.ChainID = chainID
	reply.Source = SourceLocalObservation
	reply.ObservedAt = time.Now().UTC()
	reply.ObserverNode = s.observer.NodeID()
	reply.Unavailable = map[string]string{
		"height":    UnavailableNoLinearHeight,
		"createdAt": UnavailableNotIndexed,
	}

	stats, ok := s.observer.Stats(chainID)
	if !ok {
		reason := UnavailableNotServedLocally
		if !d.view.Observation.TrackedByNode {
			reason = UnavailableNotTracked
		}
		for _, field := range []string{"observedSince", "acceptedTxs", "acceptedContainers", "lastAcceptedAt", "lastAcceptedContainerID"} {
			reply.Unavailable[field] = reason
		}
		return nil
	}
	since := stats.RegisteredAt
	txs := json.Uint64(stats.AcceptedTxs)
	containers := json.Uint64(stats.AcceptedContainers)
	reply.ObservedSince = &since
	reply.AcceptedTxs = &txs
	reply.AcceptedContainers = &containers
	if stats.LastAcceptedAt.IsZero() {
		reply.Unavailable["lastAcceptedAt"] = UnavailableNotObserved
		reply.Unavailable["lastAcceptedContainerID"] = UnavailableNotObserved
	} else {
		at := stats.LastAcceptedAt
		id := stats.LastAcceptedContainerID
		reply.LastAcceptedAt = &at
		reply.LastAcceptedContainerID = &id
	}
	return nil
}

// EngineView is the latest engine health sample.
type EngineView struct {
	Checked   bool       `json:"checked"`
	Healthy   *bool      `json:"healthy"`
	CheckedAt *time.Time `json:"checkedAt"`
}

// Dependencies reports the health of what the observation depends on.
type Dependencies struct {
	MChainBootstrapped bool       `json:"mChainBootstrapped"`
	ObserverLastPoll   *time.Time `json:"observerLastPoll"`
}

// HealthReply is the reply of arc.health.
type HealthReply struct {
	Envelope
	ChainID      ids.ID           `json:"chainID"`
	Observation  Observation      `json:"observation"`
	Engine       EngineView       `json:"engine"`
	Committee    CommitteeSummary `json:"committee"`
	Params       PolicyParams     `json:"params"`
	Dependencies Dependencies     `json:"dependencies"`
}

// Health returns the lifecycle and health inputs of one Arc.
func (s *Service) Health(r *http.Request, args *ChainArgs, reply *HealthReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	d, _, err := s.getOne(ctx, "health", args.ChainID)
	if err != nil {
		return err
	}
	reply.Envelope = envelope()
	reply.ChainID = d.view.Descriptor.ID
	reply.Observation = d.view.Observation
	reply.Engine.Checked = d.engine.Checked
	if d.engine.Checked {
		healthy := d.engine.Healthy
		at := d.engAt
		reply.Engine.Healthy = &healthy
		reply.Engine.CheckedAt = &at
	}
	reply.Committee = summarise(d.eval)
	reply.Params = s.observer.Params()
	reply.Dependencies.MChainBootstrapped = s.registry.Bootstrapped()
	if lastPoll := s.observer.LastPoll(); !lastPoll.IsZero() {
		reply.Dependencies.ObserverLastPoll = &lastPoll
	}
	return nil
}

// CapabilitiesArgs optionally select a chain.
type CapabilitiesArgs struct {
	ChainID string `json:"chainID"`
}

// ChainCapabilities describes one chain.
type ChainCapabilities struct {
	ChainID         ids.ID      `json:"chainID"`
	VMID            ids.ID      `json:"vmID"`
	VMName          string      `json:"vmName"`
	Capabilities    []string    `json:"capabilities"`
	PrivacyMode     PrivacyMode `json:"privacyMode"`
	ProtocolVersion string      `json:"protocolVersion"`
	APIs            []string    `json:"apis"`
}

// CapabilitiesReply is the reply of arc.capabilities.
type CapabilitiesReply struct {
	Envelope
	ProtocolVersion string             `json:"protocolVersion"`
	MetadataVersion uint32             `json:"metadataVersion"`
	Methods         []string           `json:"methods"`
	Profiles        []ProfileInfo      `json:"profiles"`
	SupportedVMs    []ids.ID           `json:"supportedVMs"`
	PrivacyModes    []PrivacyMode      `json:"privacyModes"`
	Lifecycles      []Lifecycle        `json:"lifecycles"`
	StakingEnabled  bool               `json:"stakingEnabled"`
	Chain           *ChainCapabilities `json:"chain"`
}

// Methods lists every method of the service.
var Methods = []string{
	"arc.list",
	"arc.get",
	"arc.validators",
	"arc.stats",
	"arc.health",
	"arc.capabilities",
	"arc.getCreationStatus",
	"arc.events",
}

// Capabilities describes the service and, optionally, one chain.
func (s *Service) Capabilities(r *http.Request, args *CapabilitiesArgs, reply *CapabilitiesReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	reply.Envelope = envelope()
	reply.ProtocolVersion = ProtocolVersion
	reply.MetadataVersion = MetadataVersion
	reply.Methods = Methods
	reply.Profiles = Profiles
	reply.SupportedVMs = SupportedVMs()
	reply.PrivacyModes = []PrivacyMode{PrivacyPublic}
	reply.Lifecycles = AllLifecycles
	reply.StakingEnabled = s.registry.StakingEnabled()
	if args.ChainID == "" {
		return nil
	}
	d, _, err := s.getOne(ctx, "capabilities", args.ChainID)
	if err != nil {
		return err
	}
	desc := d.view.Descriptor
	apis := []string{}
	for _, endpoint := range desc.RPCEndpoints {
		apis = append(apis, endpoint, endpoint+"/wallet", endpoint+"/events")
	}
	reply.Chain = &ChainCapabilities{
		ChainID:         desc.ID,
		VMID:            desc.VMID,
		VMName:          desc.VMName,
		Capabilities:    desc.Capabilities,
		PrivacyMode:     desc.PrivacyMode,
		ProtocolVersion: desc.ProtocolVersion,
		APIs:            apis,
	}
	return nil
}

// CreationStatusArgs are the arguments to arc.getCreationStatus.
type CreationStatusArgs struct {
	SubnetTxID     string   `json:"subnetTxID"`
	ChainTxID      string   `json:"chainTxID"`
	ValidatorTxIDs []string `json:"validatorTxIDs"`
}

// TxView is the M-chain status of one creation transaction.
type TxView struct {
	TxID   ids.ID   `json:"txID"`
	Status TxStatus `json:"status"`
}

// CreationStage values.
const (
	StageUnknown          = "UNKNOWN"
	StageSubnetPending    = "SUBNET_PENDING_ACCEPTANCE"
	StageSubnetRejected   = "SUBNET_REJECTED"
	StageSubnetRegistered = "SUBNET_REGISTERED"
	StageChainPending     = "CHAIN_PENDING_ACCEPTANCE"
	StageChainRejected    = "CHAIN_REJECTED"
	StageRegistered       = "REGISTERED"
)

// CreationStatusReply is the reply of arc.getCreationStatus.
type CreationStatusReply struct {
	Envelope
	Stage       string       `json:"stage"`
	Subnet      *TxView      `json:"subnet"`
	Chain       *TxView      `json:"chain"`
	Validators  []TxView     `json:"validators"`
	Observation *Observation `json:"observation"`
}

func stageFor(tx *TxView, pending, rejected, done string) string {
	switch tx.Status {
	case TxCommitted:
		return done
	case TxProcessing:
		return pending
	case TxDropped, TxAborted:
		return rejected
	default:
		return StageUnknown
	}
}

// GetCreationStatus reports the progress of a creation workflow from the
// M-chain status of its transactions.
func (s *Service) GetCreationStatus(r *http.Request, args *CreationStatusArgs, reply *CreationStatusReply) error {
	ctx, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	if args.SubnetTxID == "" && args.ChainTxID == "" {
		return apiError(CodeInvalidParams, "missing_tx_id", "subnetTxID or chainTxID is required")
	}
	if len(args.ValidatorTxIDs) > maxPageSize {
		return apiError(CodeInvalidParams, "too_many_tx_ids", "too many validator transaction IDs")
	}
	status := func(raw string) (*TxView, error) {
		txID, err := ids.FromString(raw)
		if err != nil {
			return nil, apiError(CodeInvalidParams, "invalid_tx_id", "transaction ID is invalid")
		}
		st, err := s.registry.TxStatus(ctx, txID)
		if err != nil {
			return nil, s.internal("getCreationStatus", err)
		}
		return &TxView{TxID: txID, Status: st}, nil
	}

	reply.Envelope = envelope()
	reply.Stage = StageUnknown
	reply.Validators = []TxView{}
	if args.SubnetTxID != "" {
		if reply.Subnet, err = status(args.SubnetTxID); err != nil {
			return err
		}
		reply.Stage = stageFor(reply.Subnet, StageSubnetPending, StageSubnetRejected, StageSubnetRegistered)
	}
	for _, raw := range args.ValidatorTxIDs {
		v, err := status(raw)
		if err != nil {
			return err
		}
		reply.Validators = append(reply.Validators, *v)
	}
	if args.ChainTxID == "" {
		return nil
	}
	if reply.Chain, err = status(args.ChainTxID); err != nil {
		return err
	}
	if reply.Chain.Status == TxUnknown && reply.Subnet != nil {
		return nil
	}
	reply.Stage = stageFor(reply.Chain, StageChainPending, StageChainRejected, StageRegistered)
	if reply.Chain.Status != TxCommitted {
		return nil
	}
	d, _, err := s.getOne(ctx, "getCreationStatus", args.ChainTxID)
	if err != nil {
		return err
	}
	if reply.Subnet != nil && d.view.Descriptor.SubnetID != reply.Subnet.TxID {
		return apiError(CodeInvalidParams, "subnet_mismatch", "chain is not registered on the given subnet")
	}
	reply.Observation = &d.view.Observation
	return nil
}

// EventsArgs are the arguments to arc.events.
type EventsArgs struct {
	After json.Uint64 `json:"after"`
	Limit json.Uint32 `json:"limit"`
}

// EventsReply is the reply of arc.events.
type EventsReply struct {
	Envelope
	EventPage
	ObserverNode ids.NodeID `json:"observerNode"`
}

// Events returns locally observed events after a cursor.
func (s *Service) Events(r *http.Request, args *EventsArgs, reply *EventsReply) error {
	_, done, err := s.begin(r)
	if err != nil {
		return err
	}
	defer done()

	limit := int(args.Limit)
	switch {
	case limit == 0:
		limit = defaultPageSize
	case limit > maxEventPageSize:
		return apiError(CodeInvalidParams, "limit_too_large", fmt.Sprintf("limit must be at most %d", maxEventPageSize))
	}
	reply.Envelope = envelope()
	reply.EventPage = s.observer.Events(uint64(args.After), limit)
	reply.ObserverNode = s.observer.NodeID()
	return nil
}
