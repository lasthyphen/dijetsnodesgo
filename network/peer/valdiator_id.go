

package peer

import "github.com/lasthyphen/dijetsnodesgo/ids"

// ValidatorID represents a validator that we gossip to other peers
type ValidatorID struct {
	// The validator's ID
	NodeID ids.NodeID
	// The Tx that added this into the validator set
	TxID ids.ID
}
