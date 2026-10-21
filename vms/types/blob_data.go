

package types

import (
	"encoding/json"

	"github.com/lasthyphen/dijetsnodesgo/utils/formatting"
)

// JSONByteSlice represents [[]byte] that is json marshalled to hex
type JSONByteSlice []byte

func (b JSONByteSlice) MarshalJSON() ([]byte, error) {
	hexData, err := formatting.Encode(formatting.HexNC, b)
	if err != nil {
		return nil, err
	}
	return json.Marshal(hexData)
}
