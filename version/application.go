

package version

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// ApplicationPrefix names the node in its version string, as reported by
// info.getNodeVersion and --version.
const ApplicationPrefix = "Dijets/"

// LegacyApplicationPrefix is the prefix of the 0.1.x releases. Those nodes
// accept only this prefix in the peer handshake, so it is still sent there
// (see HandshakeString) and still accepted from peers.
const LegacyApplicationPrefix = "avalanche/"

var (
	errDifferentMajor = errors.New("different major version")

	_ fmt.Stringer = (*Semantic)(nil)
)

type Application struct {
	Major int `json:"major" yaml:"major"`
	Minor int `json:"minor" yaml:"minor"`
	Patch int `json:"patch" yaml:"patch"`

	str       atomic.Value
	legacyStr atomic.Value
}

// The only difference here between Application and Semantic is that Application
// prepends "Dijets/" rather than "v".
func (a *Application) String() string {
	strIntf := a.str.Load()
	if strIntf != nil {
		return strIntf.(string)
	}

	str := fmt.Sprintf(
		"%s%d.%d.%d",
		ApplicationPrefix,
		a.Major,
		a.Minor,
		a.Patch,
	)
	a.str.Store(str)
	return str
}

// HandshakeString is the version sent to peers. It keeps the legacy prefix so
// that nodes still running 0.1.x, which reject any other prefix, can connect.
// Switch it to String once no supported release requires the legacy prefix.
func (a *Application) HandshakeString() string {
	strIntf := a.legacyStr.Load()
	if strIntf != nil {
		return strIntf.(string)
	}

	str := fmt.Sprintf(
		"%s%d.%d.%d",
		LegacyApplicationPrefix,
		a.Major,
		a.Minor,
		a.Patch,
	)
	a.legacyStr.Store(str)
	return str
}

func (a *Application) Compatible(o *Application) error {
	switch {
	case protocolMajor(a.Major) > protocolMajor(o.Major):
		return errDifferentMajor
	default:
		return nil
	}
}

// protocolMajor treats the 0.x releases as major version 1: 1.0.0 renumbered
// the 0.1.x line without changing the protocol.
func protocolMajor(major int) int {
	if major == 0 {
		return 1
	}
	return major
}

func (a *Application) Before(o *Application) bool {
	return a.Compare(o) < 0
}

// Compare returns a positive number if s > o, 0 if s == o, or a negative number
// if s < o.
func (a *Application) Compare(o *Application) int {
	if a.Major != o.Major {
		return a.Major - o.Major
	}
	if a.Minor != o.Minor {
		return a.Minor - o.Minor
	}
	return a.Patch - o.Patch
}
