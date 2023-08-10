// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package avm

import (
	"github.com/lasthyphen/dijetsnodesgo/chains/atomic"
	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
)

var _ txs.Visitor = (*executeTx)(nil)

type executeTx struct {
	tx           *txs.Tx
	batch        database.Batch
	sharedMemory atomic.SharedMemory
	parser       txs.Parser
}

func (et *executeTx) BaseTx(*txs.BaseTx) error {
	return et.batch.Write()
}

func (et *executeTx) ImportTx(t *txs.ImportTx) error {
	utxoIDs := make([][]byte, len(t.ImportedIns))
	for i, in := range t.ImportedIns {
		inputID := in.UTXOID.InputID()
		utxoIDs[i] = inputID[:]
	}
	return et.sharedMemory.Apply(
		map[ids.ID]*atomic.Requests{
			t.SourceChain: {
				RemoveRequests: utxoIDs,
			},
		},
		et.batch,
	)
}

func (et *executeTx) ExportTx(t *txs.ExportTx) error {
	txID := et.tx.ID()

	elems := make([]*atomic.Element, len(t.ExportedOuts))
	codec := et.parser.Codec()
	for i, out := range t.ExportedOuts {
		utxo := &djtx.UTXO{
			UTXOID: djtx.UTXOID{
				TxID:        txID,
				OutputIndex: uint32(len(t.Outs) + i),
			},
			Asset: djtx.Asset{ID: out.AssetID()},
			Out:   out.Out,
		}

		utxoBytes, err := codec.Marshal(txs.CodecVersion, utxo)
		if err != nil {
			return err
		}

		inputID := utxo.InputID()
		elem := &atomic.Element{
			Key:   inputID[:],
			Value: utxoBytes,
		}
		if out, ok := utxo.Out.(djtx.Addressable); ok {
			elem.Traits = out.Addresses()
		}

		elems[i] = elem
	}

	return et.sharedMemory.Apply(
		map[ids.ID]*atomic.Requests{
			t.DestinationChain: {
				PutRequests: elems,
			},
		},
		et.batch,
	)
}

func (et *executeTx) CreateAssetTx(t *txs.CreateAssetTx) error {
	return et.BaseTx(&t.BaseTx)
}

func (et *executeTx) OperationTx(t *txs.OperationTx) error {
	return et.BaseTx(&t.BaseTx)
}
