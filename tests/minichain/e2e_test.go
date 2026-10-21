//go:build minichain_e2e

package minichaine2e

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/minichain"
	"github.com/lasthyphen/dijetsnodesgo/minichain/workflow"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/x"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

const (
	assetAlias  = "MINI"
	txTimeout   = 2 * time.Minute
	stallWindow = 30 * time.Second
	activeWait  = 4 * time.Minute
)

type miniChain struct {
	name     string
	subnetID ids.ID
	chainID  ids.ID
	genesis  []byte
	members  []int
}

func ctxWithTimeout(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func buildGenesis(t *testing.T, memo string) []byte {
	holder, err := address.FormatBech32(constants.GetHRP(networkID), genesis.EWOQKey.PublicKey().Address().Bytes())
	require.NoError(t, err)
	g, err := workflow.BuildAVMGenesis(networkID, map[string]avm.AssetDefinition{
		assetAlias: {
			Name:         "Mini Chain Token",
			Symbol:       assetAlias,
			Denomination: 9,
			Memo:         memo,
			InitialState: map[string][]interface{}{
				"fixedCap": {avm.Holder{Amount: json.Uint64(1_000_000 * units.Djtx), Address: holder}},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, workflow.ValidateAVMGenesis(g))
	return g
}

// chainWallet returns a wallet for an AVM mini-chain served by [uri].
func chainWallet(t *testing.T, uri string, chainID ids.ID, kc *secp256k1fx.Keychain) (x.Wallet, ids.ID) {
	ctx := ctxWithTimeout(t, time.Minute)
	infoClient := info.NewClient(uri)
	fees, err := infoClient.GetTxFee(ctx)
	require.NoError(t, err)
	avmClient := avm.NewClient(uri, chainID.String())
	asset, err := avmClient.GetAssetDescription(ctx, assetAlias)
	require.NoError(t, err)

	xCtx := x.NewContext(networkID, chainID, asset.AssetID, uint64(fees.TxFee), uint64(fees.CreateAssetTxFee))
	utxos := primary.NewUTXOs()
	require.NoError(t, primary.AddAllUTXOs(ctx, utxos, avmClient, x.Parser.Codec(), chainID, chainID, kc.Addresses().List()))
	backend := x.NewBackend(xCtx, chainID, primary.NewChainUTXOs(chainID, utxos))
	return x.NewWallet(x.NewBuilder(kc.Addresses(), backend), x.NewSigner(kc, backend), avmClient, backend), asset.AssetID
}

func send(w x.Wallet, assetID ids.ID, to ids.ShortID, amount uint64, options ...common.Option) (ids.ID, error) {
	return w.IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: assetID},
		Out: &secp256k1fx.TransferOutput{
			Amt:          amount,
			OutputOwners: secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{to}},
		},
	}}, options...)
}

func balance(t *testing.T, uri string, chainID, assetID ids.ID, addr ids.ShortID) uint64 {
	reply, err := avm.NewClient(uri, chainID.String()).GetBalance(ctxWithTimeout(t, 30*time.Second), addr, assetID.String(), false)
	require.NoError(t, err)
	return uint64(reply.Balance)
}

func awaitLifecycle(t *testing.T, nd *node, chainID ids.ID, want minichain.Lifecycle) *minichain.HealthReply {
	t.Helper()
	client := minichain.NewClient(nd.URI())
	var last *minichain.HealthReply
	eventually(t, activeWait, 2*time.Second, func() (bool, string) {
		reply, err := client.Health(ctxWithTimeout(t, 10*time.Second), chainID)
		if err != nil {
			return false, err.Error()
		}
		last = reply
		return reply.Observation.Lifecycle == want, fmt.Sprintf("node%d chain %s lifecycle=%s reasons=%v",
			nd.idx, chainID, reply.Observation.Lifecycle, reply.Observation.Reasons)
	})
	return last
}

