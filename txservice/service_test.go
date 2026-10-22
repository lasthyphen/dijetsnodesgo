package txservice

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
	"github.com/lasthyphen/dijetsnodesgo/utils/units"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
)

type harness struct {
	t        *testing.T
	dir      string
	config   *Config
	net      *fakeNetwork
	svc      *Service
	nowLock  sync.Mutex
	now      time.Time
	user     ids.ShortID
	alice    ids.ShortID
	agent    *ClientConfig
	approver *ClientConfig
	outsider *ClientConfig
}

func newHarness(t *testing.T, mutate func(*Config)) *harness {
	h := &harness{
		t:     t,
		dir:   t.TempDir(),
		net:   newFakeNetwork(),
		now:   time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
		user:  ids.GenerateTestShortID(),
		alice: ids.GenerateTestShortID(),
	}
	h.config = &Config{
		NodeURI: "fake",
		DataDir: h.dir,
		Clients: []ClientConfig{
			{Name: "bob-agent", TokenSHA256: HashToken("a"), Role: RoleAgent, Delegations: []string{"demo"}},
			{Name: "user-approver", TokenSHA256: HashToken("b"), Role: RoleApprover, Delegations: []string{"demo"}},
			{Name: "outsider", TokenSHA256: HashToken("c"), Role: RoleApprover},
		},
		Delegations: []DelegationConfig{{
			ID:               "demo",
			Principal:        "demo-user",
			PrincipalAddress: formatVAddress(testHRP, h.user),
			AgentName:        "bob",
			AgentKeyFile:     filepath.Join(h.dir, "keys", "agent.key"),
			Assets:           []string{"DJT"},
			PerPaymentMax:    "50",
			DailyMax:         "100",
			AutoApproveMax:   "0",
		}},
	}
	// Tests make many calls quickly; TestRateLimit sets its own limits.
	generous := RateLimit{PerMinute: 1000, Burst: 1000}
	h.config.RateLimits = RateLimits{Read: generous, Prepare: generous, Submit: generous}
	h.config.applyDefaults()
	if mutate != nil {
		mutate(h.config)
	}
	require.NoError(t, h.config.validate())
	h.agent = &h.config.Clients[0]
	h.approver = &h.config.Clients[1]
	h.outsider = &h.config.Clients[2]
	h.start()
	h.net.register("alice", h.alice, 1)
	return h
}

func (h *harness) start() {
	svc, err := NewService(context.Background(), h.config, h.net, log.New(io.Discard, "", 0), h.clock)
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = svc.Close() })
	h.svc = svc
}

func (h *harness) clock() time.Time {
	h.nowLock.Lock()
	defer h.nowLock.Unlock()
	return h.now
}

func (h *harness) advance(d time.Duration) {
	h.nowLock.Lock()
	defer h.nowLock.Unlock()
	h.now = h.now.Add(d)
}

func (h *harness) delegation() *delegation {
	return h.svc.delegations["demo"]
}

// fundAllowance sends [djt] whole DJT to the allowance owner set.
func (h *harness) fundAllowance(djt uint64) {
	h.net.fund(h.delegation().owner, djt*units.Djtx)
}

func req(client *ClientConfig) *http.Request {
	r := httptest.NewRequest(http.MethodPost, Endpoint, nil)
	if client == nil {
		return r
	}
	return r.WithContext(withCaller(r.Context(), client))
}

func (h *harness) prepare(client *ClientConfig, recipient, amount string) (*Payment, error) {
	reply := PaymentReply{}
	err := h.svc.Prepare(req(client), &PrepareArgs{
		Delegation: "demo",
		Recipient:  recipient,
		Asset:      "DJT",
		Amount:     amount,
	}, &reply)
	return reply.Payment, err
}

func (h *harness) approve(p *Payment) *Payment {
	reply := PaymentReply{}
	require.NoError(h.t, h.svc.Approve(req(h.approver), &ApproveArgs{PaymentID: p.ID, Digest: p.Digest}, &reply))
	return reply.Payment
}

func (h *harness) submit(client *ClientConfig, p *Payment) (*Payment, error) {
	reply := PaymentReply{}
	err := h.svc.Submit(req(client), &PaymentArgs{PaymentID: p.ID}, &reply)
	return reply.Payment, err
}

func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, reason, ErrorReason(err), err.Error())
}

