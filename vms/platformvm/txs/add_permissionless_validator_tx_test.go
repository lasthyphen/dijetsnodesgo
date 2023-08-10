// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"errors"
	"math"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto/bls"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/fx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/reward"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/signer"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/validator"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

func TestAddPermissionlessValidatorTxSyntacticVerify(t *testing.T) {
	type test struct {
		name   string
		txFunc func(*gomock.Controller) *AddPermissionlessValidatorTx
		err    error
	}

	var (
		networkID = uint32(1337)
		chainID   = ids.GenerateTestID()
	)

	ctx := &snow.Context{
		ChainID:   chainID,
		NetworkID: networkID,
	}

	// A BaseTx that already passed syntactic verification.
	verifiedBaseTx := BaseTx{
		SyntacticallyVerified: true,
	}

	// A BaseTx that passes syntactic verification.
	validBaseTx := BaseTx{
		BaseTx: djtx.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
		},
	}

	blsSK, err := bls.NewSecretKey()
	require.NoError(t, err)

	blsPOP := signer.NewProofOfPossession(blsSK)

	// A BaseTx that fails syntactic verification.
	invalidBaseTx := BaseTx{}

	errCustom := errors.New("custom error")

	tests := []test{
		{
			name: "nil tx",
			txFunc: func(*gomock.Controller) *AddPermissionlessValidatorTx {
				return nil
			},
			err: ErrNilTx,
		},
		{
			name: "already verified",
			txFunc: func(*gomock.Controller) *AddPermissionlessValidatorTx {
				return &AddPermissionlessValidatorTx{
					BaseTx: verifiedBaseTx,
				}
			},
			err: nil,
		},
		{
			name: "empty nodeID",
			txFunc: func(*gomock.Controller) *AddPermissionlessValidatorTx {
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.EmptyNodeID,
					},
				}
			},
			err: errEmptyNodeID,
		},
		{
			name: "no provided stake",
			txFunc: func(*gomock.Controller) *AddPermissionlessValidatorTx {
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
					},
					StakeOuts: nil,
				}
			},
			err: errNoStake,
		},
		{
			name: "too many shares",
			txFunc: func(*gomock.Controller) *AddPermissionlessValidatorTx {
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
					},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					DelegationShares: reward.PercentDenominator + 1,
				}
			},
			err: errTooManyShares,
		},
		{
			name: "invalid rewards owner",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(errCustom)
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errCustom,
		},
		{
			name: "wrong signer",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: constants.PrimaryNetworkID,
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errInvalidSigner,
		},
		{
			name: "invalid stake output",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()

				stakeOut := djtx.NewMockTransferableOut(ctrl)
				stakeOut.EXPECT().Verify().Return(errCustom)
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: stakeOut,
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errCustom,
		},
		{
			name: "multiple staked assets",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
						{
							Asset: djtx.Asset{
								ID: ids.GenerateTestID(),
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errMultipleStakedAssets,
		},
		{
			name: "stake not sorted",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				assetID := ids.GenerateTestID()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 2,
							},
						},
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errOutputsNotSorted,
		},
		{
			name: "weight mismatch",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				assetID := ids.GenerateTestID()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   1,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: errValidatorWeightMismatch,
		},
		{
			name: "valid subnet validator",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				assetID := ids.GenerateTestID()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   2,
					},
					Subnet: ids.GenerateTestID(),
					Signer: &signer.Empty{},
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: nil,
		},
		{
			name: "valid primary network validator",
			txFunc: func(ctrl *gomock.Controller) *AddPermissionlessValidatorTx {
				rewardsOwner := fx.NewMockOwner(ctrl)
				rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
				assetID := ids.GenerateTestID()
				return &AddPermissionlessValidatorTx{
					BaseTx: validBaseTx,
					Validator: validator.Validator{
						NodeID: ids.GenerateTestNodeID(),
						Wght:   2,
					},
					Subnet: constants.PrimaryNetworkID,
					Signer: blsPOP,
					StakeOuts: []*djtx.TransferableOutput{
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
						{
							Asset: djtx.Asset{
								ID: assetID,
							},
							Out: &secp256k1fx.TransferOutput{
								Amt: 1,
							},
						},
					},
					ValidatorRewardsOwner: rewardsOwner,
					DelegatorRewardsOwner: rewardsOwner,
					DelegationShares:      reward.PercentDenominator,
				}
			},
			err: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			tx := tt.txFunc(ctrl)
			err := tx.SyntacticVerify(ctx)
			require.ErrorIs(err, tt.err)
		})
	}

	t.Run("invalid BaseTx", func(t *testing.T) {
		require := require.New(t)
		tx := &AddPermissionlessValidatorTx{
			BaseTx: invalidBaseTx,
			Validator: validator.Validator{
				NodeID: ids.GenerateTestNodeID(),
			},
			StakeOuts: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{
						ID: ids.GenerateTestID(),
					},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			DelegationShares: reward.PercentDenominator,
		}
		err := tx.SyntacticVerify(ctx)
		require.Error(err)
	})

	t.Run("stake overflow", func(t *testing.T) {
		require := require.New(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		rewardsOwner := fx.NewMockOwner(ctrl)
		rewardsOwner.EXPECT().Verify().Return(nil).AnyTimes()
		assetID := ids.GenerateTestID()
		tx := &AddPermissionlessValidatorTx{
			BaseTx: validBaseTx,
			Validator: validator.Validator{
				NodeID: ids.GenerateTestNodeID(),
				Wght:   1,
			},
			Subnet: ids.GenerateTestID(),
			Signer: &signer.Empty{},
			StakeOuts: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{
						ID: assetID,
					},
					Out: &secp256k1fx.TransferOutput{
						Amt: math.MaxUint64,
					},
				},
				{
					Asset: djtx.Asset{
						ID: assetID,
					},
					Out: &secp256k1fx.TransferOutput{
						Amt: 2,
					},
				},
			},
			ValidatorRewardsOwner: rewardsOwner,
			DelegatorRewardsOwner: rewardsOwner,
			DelegationShares:      reward.PercentDenominator,
		}
		err := tx.SyntacticVerify(ctx)
		require.Error(err)
	})
}

func TestAddPermissionlessValidatorTxNotDelegatorTx(t *testing.T) {
	txIntf := any((*AddPermissionlessValidatorTx)(nil))
	_, ok := txIntf.(DelegatorTx)
	require.False(t, ok)
}
