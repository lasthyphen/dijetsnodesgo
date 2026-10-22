package platformvm

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/utils/rpc"
)

// NamesClient is a typed client of the names.* API.
type NamesClient struct {
	requester rpc.EndpointRequester
}

// NewNamesClient returns a client of the node at [uri], e.g.
// http://127.0.0.1:9650.
func NewNamesClient(uri string) *NamesClient {
	return &NamesClient{requester: rpc.NewEndpointRequester(uri + "/ext/bc/M" + NamesEndpoint)}
}

func (c *NamesClient) Resolve(ctx context.Context, name string, options ...rpc.Option) (*NameResolution, error) {
	reply := &NameResolution{}
	return reply, c.requester.SendRequest(ctx, "names.resolve", &ResolveNameArgs{Name: name}, reply, options...)
}

func (c *NamesClient) Reverse(ctx context.Context, addr string, options ...rpc.Option) (*ReverseResolution, error) {
	reply := &ReverseResolution{}
	return reply, c.requester.SendRequest(ctx, "names.reverse", &ReverseNameArgs{Address: addr}, reply, options...)
}

func (c *NamesClient) History(ctx context.Context, name string, options ...rpc.Option) (*NameHistoryReply, error) {
	reply := &NameHistoryReply{}
	return reply, c.requester.SendRequest(ctx, "names.history", &ResolveNameArgs{Name: name}, reply, options...)
}

func (c *NamesClient) Validate(ctx context.Context, name string, options ...rpc.Option) (*ValidateNameReply, error) {
	reply := &ValidateNameReply{}
	return reply, c.requester.SendRequest(ctx, "names.validate", &ResolveNameArgs{Name: name}, reply, options...)
}

func (c *NamesClient) Params(ctx context.Context, options ...rpc.Option) (*NameParamsReply, error) {
	reply := &NameParamsReply{}
	return reply, c.requester.SendRequest(ctx, "names.params", &struct{}{}, reply, options...)
}
