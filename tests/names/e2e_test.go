//go:build names_e2e

// Package namese2e exercises native naming and name-based payments on a
// real 5-node network. Run with:
//
//	./scripts/build_dijets.sh
//	go test -tags names_e2e -timeout 60m -v ./tests/names/
package namese2e

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/api/info"
	"github.com/lasthyphen/dijetsnodesgo/genesis"
	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/tests/localnet"
	"github.com/lasthyphen/dijetsnodesgo/txservice"
	"github.com/lasthyphen/dijetsnodesgo/utils/constants"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/utils/formatting/address"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/avm"
	avmtxs "github.com/lasthyphen/dijetsnodesgo/vms/avm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/txs"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/p"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/x"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
)

const txTimeout = 2 * time.Minute

func ctxWithTimeout(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func newKey(t *testing.T) *crypto.PrivateKeySECP256K1R {
	key, err := (&crypto.FactorySECP256K1R{}).NewPrivateKey()
	require.NoError(t, err)
	return key.(*crypto.PrivateKeySECP256K1R)
}

func keys(k ...*crypto.PrivateKeySECP256K1R) []*crypto.PrivateKeySECP256K1R { return k }

func owner(key *crypto.PrivateKeySECP256K1R) *secp256k1fx.OutputOwners {
	return &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{key.PublicKey().Address()}}
}

func vAddr(t *testing.T, key *crypto.PrivateKeySECP256K1R) string {
	addr, err := address.Format("V", constants.GetHRP(localnet.NetworkID), key.PublicKey().Address().Bytes())
	require.NoError(t, err)
	return addr
}

type harness struct {
	t       *testing.T
	net     *localnet.Network
	pClient platformvm.Client
	names   *platformvm.NamesClient
	params  *platformvm.NameParamsReply
	pWallet p.Wallet
}

// register registers [name] for [target], owned by [target].
func (h *harness) register(name string, target *crypto.PrivateKeySECP256K1R, expiry time.Time) (ids.ID, error) {
	utx, err := h.pWallet.Builder().NewRegisterNameTx(
		name,
		target.PublicKey().Address(),
		owner(target),
		uint64(expiry.Unix()),
		uint64(h.params.RegisterFee),
	)
	if err != nil {
		return ids.Empty, err
	}
	return h.pWallet.IssueUnsignedTx(utx)
}

func (h *harness) record(name string) *names.Record {
	resolution, err := h.names.Resolve(ctxWithTimeout(h.t, 30*time.Second), name)
	require.NoError(h.t, err)
	require.NotNil(h.t, resolution.Record, name)
	o, err := primary.ParseNameOwner(resolution.Record.Owner)
	require.NoError(h.t, err)
	return &names.Record{
		Name:       resolution.Name,
		Target:     resolution.Record.TargetID,
		Owner:      o,
		Expiry:     uint64(resolution.Record.Expiry),
		Version:    uint64(resolution.Record.Version),
		Generation: uint64(resolution.Record.Generation),
	}
}

func (h *harness) update(current *names.Record, update p.NameUpdate) (ids.ID, error) {
	utx, err := h.pWallet.Builder().NewUpdateNameTx(current, update)
	if err != nil {
		return ids.Empty, err
	}
	return h.pWallet.IssueUnsignedTx(utx)
}

// issueForged signs [utx] with ewoq for its inputs and with [authKeys] for
// the trailing name credentials, one list per credential, then issues it.
func (h *harness) issueForged(utx txs.UnsignedTx, ins int, authKeys ...[]*crypto.PrivateKeySECP256K1R) error {
	signers := make([][]*crypto.PrivateKeySECP256K1R, 0, ins+len(authKeys))
	for i := 0; i < ins; i++ {
		signers = append(signers, []*crypto.PrivateKeySECP256K1R{genesis.EWOQKey})
	}
	signers = append(signers, authKeys...)
	tx, err := txs.NewSigned(utx, txs.Codec, signers)
	require.NoError(h.t, err)
	_, err = h.pClient.IssueTx(ctxWithTimeout(h.t, 30*time.Second), tx.Bytes())
	return err
}

