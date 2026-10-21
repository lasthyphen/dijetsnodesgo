

package summary

import (
	"errors"
	"math"

	"github.com/lasthyphen/dijetsnodesgo/codec"
	"github.com/lasthyphen/dijetsnodesgo/codec/linearcodec"
)

const codecVersion = 0

var (
	c codec.Manager

	errWrongCodecVersion = errors.New("wrong codec version")
)

func init() {
	lc := linearcodec.NewCustomMaxLength(math.MaxUint32)
	c = codec.NewManager(math.MaxInt32)
	if err := c.RegisterCodec(codecVersion, lc); err != nil {
		panic(err)
	}
}
