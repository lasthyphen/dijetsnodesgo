package platformvm

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/minichain"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var _ minichain.Registry = (*miniChainRegistry)(nil)

// miniChainRegistry is a read-only view of the mini-chains registered in the
// M-chain state. Every method acquires the M-chain lock itself and never
// calls into other chains while holding it.
type miniChainRegistry struct {
	vm          *VM
	addrManager djtx.AddressManager
}

func newMiniChainRegistry(vm *VM) *miniChainRegistry {
	return &miniChainRegistry{
		vm:          vm,
		addrManager: djtx.NewAddressManager(vm.ctx),
	}
}

func (r *miniChainRegistry) NetworkID() uint32 {
	return r.vm.ctx.NetworkID
}

func (r *miniChainRegistry) StakingEnabled() bool {
	return r.vm.StakingEnabled
}

func (r *miniChainRegistry) Bootstrapped() bool {
	return r.vm.bootstrapped.GetValue()
}

func (r *miniChainRegistry) TracksSubnet(subnetID ids.ID) bool {
	return r.vm.WhitelistedSubnets.Contains(subnetID)
}

func (r *miniChainRegistry) LookupChain(alias string) (ids.ID, error) {
	if chainID, err := ids.FromString(alias); err == nil {
		return chainID, nil
	}
	return r.vm.Chains.Lookup(alias)
}

func (r *miniChainRegistry) height(ctx context.Context) (uint64, error) {
	return r.vm.GetCurrentHeight(ctx)
}

func (r *miniChainRegistry) owner(subnetTx *txs.Tx) (minichain.Owner, error) {
	createSubnetTx, ok := subnetTx.Unsigned.(*txs.CreateSubnetTx)
	if !ok {
		return minichain.Owner{}, fmt.Errorf("expected *txs.CreateSubnetTx but got %T", subnetTx.Unsigned)
	}
	owners, ok := createSubnetTx.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return minichain.Owner{}, fmt.Errorf("unsupported subnet owner type %T", createSubnetTx.Owner)
	}
	owner := minichain.Owner{
		Threshold: owners.Threshold,
		Locktime:  owners.Locktime,
		Addresses: make([]string, 0, len(owners.Addrs)),
	}
	for _, addr := range owners.Addrs {
		formatted, err := r.addrManager.FormatLocalAddress(addr)
		if err != nil {
			return minichain.Owner{}, err
		}
		owner.Addresses = append(owner.Addresses, formatted)
	}
	return owner, nil
}

func (r *miniChainRegistry) subnetRecords(subnetTx *txs.Tx) ([]minichain.Record, error) {
	subnetID := subnetTx.ID()
	owner, err := r.owner(subnetTx)
	if err != nil {
		return nil, err
	}
	_, err = r.vm.state.GetSubnetTransformation(subnetID)
	transformed := err == nil
	if err != nil && err != database.ErrNotFound {
		return nil, err
	}
	chainTxs, err := r.vm.state.GetChains(subnetID)
	if err != nil {
		return nil, err
	}
	records := make([]minichain.Record, 0, len(chainTxs))
	for _, chainTx := range chainTxs {
		createChainTx, ok := chainTx.Unsigned.(*txs.CreateChainTx)
		if !ok {
			return nil, fmt.Errorf("expected *txs.CreateChainTx but got %T", chainTx.Unsigned)
		}
		records = append(records, minichain.Record{
			ChainID:          chainTx.ID(),
			SubnetID:         subnetID,
			Name:             createChainTx.ChainName,
			VMID:             createChainTx.VMID,
			FxIDs:            createChainTx.FxIDs,
			GenesisHash:      minichain.GenesisHash(createChainTx.GenesisData),
			GenesisSize:      len(createChainTx.GenesisData),
			Owner:            owner,
			Transformed:      transformed,
			SubnetChainCount: len(chainTxs),
		})
	}
	return records, nil
}

func (r *miniChainRegistry) Chains(ctx context.Context) ([]minichain.Record, uint64, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	subnets, err := r.vm.state.GetSubnets()
	if err != nil {
		return nil, 0, err
	}
	var records []minichain.Record
	for _, subnetTx := range subnets {
		subnetRecords, err := r.subnetRecords(subnetTx)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, subnetRecords...)
	}
	sort.Slice(records, func(i, j int) bool {
		return bytes.Compare(records[i].ChainID[:], records[j].ChainID[:]) < 0
	})
	height, err := r.height(ctx)
	return records, height, err
}

