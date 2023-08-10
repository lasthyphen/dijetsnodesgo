// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txs

import "github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"

var _ Visitor = (*utxoGetter)(nil)

// Allow vm to execute custom logic against the underlying transaction types.
type Visitor interface {
	BaseTx(*BaseTx) error
	CreateAssetTx(*CreateAssetTx) error
	OperationTx(*OperationTx) error
	ImportTx(*ImportTx) error
	ExportTx(*ExportTx) error
}

// utxoGetter returns the UTXOs transaction is producing.
type utxoGetter struct {
	tx    *Tx
	utxos []*djtx.UTXO
}

func (u *utxoGetter) BaseTx(tx *BaseTx) error {
	txID := u.tx.ID()
	u.utxos = make([]*djtx.UTXO, len(tx.Outs))
	for i, out := range tx.Outs {
		u.utxos[i] = &djtx.UTXO{
			UTXOID: djtx.UTXOID{
				TxID:        txID,
				OutputIndex: uint32(i),
			},
			Asset: djtx.Asset{ID: out.AssetID()},
			Out:   out.Out,
		}
	}
	return nil
}

func (u *utxoGetter) ImportTx(tx *ImportTx) error {
	return u.BaseTx(&tx.BaseTx)
}

func (u *utxoGetter) ExportTx(tx *ExportTx) error {
	return u.BaseTx(&tx.BaseTx)
}

func (u *utxoGetter) CreateAssetTx(t *CreateAssetTx) error {
	if err := u.BaseTx(&t.BaseTx); err != nil {
		return err
	}

	txID := u.tx.ID()
	for _, state := range t.States {
		for _, out := range state.Outs {
			u.utxos = append(u.utxos, &djtx.UTXO{
				UTXOID: djtx.UTXOID{
					TxID:        txID,
					OutputIndex: uint32(len(u.utxos)),
				},
				Asset: djtx.Asset{
					ID: txID,
				},
				Out: out,
			})
		}
	}
	return nil
}

func (u *utxoGetter) OperationTx(t *OperationTx) error {
	// The error is explicitly dropped here because no error is ever returned
	// from the utxoGetter.
	_ = u.BaseTx(&t.BaseTx)

	txID := u.tx.ID()
	for _, op := range t.Ops {
		asset := op.AssetID()
		for _, out := range op.Op.Outs() {
			u.utxos = append(u.utxos, &djtx.UTXO{
				UTXOID: djtx.UTXOID{
					TxID:        txID,
					OutputIndex: uint32(len(u.utxos)),
				},
				Asset: djtx.Asset{ID: asset},
				Out:   out,
			})
		}
	}
	return nil
}
