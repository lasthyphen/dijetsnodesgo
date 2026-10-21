

package avm

import "context"

// TODO: add health checks
func (*VM) HealthCheck(context.Context) (interface{}, error) {
	return nil, nil
}
