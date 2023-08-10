// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dijets

// Factory returns new instances of Consensus
type Factory interface {
	New() Consensus
}
