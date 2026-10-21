

package message

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/utils"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
)

func TestTx(t *testing.T) {
	require := require.New(t)

	tx := utils.RandomBytes(256 * units.KiB)
	builtMsg := Tx{
		Tx: tx,
	}
	builtMsgBytes, err := Build(&builtMsg)
	require.NoError(err)
	require.Equal(builtMsgBytes, builtMsg.Bytes())

	parsedMsgIntf, err := Parse(builtMsgBytes)
	require.NoError(err)
	require.Equal(builtMsgBytes, parsedMsgIntf.Bytes())

	parsedMsg, ok := parsedMsgIntf.(*Tx)
	require.True(ok)

	require.Equal(tx, parsedMsg.Tx)
}

func TestParseGibberish(t *testing.T) {
	require := require.New(t)

	randomBytes := utils.RandomBytes(256 * units.KiB)
	_, err := Parse(randomBytes)
	require.Error(err)
}
