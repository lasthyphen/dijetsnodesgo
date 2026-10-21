package minichain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestEvaluateCommittee(t *testing.T) {
	params := DefaultPolicyParams()
	tests := []struct {
		name        string
		profile     Profile
		staking     bool
		members     []Member
		enforcement Enforcement
		reasons     []string
		fraction    *float64
	}{
		{
			name:        "empty committee",
			profile:     PermissionedCommittee,
			staking:     true,
			enforcement: EnforcedByConsensus,
			reasons:     []string{ReasonNoCommittee},
		},
		{
			name:    "below minimum, all connected",
			profile: PermissionedCommittee,
			staking: true,
			members: []Member{
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
			},
			enforcement: EnforcedByConsensus,
			reasons:     []string{ReasonCommitteeBelowMinimum},
		},
		{
			name:    "connected weight below threshold",
			profile: PermissionedCommittee,
			staking: true,
			members: []Member{
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(false)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
			},
			enforcement: EnforcedByConsensus,
			reasons:     []string{ReasonConnectedWeightTooLow},
		},
		{
			name:    "connectivity not observable",
			profile: PermissionedCommittee,
			staking: true,
			members: []Member{
				{Weight: 10, Status: MemberCurrent},
				{Weight: 10, Status: MemberCurrent},
				{Weight: 10, Status: MemberCurrent},
			},
			enforcement: EnforcedByConsensus,
			reasons:     []string{ReasonConnectivityUnobserved},
		},
		{
			name:    "pending members do not count",
			profile: PermissionedCommittee,
			staking: true,
			members: []Member{
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberPending, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberPending, Connected: boolPtr(true)},
			},
			enforcement: EnforcedByConsensus,
			reasons:     []string{ReasonCommitteeBelowMinimum},
		},
		{
			name:    "staking disabled is not enforced",
			profile: PermissionedCommittee,
			staking: false,
			members: []Member{
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
			},
			enforcement: NotEnforcedStakingDisabled,
			reasons:     []string{ReasonCommitteeNotEnforced},
		},
		{
			name:        "unsupported profile is not operational",
			profile:     PermissionlessSubnet,
			staking:     true,
			enforcement: NotOperational,
			reasons:     []string{ReasonProfileNotOperational, ReasonNoCommittee},
		},
		{
			name:    "healthy",
			profile: PermissionedCommittee,
			staking: true,
			members: []Member{
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
				{Weight: 10, Status: MemberCurrent, Connected: boolPtr(true)},
			},
			enforcement: EnforcedByConsensus,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eval := EvaluateCommittee(test.profile, test.staking, test.members, params)
			require.Equal(t, test.enforcement, eval.Enforcement)
			require.Equal(t, test.reasons, eval.Reasons)
		})
	}
}

func TestOnlyPermissionedCommitteeIsOperational(t *testing.T) {
	for _, p := range Profiles {
		require.Equal(t, p.Profile == PermissionedCommittee, p.Operational, p.Profile)
		if !p.Operational {
			require.NotContains(t, p.Provides, PropertyConsensusSecurity, p.Profile)
		}
	}
	info, ok := ProfileInfoFor(SharedPrimarySecurity)
	require.True(t, ok)
	require.False(t, info.Operational)
}
