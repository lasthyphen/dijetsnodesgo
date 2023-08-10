// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import (
	"errors"
	"testing"

	"github.com/lasthyphen/dijetsnodesgo/codec"
	"github.com/lasthyphen/dijetsnodesgo/codec/linearcodec"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/utils/wrappers"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/fxs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var (
	networkID       uint32 = 10
	chainID                = ids.ID{5, 4, 3, 2, 1}
	platformChainID        = ids.Empty.Prefix(0)

	keys = crypto.BuildTestKeys()

	assetID = ids.ID{1, 2, 3}
)

func setupCodec() codec.Manager {
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	errs := wrappers.Errs{}
	errs.Add(
		c.RegisterType(&BaseTx{}),
		c.RegisterType(&CreateAssetTx{}),
		c.RegisterType(&OperationTx{}),
		c.RegisterType(&ImportTx{}),
		c.RegisterType(&ExportTx{}),
		c.RegisterType(&secp256k1fx.TransferInput{}),
		c.RegisterType(&secp256k1fx.MintOutput{}),
		c.RegisterType(&secp256k1fx.TransferOutput{}),
		c.RegisterType(&secp256k1fx.MintOperation{}),
		c.RegisterType(&secp256k1fx.Credential{}),
		m.RegisterCodec(CodecVersion, c),
	)
	if errs.Errored() {
		panic(errs.Err)
	}
	return m
}

func NewContext(tb testing.TB) *snow.Context {
	ctx := snow.DefaultContextTest()
	ctx.NetworkID = networkID
	ctx.ChainID = chainID
	djtxAssetID, err := ids.FromString("2XGxUr7VF7j1iwUp2aiGe4b6Ue2yyNghNS1SuNTNmZ77dPpXFZ")
	if err != nil {
		tb.Fatal(err)
	}
	ctx.DJTXAssetID = djtxAssetID
	ctx.XChainID = ids.Empty.Prefix(0)
	ctx.CChainID = ids.Empty.Prefix(1)
	aliaser := ctx.BCLookup.(ids.Aliaser)

	errs := wrappers.Errs{}
	errs.Add(
		aliaser.Alias(chainID, "V"),
		aliaser.Alias(chainID, chainID.String()),
		aliaser.Alias(platformChainID, "M"),
		aliaser.Alias(platformChainID, platformChainID.String()),
	)
	if errs.Errored() {
		tb.Fatal(errs.Err)
	}
	return ctx
}

func TestTxNil(t *testing.T) {
	ctx := NewContext(t)
	c := linearcodec.NewDefault()
	m := codec.NewDefaultManager()
	if err := m.RegisterCodec(CodecVersion, c); err != nil {
		t.Fatal(err)
	}

	tx := (*Tx)(nil)
	if err := tx.SyntacticVerify(ctx, m, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Should have erred due to nil tx")
	}
}

func TestTxEmpty(t *testing.T) {
	ctx := NewContext(t)
	c := setupCodec()
	tx := &Tx{}
	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Should have erred due to nil tx")
	}
}

func TestTxInvalidCredential(t *testing.T) {
	ctx := NewContext(t)
	c := setupCodec()

	tx := &Tx{
		Unsigned: &BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*djtx.TransferableInput{{
				UTXOID: djtx.UTXOID{
					TxID:        ids.Empty,
					OutputIndex: 0,
				},
				Asset: djtx.Asset{ID: assetID},
				In: &secp256k1fx.TransferInput{
					Amt: 20 * units.KiloDjtx,
					Input: secp256k1fx.Input{
						SigIndices: []uint32{
							0,
						},
					},
				},
			}},
		}},
		Creds: []*fxs.FxCredential{{Verifiable: &djtx.TestVerifiable{Err: errors.New("")}}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Tx should have failed due to an invalid credential")
	}
}

func TestTxInvalidUnsignedTx(t *testing.T) {
	ctx := NewContext(t)
	c := setupCodec()

	tx := &Tx{
		Unsigned: &BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*djtx.TransferableInput{
				{
					UTXOID: djtx.UTXOID{
						TxID:        ids.Empty,
						OutputIndex: 0,
					},
					Asset: djtx.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloDjtx,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
				{
					UTXOID: djtx.UTXOID{
						TxID:        ids.Empty,
						OutputIndex: 0,
					},
					Asset: djtx.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloDjtx,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
			},
		}},
		Creds: []*fxs.FxCredential{
			{Verifiable: &djtx.TestVerifiable{}},
			{Verifiable: &djtx.TestVerifiable{}},
		},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Tx should have failed due to an invalid unsigned tx")
	}
}

func TestTxInvalidNumberOfCredentials(t *testing.T) {
	ctx := NewContext(t)
	c := setupCodec()

	tx := &Tx{
		Unsigned: &BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    networkID,
			BlockchainID: chainID,
			Ins: []*djtx.TransferableInput{
				{
					UTXOID: djtx.UTXOID{TxID: ids.Empty, OutputIndex: 0},
					Asset:  djtx.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloDjtx,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
				{
					UTXOID: djtx.UTXOID{TxID: ids.Empty, OutputIndex: 1},
					Asset:  djtx.Asset{ID: assetID},
					In: &secp256k1fx.TransferInput{
						Amt: 20 * units.KiloDjtx,
						Input: secp256k1fx.Input{
							SigIndices: []uint32{
								0,
							},
						},
					},
				},
			},
		}},
		Creds: []*fxs.FxCredential{{Verifiable: &djtx.TestVerifiable{}}},
	}
	tx.Initialize(nil, nil)

	if err := tx.SyntacticVerify(ctx, c, ids.Empty, 0, 0, 1); err == nil {
		t.Fatalf("Tx should have failed due to an invalid number of credentials")
	}
}
