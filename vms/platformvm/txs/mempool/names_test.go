package mempool

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

func newRegisterNameTx(t *testing.T, name string, target ids.ShortID) *txs.Tx {
	utx := &txs.RegisterNameTx{
		BaseTx: txs.BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    10,
			BlockchainID: ids.Empty,
			// A distinct input for each tx, so they don't conflict on UTXOs.
			Ins: []*djtx.TransferableInput{{
				UTXOID: djtx.UTXOID{TxID: ids.GenerateTestID()},
				Asset:  djtx.Asset{ID: ids.GenerateTestID()},
				In:     &secp256k1fx.TransferInput{Amt: 1},
			}},
		}},
		Name:       name,
		Target:     target,
		Owner:      &secp256k1fx.OutputOwners{},
		Expiry:     1,
		TargetAuth: &secp256k1fx.Input{},
	}
	tx := &txs.Tx{Unsigned: utx}
	require.NoError(t, tx.Sign(txs.Codec, nil))
	return tx
}

// Two pending transactions that claim the same name or target can't share a
// block, so the mempool admits only the first.
func TestMempoolNameConflicts(t *testing.T) {
	require := require.New(t)
	mpool, err := NewMempool("mempool", prometheus.NewRegistry(), &noopBlkTimer{})
	require.NoError(err)

	alice := ids.ShortID{'a'}
	bob := ids.ShortID{'b'}

	first := newRegisterNameTx(t, "alice", alice)
	require.NoError(mpool.Add(first))

	require.Error(mpool.Add(newRegisterNameTx(t, "alice", bob)), "same name")
	require.Error(mpool.Add(newRegisterNameTx(t, "alice2", alice)), "same target")
	require.NoError(mpool.Add(newRegisterNameTx(t, "bob", bob)))

	// Once the first tx leaves the mempool, its claims are released.
	mpool.Remove([]*txs.Tx{first})
	require.NoError(mpool.Add(newRegisterNameTx(t, "alice", alice)))
}
