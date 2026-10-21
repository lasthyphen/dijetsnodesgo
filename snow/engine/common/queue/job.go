

package queue

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/set"
)

// Job defines the interface required to be placed on the job queue.
type Job interface {
	ID() ids.ID
	MissingDependencies(context.Context) (set.Set[ids.ID], error)
	// Returns true if this job has at least 1 missing dependency
	HasMissingDependencies(context.Context) (bool, error)
	Execute(context.Context) error
	Bytes() []byte
}
