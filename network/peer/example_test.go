// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package peer

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/lasthyphen/dijetsnodesgo/message"
	"github.com/lasthyphen/dijetsnodesgo/snow/networking/router"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/ips"
)

func ExampleStartTestPeer() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	peerIP := ips.IPPort{
		IP:   net.IPv6loopback,
		Port: 9651,
	}
	peer, err := StartTestPeer(
		ctx,
		peerIP,
		constants.LocalID,
		router.InboundHandlerFunc(func(_ context.Context, msg message.InboundMessage) {
			fmt.Printf("handling %s\n", msg.Op())
		}),
	)
	if err != nil {
		panic(err)
	}

	// Send messages here with [peer.Send].

	peer.StartClose()
	err = peer.AwaitClosed(ctx)
	if err != nil {
		panic(err)
	}
}
