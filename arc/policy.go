package arc

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// Profile identifies a validator security profile.
type Profile string

const (
	// PermissionedCommittee: an owner-selected committee of primary network
	// validators, recorded on the M-chain through AddSubnetValidatorTx /
	// RemoveSubnetValidatorTx and enforced by the chain's consensus engine.
	PermissionedCommittee Profile = "permissioned_committee"
	// ExpandedCommittee: declared for future use. Not operational.
	ExpandedCommittee Profile = "expanded_committee"
	// SharedPrimarySecurity: declared for future use. Not operational. The
	// current protocol does not let the primary network validate or verify
	// Arc state, so this profile cannot be offered.
	SharedPrimarySecurity Profile = "shared_primary_security"
	// PermissionlessSubnet: the chain's subnet was transformed into a
	// permissionless subnet. Recognised but not supported by this prototype.
	PermissionlessSubnet Profile = "permissionless_subnet"
)

// Enforcement describes whether, and by what, a profile is enforced on the
// network this node is connected to.
type Enforcement string

const (
	// EnforcedByConsensus: only committee members' votes are sampled by the
	// chain's consensus engine and committee membership is M-chain state.
	EnforcedByConsensus Enforcement = "consensus"
	// NotEnforcedStakingDisabled: the node runs with staking disabled; every
	// connected peer is treated as a validator of every chain.
	NotEnforcedStakingDisabled Enforcement = "not_enforced_staking_disabled"
	// NotOperational: the profile is recognised but not supported.
	NotOperational Enforcement = "not_operational"
)

const (
	// DefaultMinCommitteeSize is the minimum number of current committee
	// members below which a chain is reported as degraded. It is a reporting
	// policy, not a protocol rule: the M-chain does not prevent the owner
	// from removing members.
	DefaultMinCommitteeSize = 3
	// DefaultMinConnectedWeight is the minimum fraction of committee weight
	// that must be observed connected for the chain to be reported healthy.
	// It mirrors the M-chain's default MinPercentConnectedStakeHealthy.
	DefaultMinConnectedWeight = 0.8
)

// PolicyParams are the reporting thresholds applied to committee
// evaluations.
type PolicyParams struct {
	MinCommitteeSize   int     `json:"minCommitteeSize"`
	MinConnectedWeight float64 `json:"minConnectedWeight"`
}

// DefaultPolicyParams returns the default reporting thresholds.
func DefaultPolicyParams() PolicyParams {
	return PolicyParams{
		MinCommitteeSize:   DefaultMinCommitteeSize,
		MinConnectedWeight: DefaultMinConnectedWeight,
	}
}

// Security property identifiers used in ProfileInfo.
const (
	PropertyRegistrationSecurity = "registration_security"
	PropertyConsensusSecurity    = "consensus_security"
	PropertyCheckpointing        = "checkpointing"
)

// ProfileInfo documents what a profile does and does not provide.
type ProfileInfo struct {
	Profile     Profile  `json:"profile"`
	Operational bool     `json:"operational"`
	Provides    []string `json:"provides"`
	Description string   `json:"description"`
	Limitations []string `json:"limitations"`
}

// Profiles lists every known profile. Only PermissionedCommittee is
// operational.
var Profiles = []ProfileInfo{
	{
		Profile:     PermissionedCommittee,
		Operational: true,
		Provides:    []string{PropertyRegistrationSecurity, PropertyConsensusSecurity},
		Description: "Committee of primary-network validators selected by the chain owner. Membership is M-chain state; the chain's DAG consensus samples only committee members.",
		Limitations: []string{
			"consensus security is bounded by the committee, not by the primary network",
			"a committee holding enough weight can finalise any state on the Arc",
			"members must opt in by tracking the subnet; non-participating members reduce liveness",
			"not enforced when the network runs with staking disabled",
			"no checkpointing of Arc state to the M-chain",
		},
	},
	{
		Profile:     ExpandedCommittee,
		Operational: false,
		Provides:    []string{},
		Description: "Reserved: larger committee with additional admission rules.",
		Limitations: []string{"not implemented"},
	},
	{
		Profile:     SharedPrimarySecurity,
		Operational: false,
		Provides:    []string{},
		Description: "Reserved: requires the primary network to validate or verify Arc state, which the protocol does not support.",
		Limitations: []string{"not implemented", "requires a protocol change"},
	},
	{
		Profile:     PermissionlessSubnet,
		Operational: false,
		Provides:    []string{PropertyRegistrationSecurity},
		Description: "Chain on a subnet transformed by TransformSubnetTx. Recognised for reporting only.",
		Limitations: []string{"not supported by the Arc prototype"},
	},
}

