package blocks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// A name transaction must encode identically in the txs codec and inside a
// block, and blocks carrying one must round-trip.
func TestBanffBlockWithNameTx(t *testing.T) {
	require := require.New(t)

	tx := &txs.Tx{Unsigned: &txs.RegisterNameTx{
		Name:       "alice",
		Target:     ids.ShortID{1},
		Owner:      &secp256k1fx.OutputOwners{},
		Expiry:     1,
		TargetAuth: &secp256k1fx.Input{},
	}}
	require.NoError(tx.Sign(txs.Codec, nil))

	blk, err := NewBanffStandardBlock(time.Unix(1, 0), ids.GenerateTestID(), 1, []*txs.Tx{tx})
	require.NoError(err)

	parsed, err := Parse(Codec, blk.Bytes())
	require.NoError(err)
	require.Equal(blk.ID(), parsed.ID())
	parsedTxs := parsed.Txs()
	require.Len(parsedTxs, 1)
	require.Equal(tx.Bytes(), parsedTxs[0].Bytes())
	require.IsType(&txs.RegisterNameTx{}, parsedTxs[0].Unsigned)
}
