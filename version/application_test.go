

package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDefaultApplication(t *testing.T) {
	v := &Application{
		Major: 1,
		Minor: 2,
		Patch: 3,
	}

	require.Equal(t, "Dijets/1.2.3", v.String())
	require.Equal(t, "avalanche/1.2.3", v.HandshakeString())
	require.NoError(t, v.Compatible(v))
	require.False(t, v.Before(v))
}

func TestCurrentApplication(t *testing.T) {
	require.Equal(t, "Dijets/1.0.0", CurrentApp.String())
	require.Equal(t, "avalanche/1.0.0", CurrentApp.HandshakeString())
}

// 1.0.0 renumbered the 0.1.x line, so 1.x and 0.x nodes stay connectable; a
// later major version still is not.
func TestLegacyMajorVersion(t *testing.T) {
	v1 := &Application{Major: 1, Minor: 0, Patch: 0}
	v0 := &Application{Major: 0, Minor: 1, Patch: 3}
	v2 := &Application{Major: 2, Minor: 0, Patch: 0}

	require.NoError(t, v1.Compatible(v0))
	require.NoError(t, v0.Compatible(v1))
	require.True(t, v0.Before(v1))
	require.ErrorIs(t, v2.Compatible(v0), errDifferentMajor)
	require.ErrorIs(t, v2.Compatible(v1), errDifferentMajor)
}

// A 1.0.0 node accepts the 0.1.x peers its minimum compatible version allows.
func TestCurrentCompatibilityWithLegacyPeers(t *testing.T) {
	c := GetCompatibility(1)
	for _, peer := range []string{"avalanche/0.1.3", "avalanche/0.1.1", "Dijets/1.0.0"} {
		p, err := ParseApplication(peer)
		require.NoError(t, err)
		require.NoError(t, c.Compatible(p), peer)
	}
}

func TestComparingVersions(t *testing.T) {
	tests := []struct {
		myVersion   *Application
		peerVersion *Application
		compatible  bool
		before      bool
	}{
		{
			myVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			compatible: true,
			before:     false,
		},
		{
			myVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 4,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			compatible: true,
			before:     false,
		},
		{
			myVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 4,
			},
			compatible: true,
			before:     true,
		},
		{
			myVersion: &Application{
				Major: 1,
				Minor: 3,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			compatible: true,
			before:     false,
		},
		{
			myVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 3,
				Patch: 3,
			},
			compatible: true,
			before:     true,
		},
		{
			myVersion: &Application{
				Major: 2,
				Minor: 2,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			compatible: false,
			before:     false,
		},
		{
			myVersion: &Application{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			peerVersion: &Application{
				Major: 2,
				Minor: 2,
				Patch: 3,
			},
			compatible: true,
			before:     true,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %s", test.myVersion, test.peerVersion), func(t *testing.T) {
			err := test.myVersion.Compatible(test.peerVersion)
			if test.compatible && err != nil {
				t.Fatalf("Expected version to be compatible but returned: %s",
					err)
			} else if !test.compatible && err == nil {
				t.Fatalf("Expected version to be incompatible but returned no error")
			}
			before := test.myVersion.Before(test.peerVersion)
			if test.before && !before {
				t.Fatalf("Expected version to be before the peer version but wasn't")
			} else if !test.before && before {
				t.Fatalf("Expected version not to be before the peer version but was")
			}
		})
	}
}
