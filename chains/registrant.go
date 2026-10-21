

package chains

import (
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
)

// Registrant can register the existence of a chain
type Registrant interface {
	// Called when the chain described by [engine] is created
	// This function is called before the chain starts processing messages
	// [engine] should be an avalanche.Engine or snowman.Engine
	RegisterChain(name string, engine common.Engine)
}
