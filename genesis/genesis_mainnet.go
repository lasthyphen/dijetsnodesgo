// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package genesis

import (
	"time"

	_ "embed"

	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/reward"
)

var (
	//go:embed genesis_mainnet.json
	mainnetGenesisConfigJSON []byte

	// MainnetParams are the params used for mainnet
	MainnetParams = Params{
		TxFeeConfig: TxFeeConfig{
			TxFee:                         units.MilliDjtx,
			CreateAssetTxFee:              10 * units.MilliDjtx,
			CreateSubnetTxFee:             1 * units.Djtx,
			TransformSubnetTxFee:          1 * units.Djtx,
			CreateBlockchainTxFee:         1 * units.Djtx,
			AddPrimaryNetworkValidatorFee: 0,
			AddPrimaryNetworkDelegatorFee: 0,
			AddSubnetValidatorFee:         units.MilliDjtx,
			AddSubnetDelegatorFee:         units.MilliDjtx,
		},
		StakingConfig: StakingConfig{
			UptimeRequirement: .8, // 80%
			MinValidatorStake: 2 * units.KiloDjtx,
			MaxValidatorStake: 3 * units.MegaDjtx,
			MinDelegatorStake: 25 * units.Djtx,
			MinDelegationFee:  20000, // 2%
			MinStakeDuration:  2 * 7 * 24 * time.Hour,
			MaxStakeDuration:  365 * 24 * time.Hour,
			RewardConfig: reward.Config{
				MaxConsumptionRate: .12 * reward.PercentDenominator,
				MinConsumptionRate: .10 * reward.PercentDenominator,
				MintingPeriod:      365 * 24 * time.Hour,
				SupplyCap:          111111111 * units.Djtx,
			},
		},
	}
)