func TestMiniChainsEndToEnd(t *testing.T) {
	net := newNetwork(t)
	kc := secp256k1fx.NewKeychain(genesis.EWOQKey)
	ewoq := genesis.EWOQKey.PublicKey().Address()
	owner := &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{ewoq}}

	t.Log("== phase 1: primary network baseline")
	net.startAll()
	n1 := net.node(1)
	pClient := platformvm.NewClient(n1.URI())
	mc := minichain.NewClient(n1.URI())

	wallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), kc)
	require.NoError(t, err)
	pWallet := wallet.P()
	xWallet := wallet.X()

	regressionChecks := func(label string) {
		t.Helper()
		ctx := ctxWithTimeout(t, txTimeout)
		height, err := pClient.GetHeight(ctx)
		require.NoError(t, err, label)
		// V-chain DAG consensus: a transfer is accepted.
		xTx, err := xWallet.IssueBaseTx([]*djtx.TransferableOutput{{
			Asset: djtx.Asset{ID: xWallet.DJTXAssetID()},
			Out: &secp256k1fx.TransferOutput{
				Amt:          units.MilliDjtx,
				OutputOwners: secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{ids.GenerateTestShortID()}},
			},
		}}, common.WithContext(ctx))
		require.NoError(t, err, "%s: V-chain transfer", label)
		// U-chain EVM answers.
		block, err := evmBlockNumber(n1.URI())
		require.NoError(t, err, "%s: U-chain", label)
		t.Logf("%s: M height=%d, V tx %s accepted, U block=%s", label, height, xTx, block)
	}
	regressionChecks("baseline")

	caps, err := mc.Capabilities(ctxWithTimeout(t, 10*time.Second), "")
	require.NoError(t, err)
	require.True(t, caps.StakingEnabled)
	list, err := mc.List(ctxWithTimeout(t, 10*time.Second), minichain.ListArgs{})
	require.NoError(t, err)
	require.Equal(t, 0, list.Counts.Registered)

	// The API is read-only: there is no creation/administration method.
	code, err := rawCall(n1.URI(), "/ext/bc/M/minichain", "minichain.create")
	require.NoError(t, err)
	require.NotZero(t, code)

	t.Log("== phase 2: register subnets (M-chain transactions)")
	alpha := &miniChain{name: "alpha", members: []int{1, 2, 3}, genesis: buildGenesis(t, "0x616c706861")}
	beta := &miniChain{name: "beta", members: []int{3, 4, 5}, genesis: buildGenesis(t, "0x62657461")}
	require.NotEqual(t, minichain.GenesisHash(alpha.genesis), minichain.GenesisHash(beta.genesis))
	for _, c := range []*miniChain{alpha, beta} {
		c.subnetID, err = pWallet.IssueCreateSubnetTx(owner, common.WithContext(ctxWithTimeout(t, txTimeout)))
		require.NoError(t, err)
		t.Logf("subnet %s = %s", c.name, c.subnetID)
		status, err := mc.GetCreationStatus(ctxWithTimeout(t, 10*time.Second), minichain.CreationStatusArgs{SubnetTxID: c.subnetID.String()})
		require.NoError(t, err)
		net.dump("getCreationStatus."+c.name+".subnet-only", status)
		require.Equal(t, minichain.StageSubnetRegistered, status.Stage)
	}

	// Operators opt in: committee members track their subnet. node4 also
	// tracks alpha without being a member, to show that tracking does not
	// grant a vote.
	net.node(1).whitelist = []ids.ID{alpha.subnetID}
	net.node(2).whitelist = []ids.ID{alpha.subnetID}
	net.node(3).whitelist = []ids.ID{alpha.subnetID, beta.subnetID}
	net.node(4).whitelist = []ids.ID{alpha.subnetID, beta.subnetID}
	net.node(5).whitelist = []ids.ID{beta.subnetID}
	net.restartAll()
	regressionChecks("after whitelisting restart")

	t.Log("== phase 3: create two mini-chains with distinct committees")
	for _, c := range []*miniChain{alpha, beta} {
		committee := make([]workflow.Member, 0, len(c.members))
		for _, idx := range c.members {
			committee = append(committee, workflow.Member{NodeID: net.node(idx).nodeID, Weight: 20})
		}
		start := time.Now().Add(20 * time.Second)
		res, err := workflow.Create(ctxWithTimeout(t, 5*time.Minute), pWallet, pClient, mc, workflow.Request{
			Name:            c.name,
			Genesis:         c.genesis,
			SubnetID:        c.subnetID,
			Committee:       committee,
			ValidationStart: start,
			ValidationEnd:   start.Add(25 * time.Hour),
		})
		require.NoError(t, err)
		require.True(t, res.CreatedChain)
		require.Len(t, res.ValidatorTxIDs, len(c.members))
		c.chainID = res.ChainID
		t.Logf("mini-chain %s = %s on subnet %s", c.name, c.chainID, c.subnetID)
	}
	require.NotEqual(t, alpha.chainID, beta.chainID)

	for _, c := range []*miniChain{alpha, beta} {
		for _, idx := range c.members {
			awaitLifecycle(t, net.node(idx), c.chainID, minichain.Active)
		}
	}

	// node1 does not track beta: registered, but not observable there.
	beta1, err := mc.Get(ctxWithTimeout(t, 10*time.Second), beta.chainID)
	require.NoError(t, err)
	require.Equal(t, minichain.Unknown, beta1.Observation.Lifecycle)
	require.False(t, beta1.Observation.Healthy)
	require.True(t, beta1.Observation.Registered)
	require.Empty(t, beta1.Descriptor.RPCEndpoints)

	list, err = mc.List(ctxWithTimeout(t, 10*time.Second), minichain.ListArgs{})
	require.NoError(t, err)
	net.dump("list.node1", list)
	net.dump("get.beta.node1-untracked", beta1)
	require.Equal(t, 2, list.Counts.Registered)
	require.Equal(t, 1, list.Counts.Tracked)
	require.Equal(t, 1, list.Counts.Healthy)
	descriptorsBefore := map[ids.ID]minichain.Descriptor{}
	for _, v := range list.Chains {
		descriptorsBefore[v.Descriptor.ID] = v.Descriptor
		require.Equal(t, minichain.PermissionedCommittee, v.Descriptor.ValidatorPolicy.Profile)
		require.Equal(t, minichain.EnforcedByConsensus, v.Descriptor.ValidatorPolicy.Enforcement)
		require.Equal(t, 3, v.Descriptor.Validators.CurrentCount)
	}

	vdrs, err := mc.Validators(ctxWithTimeout(t, 10*time.Second), alpha.chainID)
	require.NoError(t, err)
	net.dump("validators.alpha.node1", vdrs)
	alphaGet, err := mc.Get(ctxWithTimeout(t, 10*time.Second), alpha.chainID)
	require.NoError(t, err)
	net.dump("get.alpha.node1", alphaGet)
	alphaHealth, err := mc.Health(ctxWithTimeout(t, 10*time.Second), alpha.chainID)
	require.NoError(t, err)
	net.dump("health.alpha.node1", alphaHealth)
	capsAlpha, err := mc.Capabilities(ctxWithTimeout(t, 10*time.Second), alpha.chainID.String())
	require.NoError(t, err)
	net.dump("capabilities.alpha.node1", capsAlpha)
	gotMembers := map[ids.NodeID]bool{}
	for _, m := range vdrs.Members {
		gotMembers[m.NodeID] = true
		require.NotNil(t, m.Connected)
		require.True(t, *m.Connected)
	}
	for _, idx := range alpha.members {
		require.True(t, gotMembers[net.node(idx).nodeID])
	}
	require.False(t, gotMembers[net.node(4).nodeID], "tracking a subnet does not make a node a member")

	t.Log("== phase 4: independent execution and state")
	recvA, recvB := ids.GenerateTestShortID(), ids.GenerateTestShortID()
	alphaWallet, alphaAsset := chainWallet(t, net.node(1).URI(), alpha.chainID, kc)
	betaWallet, betaAsset := chainWallet(t, net.node(5).URI(), beta.chainID, kc)
	require.NotEqual(t, alphaAsset, betaAsset, "distinct genesis yields distinct assets")

	_, err = send(alphaWallet, alphaAsset, recvA, 1000, common.WithContext(ctxWithTimeout(t, txTimeout)))
	require.NoError(t, err)
	_, err = send(betaWallet, betaAsset, recvB, 2000, common.WithContext(ctxWithTimeout(t, txTimeout)))
	require.NoError(t, err)

	eventually(t, time.Minute, time.Second, func() (bool, string) {
		a := balance(t, net.node(2).URI(), alpha.chainID, alphaAsset, recvA)
		b := balance(t, net.node(4).URI(), beta.chainID, betaAsset, recvB)
		return a == 1000 && b == 2000, fmt.Sprintf("alpha=%d beta=%d", a, b)
	})
	require.Zero(t, balance(t, net.node(1).URI(), alpha.chainID, alphaAsset, recvB), "no state leaks between mini-chains")
	require.Zero(t, balance(t, net.node(5).URI(), beta.chainID, betaAsset, recvA))

	stats, err := mc.Stats(ctxWithTimeout(t, 10*time.Second), alpha.chainID)
	require.NoError(t, err)
	net.dump("stats.alpha.node1", stats)
	statsBeta1, err := mc.Stats(ctxWithTimeout(t, 10*time.Second), beta.chainID)
	require.NoError(t, err)
	net.dump("stats.beta.node1-untracked", statsBeta1)
	require.Nil(t, statsBeta1.AcceptedTxs)
	require.NotNil(t, stats.AcceptedTxs)
	require.GreaterOrEqual(t, uint64(*stats.AcceptedTxs), uint64(1))
	require.Nil(t, stats.Height)

	t.Log("== phase 5: duplicate, conflicting and unauthorized creation")
	alphaCommittee := []workflow.Member{}
	for _, idx := range alpha.members {
		alphaCommittee = append(alphaCommittee, workflow.Member{NodeID: net.node(idx).nodeID, Weight: 20})
	}
	retryStart := time.Now().Add(20 * time.Second)
	retry := workflow.Request{
		Name: alpha.name, Genesis: alpha.genesis, SubnetID: alpha.subnetID,
		Committee: alphaCommittee, ValidationStart: retryStart, ValidationEnd: retryStart.Add(25 * time.Hour),
	}
	res, err := workflow.Create(ctxWithTimeout(t, time.Minute), pWallet, pClient, mc, retry)
	require.NoError(t, err)
	require.False(t, res.CreatedChain, "retry is idempotent")
	require.Empty(t, res.ValidatorTxIDs)
	require.Equal(t, alpha.chainID, res.ChainID)

	conflict := retry
	conflict.Genesis = buildGenesis(t, "0x6f74686572")
	_, err = workflow.Create(ctxWithTimeout(t, time.Minute), pWallet, pClient, mc, conflict)
	require.ErrorIs(t, err, workflow.ErrConflictingRegistered)

	// A funded payer without the subnet owner's key cannot register a chain:
	// the M-chain rejects the forged subnet authorization.
	attacker, err := (&crypto.FactorySECP256K1R{}).NewPrivateKey()
	require.NoError(t, err)
	utx, err := pWallet.Builder().NewCreateChainTx(alpha.subnetID, alpha.genesis, constants.AVMID, workflow.FxIDs, "intruder")
	require.NoError(t, err)
	signers := make([][]*crypto.PrivateKeySECP256K1R, 0, len(utx.Ins)+1)
	for range utx.Ins {
		signers = append(signers, []*crypto.PrivateKeySECP256K1R{genesis.EWOQKey})
	}
	signers = append(signers, []*crypto.PrivateKeySECP256K1R{attacker.(*crypto.PrivateKeySECP256K1R)})
	forged, err := txs.NewSigned(utx, txs.Codec, signers)
	require.NoError(t, err)
	_, err = pClient.IssueTx(ctxWithTimeout(t, 30*time.Second), forged.Bytes())
	require.Error(t, err, "unauthorized chain creation must be rejected")
	t.Logf("forged CreateChainTx rejected: %v", err)

	t.Log("== phase 6: validator outage, committee enforcement and isolation")
	net.stop(net.node(2))
	// alpha has 2/3 of its committee weight online (node1, node3). node4
	// tracks alpha but is not a member, so it cannot make up for node2.
	h := awaitLifecycle(t, net.node(1), alpha.chainID, minichain.Degraded)
	net.dump("health.alpha.node1-degraded", h)
	require.Contains(t, h.Observation.Reasons, minichain.ReasonConnectedWeightTooLow)

	alphaWallet, _ = chainWallet(t, net.node(1).URI(), alpha.chainID, kc)
	stalledTx, err := send(alphaWallet, alphaAsset, recvA, 1, common.WithContext(ctxWithTimeout(t, stallWindow)))
	require.Error(t, err, "alpha must not finalise without its committee quorum")
	require.True(t, errors.Is(err, context.DeadlineExceeded), "unexpected error: %v", err)
	status, err := avm.NewClient(net.node(1).URI(), alpha.chainID.String()).GetTxStatus(ctxWithTimeout(t, 10*time.Second), stalledTx)
	require.NoError(t, err)
	require.NotEqual(t, choices.Accepted, status)
	t.Logf("alpha tx %s stalled with status %s while node2 is down", stalledTx, status)

	// beta (nodes 3,4,5) and the primary network keep working.
	awaitLifecycle(t, net.node(5), beta.chainID, minichain.Active)
	betaWallet, _ = chainWallet(t, net.node(5).URI(), beta.chainID, kc)
	_, err = send(betaWallet, betaAsset, recvB, 1, common.WithContext(ctxWithTimeout(t, txTimeout)))
	require.NoError(t, err, "beta is isolated from alpha's outage")
	regressionChecks("during alpha outage")

	net.start(net.node(2))
	net.awaitBootstrapped(net.node(2), "M", "V", "U")
	eventually(t, 3*time.Minute, 2*time.Second, func() (bool, string) {
		st, err := avm.NewClient(net.node(1).URI(), alpha.chainID.String()).GetTxStatus(ctxWithTimeout(t, 10*time.Second), stalledTx)
		return err == nil && st == choices.Accepted, fmt.Sprintf("status=%s err=%v", st, err)
	})
	awaitLifecycle(t, net.node(1), alpha.chainID, minichain.Active)
	t.Logf("alpha recovered and accepted %s after node2 rejoined", stalledTx)

	t.Log("== phase 7: failed initialisation is isolated")
	gammaID, err := pWallet.IssueCreateChainTx(beta.subnetID, []byte("not an avm genesis"), constants.AVMID, workflow.FxIDs, "gamma",
		common.WithContext(ctxWithTimeout(t, txTimeout)))
	require.NoError(t, err, "the M-chain does not validate VM genesis; the workflow does")
	require.Error(t, workflow.ValidateAVMGenesis([]byte("not an avm genesis")))
	gammaHealth := awaitLifecycle(t, net.node(5), gammaID, minichain.Failed)
	net.dump("health.gamma.node5-failed", gammaHealth)
	require.Equal(t, []string{minichain.ReasonInitialisationFailed}, gammaHealth.Observation.Reasons)
	awaitLifecycle(t, net.node(5), beta.chainID, minichain.Active)
	regressionChecks("after gamma failure")

	// Events are recorded by the observer's poll loop, so node3 may observe
	// gamma failing up to one poll interval after node5's API reports it.
	var (
		events *minichain.EventsReply
		seen   map[minichain.EventType]int
	)
	eventually(t, time.Minute, time.Second, func() (bool, string) {
		events, err = minichain.NewClient(net.node(3).URI()).Events(ctxWithTimeout(t, 10*time.Second), 0, 500)
		if err != nil {
			return false, err.Error()
		}
		seen = map[minichain.EventType]int{}
		for _, e := range events.Events {
			seen[e.Type]++
		}
		return seen[minichain.EventFailed] > 0, fmt.Sprintf("node3 events by type: %v", seen)
	})
	net.dump("events.node3", events)
	for _, e := range events.Events {
		require.False(t, e.Authoritative)
	}
	require.Positive(t, seen[minichain.EventRegisteredObserved])
	bootBefore := events.BootID

	t.Log("== phase 8: full restart and recovery")
	// Snapshot the descriptors right before the restart: registering gamma
	// on beta's subnet changed beta's subnetChainCount since phase 3.
	list, err = mc.List(ctxWithTimeout(t, 10*time.Second), minichain.ListArgs{})
	require.NoError(t, err)
	for _, v := range list.Chains {
		descriptorsBefore[v.Descriptor.ID] = v.Descriptor
	}
	net.restartAll()
	for _, c := range []*miniChain{alpha, beta} {
		for _, idx := range c.members {
			awaitLifecycle(t, net.node(idx), c.chainID, minichain.Active)
		}
	}
	awaitLifecycle(t, net.node(5), gammaID, minichain.Failed)
	require.Equal(t, uint64(1001), balance(t, net.node(3).URI(), alpha.chainID, alphaAsset, recvA))
	require.Equal(t, uint64(2001), balance(t, net.node(3).URI(), beta.chainID, betaAsset, recvB))

	list, err = mc.List(ctxWithTimeout(t, 10*time.Second), minichain.ListArgs{})
	require.NoError(t, err)
	require.Equal(t, 3, list.Counts.Registered)
	for _, v := range list.Chains {
		before, ok := descriptorsBefore[v.Descriptor.ID]
		if !ok {
			continue
		}
		// The derived registry view is rebuilt from M-chain state after restart.
		v.Descriptor.Registration.ObservedAtMChainHeight = before.Registration.ObservedAtMChainHeight
		require.Equal(t, before, v.Descriptor)
	}
	eventsAfter, err := minichain.NewClient(net.node(3).URI()).Events(ctxWithTimeout(t, 10*time.Second), 0, 10)
	require.NoError(t, err)
	require.NotEqual(t, bootBefore, eventsAfter.BootID, "consumers can detect observer restarts")
	regressionChecks("after full restart")
}
