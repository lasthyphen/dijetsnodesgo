// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

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
	v, err := ParseApplication("dijets/1.2.3")

	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "dijets/1.2.3", v.String())
	require.Equal(t, 1, v.Major)
	require.Equal(t, 2, v.Minor)
	require.Equal(t, 3, v.Patch)
	require.NoError(t, v.Compatible(v))
	require.False(t, v.Before(v))

	badVersions := []string{
		"",
		"dijets/",
		"dijets/z.0.0",
		"dijets/0.z.0",
		"dijets/0.0.z",
	}
	for _, badVersion := range badVersions {
		_, err := ParseApplication(badVersion)
		require.Error(t, err)
	}
}
