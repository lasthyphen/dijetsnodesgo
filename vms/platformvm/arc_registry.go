package platformvm

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/lasthyphen/dijetsnodesgo/arc"
	"github.com/lasthyphen/dijetsnodesgo/database"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

var _ arc.Registry = (*arcRegistry)(nil)

// arcRegistry is a read-only view of the Arcs registered in the
// M-chain state. Every method acquires the M-chain lock itself and never
// calls into other chains while holding it.
type arcRegistry struct {
	vm          *VM
	addrManager djtx.AddressManager
}

func newArcRegistry(vm *VM) *arcRegistry {
	return &arcRegistry{
		vm:          vm,
		addrManager: djtx.NewAddressManager(vm.ctx),
	}
}

func (r *arcRegistry) NetworkID() uint32 {
	return r.vm.ctx.NetworkID
}

func (r *arcRegistry) StakingEnabled() bool {
	return r.vm.StakingEnabled
}

func (r *arcRegistry) Bootstrapped() bool {
	return r.vm.bootstrapped.GetValue()
}

func (r *arcRegistry) TracksSubnet(subnetID ids.ID) bool {
	return r.vm.WhitelistedSubnets.Contains(subnetID)
}

func (r *arcRegistry) LookupChain(alias string) (ids.ID, error) {
	if chainID, err := ids.FromString(alias); err == nil {
		return chainID, nil
	}
	return r.vm.Chains.Lookup(alias)
}

func (r *arcRegistry) height(ctx context.Context) (uint64, error) {
	return r.vm.GetCurrentHeight(ctx)
}

func (r *arcRegistry) owner(subnetTx *txs.Tx) (arc.Owner, error) {
	createSubnetTx, ok := subnetTx.Unsigned.(*txs.CreateSubnetTx)
	if !ok {
		return arc.Owner{}, fmt.Errorf("expected *txs.CreateSubnetTx but got %T", subnetTx.Unsigned)
	}
	owners, ok := createSubnetTx.Owner.(*secp256k1fx.OutputOwners)
	if !ok {
		return arc.Owner{}, fmt.Errorf("unsupported subnet owner type %T", createSubnetTx.Owner)
	}
	owner := arc.Owner{
		Threshold: owners.Threshold,
		Locktime:  owners.Locktime,
		Addresses: make([]string, 0, len(owners.Addrs)),
	}
	for _, addr := range owners.Addrs {
		formatted, err := r.addrManager.FormatLocalAddress(addr)
		if err != nil {
			return arc.Owner{}, err
		}
		owner.Addresses = append(owner.Addresses, formatted)
	}
	return owner, nil
}

func (r *arcRegistry) subnetRecords(subnetTx *txs.Tx) ([]arc.Record, error) {
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
	records := make([]arc.Record, 0, len(chainTxs))
	for _, chainTx := range chainTxs {
		createChainTx, ok := chainTx.Unsigned.(*txs.CreateChainTx)
		if !ok {
			return nil, fmt.Errorf("expected *txs.CreateChainTx but got %T", chainTx.Unsigned)
		}
		records = append(records, arc.Record{
			ChainID:          chainTx.ID(),
			SubnetID:         subnetID,
			Name:             createChainTx.ChainName,
			VMID:             createChainTx.VMID,
			FxIDs:            createChainTx.FxIDs,
			GenesisHash:      arc.GenesisHash(createChainTx.GenesisData),
			GenesisSize:      len(createChainTx.GenesisData),
			Owner:            owner,
			Transformed:      transformed,
			SubnetChainCount: len(chainTxs),
		})
	}
	return records, nil
}

func (r *arcRegistry) Chains(ctx context.Context) ([]arc.Record, uint64, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	subnets, err := r.vm.state.GetSubnets()
	if err != nil {
		return nil, 0, err
	}
	var records []arc.Record
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

func (r *arcRegistry) Chain(ctx context.Context, chainID ids.ID) (arc.Record, uint64, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	chainTx, _, err := r.vm.state.GetTx(chainID)
	if err == database.ErrNotFound {
		return arc.Record{}, 0, arc.ErrNotFound
	}
	if err != nil {
		return arc.Record{}, 0, err
	}
	createChainTx, ok := chainTx.Unsigned.(*txs.CreateChainTx)
	if !ok {
		return arc.Record{}, 0, arc.ErrNotFound
	}
	if createChainTx.SubnetID == constants.PrimaryNetworkID {
		return arc.Record{}, 0, arc.ErrNotAnArc
	}
	subnetTx, _, err := r.vm.state.GetTx(createChainTx.SubnetID)
	if err != nil {
		return arc.Record{}, 0, err
	}
	records, err := r.subnetRecords(subnetTx)
	if err != nil {
		return arc.Record{}, 0, err
	}
	height, err := r.height(ctx)
	if err != nil {
		return arc.Record{}, 0, err
	}
	for _, rec := range records {
		if rec.ChainID == chainID {
			return rec, height, nil
		}
	}
	return arc.Record{}, 0, arc.ErrNotFound
}

func (r *arcRegistry) Committee(_ context.Context, subnetID ids.ID) ([]arc.Member, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	observable := r.vm.WhitelistedSubnets.Contains(subnetID)
	var members []arc.Member

	current, err := r.vm.state.GetCurrentStakerIterator()
	if err != nil {
		return nil, err
	}
	members, err = r.collect(current, subnetID, arc.MemberCurrent, observable, members)
	current.Release()
	if err != nil {
		return nil, err
	}

	pending, err := r.vm.state.GetPendingStakerIterator()
	if err != nil {
		return nil, err
	}
	members, err = r.collect(pending, subnetID, arc.MemberPending, observable, members)
	pending.Release()
	return members, err
}

func (r *arcRegistry) collect(
	it state.StakerIterator,
	subnetID ids.ID,
	status arc.MemberStatus,
	observable bool,
	members []arc.Member,
) ([]arc.Member, error) {
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
		m := arc.Member{
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
			if status == arc.MemberCurrent {
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

func (r *arcRegistry) TxStatus(_ context.Context, txID ids.ID) (arc.TxStatus, error) {
	r.vm.ctx.Lock.Lock()
	defer r.vm.ctx.Lock.Unlock()

	reply := GetTxStatusResponse{}
	if err := (&Service{vm: r.vm}).GetTxStatus(nil, &GetTxStatusArgs{TxID: txID}, &reply); err != nil {
		return "", err
	}
	switch s := arc.TxStatus(reply.Status.String()); s {
	case arc.TxCommitted, arc.TxProcessing, arc.TxAborted, arc.TxDropped:
		return s, nil
	default:
		return arc.TxUnknown, nil
	}
}
