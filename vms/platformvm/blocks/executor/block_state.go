// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package executor

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/chains/atomic"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/blocks"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state"
)

type standardBlockState struct {
	onAcceptFunc func()
	inputs       set.Set[ids.ID]
}

type proposalBlockState struct {
	initiallyPreferCommit bool
	onCommitState         state.Diff
	onAbortState          state.Diff
}

// The state of a block.
// Note that not all fields will be set for a given block.
type blockState struct {
	standardBlockState
	proposalBlockState
	statelessBlock blocks.Block
	onAcceptState  state.Diff

	timestamp      time.Time
	atomicRequests map[ids.ID]*atomic.Requests
}