func TestPayByName(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	h.fundAllowance(100)

	p, err := h.prepare(h.agent, "Alice", "10")
	require.NoError(err)
	require.Equal(StatusPrepared, p.Status)
	require.True(p.ApprovalRequired)
	require.Equal("alice", p.To.Name)
	require.Equal(formatVAddress(testHRP, h.alice), p.To.Address)
	require.EqualValues(1, p.To.Record.Version)
	require.EqualValues(10*units.Djtx, p.Amount.Units)
	require.Equal("10", p.Amount.Display)
	require.Equal("0.001", p.Fee.Display)
	require.Equal("djtname/1 to=alice g=1 v=1", p.Memo)
	require.Equal("demo-user", p.From.Owner.Name)
	require.Equal("bob", p.From.Agent.Name)

	// Preparing never sends anything.
	require.Zero(h.net.issued)

	// The agent can't submit or approve on the user's behalf.
	_, err = h.submit(h.agent, p)
	requireReason(t, err, ReasonApprovalRequired)
	err = h.svc.Approve(req(h.agent), &ApproveArgs{PaymentID: p.ID, Digest: p.Digest}, &PaymentReply{})
	requireReason(t, err, ReasonForbidden)

	// Approval must quote the digest of the payment shown.
	err = h.svc.Approve(req(h.approver), &ApproveArgs{PaymentID: p.ID, Digest: "00"}, &PaymentReply{})
	requireReason(t, err, ReasonApprovalMismatch)

	p = h.approve(p)
	require.Equal(StatusApproved, p.Status)
	require.Equal("user-approver", p.ApprovedBy)

	// An approved payment that policy didn't approve still needs the
	// approver to submit it.
	_, err = h.submit(h.agent, p)
	requireReason(t, err, ReasonForbidden)

	p, err = h.submit(h.approver, p)
	require.NoError(err)
	require.Equal(StatusAccepted, p.Status)
	require.NotEqual(ids.Empty, p.TxID)
	require.Equal(1, h.net.issued)

	receipt := Receipt{}
	require.NoError(h.svc.Receipt(req(h.agent), &PaymentArgs{PaymentID: p.ID}, &receipt))
	view := receipt.Transaction
	require.Equal("Accepted", view.Status)
	require.Equal("1-of-2 {"+view.From.Addresses[0].Address+", "+view.From.Addresses[1].Address+"}", view.From.Label)
	require.Len(view.To, 1)
	require.Equal("alice", view.To[0].Label)
	require.Len(view.Fees, 1)
	require.Equal("0.001", view.Fees[0].Amount.Display)
	require.NotNil(view.MemoClaim)
	require.True(view.MemoClaim.Verified)

	var payment, change int
	for _, out := range view.Outputs {
		switch out.Role {
		case RolePayment:
			payment++
			require.Equal("10", out.Amount.Display)
		case RoleChange:
			change++
			require.Equal("89.999", out.Amount.Display)
		}
	}
	require.Equal(1, payment)
	require.Equal(1, change)

	// The audit chain recorded every step.
	entries, err := ReadAuditLog(filepath.Join(h.dir, "audit.jsonl"))
	require.NoError(err)
	var events []string
	for _, e := range entries {
		events = append(events, e.Event)
	}
	require.Equal([]string{"prepared", "approval_mismatch", "approved", "submitted", "decided"}, events)
}

func TestNameChangedBeforeSubmission(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	h.fundAllowance(100)

	p, err := h.prepare(h.agent, "alice", "10")
	require.NoError(err)
	p = h.approve(p)

	// Alice moves her name to a new address after the user approved.
	h.net.register("alice", ids.GenerateTestShortID(), 2)

	_, err = h.submit(h.approver, p)
	requireReason(t, err, ReasonRecipientChanged)
	require.Zero(h.net.issued, "nothing may be sent to either address")

	reply := PaymentReply{}
	require.NoError(h.svc.Get(req(h.agent), &PaymentArgs{PaymentID: p.ID}, &reply))
	require.Equal(StatusFailed, reply.Payment.Status)
	require.Equal(ReasonRecipientChanged, reply.Payment.FailureReason)

	// Preparing again shows the new address for approval.
	again, err := h.prepare(h.agent, "alice", "10")
	require.NoError(err)
	require.NotEqual(p.To.Address, again.To.Address)
	require.EqualValues(2, again.To.Record.Version)
}

