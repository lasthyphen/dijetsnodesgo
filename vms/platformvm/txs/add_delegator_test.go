// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/timer/mockable"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/stakeable"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/validator"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var preFundedKeys = crypto.BuildTestKeys()

func TestAddDelegatorTxSyntacticVerify(t *testing.T) {
	require := require.New(t)
	clk := mockable.Clock{}
	ctx := snow.DefaultContextTest()
	ctx.DJTXAssetID = ids.GenerateTestID()
	signers := [][]*crypto.PrivateKeySECP256K1R{preFundedKeys}

	var (
		stx            *Tx
		addDelegatorTx *AddDelegatorTx
		err            error
	)

	// Case : signed tx is nil
	require.ErrorIs(stx.SyntacticVerify(ctx), ErrNilSignedTx)

	// Case : unsigned tx is nil
	require.ErrorIs(addDelegatorTx.SyntacticVerify(ctx), ErrNilTx)

	validatorWeight := uint64(2022)
	inputs := []*djtx.TransferableInput{{
		UTXOID: djtx.UTXOID{
			TxID:        ids.ID{'t', 'x', 'I', 'D'},
			OutputIndex: 2,
		},
		Asset: djtx.Asset{ID: ctx.DJTXAssetID},
		In: &secp256k1fx.TransferInput{
			Amt:   uint64(5678),
			Input: secp256k1fx.Input{SigIndices: []uint32{0}},
		},
	}}
	outputs := []*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: ctx.DJTXAssetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(1234),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
			},
		},
	}}
	stakes := []*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: ctx.DJTXAssetID},
		Out: &stakeable.LockOut{
			Locktime: uint64(clk.Time().Add(time.Second).Unix()),
			TransferableOut: &secp256k1fx.TransferOutput{
				Amt: validatorWeight,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
				},
			},
		},
	}}
	addDelegatorTx = &AddDelegatorTx{
		BaseTx: BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    ctx.NetworkID,
			BlockchainID: ctx.ChainID,
			Outs:         outputs,
			Ins:          inputs,
			Memo:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}},
		Validator: validator.Validator{
			NodeID: ctx.NodeID,
			Start:  uint64(clk.Time().Unix()),
			End:    uint64(clk.Time().Add(time.Hour).Unix()),
			Wght:   validatorWeight,
		},
		StakeOuts: stakes,
		DelegationRewardsOwner: &secp256k1fx.OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
		},
	}

	// Case: signed tx not initialized
	stx = &Tx{Unsigned: addDelegatorTx}
	require.ErrorIs(stx.SyntacticVerify(ctx), errSignedTxNotInitialized)

	// Case: valid tx
	stx, err = NewSigned(addDelegatorTx, Codec, signers)
	require.NoError(err)
	require.NoError(stx.SyntacticVerify(ctx))

	// Case: Wrong network ID
	addDelegatorTx.SyntacticallyVerified = false
	addDelegatorTx.NetworkID++
	stx, err = NewSigned(addDelegatorTx, Codec, signers)
	require.NoError(err)
	err = stx.SyntacticVerify(ctx)
	require.Error(err)
	addDelegatorTx.NetworkID--

	// Case: delegator weight is not equal to total stake weight
	addDelegatorTx.SyntacticallyVerified = false
	addDelegatorTx.Validator.Wght = 2 * validatorWeight
	stx, err = NewSigned(addDelegatorTx, Codec, signers)
	require.NoError(err)
	require.ErrorIs(stx.SyntacticVerify(ctx), errDelegatorWeightMismatch)
	addDelegatorTx.Validator.Wght = validatorWeight
}

func TestAddDelegatorTxSyntacticVerifyNotDJTX(t *testing.T) {
	require := require.New(t)
	clk := mockable.Clock{}
	ctx := snow.DefaultContextTest()
	ctx.DJTXAssetID = ids.GenerateTestID()
	signers := [][]*crypto.PrivateKeySECP256K1R{preFundedKeys}

	var (
		stx            *Tx
		addDelegatorTx *AddDelegatorTx
		err            error
	)

	assetID := ids.GenerateTestID()
	validatorWeight := uint64(2022)
	inputs := []*djtx.TransferableInput{{
		UTXOID: djtx.UTXOID{
			TxID:        ids.ID{'t', 'x', 'I', 'D'},
			OutputIndex: 2,
		},
		Asset: djtx.Asset{ID: assetID},
		In: &secp256k1fx.TransferInput{
			Amt:   uint64(5678),
			Input: secp256k1fx.Input{SigIndices: []uint32{0}},
		},
	}}
	outputs := []*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt: uint64(1234),
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
			},
		},
	}}
	stakes := []*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: assetID},
		Out: &stakeable.LockOut{
			Locktime: uint64(clk.Time().Add(time.Second).Unix()),
			TransferableOut: &secp256k1fx.TransferOutput{
				Amt: validatorWeight,
				OutputOwners: secp256k1fx.OutputOwners{
					Threshold: 1,
					Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
				},
			},
		},
	}}
	addDelegatorTx = &AddDelegatorTx{
		BaseTx: BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    ctx.NetworkID,
			BlockchainID: ctx.ChainID,
			Outs:         outputs,
			Ins:          inputs,
			Memo:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}},
		Validator: validator.Validator{
			NodeID: ctx.NodeID,
			Start:  uint64(clk.Time().Unix()),
			End:    uint64(clk.Time().Add(time.Hour).Unix()),
			Wght:   validatorWeight,
		},
		StakeOuts: stakes,
		DelegationRewardsOwner: &secp256k1fx.OutputOwners{
			Locktime:  0,
			Threshold: 1,
			Addrs:     []ids.ShortID{preFundedKeys[0].PublicKey().Address()},
		},
	}

	stx, err = NewSigned(addDelegatorTx, Codec, signers)
	require.NoError(err)
	require.Error(stx.SyntacticVerify(ctx))
}

func TestAddDelegatorTxNotValidatorTx(t *testing.T) {
	txIntf := any((*AddDelegatorTx)(nil))
	_, ok := txIntf.(ValidatorTx)
	require.False(t, ok)
}
