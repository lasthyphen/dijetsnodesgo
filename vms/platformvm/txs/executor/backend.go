// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package executor

import (
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/snow/uptime"
	"github.com/lasthyphen/dijetsnodesgo/utils"
	"github.com/lasthyphen/dijetsnodesgo/utils/timer/mockable"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/config"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/reward"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/utxo"
)

type Backend struct {
	Config       *config.Config
	Ctx          *snow.Context
	Clk          *mockable.Clock
	Fx           fx.Fx
	FlowChecker  utxo.Verifier
	Uptimes      uptime.Manager
	Rewards      reward.Calculator
	Bootstrapped *utils.AtomicBool
}
