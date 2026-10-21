

package common

import "context"

type Fetcher struct {
	// tracks which validators were asked for which containers in which requests
	OutstandingRequests Requests

	// Called when bootstrapping is done on a specific chain
	OnFinished func(ctx context.Context, lastReqID uint32) error
}
