

package version

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	v, err := Parse("v1.2.3")

	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "v1.2.3", v.String())
	require.Equal(t, 1, v.Major)
	require.Equal(t, 2, v.Minor)
	require.Equal(t, 3, v.Patch)

	badVersions := []string{
		"",
		"1.2.3",
		"vz.2.3",
		"v1.z.3",
		"v1.2.z",
	}
	for _, badVersion := range badVersions {
		_, err := Parse(badVersion)
		require.Error(t, err)
	}
}

func TestParseApplication(t *testing.T) {
	v, err := ParseApplication("Dijets/1.2.3")

	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "Dijets/1.2.3", v.String())
	require.Equal(t, 1, v.Major)
	require.Equal(t, 2, v.Minor)
	require.Equal(t, 3, v.Patch)
	require.NoError(t, v.Compatible(v))
	require.False(t, v.Before(v))

	// Peers on 0.1.x send the legacy prefix.
	legacy, err := ParseApplication("avalanche/0.1.3")
	require.NoError(t, err)
	require.Equal(t, &Application{Major: 0, Minor: 1, Patch: 3}, legacy)

	badVersions := []string{
		"",
		"Dijets/",
		"Dijets/z.0.0",
		"Dijets/0.z.0",
		"Dijets/0.0.z",
		"avalanche/",
		"dijets/1.2.3",
		"avalanchego/1.2.3",
		"v1.2.3",
	}
	for _, badVersion := range badVersions {
		_, err := ParseApplication(badVersion)
		require.Error(t, err)
	}
}
