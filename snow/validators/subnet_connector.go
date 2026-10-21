

package validators

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

// SubnetConnector represents a handler that is called when a connection is
// marked as connected to a subnet
type SubnetConnector interface {
	ConnectedSubnet(ctx context.Context, nodeID ids.NodeID, subnetID ids.ID) error
}
