package txs

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// The naming transactions must not shift the type IDs of existing
// transactions or of the Banff blocks, which the blocks codec registers after
// [RegisterUnsignedTxsTypes].
func TestNamingTxTypeIDs(t *testing.T) {
	require := require.New(t)

	advanceTime := marshalledTypeID(t, &AdvanceTimeTx{})
	register := marshalledTypeID(t, &RegisterNameTx{
		Owner:      &secp256k1fx.OutputOwners{},
		TargetAuth: &secp256k1fx.Input{},
	})
	update := marshalledTypeID(t, &UpdateNameTx{
		Owner:      &secp256k1fx.OutputOwners{},
		NameAuth:   &secp256k1fx.Input{},
		TargetAuth: &secp256k1fx.Input{},
	})

	// After AdvanceTimeTx come RewardValidatorTx, stakeable.LockIn and
	// LockOut, the four Banff transactions and the two BLS signer types (9),
	// then the four Banff blocks (4).
	require.Equal(advanceTime+9+4+1, register)
	require.Equal(advanceTime+9+4+2, update)
}

func marshalledTypeID(t *testing.T, utx UnsignedTx) uint32 {
	b, err := Codec.Marshal(Version, &utx)
	require.NoError(t, err)
	// The first two bytes are the codec version.
	return uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[4])<<8 | uint32(b[5])
}