func (r *miniChainRegistry) Chain(ctx context.Context, chainID ids.ID) (minichain.Record, uint64, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	chainTx, _, err := r.vm.state.GetTx(chainID)
	if err == database.ErrNotFound {
		return minichain.Record{}, 0, minichain.ErrNotFound
	}
	if err != nil {
		return minichain.Record{}, 0, err
	}
	createChainTx, ok := chainTx.Unsigned.(*txs.CreateChainTx)
	if !ok {
		return minichain.Record{}, 0, minichain.ErrNotFound
	}
	if createChainTx.SubnetID == constants.PrimaryNetworkID {
		return minichain.Record{}, 0, minichain.ErrNotAMiniChain
	}
	subnetTx, _, err := r.vm.state.GetTx(createChainTx.SubnetID)
	if err != nil {
		return minichain.Record{}, 0, err
	}
	records, err := r.subnetRecords(subnetTx)
	if err != nil {
		return minichain.Record{}, 0, err
	}
	height, err := r.height(ctx)
	if err != nil {
		return minichain.Record{}, 0, err
	}
	for _, rec := range records {
		if rec.ChainID == chainID {
			return rec, height, nil
		}
	}
	return minichain.Record{}, 0, minichain.ErrNotFound
}

func (r *miniChainRegistry) Committee(_ context.Context, subnetID ids.ID) ([]minichain.Member, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	observable := r.vm.WhitelistedSubnets.Contains(subnetID)
	var members []minichain.Member

	current, err := r.vm.state.GetCurrentStakerIterator()
	if err != nil {
		return nil, err
	}
	members, err = r.collect(current, subnetID, minichain.MemberCurrent, observable, members)
	current.Release()
	if err != nil {
		return nil, err
	}

	pending, err := r.vm.state.GetPendingStakerIterator()
	if err != nil {
		return nil, err
	}
	members, err = r.collect(pending, subnetID, minichain.MemberPending, observable, members)
	pending.Release()
	return members, err
}

func (r *miniChainRegistry) collect(
	it state.StakerIterator,
	subnetID ids.ID,
	status minichain.MemberStatus,
	observable bool,
	members []minichain.Member,
) ([]minichain.Member, error) {
	for it.Next() {
		staker := it.Value()
		if staker.SubnetID != subnetID {
			continue
		}
		tx, _, err := r.vm.state.GetTx(staker.TxID)
		if err != nil {
			return nil, err
		}
		switch tx.Unsigned.(type) {
		case *txs.AddSubnetValidatorTx, txs.ValidatorTx:
		default:
			continue // delegators are not committee members
		}
		m := minichain.Member{
			NodeID:    staker.NodeID,
			TxID:      staker.TxID,
			Weight:    staker.Weight,
			StartTime: staker.StartTime,
			EndTime:   staker.EndTime,
			Status:    status,
		}
		if observable {
			connected := r.vm.uptimeManager.IsConnected(staker.NodeID, subnetID)
			m.Connected = &connected
			if status == minichain.MemberCurrent {
				if uptime, err := r.vm.uptimeManager.CalculateUptimePercentFrom(staker.NodeID, subnetID, staker.StartTime); err == nil {
					percent := uptime * 100
					m.UptimePercent = &percent
				}
			}
		}
		members = append(members, m)
	}
	return members, nil
}

func (r *miniChainRegistry) TxStatus(_ context.Context, txID ids.ID) (minichain.TxStatus, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	reply := GetTxStatusResponse{}
	if err := (&Service{vm: r.vm}).GetTxStatus(nil, &GetTxStatusArgs{TxID: txID}, &reply); err != nil {
		return "", err
	}
	switch s := minichain.TxStatus(reply.Status.String()); s {
	case minichain.TxCommitted, minichain.TxProcessing, minichain.TxAborted, minichain.TxDropped:
		return s, nil
	default:
		return minichain.TxUnknown, nil
	}
}
