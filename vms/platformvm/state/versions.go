

package state

import (
	"github.com/lasthyphen/dijetsnodesgo/ids"
)

type Versions interface {
	GetState(blkID ids.ID) (Chain, bool)
}