// ProfileInfoFor returns the documentation of [p].
func ProfileInfoFor(p Profile) (ProfileInfo, bool) {
	for _, info := range Profiles {
		if info.Profile == p {
			return info, true
		}
	}
	return ProfileInfo{}, false
}

// MemberStatus is the M-chain status of a committee member.
type MemberStatus string

const (
	MemberCurrent MemberStatus = "current"
	MemberPending MemberStatus = "pending"
)

// Member is a committee member as recorded on the M-chain, optionally
// annotated with this node's connectivity observation.
type Member struct {
	NodeID    ids.NodeID
	TxID      ids.ID
	Weight    uint64
	StartTime time.Time
	EndTime   time.Time
	Status    MemberStatus
	// Connected is nil when this node cannot observe the member's
	// connectivity to the subnet (it does not track the subnet).
	Connected *bool
	// UptimePercent is nil when unavailable.
	UptimePercent *float64
}

// CommitteeEvaluation is the result of evaluating a committee against a
// profile and the reporting thresholds.
type CommitteeEvaluation struct {
	Profile          Profile
	Enforcement      Enforcement
	CurrentSize      int
	PendingSize      int
	CurrentWeight    uint64
	ConnectedWeight  uint64
	ConnectedSize    int
	ConnectivitySeen bool
	// ConnectedFraction is nil when connectivity is not observable.
	ConnectedFraction *float64
	Reasons           []string
}

// EvaluateCommittee applies [params] to [members] for a chain secured by
// [profile].
func EvaluateCommittee(
	profile Profile,
	stakingEnabled bool,
	members []Member,
	params PolicyParams,
) CommitteeEvaluation {
	eval := CommitteeEvaluation{
		Profile:     profile,
		Enforcement: EnforcedByConsensus,
	}
	info, known := ProfileInfoFor(profile)
	switch {
	case !known || !info.Operational:
		eval.Enforcement = NotOperational
		eval.Reasons = append(eval.Reasons, ReasonProfileNotOperational)
	case !stakingEnabled:
		eval.Enforcement = NotEnforcedStakingDisabled
		eval.Reasons = append(eval.Reasons, ReasonCommitteeNotEnforced)
	}

	observable := true
	for _, m := range members {
		if m.Status == MemberPending {
			eval.PendingSize++
			continue
		}
		eval.CurrentSize++
		eval.CurrentWeight += m.Weight
		if m.Connected == nil {
			observable = false
			continue
		}
		if *m.Connected {
			eval.ConnectedSize++
			eval.ConnectedWeight += m.Weight
		}
	}
	eval.ConnectivitySeen = observable && eval.CurrentSize > 0

	switch {
	case eval.CurrentSize == 0:
		eval.Reasons = append(eval.Reasons, ReasonNoCommittee)
	case eval.CurrentSize < params.MinCommitteeSize:
		eval.Reasons = append(eval.Reasons, ReasonCommitteeBelowMinimum)
	}
	if eval.CurrentSize > 0 {
		if !eval.ConnectivitySeen {
			eval.Reasons = append(eval.Reasons, ReasonConnectivityUnobserved)
		} else {
			fraction := float64(eval.ConnectedWeight) / float64(eval.CurrentWeight)
			eval.ConnectedFraction = &fraction
			if fraction < params.MinConnectedWeight {
				eval.Reasons = append(eval.Reasons, ReasonConnectedWeightTooLow)
			}
		}
	}
	return eval
}
