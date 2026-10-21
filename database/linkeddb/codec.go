

package linkeddb

import (
	"math"

	"github.com/lasthyphen/dijetsnodesgo/codec"
	"github.com/lasthyphen/dijetsnodesgo/codec/linearcodec"
)

const (
	codecVersion = 0
)

// c does serialization and deserialization
var (
	c codec.Manager
)

func init() {
	lc := linearcodec.NewCustomMaxLength(math.MaxUint32)
	c = codec.NewManager(math.MaxInt32)

	if err := c.RegisterCodec(codecVersion, lc); err != nil {
		panic(err)
	}
}
