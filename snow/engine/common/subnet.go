

package common

import (
	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// Subnet describes the standard interface of a subnet description
type Subnet interface {
	// Returns true iff the subnet is done bootstrapping
	IsBootstrapped() bool

	// Bootstrapped marks the named chain as being bootstrapped
	Bootstrapped(chainID ids.ID)
}
