

package queue

import "context"

// Parser allows parsing a job from bytes.
type Parser interface {
	Parse(context.Context, []byte) (Job, error)
}
