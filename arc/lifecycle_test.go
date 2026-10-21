package arc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/chains"
	"github.com/lasthyphen/dijetsnodesgo/snow"
)

func healthyCommittee() CommitteeEvaluation {
	connected := true
	members := []Member{
		{Weight: 1, Status: MemberCurrent, Connected: &connected},
		{Weight: 1, Status: MemberCurrent, Connected: &connected},
		{Weight: 1, Status: MemberCurrent, Connected: &connected},
	}
	return EvaluateCommittee(PermissionedCommittee, true, members, DefaultPolicyParams())
}

func normalOp() chains.LocalChainStatus {
	return chains.LocalChainStatus{Created: true, State: snow.NormalOp}
}

func TestDerive(t *testing.T) {
	healthy := EngineHealth{Checked: true, Healthy: true}
	tests := []struct {
		name      string
		in        ObservationInput
		lifecycle Lifecycle
		reasons   []string
	}{
		{
			name:      "not tracked",
			in:        ObservationInput{Tracked: false, LocalFound: true, Local: normalOp()},
			lifecycle: Unknown,
			reasons:   []string{ReasonNotTracked},
		},
		{
			name:      "not staged",
			in:        ObservationInput{Tracked: true},
			lifecycle: Pending,
			reasons:   []string{ReasonNotStaged},
		},
		{
			name:      "queued",
			in:        ObservationInput{Tracked: true, LocalFound: true},
			lifecycle: Pending,
			reasons:   []string{ReasonQueued},
		},
		{
			name: "creation failed",
			in: ObservationInput{Tracked: true, LocalFound: true, Local: chains.LocalChainStatus{
				CreationErr: errors.New("bad genesis"),
			}},
			lifecycle: Failed,
			reasons:   []string{ReasonInitialisationFailed},
		},
		{
			name: "bootstrapping",
			in: ObservationInput{Tracked: true, LocalFound: true, Local: chains.LocalChainStatus{
				Created: true, State: snow.Bootstrapping,
			}},
			lifecycle: Initialising,
			reasons:   []string{ReasonBootstrapping},
		},
		{
			name: "state syncing",
			in: ObservationInput{Tracked: true, LocalFound: true, Local: chains.LocalChainStatus{
				Created: true, State: snow.StateSyncing,
			}},
			lifecycle: Initialising,
			reasons:   []string{ReasonStateSyncing},
		},
		{
			name:      "active",
			in:        ObservationInput{Tracked: true, LocalFound: true, Local: normalOp(), Engine: healthy, Committee: healthyCommittee()},
			lifecycle: Active,
		},
		{
			name:      "engine never checked is degraded, not healthy",
			in:        ObservationInput{Tracked: true, LocalFound: true, Local: normalOp(), Committee: healthyCommittee()},
			lifecycle: Degraded,
			reasons:   []string{ReasonEngineHealthUnknown},
		},
		{
			name:      "engine unhealthy",
			in:        ObservationInput{Tracked: true, LocalFound: true, Local: normalOp(), Engine: EngineHealth{Checked: true}, Committee: healthyCommittee()},
			lifecycle: Degraded,
			reasons:   []string{ReasonEngineUnhealthy},
		},
		{
			name: "handler stopped unexpectedly",
			in: ObservationInput{Tracked: true, LocalFound: true, Local: chains.LocalChainStatus{
				Created: true, State: snow.NormalOp, Stopped: true,
			}},
			lifecycle: Failed,
			reasons:   []string{ReasonHandlerStopped},
		},
		{
			name: "stopped during shutdown",
			in: ObservationInput{Tracked: true, LocalFound: true, ShuttingDown: true, Local: chains.LocalChainStatus{
				Created: true, State: snow.NormalOp, Stopped: true,
			}},
			lifecycle: Stopped,
			reasons:   []string{ReasonNodeShuttingDown},
		},
		{
			name:      "stopping",
			in:        ObservationInput{Tracked: true, LocalFound: true, ShuttingDown: true, Local: normalOp()},
			lifecycle: Stopping,
			reasons:   []string{ReasonNodeShuttingDown},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lifecycle, reasons := Derive(test.in)
			require.Equal(t, test.lifecycle, lifecycle)
			require.Equal(t, test.reasons, reasons)
		})
	}
}

func TestRegisteredIsNeverHealthyWithoutObservation(t *testing.T) {
	// A registered chain that this node knows nothing about must not be
	// reported operational or healthy.
	for _, in := range []ObservationInput{
		{Tracked: false},
		{Tracked: true},
		{Tracked: true, LocalFound: true},
		{Tracked: true, LocalFound: true, Local: normalOp()},
	} {
		lifecycle, _ := Derive(in)
		require.False(t, lifecycle.IsHealthy(), "%+v", in)
	}
}

func TestValidTransition(t *testing.T) {
	require := require.New(t)

	valid := [][2]Lifecycle{
		{Pending, Initialising},
		{Initialising, Active},
		{Active, Degraded},
		{Degraded, Active},
		{Active, Stopping},
		{Stopping, Stopped},
		{Stopped, Pending},
		{Failed, Pending},
		{Pending, Failed},
		{Active, Active},
		{Unknown, Active},
		{Active, Unknown},
	}
	for _, tr := range valid {
		require.True(ValidTransition(tr[0], tr[1]), "%s -> %s", tr[0], tr[1])
	}
	invalid := [][2]Lifecycle{
		{Pending, Active},
		{Stopped, Active},
		{Failed, Active},
		{Active, Initialising},
		{Stopping, Active},
	}
	for _, tr := range invalid {
		require.False(ValidTransition(tr[0], tr[1]), "%s -> %s", tr[0], tr[1])
	}
}

func TestLifecyclePredicates(t *testing.T) {
	require := require.New(t)
	for _, l := range AllLifecycles {
		require.Equal(l == Active || l == Degraded, l.IsOperational(), l)
		require.Equal(l == Active, l.IsHealthy(), l)
	}
}