// requireConsistentResolution checks that every node resolves [name]
// identically once they have accepted the same M-chain block.
func (h *harness) requireConsistentResolution(name string) *platformvm.NameResolution {
	var first *platformvm.NameResolution
	localnet.Eventually(h.t, time.Minute, time.Second, func() (bool, string) {
		var answers []*platformvm.NameResolution
		for _, nd := range h.net.Nodes {
			r, err := platformvm.NewNamesClient(nd.URI()).Resolve(context.Background(), name)
			if err != nil {
				return false, err.Error()
			}
			answers = append(answers, r)
		}
		first = answers[0]
		for i, r := range answers[1:] {
			if r.At.BlockID != first.At.BlockID {
				return false, "nodes are at different blocks"
			}
			if r.Status != first.Status || (r.Record == nil) != (first.Record == nil) ||
				(r.Record != nil && (r.Record.Version != first.Record.Version || r.Record.TargetID != first.Record.TargetID)) {
				h.t.Fatalf("node%d resolves %q differently at the same block: %+v vs %+v", i+2, name, r, first)
			}
		}
		return true, ""
	})
	return first
}

// tick issues an M-chain transaction so that chain time catches up with
// wall-clock time.
func (h *harness) tick() {
	_, err := h.pWallet.IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: h.pWallet.DJTXAssetID()},
		Out: &secp256k1fx.TransferOutput{
			Amt:          units.MilliDjtx,
			OutputOwners: *owner(genesis.EWOQKey),
		},
	}})
	require.NoError(h.t, err)
}

func balance(t *testing.T, uri string, key *crypto.PrivateKeySECP256K1R) uint64 {
	reply, err := avm.NewClient(uri, "V").GetBalance(ctxWithTimeout(t, 30*time.Second), key.PublicKey().Address(), "DJT", false)
	require.NoError(t, err)
	return uint64(reply.Balance)
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, reason, txservice.ErrorReason(err), err.Error())
}

