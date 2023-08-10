// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package blocks

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/verify"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
)

func TestNewApricotAtomicBlock(t *testing.T) {
	require := require.New(t)

	parentID := ids.GenerateTestID()
	height := uint64(1337)
	tx := &txs.Tx{
		Unsigned: &txs.ImportTx{
			BaseTx: txs.BaseTx{
				BaseTx: djtx.BaseTx{
					Ins:  []*djtx.TransferableInput{},
					Outs: []*djtx.TransferableOutput{},
				},
			},
			ImportedInputs: []*djtx.TransferableInput{},
		},
		Creds: []verify.Verifiable{},
	}
	require.NoError(tx.Sign(txs.Codec, nil))

	blk, err := NewApricotAtomicBlock(
		parentID,
		height,
		tx,
	)
	require.NoError(err)

	// Make sure the block and tx are initialized
	require.NotNil(blk.Bytes())
	require.NotNil(blk.Tx.Bytes())
	require.NotEqual(ids.Empty, blk.Tx.ID())
	require.Equal(tx.Bytes(), blk.Tx.Bytes())
	require.Equal(parentID, blk.Parent())
	require.Equal(height, blk.Height())
}
