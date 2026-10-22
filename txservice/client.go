package txservice

import (
	"context"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/rpc"
)

// Client is a typed client of the payments.* API.
type Client struct {
	requester rpc.EndpointRequester
	auth      rpc.Option
}

// NewClient returns a client of the service at [uri] authenticating with
// the bearer [token].
func NewClient(uri, token string) *Client {
	return &Client{
		requester: rpc.NewEndpointRequester(uri + Endpoint),
		auth:      rpc.WithHeader("Authorization", "Bearer "+token),
	}
}

func (c *Client) call(ctx context.Context, method string, args, reply interface{}) error {
	return c.requester.SendRequest(ctx, ServiceName+"."+method, args, reply, c.auth)
}

func (c *Client) Info(ctx context.Context) (*InfoReply, error) {
	reply := &InfoReply{}
	return reply, c.call(ctx, "info", &struct{}{}, reply)
}

func (c *Client) Resolve(ctx context.Context, name string) (*ResolveReply, error) {
	reply := &ResolveReply{}
	return reply, c.call(ctx, "resolve", &ResolveArgs{Name: name}, reply)
}

func (c *Client) Prepare(ctx context.Context, args PrepareArgs) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "prepare", &args, reply)
}

func (c *Client) Get(ctx context.Context, paymentID string) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "get", &PaymentArgs{PaymentID: paymentID}, reply)
}

func (c *Client) Approve(ctx context.Context, paymentID, digest string) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "approve", &ApproveArgs{PaymentID: paymentID, Digest: digest}, reply)
}

func (c *Client) Cancel(ctx context.Context, paymentID string) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "cancel", &PaymentArgs{PaymentID: paymentID}, reply)
}

func (c *Client) Submit(ctx context.Context, paymentID string) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "submit", &PaymentArgs{PaymentID: paymentID}, reply)
}

func (c *Client) Status(ctx context.Context, paymentID string) (*Payment, error) {
	reply := &PaymentReply{}
	return reply.Payment, c.call(ctx, "status", &PaymentArgs{PaymentID: paymentID}, reply)
}

func (c *Client) Receipt(ctx context.Context, paymentID string) (*Receipt, error) {
	reply := &Receipt{}
	return reply, c.call(ctx, "receipt", &PaymentArgs{PaymentID: paymentID}, reply)
}

func (c *Client) Balance(ctx context.Context, delegation string) (*BalanceReply, error) {
	reply := &BalanceReply{}
	return reply, c.call(ctx, "balance", &DelegationArgs{Delegation: delegation}, reply)
}

func (c *Client) History(ctx context.Context, delegation string, limit int) (*HistoryReply, error) {
	reply := &HistoryReply{}
	return reply, c.call(ctx, "history", &HistoryArgs{Delegation: delegation, Limit: limit}, reply)
}

func (c *Client) DescribeTx(ctx context.Context, txID ids.ID) (*TxView, error) {
	reply := &TxView{}
	return reply, c.call(ctx, "describeTx", &DescribeTxArgs{TxID: txID}, reply)
}

func (c *Client) RevokeDelegation(ctx context.Context, delegation string) (*RevokeReply, error) {
	reply := &RevokeReply{}
	return reply, c.call(ctx, "revokeDelegation", &DelegationArgs{Delegation: delegation}, reply)
}
