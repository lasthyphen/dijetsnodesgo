// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utxo

import (
	"math"
	"testing"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/database/memdb"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/timer/mockable"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/verify"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/stakeable"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var _ txs.UnsignedTx = (*dummyUnsignedTx)(nil)

type dummyUnsignedTx struct {
	txs.BaseTx
}

func (*dummyUnsignedTx) Visit(txs.Visitor) error {
	return nil
}

func TestVerifySpendUTXOs(t *testing.T) {
	fx := &secp256k1fx.Fx{}

	if err := fx.InitializeVM(&secp256k1fx.TestVM{}); err != nil {
		t.Fatal(err)
	}
	if err := fx.Bootstrapped(); err != nil {
		t.Fatal(err)
	}

	h := &handler{
		ctx: snow.DefaultContextTest(),
		clk: &mockable.Clock{},
		utxosReader: djtx.NewUTXOState(
			memdb.New(),
			txs.Codec,
		),
		fx: fx,
	}

	// The handler time during a test, unless [chainTimestamp] is set
	now := time.Unix(1607133207, 0)

	unsignedTx := dummyUnsignedTx{
		BaseTx: txs.BaseTx{},
	}
	unsignedTx.Initialize([]byte{0})

	customAssetID := ids.GenerateTestID()

	// Note that setting [chainTimestamp] also set's the handler's clock.
	// Adjust input/output locktimes accordingly.
	tests := []struct {
		description     string
		utxos           []*djtx.UTXO
		ins             []*djtx.TransferableInput
		outs            []*djtx.TransferableOutput
		creds           []verify.Verifiable
		producedAmounts map[ids.ID]uint64
		shouldErr       bool
	}{
		{
			description:     "no inputs, no outputs, no fee",
			utxos:           []*djtx.UTXO{},
			ins:             []*djtx.TransferableInput{},
			outs:            []*djtx.TransferableOutput{},
			creds:           []verify.Verifiable{},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       false,
		},
		{
			description: "no inputs, no outputs, positive fee",
			utxos:       []*djtx.UTXO{},
			ins:         []*djtx.TransferableInput{},
			outs:        []*djtx.TransferableOutput{},
			creds:       []verify.Verifiable{},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "wrong utxo assetID, one input, no outputs, no fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: customAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "one wrong assetID input, no outputs, no fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: customAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "one input, one wrong assetID output, no fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempt to consume locked output as unlocked",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &stakeable.LockOut{
					Locktime: uint64(now.Add(time.Second).Unix()),
					TransferableOut: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempt to modify locktime",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &stakeable.LockOut{
					Locktime: uint64(now.Add(time.Second).Unix()),
					TransferableOut: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &stakeable.LockIn{
					Locktime: uint64(now.Unix()),
					TransferableIn: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "one input, no outputs, positive fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "wrong number of credentials",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs:  []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "wrong number of UTXOs",
			utxos:       []*djtx.UTXO{},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "invalid credential",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				(*secp256k1fx.Credential)(nil),
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "invalid signature",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
					OutputOwners: secp256k1fx.OutputOwners{
						Threshold: 1,
						Addrs: []ids.ShortID{
							ids.GenerateTestShortID(),
						},
					},
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{0},
					},
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{
					Sigs: [][crypto.SECP256K1RSigLen]byte{
						{},
					},
				},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "one input, no outputs, positive fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &secp256k1fx.TransferOutput{
					Amt: 1,
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &secp256k1fx.TransferInput{
					Amt: 1,
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "locked one input, no outputs, no fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &stakeable.LockOut{
					Locktime: uint64(now.Unix()) + 1,
					TransferableOut: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &stakeable.LockIn{
					Locktime: uint64(now.Unix()) + 1,
					TransferableIn: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       false,
		},
		{
			description: "locked one input, no outputs, positive fee",
			utxos: []*djtx.UTXO{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				Out: &stakeable.LockOut{
					Locktime: uint64(now.Unix()) + 1,
					TransferableOut: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			}},
			ins: []*djtx.TransferableInput{{
				Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
				In: &stakeable.LockIn{
					Locktime: uint64(now.Unix()) + 1,
					TransferableIn: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			}},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "one locked and one unlocked input, one locked output, positive fee",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) + 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &stakeable.LockIn{
						Locktime: uint64(now.Unix()) + 1,
						TransferableIn: &secp256k1fx.TransferInput{
							Amt: 1,
						},
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) + 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "one locked and one unlocked input, one locked output, positive fee, partially locked",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) + 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 2,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &stakeable.LockIn{
						Locktime: uint64(now.Unix()) + 1,
						TransferableIn: &secp256k1fx.TransferInput{
							Amt: 1,
						},
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 2,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) + 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 2,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "one unlocked input, one locked output, zero fee",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) - 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       false,
		},
		{
			description: "attempted overflow",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 2,
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: math.MaxUint64,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempted mint",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 2,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempted mint through locking",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 2,
						},
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: math.MaxUint64,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempted mint through mixed locking (low then high)",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 2,
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: math.MaxUint64,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempted mint through mixed locking (high then low)",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: math.MaxUint64,
					},
				},
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &stakeable.LockOut{
						Locktime: 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 2,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "transfer non-djtx asset",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       false,
		},
		{
			description: "lock non-djtx asset",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Add(time.Second).Unix()),
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       false,
		},
		{
			description: "attempted asset conversion",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{},
			shouldErr:       true,
		},
		{
			description: "attempted asset conversion with burn",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "two inputs, one output with custom asset, with fee",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "one input, fee, custom asset",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "one input, custom fee",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				customAssetID: 1,
			},
			shouldErr: false,
		},
		{
			description: "one input, custom fee, wrong burn",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				customAssetID: 1,
			},
			shouldErr: true,
		},
		{
			description: "two inputs, multiple fee",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: h.ctx.DJTXAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
				&secp256k1fx.Credential{},
			},
			producedAmounts: map[ids.ID]uint64{
				h.ctx.DJTXAssetID: 1,
				customAssetID:     1,
			},
			shouldErr: false,
		},
		{
			description: "one unlock input, one locked output, zero fee, unlocked, custom asset",
			utxos: []*djtx.UTXO{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &stakeable.LockOut{
						Locktime: uint64(now.Unix()) - 1,
						TransferableOut: &secp256k1fx.TransferOutput{
							Amt: 1,
						},
					},
				},
			},
			ins: []*djtx.TransferableInput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					In: &secp256k1fx.TransferInput{
						Amt: 1,
					},
				},
			},
			outs: []*djtx.TransferableOutput{
				{
					Asset: djtx.Asset{ID: customAssetID},
					Out: &secp256k1fx.TransferOutput{
						Amt: 1,
					},
				},
			},
			creds: []verify.Verifiable{
				&secp256k1fx.Credential{},
			},
			producedAmounts: make(map[ids.ID]uint64),
			shouldErr:       false,
		},
	}

	for _, test := range tests {
		h.clk.Set(now)

		t.Run(test.description, func(t *testing.T) {
			err := h.VerifySpendUTXOs(
				&unsignedTx,
				test.utxos,
				test.ins,
				test.outs,
				test.creds,
				test.producedAmounts,
			)

			if err == nil && test.shouldErr {
				t.Fatalf("expected error but got none")
			} else if err != nil && !test.shouldErr {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}
