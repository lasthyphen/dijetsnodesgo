package txservice

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/x"
)

// Network is everything the service needs from the blockchain. All reads
// are of accepted state.
type Network interface {
	// Context returns the V-chain's network ID, chain ID, fee asset and fee.
	Context() x.Context
	HRP() string
	// Asset returns the description of an asset by symbol or ID.
	Asset(ctx context.Context, asset string) (Asset, error)

	Resolve(ctx context.Context, name string) (*platformvm.NameResolution, error)
	Reverse(ctx context.Context, addr string) (*platformvm.ReverseResolution, error)
	History(ctx context.Context, name string) (*platformvm.NameHistoryReply, error)

	// UTXOs returns the accepted V-chain UTXOs that reference [addr].
	UTXOs(ctx context.Context, addr ids.ShortID) ([]*djtx.UTXO, error)
	IssueTx(ctx context.Context, txBytes []byte) (ids.ID, error)
	TxStatus(ctx context.Context, txID ids.ID) (choices.Status, error)
	GetTx(ctx context.Context, txID ids.ID) (*txs.Tx, error)
}

type nodeNetwork struct {
	xCtx  x.Context
	hrp   string
	avm   avm.Client
	names *platformvm.NamesClient

	assetsLock sync.Mutex
	assets     map[string]Asset
}

// NewNodeNetwork connects to the node at [uri].
func NewNodeNetwork(ctx context.Context, uri string) (Network, error) {
	xCtx, err := x.NewContextFromURI(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the V-chain context: %w", err)
	}
	networkID, err := info.NewClient(uri).GetNetworkID(ctx)
	if err != nil {
		return nil, err
	}
	return &nodeNetwork{
		xCtx:   xCtx,
		hrp:    constants.GetHRP(networkID),
		avm:    avm.NewClient(uri, "V"),
		names:  platformvm.NewNamesClient(uri),
		assets: make(map[string]Asset),
	}, nil
}

func (n *nodeNetwork) Context() x.Context { return n.xCtx }

func (n *nodeNetwork) HRP() string { return n.hrp }

func (n *nodeNetwork) Asset(ctx context.Context, asset string) (Asset, error) {
	n.assetsLock.Lock()
	defer n.assetsLock.Unlock()
	if a, ok := n.assets[asset]; ok {
		return a, nil
	}
	reply, err := n.avm.GetAssetDescription(ctx, asset)
	if err != nil {
		return Asset{}, err
	}
	a := Asset{
		ID:           reply.AssetID,
		Symbol:       reply.Symbol,
		Denomination: int(reply.Denomination),
	}
	n.assets[asset] = a
	return a, nil
}

func (n *nodeNetwork) Resolve(ctx context.Context, name string) (*platformvm.NameResolution, error) {
	return n.names.Resolve(ctx, name)
}

func (n *nodeNetwork) Reverse(ctx context.Context, addr string) (*platformvm.ReverseResolution, error) {
	return n.names.Reverse(ctx, addr)
}

func (n *nodeNetwork) History(ctx context.Context, name string) (*platformvm.NameHistoryReply, error) {
	return n.names.History(ctx, name)
}

func (n *nodeNetwork) UTXOs(ctx context.Context, addr ids.ShortID) ([]*djtx.UTXO, error) {
	var (
		utxos     []*djtx.UTXO
		startAddr ids.ShortID
		startUTXO ids.ID
	)
	for {
		utxosBytes, endAddr, endUTXO, err := n.avm.GetUTXOs(ctx, []ids.ShortID{addr}, 1024, startAddr, startUTXO)
		if err != nil {
			return nil, err
		}
		for _, utxoBytes := range utxosBytes {
			utxo := &djtx.UTXO{}
			if _, err := x.Parser.Codec().Unmarshal(utxoBytes, utxo); err != nil {
				return nil, err
			}
			utxos = append(utxos, utxo)
		}
		if len(utxosBytes) < 1024 {
			return utxos, nil
		}
		startAddr, startUTXO = endAddr, endUTXO
	}
}

func (n *nodeNetwork) IssueTx(ctx context.Context, txBytes []byte) (ids.ID, error) {
	return n.avm.IssueTx(ctx, txBytes)
}

func (n *nodeNetwork) TxStatus(ctx context.Context, txID ids.ID) (choices.Status, error) {
	return n.avm.GetTxStatus(ctx, txID)
}

func (n *nodeNetwork) GetTx(ctx context.Context, txID ids.ID) (*txs.Tx, error) {
	txBytes, err := n.avm.GetTx(ctx, txID)
	if err != nil {
		return nil, err
	}
	return x.Parser.Parse(txBytes)
}

// formatVAddress formats an identity as a V-chain address.
func formatVAddress(hrp string, addr ids.ShortID) string {
	s, err := address.Format("V", hrp, addr[:])
	if err != nil {
		// Formatting 20 bytes with a valid HRP can't fail.
		panic(err)
	}
	return s
}

// parseVAddress parses a V-chain address. A bare bech32 address is
// accepted; any other chain prefix is refused.
func parseVAddress(hrp, s string) (ids.ShortID, error) {
	bech32 := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		if !strings.EqualFold(s[:i], "V") {
			return ids.ShortID{}, fmt.Errorf("%q is not a V-chain address", s)
		}
		bech32 = s[i+1:]
	}
	gotHRP, payload, err := address.ParseBech32(bech32)
	if err != nil {
		return ids.ShortID{}, err
	}
	if gotHRP != hrp {
		return ids.ShortID{}, fmt.Errorf("address is for network %q, not %q", gotHRP, hrp)
	}
	return ids.ToShortID(payload)
}

// looksLikeAddress reports whether a recipient was meant as an address
// rather than a name: it has a one-character chain prefix ("V-…"), which
// names never have, or it is longer than any name, as bare bech32 addresses
// are.
func looksLikeAddress(s string) bool {
	return (len(s) > 2 && s[1] == '-') || len(s) > names.MaxLength
}