func TestRecipientErrors(t *testing.T) {
	h := newHarness(t, nil)
	h.fundAllowance(100)
	h.net.register("lapsed", ids.GenerateTestShortID(), 1)
	h.net.expired["lapsed"] = true
	h.net.register("soon", ids.GenerateTestShortID(), 1)
	// Expires within the service's safety margin.
	h.net.records["soon"][0].Expiry = json.Uint64(h.now.Add(time.Minute).Unix())

	tests := []struct {
		recipient string
		reason    string
	}{
		{"nobody", ReasonNameNotFound},
		{"al ice", ReasonInvalidName},
		{"аlice", ReasonInvalidName}, // Cyrillic "а"
		{"lapsed", ReasonNameNotResolvable},
		{"soon", ReasonNameExpiring},
		{"V-local1notanaddress", ReasonInvalidRecipient},
		{"M-" + formatVAddress(testHRP, h.alice)[2:], ReasonInvalidRecipient},
		{"", ReasonInvalidRecipient},
	}
	for _, test := range tests {
		t.Run(test.recipient, func(t *testing.T) {
			_, err := h.prepare(h.agent, test.recipient, "1")
			requireReason(t, err, test.reason)
		})
	}

	// Paying the allowance itself is refused.
	_, err := h.prepare(h.agent, formatVAddress(testHRP, h.user), "1")
	requireReason(t, err, ReasonSelfPayment)

	// A raw address is accepted and labelled with its name if it has one.
	p, err := h.prepare(h.agent, formatVAddress(testHRP, h.alice), "1")
	require.NoError(t, err)
	require.Equal(t, "alice", p.To.Name)
	require.Nil(t, p.To.Record)
	require.Empty(t, p.Memo)
	require.Zero(t, h.net.issued)
}

func TestAmountAndAssetErrors(t *testing.T) {
	h := newHarness(t, nil)
	for _, amount := range []string{"", "0", "-1", "1e3", "1,000", "0.0000000001", "ten", "18446744073709551616"} {
		_, err := h.prepare(h.agent, "alice", amount)
		requireReason(t, err, ReasonInvalidAmount)
	}
	err := h.svc.Prepare(req(h.agent), &PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "BTC", Amount: "1"}, &PaymentReply{})
	requireReason(t, err, ReasonUnsupportedAsset)
}

func TestPolicyLimits(t *testing.T) {
	h := newHarness(t, nil)
	h.fundAllowance(200)

	_, err := h.prepare(h.agent, "alice", "50.000000001")
	requireReason(t, err, ReasonPolicyDenied)

	// Spend 2 x 45 of the 100 daily limit.
	for i := 0; i < 2; i++ {
		p, err := h.prepare(h.agent, "alice", "45")
		require.NoError(t, err)
		_, err = h.submit(h.approver, h.approve(p))
		require.NoError(t, err)
	}
	_, err = h.prepare(h.agent, "alice", "10")
	requireReason(t, err, ReasonPolicyDenied)

	// The window is a trailing 24 hours.
	h.advance(24*time.Hour + time.Second)
	_, err = h.prepare(h.agent, "alice", "10")
	require.NoError(t, err)
}

func TestNamedRecipientsOnly(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Delegations[0].NamedRecipientsOnly = true })
	_, err := h.prepare(h.agent, formatVAddress(testHRP, h.alice), "1")
	requireReason(t, err, ReasonPolicyDenied)
}

func TestInsufficientFunds(t *testing.T) {
	h := newHarness(t, nil)
	h.fundAllowance(5)
	// A UTXO the agent shares with someone else is not allowance.
	h.net.fund(&secp256k1fx.OutputOwners{Threshold: 1, Addrs: []ids.ShortID{h.delegation().agent}}, 100*units.Djtx)

	p, err := h.prepare(h.agent, "alice", "10")
	require.NoError(t, err)
	_, err = h.submit(h.approver, h.approve(p))
	requireReason(t, err, ReasonInsufficientFunds)
	require.Zero(t, h.net.issued)
}