func TestNamesEndToEnd(t *testing.T) {
	net := localnet.New(t, localnet.Options{Name: "names", BasePort: 19750})
	net.StartAll()
	n1 := net.Node(1)

	ewoq := genesis.EWOQKey
	alice := newKey(t)
	alice2 := newKey(t)
	carol := newKey(t)
	user := newKey(t)
	shortlived := newKey(t)
	retired := newKey(t)
	merchant := newKey(t)

	dataDir := t.TempDir()
	agentKeyFile := filepath.Join(dataDir, "keys", "bob.key")
	agent, err := txservice.LoadOrCreateKey(agentKeyFile)
	require.NoError(t, err)

	kc := secp256k1fx.NewKeychain(ewoq, alice, alice2, user, agent, shortlived, retired, merchant)
	wallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), kc)
	require.NoError(t, err)
	h := &harness{
		t:       t,
		net:     net,
		pClient: platformvm.NewClient(n1.URI()),
		names:   platformvm.NewNamesClient(n1.URI()),
		pWallet: wallet.P(),
	}
	// V-chain transfers use a wallet holding only ewoq's key: a wallet that
	// also held the agent's key would track the allowance's UTXOs, which the
	// transaction service spends behind its back.
	ewoqWallet, err := primary.NewWalletFromURI(ctxWithTimeout(t, time.Minute), n1.URI(), secp256k1fx.NewKeychain(ewoq))
	require.NoError(t, err)
	xWallet := ewoqWallet.X()

	t.Log("== phase 1: registry parameters")
	h.params, err = h.names.Params(ctxWithTimeout(t, 30*time.Second))
	require.NoError(t, err)
	require.True(t, h.params.Active)
	net.Dump("names.params", h.params)

	t.Log("== phase 2: registration and consistent resolution")
	inAnHour := time.Now().Add(time.Hour)
	registerTxID, err := h.register("alice", alice, inAnHour)
	require.NoError(t, err)
	resolution := h.requireConsistentResolution("Alice")
	require.Equal(t, "ACTIVE", resolution.Status)
	require.True(t, resolution.Resolvable)
	require.Equal(t, vAddr(t, alice), resolution.Record.Target)
	require.Equal(t, registerTxID, resolution.Record.RecordID)
	net.Dump("names.resolve.alice", resolution)

	for _, k := range []struct {
		name string
		key  *crypto.PrivateKeySECP256K1R
	}{{"demo-user", user}, {"bob", agent}, {"dijets-cafe", merchant}} {
		_, err := h.register(k.name, k.key, inAnHour)
		require.NoError(t, err, k.name)
	}

	t.Log("== phase 3: duplicate, malformed and unconsented registrations are rejected")
	_, err = h.register("alice", carol, inAnHour)
	require.Error(t, err, "the wallet can't sign for carol's consent")
	// Build with ewoq as a consenting target, then sign with carol's key.
	utx, err := h.pWallet.Builder().NewRegisterNameTx("alice", ewoq.PublicKey().Address(), owner(ewoq), uint64(inAnHour.Unix()), uint64(h.params.RegisterFee))
	require.NoError(t, err)
	err = h.issueForged(utx, len(utx.Ins), keys(ewoq))
	require.ErrorContains(t, err, "not available for registration")
	for _, bad := range []string{"Alice", "al ice", "admin", "a", "v-alice", "xn--alice"} {
		utx.Name = bad
		err = h.issueForged(utx, len(utx.Ins), keys(ewoq))
		require.ErrorContains(t, err, "invalid name", bad)
	}
	utx.Name = "carol"
	utx.Target = carol.PublicKey().Address()
	err = h.issueForged(utx, len(utx.Ins), keys(ewoq))
	require.ErrorContains(t, err, "target did not authorize the name")

	t.Log("== phase 4: unauthorized updates and ownership transfers are rejected")
	current := h.record("alice")
	forgedOwner := *current
	forgedOwner.Owner = owner(ewoq) // lets the builder authorize as ewoq
	uutx, err := h.pWallet.Builder().NewUpdateNameTx(&forgedOwner, p.NameUpdate{
		Target: current.Target,
		Owner:  owner(ewoq),
		Expiry: current.Expiry,
	})
	require.NoError(t, err)
	err = h.issueForged(uutx, len(uutx.Ins), keys(ewoq), keys())
	require.ErrorContains(t, err, "unauthorized name update", "ownership transfer by a non-owner")
	uutx.Owner = owner(alice)
	uutx.Target = ewoq.PublicKey().Address()
	uutx.TargetAuth = &secp256k1fx.Input{SigIndices: []uint32{0}}
	err = h.issueForged(uutx, len(uutx.Ins), keys(ewoq), keys(ewoq))
	require.ErrorContains(t, err, "unauthorized name update", "redirection by a non-owner")
	require.EqualValues(t, 1, h.record("alice").Version)

	t.Log("== phase 5: the owner moves the name; stale updates fail")
	v1 := h.record("alice")
	_, err = h.update(v1, p.NameUpdate{Target: alice2.PublicKey().Address(), Owner: owner(alice), Expiry: v1.Expiry})
	require.NoError(t, err)
	resolution = h.requireConsistentResolution("alice")
	require.EqualValues(t, 2, resolution.Record.Version)
	require.Equal(t, vAddr(t, alice2), resolution.Record.Target)
	_, err = h.update(v1, p.NameUpdate{Target: alice.PublicKey().Address(), Owner: owner(alice), Expiry: v1.Expiry})
	require.ErrorContains(t, err, "version mismatch")
	reverse, err := h.names.Reverse(ctxWithTimeout(t, 30*time.Second), vAddr(t, alice))
	require.NoError(t, err)
	require.Empty(t, reverse.Name, "the old address no longer carries the name")
	reverse, err = h.names.Reverse(ctxWithTimeout(t, 30*time.Second), vAddr(t, alice2))
	require.NoError(t, err)
	require.Equal(t, "alice", reverse.Name)
	history, err := h.names.History(ctxWithTimeout(t, 30*time.Second), "alice")
	require.NoError(t, err)
	require.Len(t, history.Versions, 2)
	net.Dump("names.history.alice", history)

	t.Log("== phase 6: revocation and expiry")
	shortExpiry := time.Now().Add(time.Duration(h.params.MinTerm)*time.Second + 15*time.Second)
	_, err = h.register("shortlived", shortlived, shortExpiry)
	require.NoError(t, err)
	_, err = h.register("retired", retired, inAnHour)
	require.NoError(t, err)
	r := h.record("retired")
	_, err = h.update(r, p.NameUpdate{Target: r.Target, Owner: owner(retired), Expiry: r.Expiry, Revoke: true})
	require.NoError(t, err)
	require.Equal(t, "REVOKED", h.requireConsistentResolution("retired").Status)

	t.Log("== phase 7: the transaction service pays by name")
	djtAsset, err := avm.NewClient(n1.URI(), "V").GetAssetDescription(ctxWithTimeout(t, 30*time.Second), "DJT")
	require.NoError(t, err)
	allowance := &secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{user.PublicKey().Address(), agent.PublicKey().Address()}}
	allowance.Sort()
	_, err = xWallet.IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: djtAsset.AssetID},
		Out:   &secp256k1fx.TransferOutput{Amt: 100 * units.Djtx, OutputOwners: *allowance},
	}})
	require.NoError(t, err)

	config := &txservice.Config{
		NodeURI: n1.URI(),
		DataDir: filepath.Join(dataDir, "txservice"),
		Clients: []txservice.ClientConfig{
			{Name: "bob", TokenSHA256: txservice.HashToken("agent-token"), Role: txservice.RoleAgent, Delegations: []string{"demo"}},
			{Name: "user", TokenSHA256: txservice.HashToken("approver-token"), Role: txservice.RoleApprover, Delegations: []string{"demo"}},
		},
		Delegations: []txservice.DelegationConfig{{
			ID:               "demo",
			Principal:        "demo-user",
			PrincipalAddress: vAddr(t, user),
			AgentName:        "bob",
			AgentKeyFile:     agentKeyFile,
			Assets:           []string{"DJT"},
			PerPaymentMax:    "60",
			DailyMax:         "1000",
			AutoApproveMax:   "0",
		}},
		ExpiryMargin: txservice.Duration(10 * time.Second),
		// Rate limiting is covered by the unit tests; this test calls the
		// service faster than a person would.
		RateLimits: txservice.RateLimits{
			Read:    txservice.RateLimit{PerMinute: 1000, Burst: 1000},
			Prepare: txservice.RateLimit{PerMinute: 1000, Burst: 1000},
			Submit:  txservice.RateLimit{PerMinute: 1000, Burst: 1000},
		},
	}
	serviceNetwork, err := txservice.NewNodeNetwork(ctxWithTimeout(t, 30*time.Second), n1.URI())
	require.NoError(t, err)
	var stopService func()
	startService := func() (*txservice.Client, *txservice.Client) {
		svc, err := txservice.NewService(ctxWithTimeout(t, 30*time.Second), config, serviceNetwork, log.New(io.Discard, "", 0), time.Now)
		require.NoError(t, err)
		handler, err := txservice.NewHandler(svc)
		require.NoError(t, err)
		server := httptest.NewServer(handler)
		stopService = func() {
			server.Close()
			_ = svc.Close()
		}
		t.Cleanup(stopService)
		return txservice.NewClient(server.URL, "agent-token"), txservice.NewClient(server.URL, "approver-token")
	}
	bob, approver := startService()

	ctx := ctxWithTimeout(t, 10*time.Minute)
	aliceBefore := balance(t, n1.URI(), alice2)
	payment, err := bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "10", IdempotencyKey: "first"})
	require.NoError(t, err)
	require.Equal(t, vAddr(t, alice2), payment.To.Address)
	_, err = bob.Submit(ctx, payment.ID)
	requireReason(t, err, txservice.ReasonApprovalRequired)
	_, err = bob.Approve(ctx, payment.ID, payment.Digest)
	requireReason(t, err, txservice.ReasonForbidden)
	_, err = approver.Approve(ctx, payment.ID, payment.Digest)
	require.NoError(t, err)
	payment, err = approver.Submit(ctx, payment.ID)
	require.NoError(t, err)
	require.Equal(t, txservice.StatusAccepted, payment.Status)
	firstTxID := payment.TxID
	require.Equal(t, aliceBefore+10*units.Djtx, balance(t, n1.URI(), alice2))
	for _, nd := range net.Nodes {
		localnet.Eventually(t, time.Minute, time.Second, func() (bool, string) {
			status, err := avm.NewClient(nd.URI(), "V").GetTxStatus(context.Background(), firstTxID)
			return err == nil && status == choices.Accepted, "payment not accepted everywhere yet"
		})
	}
	receipt, err := bob.Receipt(ctx, payment.ID)
	require.NoError(t, err)
	view := receipt.Transaction
	require.Equal(t, "Accepted", view.Status)
	require.Equal(t, "1-of-2 {", view.From.Label[:len("1-of-2 {")])
	require.Contains(t, view.From.Label, "demo-user")
	require.Contains(t, view.From.Label, "bob")
	require.Equal(t, "alice", view.To[0].Label)
	require.Equal(t, "0.001", view.Fees[0].Amount.Display)
	require.True(t, view.MemoClaim.Verified)
	roles := map[string]int{}
	for _, out := range view.Outputs {
		roles[out.Role]++
	}
	require.Equal(t, map[string]int{"payment": 1, "change": 1}, roles)
	net.Dump("payments.receipt", receipt)

	t.Log("== phase 8: a name change between preparation and submission is refused")
	payment, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "5"})
	require.NoError(t, err)
	_, err = approver.Approve(ctx, payment.ID, payment.Digest)
	require.NoError(t, err)
	v2 := h.record("alice")
	_, err = h.update(v2, p.NameUpdate{Target: alice.PublicKey().Address(), Owner: owner(alice), Expiry: v2.Expiry})
	require.NoError(t, err)
	h.requireConsistentResolution("alice")
	before := balance(t, n1.URI(), alice2) + balance(t, n1.URI(), alice)
	_, err = approver.Submit(ctx, payment.ID)
	requireReason(t, err, txservice.ReasonRecipientChanged)
	require.Equal(t, before, balance(t, n1.URI(), alice2)+balance(t, n1.URI(), alice))

	// The earlier payment keeps its verified memo, but the explorer view no
	// longer attributes the old address to alice.
	view, err = bob.DescribeTx(ctx, firstTxID)
	require.NoError(t, err)
	require.Equal(t, vAddr(t, alice2), view.To[0].Label)
	require.True(t, view.MemoClaim.Verified)

	t.Log("== phase 9: recipients that can't be paid")
	for recipient, reason := range map[string]string{
		"nobody":     txservice.ReasonNameNotFound,
		"retired":    txservice.ReasonNameNotResolvable,
		"Al ice":     txservice.ReasonInvalidName,
		"V-custom1x": txservice.ReasonInvalidRecipient,
	} {
		_, err := bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: recipient, Asset: "DJT", Amount: "1"})
		requireReason(t, err, reason)
	}
	_, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "61"})
	requireReason(t, err, txservice.ReasonPolicyDenied)

	t.Log("== phase 10: concurrent payments never double-spend; overspending is refused")
	var (
		wg       sync.WaitGroup
		approved []*txservice.Payment
	)
	for i := 0; i < 3; i++ {
		pmt, err := bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "dijets-cafe", Asset: "DJT", Amount: "20"})
		require.NoError(t, err)
		pmt, err = approver.Approve(ctx, pmt.ID, pmt.Digest)
		require.NoError(t, err)
		approved = append(approved, pmt)
	}
	results := make([]*txservice.Payment, len(approved))
	errs := make([]error, len(approved))
	for i, pmt := range approved {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i], errs[i] = approver.Submit(ctx, id)
		}(i, pmt.ID)
	}
	wg.Wait()
	// 100 - 10.001 leaves 89.999: three payments of 20.001 fit, and the
	// allowance ends with 29.996.
	for i := range approved {
		require.NoError(t, errs[i])
		require.Equal(t, txservice.StatusAccepted, results[i].Status)
	}
	require.Equal(t, 60*units.Djtx, balance(t, n1.URI(), merchant))
	balanceReply, err := bob.Balance(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, "29.996", balanceReply.Available.Display)

	payment, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "dijets-cafe", Asset: "DJT", Amount: "30"})
	require.NoError(t, err)
	_, err = approver.Approve(ctx, payment.ID, payment.Digest)
	require.NoError(t, err)
	_, err = approver.Submit(ctx, payment.ID)
	requireReason(t, err, txservice.ReasonInsufficientFunds)

	t.Log("== phase 11: replayed and conflicting transactions")
	firstTx, err := avm.NewClient(n1.URI(), "V").GetTx(ctx, firstTxID)
	require.NoError(t, err)
	merchantBefore := balance(t, n1.URI(), merchant)
	aliceBefore = balance(t, n1.URI(), alice2)
	replayID, replayErr := avm.NewClient(net.Node(3).URI(), "V").IssueTx(ctx, firstTx)
	t.Logf("re-issuing an accepted payment: id=%s err=%v", replayID, replayErr)
	time.Sleep(3 * time.Second)
	require.Equal(t, aliceBefore, balance(t, n1.URI(), alice2), "a replayed payment must not pay again")

	// Two conventional transfers spending the same UTXO, sent to different
	// nodes: snowstorm accepts at most one. A dedicated key keeps the
	// wallet's own UTXOs out of it.
	spender := newKey(t)
	_, err = xWallet.IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: djtAsset.AssetID},
		Out:   &secp256k1fx.TransferOutput{Amt: 5 * units.Djtx, OutputOwners: *owner(spender)},
	}})
	require.NoError(t, err)
	utxoBytes, _, _, err := avm.NewClient(n1.URI(), "V").GetUTXOs(ctx, []ids.ShortID{spender.PublicKey().Address()}, 10, ids.ShortEmpty, ids.Empty)
	require.NoError(t, err)
	require.Len(t, utxoBytes, 1)
	spendable := &djtx.UTXO{}
	_, err = x.Parser.Codec().Unmarshal(utxoBytes[0], spendable)
	require.NoError(t, err)
	conflicts := make([]ids.ID, 2)
	for i, to := range []*crypto.PrivateKeySECP256K1R{carol, retired} {
		tx := conflictingTransfer(t, xWallet, spendable, spender, to)
		conflicts[i], err = avm.NewClient(net.Node(i+1).URI(), "V").IssueTx(ctx, tx.Bytes())
		t.Logf("conflicting transfer %d: %s err=%v", i, conflicts[i], err)
	}
	// Snowstorm guarantees safety for conflicting transactions, not
	// liveness: neither may ever be decided. Honest traffic must keep being
	// accepted meanwhile, and no node may ever accept both.
	for i := 0; i < 5; i++ {
		_, err := xWallet.IssueBaseTx([]*djtx.TransferableOutput{{
			Asset: djtx.Asset{ID: djtAsset.AssetID},
			Out:   &secp256k1fx.TransferOutput{Amt: units.MilliDjtx, OutputOwners: *owner(merchant)},
		}})
		require.NoError(t, err, "honest transfers are accepted while a double spend is pending")
	}
	for _, nd := range net.Nodes {
		accepted := 0
		statuses := make([]choices.Status, len(conflicts))
		for i, id := range conflicts {
			statuses[i], err = avm.NewClient(nd.URI(), "V").GetTxStatus(ctx, id)
			require.NoError(t, err)
			if statuses[i] == choices.Accepted {
				accepted++
			}
		}
		t.Logf("node%d conflicting transfer statuses: %v", nd.Idx, statuses)
		require.LessOrEqual(t, accepted, 1, "node%d accepted both conflicting transfers", nd.Idx)
	}
	require.LessOrEqual(t, balance(t, n1.URI(), carol)+balance(t, n1.URI(), retired), uint64(units.Djtx), "at most one transfer paid")
	merchantBefore += 5 * units.MilliDjtx
	require.Equal(t, merchantBefore, balance(t, n1.URI(), merchant))

	t.Log("== phase 12: conventional transfers are unaffected and shown by address")
	plainTxID, err := xWallet.IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: djtAsset.AssetID},
		Out:   &secp256k1fx.TransferOutput{Amt: units.Djtx, OutputOwners: *owner(carol)},
	}})
	require.NoError(t, err)
	view, err = bob.DescribeTx(ctx, plainTxID)
	require.NoError(t, err)
	require.Nil(t, view.MemoClaim)
	for _, out := range view.Outputs {
		if out.Role == txservice.RolePayment {
			require.Equal(t, vAddr(t, carol), out.Owner.Label, "an unnamed address is shown as the address")
		}
	}

	t.Log("== phase 13: expiry, release and re-registration")
	time.Sleep(time.Until(shortExpiry.Add(2 * time.Second)))
	h.tick()
	require.Equal(t, "EXPIRED", h.requireConsistentResolution("shortlived").Status)
	_, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "shortlived", Asset: "DJT", Amount: "1"})
	requireReason(t, err, txservice.ReasonNameNotResolvable)
	time.Sleep(time.Duration(h.params.GracePeriod)*time.Second + 2*time.Second)
	h.tick()
	require.Equal(t, "RELEASED", h.requireConsistentResolution("shortlived").Status)
	_, err = h.register("shortlived", merchant, inAnHour)
	require.ErrorContains(t, err, "already bound", "merchant's address already carries dijets-cafe")
	_, err = h.register("shortlived", alice2, inAnHour)
	require.NoError(t, err)
	resolution = h.requireConsistentResolution("shortlived")
	require.EqualValues(t, 2, resolution.Record.Generation)

	t.Log("== phase 14: restart and recovery of naming state and payments")
	before14 := h.requireConsistentResolution("alice")
	net.RestartAll()
	after := h.requireConsistentResolution("alice")
	require.Equal(t, before14.Record, after.Record)
	history, err = platformvm.NewNamesClient(net.Node(4).URI()).History(ctxWithTimeout(t, 30*time.Second), "alice")
	require.NoError(t, err)
	require.Len(t, history.Versions, 3)
	stopService()
	bob, approver = startService()
	_, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "11", IdempotencyKey: "first"})
	requireReason(t, err, txservice.ReasonIdempotencyConflict)
	hist, err := bob.History(ctx, "demo", 50)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hist.Payments), 6)

	t.Log("== phase 15: revocation, and the user's own authority over the allowance")
	revoked, err := approver.RevokeDelegation(ctx, "demo")
	require.NoError(t, err)
	require.True(t, revoked.Revoked)
	_, err = bob.Prepare(ctx, txservice.PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "1"})
	requireReason(t, err, txservice.ReasonDelegationRevoked)

	userWallet, err := primary.NewWalletFromURI(ctx, n1.URI(), secp256k1fx.NewKeychain(user))
	require.NoError(t, err)
	userBefore := balance(t, n1.URI(), user)
	_, err = userWallet.X().IssueBaseTx([]*djtx.TransferableOutput{{
		Asset: djtx.Asset{ID: djtAsset.AssetID},
		Out:   &secp256k1fx.TransferOutput{Amt: 29_995 * units.MilliDjtx, OutputOwners: *owner(user)},
	}})
	require.NoError(t, err, "the user sweeps the allowance back with their own key")
	require.Equal(t, userBefore+29_995*units.MilliDjtx, balance(t, n1.URI(), user))
	balanceReply, err = bob.Balance(ctx, "demo")
	require.NoError(t, err)
	require.Equal(t, "0", balanceReply.Available.Display)

	t.Log("== phase 16: M, V and U chains are still healthy on every node")
	h.tick()
	for _, nd := range net.Nodes {
		for _, chain := range []string{"M", "V", "U"} {
			ok, err := info.NewClient(nd.URI()).IsBootstrapped(ctxWithTimeout(t, 30*time.Second), chain)
			require.NoError(t, err)
			require.True(t, ok, "node%d chain %s", nd.Idx, chain)
		}
		block, err := localnet.EVMBlockNumber(nd.URI())
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(block, "0x"))
	}
}

