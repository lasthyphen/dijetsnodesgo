package arc

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/utils/rpc"
)

// Client is a typed client of the arc.* API.
type Client struct {
	requester rpc.EndpointRequester
}

// NewClient returns a client of the node at [uri], e.g. http://127.0.0.1:9650.
func NewClient(uri string) *Client {
	return &Client{requester: rpc.NewEndpointRequester(uri + "/ext/bc/M" + Endpoint)}
}

func (c *Client) List(ctx context.Context, args ListArgs, options ...rpc.Option) (*ListReply, error) {
	reply := &ListReply{}
	return reply, c.requester.SendRequest(ctx, "arc.list", &args, reply, options...)
}

func (c *Client) Get(ctx context.Context, chainID ids.ID, options ...rpc.Option) (*GetReply, error) {
	reply := &GetReply{}
	return reply, c.requester.SendRequest(ctx, "arc.get", &ChainArgs{ChainID: chainID.String()}, reply, options...)
}

func (c *Client) Validators(ctx context.Context, chainID ids.ID, options ...rpc.Option) (*ValidatorsReply, error) {
	reply := &ValidatorsReply{}
	return reply, c.requester.SendRequest(ctx, "arc.validators", &ChainArgs{ChainID: chainID.String()}, reply, options...)
}

func (c *Client) Stats(ctx context.Context, chainID ids.ID, options ...rpc.Option) (*StatsReply, error) {
	reply := &StatsReply{}
	return reply, c.requester.SendRequest(ctx, "arc.stats", &ChainArgs{ChainID: chainID.String()}, reply, options...)
}

func (c *Client) Health(ctx context.Context, chainID ids.ID, options ...rpc.Option) (*HealthReply, error) {
	reply := &HealthReply{}
	return reply, c.requester.SendRequest(ctx, "arc.health", &ChainArgs{ChainID: chainID.String()}, reply, options...)
}

func (c *Client) Capabilities(ctx context.Context, chainID string, options ...rpc.Option) (*CapabilitiesReply, error) {
	reply := &CapabilitiesReply{}
	return reply, c.requester.SendRequest(ctx, "arc.capabilities", &CapabilitiesArgs{ChainID: chainID}, reply, options...)
}

func (c *Client) GetCreationStatus(ctx context.Context, args CreationStatusArgs, options ...rpc.Option) (*CreationStatusReply, error) {
	reply := &CreationStatusReply{}
	return reply, c.requester.SendRequest(ctx, "arc.getCreationStatus", &args, reply, options...)
}

func (c *Client) Events(ctx context.Context, after uint64, limit uint32, options ...rpc.Option) (*EventsReply, error) {
	reply := &EventsReply{}
	args := &EventsArgs{After: json.Uint64(after), Limit: json.Uint32(limit)}
	return reply, c.requester.SendRequest(ctx, "arc.events", args, reply, options...)
}
