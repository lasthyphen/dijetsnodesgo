

package validators

import (
	"context"
	"fmt"

	"github.com/lasthyphen/dijetsnodesgo/ids"
)

var UnhandledSubnetConnector SubnetConnector = &unhandledSubnetConnector{}

type unhandledSubnetConnector struct{}

func (unhandledSubnetConnector) ConnectedSubnet(_ context.Context, nodeID ids.NodeID, subnetID ids.ID) error {
	return fmt.Errorf(
		"unhandled ConnectedSubnet with nodeID=%q and subnetID=%q",
		nodeID,
		subnetID,
	)
}
