

package snow

import "errors"

const (
	Initializing = iota
	StateSyncing
	Bootstrapping
	NormalOp
)

var ErrUnknownState = errors.New("unknown state")

type State uint8

func (st State) String() string {
	switch st {
	case Initializing:
		return "Initialising state"
	case StateSyncing:
		return "State syncing state"
	case Bootstrapping:
		return "Bootstrapping state"
	case NormalOp:
		return "Normal operations state"
	default:
		return "Unknown state"
	}
}