func TestIdempotencyAndRepeatedSubmission(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	h.fundAllowance(100)

	args := &PrepareArgs{Delegation: "demo", Recipient: "alice", Asset: "DJT", Amount: "10", IdempotencyKey: "k1"}
	first, second := PaymentReply{}, PaymentReply{}
	require.NoError(h.svc.Prepare(req(h.agent), args, &first))
	require.NoError(h.svc.Prepare(req(h.agent), args, &second))
	require.Equal(first.Payment.ID, second.Payment.ID)

	args.Amount = "11"
	requireReason(t, h.svc.Prepare(req(h.agent), args, &PaymentReply{}), ReasonIdempotencyConflict)

	p := h.approve(first.Payment)
	sent, err := h.submit(h.approver, p)
	require.NoError(err)
	again, err := h.submit(h.approver, p)
	require.NoError(err)
	require.Equal(sent.TxID, again.TxID)
	require.Equal(1, h.net.issued, "a payment is sent at most once")
}

func TestConcurrentSpending(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	// A single UTXO: concurrent payments must not spend it twice.
	h.fundAllowance(100)

	var payments []*Payment
	for i := 0; i < 3; i++ {
		p, err := h.prepare(h.agent, "alice", "10")
		require.NoError(err)
		payments = append(payments, h.approve(p))
	}

	var wg sync.WaitGroup
	results := make([]*Payment, len(payments))
	errs := make([]error, len(payments))
	for i, p := range payments {
		wg.Add(1)
		go func(i int, p *Payment) {
			defer wg.Done()
			results[i], errs[i] = h.submit(h.approver, p)
		}(i, p)
	}
	wg.Wait()

	// Spends are serialized, and each spends the previous one's change.
	for i := range payments {
		require.NoError(errs[i])
		require.Equal(StatusAccepted, results[i].Status)
	}
	require.Equal(3, h.net.issued)
}

func TestUndecidedPaymentReservesItsInputs(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.DecisionTimeout = Duration(10 * time.Millisecond) })
	h.fundAllowance(100)
	h.net.hold = true

	first, err := h.prepare(h.agent, "alice", "10")
	require.NoError(t, err)
	first, err = h.submit(h.approver, h.approve(first))
	require.NoError(t, err)
	require.Equal(t, StatusSubmitted, first.Status)

	// The only UTXO is reserved by the undecided payment, and its change
	// doesn't exist until it is accepted.
	second, err := h.prepare(h.agent, "alice", "10")
	require.NoError(t, err)
	_, err = h.submit(h.approver, h.approve(second))
	requireReason(t, err, ReasonInsufficientFunds)
	require.Equal(t, 1, h.net.issued)
}

func TestAuthorization(t *testing.T) {
	h := newHarness(t, nil)

	err := h.svc.Prepare(req(nil), &PrepareArgs{Delegation: "demo"}, &PaymentReply{})
	requireReason(t, err, ReasonUnauthorized)

	// A client can't see delegations or payments it isn't granted.
	_, err = h.prepare(h.outsider, "alice", "1")
	requireReason(t, err, ReasonDelegationNotFound)
	p, err := h.prepare(h.agent, "alice", "1")
	require.NoError(t, err)
	err = h.svc.Get(req(h.outsider), &PaymentArgs{PaymentID: p.ID}, &PaymentReply{})
	requireReason(t, err, ReasonPaymentNotFound)
	err = h.svc.Approve(req(h.outsider), &ApproveArgs{PaymentID: p.ID, Digest: p.Digest}, &PaymentReply{})
	requireReason(t, err, ReasonPaymentNotFound)

	err = h.svc.RevokeDelegation(req(h.agent), &DelegationArgs{Delegation: "demo"}, &RevokeReply{})
	requireReason(t, err, ReasonForbidden)
}

func TestAutoApproval(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.Delegations[0].AutoApproveMax = "1" })
	h.fundAllowance(100)

	small, err := h.prepare(h.agent, "alice", "1")
	require.NoError(t, err)
	require.False(t, small.ApprovalRequired)
	require.Equal(t, StatusApproved, small.Status)
	sent, err := h.submit(h.agent, small)
	require.NoError(t, err)
	require.Equal(t, StatusAccepted, sent.Status)

	large, err := h.prepare(h.agent, "alice", "1.5")
	require.NoError(t, err)
	require.True(t, large.ApprovalRequired)
}

func TestRateLimit(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.RateLimits.Prepare = RateLimit{PerMinute: 1, Burst: 2} })
	for i := 0; i < 2; i++ {
		_, err := h.prepare(h.agent, "alice", "1")
		require.NoError(t, err)
	}
	_, err := h.prepare(h.agent, "alice", "1")
	requireReason(t, err, ReasonRateLimited)

	h.advance(time.Minute)
	_, err = h.prepare(h.agent, "alice", "1")
	require.NoError(t, err)
}

