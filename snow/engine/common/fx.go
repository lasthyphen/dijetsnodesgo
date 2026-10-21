

package common

import (
	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// Fx wraps an instance of a feature extension
type Fx struct {
	ID ids.ID
	Fx interface{}
}
