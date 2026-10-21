

package benchlist

import (
	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// Benchable is notified when a validator is benched or unbenched from a given chain
type Benchable interface {
	// Mark that [validatorID] has been benched on the given chain
	Benched(chainID ids.ID, validatorID ids.NodeID)
	// Mark that [validatorID] has been unbenched from the given chain
	Unbenched(chainID ids.ID, validatorID ids.NodeID)
}
