package minichain

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"
)

func newTestObserver(t *testing.T) *Observer {
	o, err := NewObserver(ObserverConfig{
		Log:                    logging.NoLog{},
		NodeID:                 ids.GenerateTestNodeID(),
		DecisionAcceptorGroup:  snow.NewAcceptorGroup(logging.NoLog{}),
		ConsensusAcceptorGroup: snow.NewAcceptorGroup(logging.NoLog{}),
	})
	require.NoError(t, err)
	return o
}

func registerTestChain(t *testing.T, o *Observer, health func(context.Context) (interface{}, error)) *snow.ConsensusContext {
	ctx := snow.DefaultConsensusContextTest()
	ctx.ChainID = ids.GenerateTestID()
	ctx.SubnetID = ids.GenerateTestID()
	o.RegisterChain("test", &common.EngineTest{
		T:        t,
		ContextF: func() *snow.ConsensusContext { return ctx },
		HealthF:  health,
	})
	return ctx
}

// Regression: the consensus engine's health check dereferences state that is
// only initialised once the chain reaches normal operation. Sampling it while
// bootstrapping crashed the node.
func TestObserverDoesNotCheckEngineBeforeNormalOp(t *testing.T) {
	require := require.New(t)
	o := newTestObserver(t)
	called := false
	ctx := registerTestChain(t, o, func(context.Context) (interface{}, error) {
		called = true
		return nil, nil
	})

	for _, state := range []snow.State{snow.Initializing, snow.StateSyncing, snow.Bootstrapping} {
		ctx.SetState(state)
		o.Poll(context.Background())
		require.False(called, state)
		health, _ := o.EngineHealth(ctx.ChainID)
		require.False(health.Checked)
	}

	ctx.SetState(snow.NormalOp)
	o.Poll(context.Background())
	require.True(called)
	health, _ := o.EngineHealth(ctx.ChainID)
	require.Equal(EngineHealth{Checked: true, Healthy: true}, health)
}

func TestObserverContainsHealthCheckPanics(t *testing.T) {
	require := require.New(t)
	o := newTestObserver(t)
	ctx := registerTestChain(t, o, func(context.Context) (interface{}, error) {
		panic("boom")
	})
	ctx.SetState(snow.NormalOp)

	require.NotPanics(func() { o.Poll(context.Background()) })
	health, _ := o.EngineHealth(ctx.ChainID)
	require.Equal(EngineHealth{Checked: true, Healthy: false}, health)

	// The chain lock was released despite the panic.
	require.True(ctx.Lock.TryLock())
	ctx.Lock.Unlock()
}

func TestObserverSkipsBusyChain(t *testing.T) {
	require := require.New(t)
	o := newTestObserver(t)
	called := false
	ctx := registerTestChain(t, o, func(context.Context) (interface{}, error) {
		called = true
		return nil, nil
	})
	ctx.SetState(snow.NormalOp)

	ctx.Lock.Lock()
	o.Poll(context.Background())
	ctx.Lock.Unlock()
	require.False(called, "a busy chain is not blocked on")
	health, _ := o.EngineHealth(ctx.ChainID)
	require.False(health.Checked)
}

func TestObserverShutdown(t *testing.T) {
	require := require.New(t)
	o := newTestObserver(t)
	o.Start()
	require.False(o.ShuttingDown())
	o.MarkShuttingDown()
	o.MarkShuttingDown() // idempotent
	require.True(o.ShuttingDown())
	<-o.done
}

func TestObserverInitialisationMetrics(t *testing.T) {
	o := newTestObserver(t)
	ok := Evaluation{ChainID: ids.GenerateTestID(), SubnetID: ids.GenerateTestID()}
	bad := Evaluation{ChainID: ids.GenerateTestID(), SubnetID: ids.GenerateTestID()}
	restarted := Evaluation{ChainID: ids.GenerateTestID(), SubnetID: ids.GenerateTestID(), Lifecycle: Active}

	at := func(e Evaluation, l Lifecycle, reasons ...string) Evaluation {
		e.Lifecycle = l
		e.Reasons = reasons
		return e
	}
	o.record(3, []Evaluation{at(ok, Pending, ReasonQueued), at(bad, Pending, ReasonQueued), restarted})
	o.record(3, []Evaluation{at(ok, Initialising, ReasonBootstrapping), at(bad, Failed, ReasonInitialisationFailed), restarted})
	o.record(3, []Evaluation{at(ok, Degraded, ReasonConnectedWeightTooLow), at(bad, Failed, ReasonInitialisationFailed), restarted})
	o.record(3, []Evaluation{at(ok, Active), at(bad, Failed, ReasonInitialisationFailed), restarted})

	// Only [ok] is measured: [restarted] was never observed initialising.
	m := &dto.Metric{}
	require.NoError(t, o.metrics.initDuration.Write(m))
	require.Equal(t, uint64(1), m.GetHistogram().GetSampleCount())
	// [bad] is counted once although it is observed FAILED three times.
	require.Equal(t, 1.0, testutil.ToFloat64(o.metrics.initFailures))
	require.Empty(t, o.initStartedAt)
}
