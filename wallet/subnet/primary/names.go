package primary

import (
	"context"
	"fmt"
	"strings"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/p"
)

var _ p.NameOwnerBackend = (*nameOwnerBackend)(nil)

// nameOwnerBackend lets the P-chain signer sign UpdateNameTx by looking up
// the name's current owner through the names API.
type nameOwnerBackend struct {
	p.Backend
	names *platformvm.NamesClient
}

func newNameOwnerBackend(backend p.Backend, uri string) *nameOwnerBackend {
	return &nameOwnerBackend{
		Backend: backend,
		names:   platformvm.NewNamesClient(uri),
	}
}

func (b *nameOwnerBackend) GetNameOwner(ctx context.Context, name string) (*secp256k1fx.OutputOwners, error) {
	resolution, err := b.names.Resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	if resolution.Record == nil {
		return nil, fmt.Errorf("name %q is not registered", name)
	}
	return ParseNameOwner(resolution.Record.Owner)
}

// ParseNameOwner converts the owner reported by the names API back into an
// OutputOwners.
func ParseNameOwner(owner platformvm.NameOwner) (*secp256k1fx.OutputOwners, error) {
	addrs := make([]ids.ShortID, len(owner.Addresses))
	for i, addr := range owner.Addresses {
		bech32 := addr
		if j := strings.IndexByte(addr, '-'); j >= 0 {
			bech32 = addr[j+1:]
		}
		_, payload, err := address.ParseBech32(bech32)
		if err != nil {
			return nil, err
		}
		addrs[i], err = ids.ToShortID(payload)
		if err != nil {
			return nil, err
		}
	}
	return &secp256k1fx.OutputOwners{
		Locktime:  uint64(owner.Locktime),
		Threshold: uint32(owner.Threshold),
		Addrs:     addrs,
	}, nil
}
