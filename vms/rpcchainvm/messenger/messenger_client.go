// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package messenger

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/snow/engine/common"

	messengerpb "github.com/lasthyphen/dijetsnodesgo/proto/pb/messenger"
)

// Client is an implementation of a messenger channel that talks over RPC.
type Client struct {
	client messengerpb.MessengerClient
}

// NewClient returns a client that is connected to a remote channel
func NewClient(client messengerpb.MessengerClient) *Client {
	return &Client{client: client}
}

func (c *Client) Notify(msg common.Message) error {
	_, err := c.client.Notify(context.Background(), &messengerpb.NotifyRequest{
		Message: uint32(msg),
	})
	return err
}