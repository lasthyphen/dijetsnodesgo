// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/dijets/vertex"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"
	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common/queue"
)

type Config struct {
	common.Config
	common.AllGetsServer

	// VtxBlocked tracks operations that are blocked on vertices
	VtxBlocked *queue.JobsWithMissing
	// TxBlocked tracks operations that are blocked on transactions
	TxBlocked *queue.Jobs

	Manager vertex.Manager
	VM      vertex.DAGVM
}
