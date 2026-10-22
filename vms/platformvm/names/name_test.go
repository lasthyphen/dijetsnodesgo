package names

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		rule Rule // empty when valid
	}{
		{"alice", ""},
		{"bob", ""},
		{"dijets-cafe", ""},
		{"a1b", ""},
		{"abcdefghijklmnopqrstuvwxyz012345", ""},
		{"", RuleEmpty},
		{"ab", RuleTooShort},
		{"abcdefghijklmnopqrstuvwxyz0123456", RuleTooLong},
		{"Alice", RuleNotCanonical},
		{"alice.team", RuleHierarchical},
		{"al_ce", RuleCharacter},
		{"al ce", RuleCharacter},
		{"al\x00ce", RuleCharacter},
		{"alicé", RuleNonASCII},
		{"1alice", RuleFirstCharacter},
		{"-alice", RuleFirstCharacter},
		{"alice-", RuleLastCharacter},
		{"v-local1abc", RuleChainPrefix},
		{"m-alice", RuleChainPrefix},
		{"xn--alice", RuleDoubleHyphen},
		{"ali--ce", RuleDoubleHyphen},
		{"admin", RuleReserved},
		{"dijets", RuleReserved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.name)
			if test.rule == "" {
				require.NoError(t, err)
				return
			}
			var nameErr *Error
			require.ErrorAs(t, err, &nameErr)
			require.Equal(t, test.rule, nameErr.Rule)
			require.True(t, errors.Is(err, ErrInvalidName))
		})
	}
}

func TestCanonicalise(t *testing.T) {
	for input, want := range map[string]string{
		"alice":        "alice",
		"Alice":        "alice",
		"  ALICE  ":    "alice",
		"@alice":       "alice",
		"@Dijets-Cafe": "dijets-cafe",
	} {
		got, err := Canonicalise(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}

	// Lookalikes are rejected rather than folded onto an ASCII name: the
	// Cyrillic "а" in "аlice" and the Kelvin sign that Unicode case folding
	// would map to "k".
	for _, input := range []string{"аlice", "Kate", "ali​ce"} {
		_, err := Canonicalise(input)
		var nameErr *Error
		require.ErrorAs(t, err, &nameErr, input)
		require.Equal(t, RuleNonASCII, nameErr.Rule, input)
	}

	_, err := Canonicalise("@@alice")
	require.ErrorIs(t, err, ErrInvalidName)
}

func TestStatusAt(t *testing.T) {
	const grace = time.Minute
	at := func(sec uint64) time.Time { return time.Unix(int64(sec), 0) }
	r := &Record{Expiry: 1000, UpdatedAt: 500}

	require.Equal(t, StatusNotFound, (*Record)(nil).StatusAt(at(0), grace))
	require.Equal(t, StatusActive, r.StatusAt(at(999), grace))
	require.Equal(t, StatusExpired, r.StatusAt(at(1000), grace))
	require.Equal(t, StatusExpired, r.StatusAt(at(1059), grace))
	require.Equal(t, StatusReleased, r.StatusAt(at(1060), grace))

	revoked := &Record{Expiry: 1000, UpdatedAt: 500, Revoked: true}
	require.Equal(t, StatusRevoked, revoked.StatusAt(at(500), grace))
	require.Equal(t, StatusRevoked, revoked.StatusAt(at(559), grace))
	require.Equal(t, StatusReleased, revoked.StatusAt(at(560), grace))

	require.True(t, StatusActive.Resolvable())
	for _, s := range []Status{StatusNotFound, StatusExpired, StatusRevoked, StatusReleased} {
		require.False(t, s.Resolvable(), s)
	}
	require.True(t, StatusExpired.Updatable())
	require.False(t, StatusRevoked.Updatable())
	require.True(t, StatusReleased.Registrable())
	require.False(t, StatusExpired.Registrable())
	require.True(t, StatusRevoked.HoldsTarget())
	require.False(t, StatusReleased.HoldsTarget())
}

func TestPaymentMemo(t *testing.T) {
	m := PaymentMemo{Name: "alice", Generation: 1, Version: 3}
	require.Equal(t, "djtname/1 to=alice g=1 v=3", m.String())

	parsed, ok := ParsePaymentMemo([]byte(m.String()))
	require.True(t, ok)
	require.Equal(t, m, parsed)

	for _, memo := range []string{
		"",
		"djtname/1 to=alice g=1",
		"djtname/2 to=alice g=1 v=3",
		"djtname/1 to=Alice g=1 v=3",
		"djtname/1 to=alice g=0 v=3",
		"djtname/1 to=alice g=01 v=3",
		"djtname/1 to=alice g=1 v=3 ",
	} {
		_, ok := ParsePaymentMemo([]byte(memo))
		require.False(t, ok, memo)
	}
}

func TestClaimsAreDistinct(t *testing.T) {
	require.NotEqual(t, NameClaim("alice"), NameClaim("alicf"))
	require.Equal(t, NameClaim("alice"), NameClaim("alice"))
}
