

package version

import (
	"fmt"
	"strconv"
	"strings"
)

func Parse(s string) (*Semantic, error) {
	if !strings.HasPrefix(s, "v") {
		return nil, fmt.Errorf("version string %q missing required prefix", s)
	}

	s = s[1:]
	major, minor, patch, err := parseVersions(s)
	if err != nil {
		return nil, err
	}

	return &Semantic{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// ParseApplication parses a peer's version. It accepts the current prefix and
// the legacy prefix of the 0.1.x releases.
func ParseApplication(s string) (*Application, error) {
	switch {
	case strings.HasPrefix(s, ApplicationPrefix):
		s = s[len(ApplicationPrefix):]
	case strings.HasPrefix(s, LegacyApplicationPrefix):
		s = s[len(LegacyApplicationPrefix):]
	default:
		return nil, fmt.Errorf("application string %q missing required prefix", s)
	}

	major, minor, patch, err := parseVersions(s)
	if err != nil {
		return nil, err
	}

	return &Application{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

func parseVersions(s string) (int, int, int, error) {
	splitVersion := strings.SplitN(s, ".", 3)
	if len(splitVersion) != 3 {
		return 0, 0, 0, fmt.Errorf("failed to parse %s as a version", s)
	}

	major, err := strconv.Atoi(splitVersion[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse %s as a version: %w", s, err)
	}

	minor, err := strconv.Atoi(splitVersion[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse %s as a version: %w", s, err)
	}

	patch, err := strconv.Atoi(splitVersion[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse %s as a version: %w", s, err)
	}

	return major, minor, patch, nil
}