func TestRevocation(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	h.fundAllowance(100)

	pending, err := h.prepare(h.agent, "alice", "1")
	require.NoError(err)
	approved, err := h.prepare(h.agent, "alice", "2")
	require.NoError(err)
	approved = h.approve(approved)

	reply := RevokeReply{}
	require.NoError(h.svc.RevokeDelegation(req(h.approver), &DelegationArgs{Delegation: "demo"}, &reply))
	require.ElementsMatch([]string{pending.ID, approved.ID}, reply.Cancelled)

	_, err = h.prepare(h.agent, "alice", "1")
	requireReason(t, err, ReasonDelegationRevoked)
	_, err = h.submit(h.approver, approved)
	requireReason(t, err, ReasonPaymentState)
	require.Zero(h.net.issued)

	// Revocation survives a restart.
	require.NoError(h.svc.Close())
	h.start()
	_, err = h.prepare(h.agent, "alice", "1")
	requireReason(t, err, ReasonDelegationRevoked)
}

func TestPaymentExpiry(t *testing.T) {
	h := newHarness(t, nil)
	h.fundAllowance(100)
	p, err := h.prepare(h.agent, "alice", "1")
	require.NoError(t, err)
	p = h.approve(p)

	h.advance(time.Duration(h.config.PreparedTTL) + time.Second)
	_, err = h.submit(h.approver, p)
	requireReason(t, err, ReasonPaymentExpired)
	require.Zero(t, h.net.issued)
}

func TestRestartKeepsPaymentsAndLimits(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	h.fundAllowance(200)

	p, err := h.prepare(h.agent, "alice", "50")
	require.NoError(err)
	sent, err := h.submit(h.approver, h.approve(p))
	require.NoError(err)
	p2, err := h.prepare(h.agent, "alice", "49")
	require.NoError(err)
	_, err = h.submit(h.approver, h.approve(p2))
	require.NoError(err)

	require.NoError(h.svc.Close())
	h.start()

	reply := PaymentReply{}
	require.NoError(h.svc.Get(req(h.agent), &PaymentArgs{PaymentID: p.ID}, &reply))
	require.Equal(sent.TxID, reply.Payment.TxID)

	// 99 DJT plus fees spent today: the daily limit still applies.
	_, err = h.prepare(h.agent, "alice", "1")
	requireReason(t, err, ReasonPolicyDenied)
}

func TestAuditTamperDetection(t *testing.T) {
	require := require.New(t)
	h := newHarness(t, nil)
	_, err := h.prepare(h.agent, "alice", "1")
	require.NoError(err)
	_, err = h.prepare(h.agent, "alice", "2")
	require.NoError(err)
	require.NoError(h.svc.Close())

	path := filepath.Join(h.dir, "audit.jsonl")
	entries, err := ReadAuditLog(path)
	require.NoError(err)
	require.Len(entries, 2)

	b, err := os.ReadFile(path)
	require.NoError(err)
	// Change one digit of the first entry's timestamp.
	b[len(`{"seq":1,"time":"`)+3] = '9'
	require.NoError(os.WriteFile(path, b, 0o600))

	_, err = ReadAuditLog(path)
	require.ErrorIs(err, ErrAuditChainBroken)
	_, err = NewService(context.Background(), h.config, h.net, log.New(io.Discard, "", 0), h.clock)
	require.ErrorIs(err, ErrAuditChainBroken)
}

func TestNetworkUnavailable(t *testing.T) {
	h := newHarness(t, nil)
	h.net.down = true
	_, err := h.prepare(h.agent, "alice", "1")
	requireReason(t, err, ReasonNetworkUnavailable)
}

func TestAmounts(t *testing.T) {
	for s, want := range map[string]uint64{
		"10":          10 * units.Djtx,
		"0.5":         500 * units.MilliDjtx,
		"0.000000001": 1,
		" 1.25 ":      1250 * units.MilliDjtx,
	} {
		got, err := ParseAmount(s, 9)
		require.NoError(t, err, s)
		require.Equal(t, want, got, s)
	}
	require.Equal(t, "89.999", FormatAmount(89_999_000_000, 9))
	require.Equal(t, "0.001", FormatAmount(1_000_000, 9))
	require.Equal(t, "0", FormatAmount(0, 9))
	require.Equal(t, "10", FormatAmount(10*units.Djtx, 9))
}