// conflictingTransfer spends all of [utxo], owned by [from], sending 1 DJT
// to [to] and the rest, less the fee, back to [from].
func conflictingTransfer(
	t *testing.T,
	ctx x.Context,
	utxo *djtx.UTXO,
	from *crypto.PrivateKeySECP256K1R,
	to *crypto.PrivateKeySECP256K1R,
) *avmtxs.Tx {
	in := utxo.Out.(*secp256k1fx.TransferOutput)
	outs := []*djtx.TransferableOutput{
		{
			Asset: utxo.Asset,
			Out:   &secp256k1fx.TransferOutput{Amt: units.Djtx, OutputOwners: *owner(to)},
		},
		{
			Asset: utxo.Asset,
			Out:   &secp256k1fx.TransferOutput{Amt: in.Amt - units.Djtx - ctx.BaseTxFee(), OutputOwners: *owner(from)},
		},
	}
	djtx.SortTransferableOutputs(outs, x.Parser.Codec())
	tx := &avmtxs.Tx{Unsigned: &avmtxs.BaseTx{BaseTx: djtx.BaseTx{
		NetworkID:    ctx.NetworkID(),
		BlockchainID: ctx.BlockchainID(),
		Ins: []*djtx.TransferableInput{{
			UTXOID: utxo.UTXOID,
			Asset:  utxo.Asset,
			In: &secp256k1fx.TransferInput{
				Amt:   in.Amt,
				Input: secp256k1fx.Input{SigIndices: []uint32{0}},
			},
		}},
		Outs: outs,
	}}}
	require.NoError(t, tx.SignSECP256K1Fx(x.Parser.Codec(), [][]*crypto.PrivateKeySECP256K1R{{from}}))
	return tx
}
