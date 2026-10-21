package platformvm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/arc"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/logging"

	txexecutor "github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs/executor"
)

func TestArcRegistry(t *testing.T) {
	require := require.New(t)
	vm, _, _ := defaultVM()
	locked := false
	lock := func() { vm.ctx.Lock.Lock(); locked = true }
	unlock := func() { locked = false; vm.ctx.Lock.Unlock() }
	defer func() {
		if !locked {
			lock()
		}
		require.NoError(vm.Shutdown(context.Background()))
		unlock()
	}()

	genesis := []byte("Arc genesis")
	controlKeys := []*crypto.PrivateKeySECP256K1R{testSubnet1ControlKeys[0], testSubnet1ControlKeys[1]}

	lock()
	chainTx, err := vm.txBuilder.NewCreateChainTx(
		testSubnet1.ID(),
		genesis,
		constants.AVMID,
		nil,
		"alpha",
		controlKeys,
		ids.ShortEmpty,
	)
	require.NoError(err)
	require.NoError(vm.Builder.AddUnverifiedTx(chainTx))
	blk, err := vm.Builder.BuildBlock(context.Background())
	require.NoError(err)
	require.NoError(blk.Verify(context.Background()))
	require.NoError(blk.Accept(context.Background()))
	require.NoError(vm.SetPreference(context.Background(), vm.manager.LastAccepted()))

	startTime := vm.clock.Time().Add(txexecutor.SyncBound).Add(time.Second)
	nodeID := ids.NodeID(keys[0].PublicKey().Address())
	vdrTx, err := vm.txBuilder.NewAddSubnetValidatorTx(
		defaultWeight,
		uint64(startTime.Unix()),
		uint64(startTime.Add(defaultMinStakingDuration).Unix()),
		nodeID,
		testSubnet1.ID(),
		controlKeys,
		ids.ShortEmpty,
	)
	require.NoError(err)
	require.NoError(vm.Builder.AddUnverifiedTx(vdrTx))
	blk, err = vm.Builder.BuildBlock(context.Background())
	require.NoError(err)
	require.NoError(blk.Verify(context.Background()))
	require.NoError(blk.Accept(context.Background()))
	require.NoError(vm.SetPreference(context.Background(), vm.manager.LastAccepted()))
	unlock()

	// The registry acquires the M-chain lock itself.
	reg := newArcRegistry(vm)
	ctx := context.Background()

	records, height, err := reg.Chains(ctx)
	require.NoError(err)
	require.Positive(height)
	require.Len(records, 1, "primary network chains are not Arcs")
	rec := records[0]
	require.Equal(chainTx.ID(), rec.ChainID)
	require.Equal(testSubnet1.ID(), rec.SubnetID)
	require.Equal("alpha", rec.Name)
	require.Equal(constants.AVMID, rec.VMID)
	require.Equal(arc.GenesisHash(genesis), rec.GenesisHash)
	require.Equal(len(genesis), rec.GenesisSize)
	require.False(rec.Transformed)
	require.Equal(arc.PermissionedCommittee, rec.Profile())
	require.Equal(uint32(2), rec.Owner.Threshold)
	require.Len(rec.Owner.Addresses, 3)
	require.Equal(1, rec.SubnetChainCount)

	got, _, err := reg.Chain(ctx, chainTx.ID())
	require.NoError(err)
	require.Equal(rec, got)

	_, _, err = reg.Chain(ctx, ids.GenerateTestID())
	require.ErrorIs(err, arc.ErrNotFound)

	// A non-chain transaction is not an Arc.
	_, _, err = reg.Chain(ctx, vdrTx.ID())
	require.ErrorIs(err, arc.ErrNotFound)

	members, err := reg.Committee(ctx, testSubnet1.ID())
	require.NoError(err)
	require.Len(members, 1)
	require.Equal(nodeID, members[0].NodeID)
	require.Equal(arc.MemberPending, members[0].Status)
	require.Equal(vdrTx.ID(), members[0].TxID)
	require.Nil(members[0].Connected, "subnet is not tracked: connectivity is not observable")

	st, err := reg.TxStatus(ctx, chainTx.ID())
	require.NoError(err)
	require.Equal(arc.TxCommitted, st)
	st, err = reg.TxStatus(ctx, ids.GenerateTestID())
	require.NoError(err)
	require.Equal(arc.TxUnknown, st)

	require.False(reg.TracksSubnet(testSubnet1.ID()))
	require.Equal(vm.ctx.NetworkID, reg.NetworkID())
}

func TestArcHandlerMountedOnlyWithObserver(t *testing.T) {
	require := require.New(t)
	vm, _, _ := defaultVM()
	vm.ctx.Lock.Lock()
	defer func() {
		require.NoError(vm.Shutdown(context.Background()))
		vm.ctx.Lock.Unlock()
	}()

	handlers, err := vm.CreateHandlers(context.Background())
	require.NoError(err)
	require.NotContains(handlers, arc.Endpoint)

	observer, err := arc.NewObserver(arc.ObserverConfig{Log: logging.NoLog{}})
	require.NoError(err)
	vm.ArcObserver = observer
	handlers, err = vm.CreateHandlers(context.Background())
	require.NoError(err)
	require.Contains(handlers, arc.Endpoint)
	require.Contains(handlers, "")
}
