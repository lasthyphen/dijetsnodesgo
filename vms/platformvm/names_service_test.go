package platformvm

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/rpc/v2/json2"
	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/utxo"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

// The names API answers from accepted state after a RegisterNameTx has gone
// through the mempool, block building, verification and acceptance.
func TestNamesService(t *testing.T) {
	require := require.New(t)
	vm, _, _ := defaultVM()
	vm.ctx.Lock.Lock()
	defer func() {
		require.NoError(vm.Shutdown(context.Background()))
		vm.ctx.Lock.Unlock()
	}()
	vm.NamingParams = names.DevelopmentParams

	payer := keys[0]
	target := keys[1]
	handler := utxo.NewHandler(vm.ctx, &vm.clock, vm.state, vm.fx)
	ins, outs, _, signers, err := handler.Spend(keys[:1], 0, names.DevelopmentParams.RegisterFee, payer.PublicKey().Address())
	require.NoError(err)
	tx := &txs.Tx{Unsigned: &txs.RegisterNameTx{
		BaseTx: txs.BaseTx{BaseTx: djtx.BaseTx{
			NetworkID:    vm.ctx.NetworkID,
			BlockchainID: vm.ctx.ChainID,
			Ins:          ins,
			Outs:         outs,
		}},
		Name:   "alice",
		Target: target.PublicKey().Address(),
		Owner: &secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs:     []ids.ShortID{target.PublicKey().Address()},
		},
		Expiry:     uint64(vm.clock.Time().Add(time.Hour).Unix()),
		TargetAuth: &secp256k1fx.Input{SigIndices: []uint32{0}},
	}}
	require.NoError(tx.Sign(txs.Codec, append(signers, keys[1:2])))

	require.NoError(vm.Builder.AddUnverifiedTx(tx))
	blk, err := vm.Builder.BuildBlock(context.Background())
	require.NoError(err)
	require.NoError(blk.Verify(context.Background()))
	require.NoError(blk.Accept(context.Background()))
	require.NoError(vm.SetPreference(context.Background(), vm.manager.LastAccepted()))

	// The service takes the lock itself.
	vm.ctx.Lock.Unlock()
	defer vm.ctx.Lock.Lock()
	service := &NamesService{vm: vm, limiter: make(chan struct{}, 1)}

	hrp := constants.GetHRP(vm.ctx.NetworkID)
	targetAddr, err := address.Format("V", hrp, target.PublicKey().Address().Bytes())
	require.NoError(err)

	resolution := NameResolution{}
	require.NoError(service.Resolve(nil, &ResolveNameArgs{Name: " @Alice "}, &resolution))
	require.Equal(NamesAPIVersion, resolution.APIVersion)
	require.Equal("alice", resolution.Name)
	require.Equal("ACTIVE", resolution.Status)
	require.True(resolution.Resolvable)
	require.Equal(targetAddr, resolution.Record.Target)
	require.Equal(tx.ID(), resolution.Record.RecordID)
	require.EqualValues(1, resolution.Record.Version)
	require.EqualValues(1, resolution.Record.Generation)
	require.Equal(blk.ID(), resolution.At.BlockID)
	require.EqualValues(blk.Height(), resolution.At.Height)

	unknown := NameResolution{}
	require.NoError(service.Resolve(nil, &ResolveNameArgs{Name: "nobody"}, &unknown))
	require.Equal("NOT_FOUND", unknown.Status)
	require.False(unknown.Resolvable)
	require.Nil(unknown.Record)

	err = service.Resolve(nil, &ResolveNameArgs{Name: "al ice"}, &NameResolution{})
	requireNamesError(t, err, NamesCodeInvalidParams, "invalid_name:character")

	reverse := ReverseResolution{}
	require.NoError(service.Reverse(nil, &ReverseNameArgs{Address: targetAddr}, &reverse))
	require.Equal("alice", reverse.Name)

	payerAddr, err := address.Format("M", hrp, payer.PublicKey().Address().Bytes())
	require.NoError(err)
	reverse = ReverseResolution{}
	require.NoError(service.Reverse(nil, &ReverseNameArgs{Address: payerAddr}, &reverse))
	require.Empty(reverse.Name)

	err = service.Reverse(nil, &ReverseNameArgs{Address: "V-dijets1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"}, &ReverseResolution{})
	requireNamesError(t, err, NamesCodeInvalidParams, "invalid_address")

	history := NameHistoryReply{}
	require.NoError(service.History(nil, &ResolveNameArgs{Name: "alice"}, &history))
	require.Len(history.Versions, 1)

	params := NameParamsReply{}
	require.NoError(service.Params(nil, nil, &params))
	require.True(params.Active)
	require.EqualValues(names.DevelopmentParams.RegisterFee, params.RegisterFee)
}

func requireNamesError(t *testing.T, err error, code json2.ErrorCode, reason string) {
	t.Helper()
	jsonErr, ok := err.(*json2.Error)
	require.True(t, ok, "expected a JSON-RPC error, got %v", err)
	require.Equal(t, code, jsonErr.Code)
	require.Equal(t, reason, jsonErr.Data.(NamesErrorData).Reason)
}
