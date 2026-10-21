package minichain

import (
	"github.com/lasthyphen/dijetsnodesgo/chains"
	"github.com/lasthyphen/dijetsnodesgo/snow"
)

// Lifecycle is the lifecycle state of a mini-chain as observed by a single
// node. It is derived on demand from authoritative registration (M-chain
// state) and this node's chain manager and engine. It is never persisted and
// is never consensus state.
type Lifecycle string

const (
	// Pending: registered on the M-chain, this node is expected to run the
	// chain but has not built it yet (queued, or the M-chain is still
	// bootstrapping).
	Pending Lifecycle = "PENDING"
	// Initialising: the chain's VM and engine were built and the chain is
	// state syncing or bootstrapping.
	Initialising Lifecycle = "INITIALISING"
	// Active: the chain is in normal operation and every health input is
	// satisfied.
	Active Lifecycle = "ACTIVE"
	// Degraded: the chain is in normal operation but at least one health
	// input is not satisfied (engine health, committee size, connectivity).
	Degraded Lifecycle = "DEGRADED"
	// Stopping: the node is shutting down and the chain has not stopped yet.
	Stopping Lifecycle = "STOPPING"
	// Stopped: the chain's handler stopped as part of a node shutdown.
	Stopped Lifecycle = "STOPPED"
	// Failed: the chain could not be initialised, or stopped unexpectedly.
	Failed Lifecycle = "FAILED"
	// Unknown: the chain is registered but this node does not run it, so it
	// has no local observation. This is an observability state, not part of
	// the chain's state machine.
	Unknown Lifecycle = "UNKNOWN"
)

// Reason codes explaining a derived lifecycle state. They are stable,
// machine-readable and never contain internal error details.
const (
	ReasonNotTracked              = "node_not_tracking_subnet"
	ReasonNotStaged               = "creation_not_yet_staged"
	ReasonQueued                  = "queued_for_creation"
	ReasonInitialisationFailed    = "initialisation_failed"
	ReasonHandlerStopped          = "chain_handler_stopped_unexpectedly"
	ReasonNodeShuttingDown        = "node_shutting_down"
	ReasonStateSyncing            = "state_syncing"
	ReasonBootstrapping           = "bootstrapping"
	ReasonInitializing            = "initializing"
	ReasonEngineUnhealthy         = "engine_health_check_failing"
	ReasonEngineHealthUnknown     = "engine_health_not_observed"
	ReasonNoCommittee             = "no_current_committee"
	ReasonCommitteeBelowMinimum   = "committee_below_minimum_size"
	ReasonConnectedWeightTooLow   = "connected_committee_weight_below_threshold"
	ReasonConnectivityUnobserved  = "committee_connectivity_not_observed"
	ReasonCommitteeNotEnforced    = "committee_not_enforced_staking_disabled"
	ReasonProfileNotOperational   = "security_profile_not_operational"
	ReasonObservationUnsupported  = "observation_not_supported"
	ReasonRegistryUnavailable     = "registry_unavailable"
	ReasonCreationStatusUnknownTx = "unknown_transaction"
)

var validTransitions = map[Lifecycle][]Lifecycle{
	Pending:      {Initialising, Failed, Stopping, Stopped},
	Initialising: {Active, Degraded, Failed, Stopping, Stopped},
	Active:       {Degraded, Failed, Stopping, Stopped},
	Degraded:     {Active, Failed, Stopping, Stopped},
	Stopping:     {Stopped, Failed},
	Stopped:      {Pending},
	Failed:       {Pending},
}

// AllLifecycles lists every lifecycle value in a stable order.
var AllLifecycles = []Lifecycle{Pending, Initialising, Active, Degraded, Stopping, Stopped, Failed, Unknown}

// ValidTransition reports whether a chain may be observed moving from [from]
// to [to]. Self transitions are always valid. Transitions into or out of
// Unknown are always valid because they reflect this node starting or
// stopping to observe the chain, not a change of the chain itself.
func ValidTransition(from, to Lifecycle) bool {
	if from == to || from == Unknown || to == Unknown {
		return true
	}
	for _, next := range validTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// IsOperational reports whether the chain is running in normal operation on
// this node.
func (l Lifecycle) IsOperational() bool {
	return l == Active || l == Degraded
}

// IsHealthy reports whether every health input was satisfied.
func (l Lifecycle) IsHealthy() bool {
	return l == Active
}

// EngineHealth is the latest result of the chain engine's health check.
type EngineHealth struct {
	// Checked is false if the engine health was never observed.
	Checked bool
	Healthy bool
}

// ObservationInput collects every input used to derive a lifecycle state.
type ObservationInput struct {
	// Tracked is true if this node is configured to run the chain.
	Tracked bool
	// LocalFound is false if this node's chain manager does not know the
	// chain.
	LocalFound bool
	Local      chains.LocalChainStatus
	// ShuttingDown is true once node shutdown started.
	ShuttingDown bool
	Engine       EngineHealth
	Committee    CommitteeEvaluation
}

// Derive computes the local lifecycle state and the reasons for it. It is a
// pure function so that every caller derives identical results from
// identical inputs.
func Derive(in ObservationInput) (Lifecycle, []string) {
	switch {
	case !in.Tracked:
		return Unknown, []string{ReasonNotTracked}
	case !in.LocalFound:
		if in.ShuttingDown {
			return Stopped, []string{ReasonNodeShuttingDown}
		}
		return Pending, []string{ReasonNotStaged}
	case in.Local.CreationErr != nil:
		return Failed, []string{ReasonInitialisationFailed}
	case !in.Local.Created:
		if in.ShuttingDown {
			return Stopped, []string{ReasonNodeShuttingDown}
		}
		return Pending, []string{ReasonQueued}
	case in.Local.Stopped:
		if in.ShuttingDown {
			return Stopped, []string{ReasonNodeShuttingDown}
		}
		return Failed, []string{ReasonHandlerStopped}
	case in.ShuttingDown:
		return Stopping, []string{ReasonNodeShuttingDown}
	}

	switch in.Local.State {
	case snow.StateSyncing:
		return Initialising, []string{ReasonStateSyncing}
	case snow.Bootstrapping:
		return Initialising, []string{ReasonBootstrapping}
	case snow.NormalOp:
	default:
		return Initialising, []string{ReasonInitializing}
	}

	var reasons []string
	switch {
	case !in.Engine.Checked:
		reasons = append(reasons, ReasonEngineHealthUnknown)
	case !in.Engine.Healthy:
		reasons = append(reasons, ReasonEngineUnhealthy)
	}
	reasons = append(reasons, in.Committee.Reasons...)
	if len(reasons) > 0 {
		return Degraded, reasons
	}
	return Active, nil
}
