// Package txservice is the trusted transaction service: the only component
// of the payments stack that holds signing keys. It resolves recipients
// through the native naming registry, enforces delegation policy, requires
// approval, and builds, signs and submits ordinary V-chain transactions.
//
// Clients, including an AI agent's runtime, can ask for payments but can't
// authorize them: authorization is the policy below and an approval bound to
// the payment's digest.
package txservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/rpc/v2/json2"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/snow/choices"
	"github.com/lasthyphen/dijetsnodesgo/utils"
	"github.com/lasthyphen/dijetsnodesgo/utils/crypto"
	"github.com/lasthyphen/dijetsnodesgo/vms/components/djtx"
	"github.com/lasthyphen/dijetsnodesgo/vms/platformvm/names"
	"github.com/lasthyphen/dijetsnodesgo/vms/secp256k1fx"
	"github.com/lasthyphen/dijetsnodesgo/wallet/chain/x"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary"
	"github.com/lasthyphen/dijetsnodesgo/wallet/subnet/primary/common"
)

const (
	maxNoteLength   = 280
	maxHistory      = 100
	decisionPolling = 250 * time.Millisecond
)

// delegation is a loaded DelegationConfig.
type delegation struct {
	config    DelegationConfig
	principal ids.ShortID
	agentKey  *crypto.PrivateKeySECP256K1R
	agent     ids.ShortID
	// owner is the allowance owner set: 1-of-2 {principal, agent}.
	owner          *secp256k1fx.OutputOwners
	perPaymentMax  uint64
	dailyMax       uint64
	autoApproveMax uint64
	asset          Asset

	// lock serializes spends from the allowance.
	lock sync.Mutex
}

// Service implements the payments.* API.
type Service struct {
	config      *Config
	network     Network
	audit       *AuditLog
	store       *store
	delegations map[string]*delegation
	now         func() time.Time
	log         *log.Logger

	readLimiter    *rateLimiter
	prepareLimiter *rateLimiter
	submitLimiter  *rateLimiter
}

// NewService loads keys, the payment store and the audit log.
func NewService(ctx context.Context, config *Config, network Network, logger *log.Logger, now func() time.Time) (*Service, error) {
	config.applyDefaults()
	if err := config.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return nil, err
	}
	audit, err := OpenAuditLog(filepath.Join(config.DataDir, "audit.jsonl"), now)
	if err != nil {
		return nil, err
	}
	store, err := openStore(filepath.Join(config.DataDir, "payments.json"))
	if err != nil {
		return nil, err
	}
	s := &Service{
		config:         config,
		network:        network,
		audit:          audit,
		store:          store,
		delegations:    make(map[string]*delegation),
		now:            now,
		log:            logger,
		readLimiter:    newRateLimiter(config.RateLimits.Read, now),
		prepareLimiter: newRateLimiter(config.RateLimits.Prepare, now),
		submitLimiter:  newRateLimiter(config.RateLimits.Submit, now),
	}
	for _, dc := range config.Delegations {
		d, err := s.loadDelegation(ctx, dc)
		if err != nil {
			return nil, fmt.Errorf("delegation %q: %w", dc.ID, err)
		}
		s.delegations[dc.ID] = d
	}
	return s, nil
}

func (s *Service) Close() error {
	return s.audit.Close()
}

func (s *Service) loadDelegation(ctx context.Context, dc DelegationConfig) (*delegation, error) {
	hrp := s.network.HRP()
	principal, err := parseVAddress(hrp, dc.PrincipalAddress)
	if err != nil {
		return nil, fmt.Errorf("principalAddress: %w", err)
	}
	key, err := LoadOrCreateKey(dc.AgentKeyFile)
	if err != nil {
		return nil, err
	}
	agent := key.PublicKey().Address()
	if agent == principal {
		return nil, errors.New("the agent key must differ from the principal's key")
	}
	if len(dc.Assets) != 1 {
		return nil, errors.New("exactly one asset is supported per delegation")
	}
	asset, err := s.network.Asset(ctx, dc.Assets[0])
	if err != nil {
		return nil, fmt.Errorf("asset %q: %w", dc.Assets[0], err)
	}
	limit := func(field, value string) (uint64, error) {
		if value == "0" {
			return 0, nil
		}
		units, err := ParseAmount(value, asset.Denomination)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", field, err)
		}
		return units, nil
	}
	d := &delegation{
		config:    dc,
		principal: principal,
		agentKey:  key,
		agent:     agent,
		owner: &secp256k1fx.OutputOwners{
			Threshold: 1,
			Addrs:     []ids.ShortID{principal, agent},
		},
		asset: asset,
	}
	utils.Sort(d.owner.Addrs)
	if d.perPaymentMax, err = limit("perPaymentMax", dc.PerPaymentMax); err != nil {
		return nil, err
	}
	if d.dailyMax, err = limit("dailyMax", dc.DailyMax); err != nil {
		return nil, err
	}
	if d.autoApproveMax, err = limit("autoApproveMax", dc.AutoApproveMax); err != nil {
		return nil, err
	}
	return d, nil
}

// LoadOrCreateKey reads a private key file, creating one with mode 0600 if
// it doesn't exist.
func LoadOrCreateKey(path string) (*crypto.PrivateKeySECP256K1R, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		factory := crypto.FactorySECP256K1R{}
		keyIntf, err := factory.NewPrivateKey()
		if err != nil {
			return nil, err
		}
		key := keyIntf.(*crypto.PrivateKeySECP256K1R)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		return key, os.WriteFile(path, []byte(key.String()+"\n"), 0o600)
	}
	if err != nil {
		return nil, err
	}
	// The key's text form must be quoted to be parsed.
	key := &crypto.PrivateKeySECP256K1R{}
	return key, key.UnmarshalJSON([]byte(strconv.Quote(strings.TrimSpace(string(b)))))
}

// AgentAddress returns the agent's V-chain address for a delegation, so that
// operators can fund the allowance.
func (s *Service) AgentAddress(delegationID string) (string, bool) {
	d, ok := s.delegations[delegationID]
	if !ok {
		return "", false
	}
	return formatVAddress(s.network.HRP(), d.agent), true
}

func (s *Service) allowance(d *delegation) Allowance {
	hrp := s.network.HRP()
	addrs := make([]string, len(d.owner.Addrs))
	for i, addr := range d.owner.Addrs {
		addrs[i] = formatVAddress(hrp, addr)
	}
	return Allowance{
		Owner:      Party{Name: d.config.Principal, Address: formatVAddress(hrp, d.principal)},
		Agent:      Party{Name: d.config.AgentName, Address: formatVAddress(hrp, d.agent)},
		Threshold:  int(d.owner.Threshold),
		Addresses:  addrs,
		Delegation: d.config.ID,
	}
}

func (d *delegation) policy() PolicySummary {
	p := PolicySummary{
		Assets:              d.config.Assets,
		PerPaymentMax:       newAmount(d.perPaymentMax, d.asset.Denomination),
		DailyMax:            newAmount(d.dailyMax, d.asset.Denomination),
		AutoApproveMax:      newAmount(d.autoApproveMax, d.asset.Denomination),
		NamedRecipientsOnly: d.config.NamedRecipientsOnly,
	}
	if d.config.NotAfter != nil {
		p.NotAfter = d.config.NotAfter.UTC().Format(time.RFC3339)
	}
	return p
}

// ---------------------------------------------------------------------------
// Request plumbing

type callerKey struct{}

func withCaller(ctx context.Context, client *ClientConfig) context.Context {
	return context.WithValue(ctx, callerKey{}, client)
}

func callerOf(r *http.Request) (*ClientConfig, error) {
	if r == nil {
		return nil, apiError(CodeUnauthorized, ReasonUnauthorized, "", "missing credentials")
	}
	client, ok := r.Context().Value(callerKey{}).(*ClientConfig)
	if !ok {
		return nil, apiError(CodeUnauthorized, ReasonUnauthorized, "", "missing or invalid bearer token")
	}
	return client, nil
}

func (s *Service) begin(r *http.Request, limiter *rateLimiter) (*ClientConfig, error) {
	client, err := callerOf(r)
	if err != nil {
		return nil, err
	}
	if !limiter.Allow(client.Name) {
		_ = s.audit.Append(AuditEntry{Event: "rate_limited", Client: client.Name})
		return nil, apiError(CodeRateLimited, ReasonRateLimited, "", "too many requests, retry later")
	}
	return client, nil
}

func (s *Service) delegationFor(client *ClientConfig, id string) (*delegation, error) {
	d, ok := s.delegations[id]
	if !ok || !client.mayUse(id) {
		// Don't reveal delegations the client may not use.
		return nil, apiError(CodeNotFound, ReasonDelegationNotFound, "", "delegation %q not found", id)
	}
	return d, nil
}

func (s *Service) paymentFor(client *ClientConfig, id string) (*Payment, *delegation, error) {
	p, ok := s.store.get(id)
	if !ok || !client.mayUse(p.Delegation) {
		return nil, nil, apiError(CodeNotFound, ReasonPaymentNotFound, "", "payment %q not found", id)
	}
	return p, s.delegations[p.Delegation], nil
}

func (s *Service) internal(op string, err error) error {
	s.log.Printf("%s failed: %v", op, err)
	return apiError(CodeInternal, ReasonInternal, "", "internal error")
}

func networkError(err error) error {
	return apiError(CodeUnavailable, ReasonNetworkUnavailable, "", "the network could not be reached: %s", err)
}

func newPaymentID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "pay_" + hex.EncodeToString(b[:])
}

// ---------------------------------------------------------------------------
// Recipient resolution

// resolveRecipient turns a name or address into a recipient. For a name it
// enforces that the registration is ACTIVE and not about to expire, and
// pins the record version.
func (s *Service) resolveRecipient(ctx context.Context, input string) (Party, ids.ShortID, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Party{}, ids.ShortID{}, apiError(CodeInvalidParams, ReasonInvalidRecipient, "empty", "recipient is required")
	}
	hrp := s.network.HRP()
	if looksLikeAddress(input) {
		addr, err := parseVAddress(hrp, input)
		if err != nil {
			return Party{}, ids.ShortID{}, apiError(CodeInvalidParams, ReasonInvalidRecipient, "address", "recipient address is invalid: %s", err)
		}
		return Party{Address: formatVAddress(hrp, addr)}, addr, nil
	}

	name, err := names.Canonicalise(input)
	if err != nil {
		detail := ""
		var nameErr *names.Error
		if errors.As(err, &nameErr) {
			detail = string(nameErr.Rule)
		}
		return Party{}, ids.ShortID{}, apiError(CodeInvalidParams, ReasonInvalidName, detail, "%q is not a valid name (%s)", input, detail)
	}
	resolution, err := s.network.Resolve(ctx, name)
	if err != nil {
		return Party{}, ids.ShortID{}, networkError(err)
	}
	record := resolution.Record
	switch {
	case record == nil:
		return Party{}, ids.ShortID{}, apiError(CodeRecipient, ReasonNameNotFound, "", "no one is registered as %q", name)
	case !resolution.Resolvable:
		return Party{}, ids.ShortID{}, apiError(CodeRecipient, ReasonNameNotResolvable, strings.ToLower(resolution.Status),
			"%q can't receive payments: its registration is %s", name, strings.ToLower(resolution.Status))
	}
	margin := time.Duration(s.config.ExpiryMargin)
	if expiry := time.Unix(int64(record.Expiry), 0); expiry.Before(s.now().Add(margin)) {
		return Party{}, ids.ShortID{}, apiError(CodeRecipient, ReasonNameExpiring, "",
			"%q's registration expires at %s, too soon to pay safely", name, expiry.UTC().Format(time.RFC3339))
	}
	return Party{
		Name:    name,
		Address: formatVAddress(hrp, record.TargetID),
		Record: &RecordPin{
			RecordID:   record.RecordID,
			Generation: record.Generation,
			Version:    record.Version,
			Expiry:     record.Expiry,
			BlockID:    resolution.At.BlockID,
			Height:     resolution.At.Height,
		},
	}, record.TargetID, nil
}

// Resolve reports whether a payment could be prepared to a name now.
func (s *Service) Resolve(r *http.Request, args *ResolveArgs, reply *ResolveReply) error {
	if _, err := s.begin(r, s.readLimiter); err != nil {
		return err
	}
	*reply = ResolveReply{Envelope: envelope, Input: args.Name}
	party, _, err := s.resolveRecipient(r.Context(), args.Name)
	if err != nil {
		reason := ErrorReason(err)
		if reason == ReasonNetworkUnavailable || reason == ReasonInvalidName || reason == ReasonInvalidRecipient {
			return err
		}
		reply.Name, _ = names.Canonicalise(args.Name)
		reply.Status = string(names.StatusNotFound)
		reply.Reason = reason
		if resolution, rerr := s.network.Resolve(r.Context(), reply.Name); rerr == nil {
			reply.Status = resolution.Status
		}
		return nil
	}
	reply.Name = party.Name
	reply.Status = string(names.StatusActive)
	reply.Payable = true
	reply.Address = party.Address
	reply.Record = party.Record
	return nil
}

// ---------------------------------------------------------------------------
// Policy

func (s *Service) checkDelegationLive(d *delegation) error {
	if s.store.isRevoked(d.config.ID) {
		return apiError(CodePolicy, ReasonDelegationRevoked, "", "delegation %q has been revoked", d.config.ID)
	}
	if d.config.NotAfter != nil && s.now().After(*d.config.NotAfter) {
		return apiError(CodePolicy, ReasonDelegationRevoked, "expired", "delegation %q has expired", d.config.ID)
	}
	return nil
}

func (s *Service) checkLimits(d *delegation, amount, fee uint64) error {
	if d.perPaymentMax > 0 && amount > d.perPaymentMax {
		return apiError(CodePolicy, ReasonPolicyDenied, "per_payment_max",
			"%s %s exceeds the per-payment limit of %s %s",
			FormatAmount(amount, d.asset.Denomination), d.asset.Symbol,
			FormatAmount(d.perPaymentMax, d.asset.Denomination), d.asset.Symbol)
	}
	spent := s.store.spentSince(d.config.ID, s.now().Add(-24*time.Hour))
	if d.dailyMax > 0 && spent+amount+fee > d.dailyMax {
		return apiError(CodePolicy, ReasonPolicyDenied, "daily_max",
			"this payment would exceed the 24-hour limit of %s %s (%s already spent)",
			FormatAmount(d.dailyMax, d.asset.Denomination), d.asset.Symbol,
			FormatAmount(spent, d.asset.Denomination))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Prepare, approve, submit

// paymentDigest commits to every field an approver sees and that determines
// the transaction.
func paymentDigest(p *Payment) string {
	b, err := json.Marshal(struct {
		ID         string    `json:"id"`
		Delegation string    `json:"delegation"`
		From       []string  `json:"from"`
		To         Party     `json:"to"`
		Asset      ids.ID    `json:"asset"`
		Amount     uint64    `json:"amount"`
		Fee        uint64    `json:"fee"`
		Memo       string    `json:"memo"`
		ExpiresAt  time.Time `json:"expiresAt"`
	}{
		ID:         p.ID,
		Delegation: p.Delegation,
		From:       p.From.Addresses,
		To:         p.To,
		Asset:      p.Asset.ID,
		Amount:     uint64(p.Amount.Units),
		Fee:        uint64(p.Fee.Units),
		Memo:       p.Memo,
		ExpiresAt:  p.ExpiresAt.UTC(),
	})
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Prepare resolves the recipient, checks policy and records a payment that
// still needs approval and submission. Nothing is signed or sent.
func (s *Service) Prepare(r *http.Request, args *PrepareArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.prepareLimiter)
	if err != nil {
		return err
	}
	d, err := s.delegationFor(client, args.Delegation)
	if err != nil {
		return err
	}
	if err := s.checkDelegationLive(d); err != nil {
		return err
	}
	if len(args.Note) > maxNoteLength {
		return apiError(CodeInvalidParams, ReasonInvalidParams, "note", "note is longer than %d bytes", maxNoteLength)
	}
	if !strings.EqualFold(strings.TrimSpace(args.Asset), d.asset.Symbol) && args.Asset != d.asset.ID.String() {
		return apiError(CodeInvalidParams, ReasonUnsupportedAsset, "", "asset %q is not allowed by this delegation; allowed: %s", args.Asset, d.asset.Symbol)
	}
	amount, err := ParseAmount(args.Amount, d.asset.Denomination)
	if err != nil {
		return apiError(CodeInvalidParams, ReasonInvalidAmount, "", "%s", err)
	}

	if args.IdempotencyKey != "" {
		if existing, ok := s.store.byIdempotencyKey(client.Name, d.config.ID, args.IdempotencyKey); ok {
			if existing.RecipientInput != args.Recipient || uint64(existing.Amount.Units) != amount {
				return apiError(CodeConflict, ReasonIdempotencyConflict, "",
					"idempotency key %q was already used for a different payment", args.IdempotencyKey)
			}
			*reply = PaymentReply{Envelope: envelope, Payment: existing}
			return nil
		}
	}

	ctx := r.Context()
	to, target, err := s.resolveRecipient(ctx, args.Recipient)
	if err != nil {
		return err
	}
	if to.Name == "" && d.config.NamedRecipientsOnly {
		return apiError(CodePolicy, ReasonPolicyDenied, "named_recipients_only", "this delegation only pays registered names")
	}
	if target == d.principal || target == d.agent {
		return apiError(CodeInvalidParams, ReasonSelfPayment, "", "the recipient is the allowance itself")
	}
	if to.Name == "" {
		// Show a name for a raw address only if it is reliably known.
		if reverse, err := s.network.Reverse(ctx, to.Address); err == nil {
			to.Name = reverse.Name
		}
	}

	fee := s.network.Context().BaseTxFee()
	if err := s.checkLimits(d, amount, fee); err != nil {
		return err
	}

	now := s.now().UTC()
	p := &Payment{
		ID:             newPaymentID(),
		Delegation:     d.config.ID,
		Status:         StatusPrepared,
		From:           s.allowance(d),
		To:             to,
		RecipientInput: args.Recipient,
		Asset:          d.asset,
		Amount:         newAmount(amount, d.asset.Denomination),
		Fee:            newAmount(fee, d.asset.Denomination),
		Note:           args.Note,
		PreparedBy:     client.Name,
		IdempotencyKey: args.IdempotencyKey,
		CreatedAt:      now,
		ExpiresAt:      now.Add(time.Duration(s.config.PreparedTTL)),
	}
	if to.Record != nil {
		p.Memo = names.PaymentMemo{
			Name:       to.Name,
			Generation: uint64(to.Record.Generation),
			Version:    uint64(to.Record.Version),
		}.String()
	}
	p.Digest = paymentDigest(p)
	p.ApprovalRequired = amount > d.autoApproveMax
	if !p.ApprovalRequired {
		p.Status = StatusApproved
		p.ApprovedBy = "policy:autoApproveMax"
		p.ApprovedAt = &now
	}

	if err := s.audit.Append(AuditEntry{
		Event:      "prepared",
		Client:     client.Name,
		Delegation: d.config.ID,
		PaymentID:  p.ID,
		Fields: map[string]string{
			"recipientInput": args.Recipient,
			"to":             to.Address,
			"toName":         to.Name,
			"memo":           p.Memo,
			"amount":         p.Amount.Display,
			"asset":          d.asset.Symbol,
			"digest":         p.Digest,
			"note":           args.Note,
		},
	}); err != nil {
		return s.internal("audit", err)
	}
	if err := s.store.put(p); err != nil {
		return s.internal("store", err)
	}
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

// Get returns a payment.
func (s *Service) Get(r *http.Request, args *PaymentArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	p, _, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}
	p, err = s.expireIfStale(p)
	if err != nil {
		return err
	}
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

func (s *Service) expireIfStale(p *Payment) (*Payment, error) {
	if (p.Status == StatusPrepared || p.Status == StatusApproved) && s.now().After(p.ExpiresAt) {
		p.Status = StatusExpired
		if err := s.store.put(p); err != nil {
			return nil, s.internal("store", err)
		}
		_ = s.audit.Append(AuditEntry{Event: "expired", Delegation: p.Delegation, PaymentID: p.ID})
	}
	return p, nil
}

// Approve records the user's approval of exactly the payment they were
// shown, identified by its digest.
func (s *Service) Approve(r *http.Request, args *ApproveArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.submitLimiter)
	if err != nil {
		return err
	}
	if client.Role != RoleApprover {
		return apiError(CodeUnauthorized, ReasonForbidden, "", "this client may not approve payments")
	}
	p, d, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}
	d.lock.Lock()
	defer d.lock.Unlock()
	p, _ = s.store.get(p.ID)
	if p, err = s.expireIfStale(p); err != nil {
		return err
	}

	switch {
	case p.Status == StatusExpired:
		return apiError(CodeStale, ReasonPaymentExpired, "", "the payment expired before it was approved; prepare it again")
	case args.Digest != p.Digest:
		_ = s.audit.Append(AuditEntry{Event: "approval_mismatch", Client: client.Name, Delegation: p.Delegation, PaymentID: p.ID})
		return apiError(CodePolicy, ReasonApprovalMismatch, "", "the approval does not match the payment")
	case p.Status == StatusApproved:
		*reply = PaymentReply{Envelope: envelope, Payment: p}
		return nil
	case p.Status != StatusPrepared:
		return apiError(CodeConflict, ReasonPaymentState, strings.ToLower(p.Status), "the payment is %s", strings.ToLower(p.Status))
	}
	if err := s.checkDelegationLive(d); err != nil {
		return err
	}

	now := s.now().UTC()
	p.Status = StatusApproved
	p.ApprovedBy = client.Name
	p.ApprovedAt = &now
	if err := s.audit.Append(AuditEntry{
		Event:      "approved",
		Client:     client.Name,
		Delegation: p.Delegation,
		PaymentID:  p.ID,
		Fields:     map[string]string{"digest": p.Digest},
	}); err != nil {
		return s.internal("audit", err)
	}
	if err := s.store.put(p); err != nil {
		return s.internal("store", err)
	}
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

// Cancel abandons a payment that has not been submitted.
func (s *Service) Cancel(r *http.Request, args *PaymentArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	p, d, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}
	d.lock.Lock()
	defer d.lock.Unlock()
	p, _ = s.store.get(p.ID)
	if p.Status != StatusPrepared && p.Status != StatusApproved {
		return apiError(CodeConflict, ReasonPaymentState, strings.ToLower(p.Status), "the payment is %s", strings.ToLower(p.Status))
	}
	p.Status = StatusCancelled
	if err := s.audit.Append(AuditEntry{Event: "cancelled", Client: client.Name, Delegation: p.Delegation, PaymentID: p.ID}); err != nil {
		return s.internal("audit", err)
	}
	if err := s.store.put(p); err != nil {
		return s.internal("store", err)
	}
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

// fail records a submission that was refused before anything was sent.
func (s *Service) fail(client *ClientConfig, p *Payment, cause error) error {
	p.Status = StatusFailed
	p.FailureReason = ErrorReason(cause)
	_ = s.audit.Append(AuditEntry{
		Event:      "submission_refused",
		Client:     client.Name,
		Delegation: p.Delegation,
		PaymentID:  p.ID,
		Fields:     map[string]string{"reason": p.FailureReason},
	})
	if err := s.store.put(p); err != nil {
		return s.internal("store", err)
	}
	return cause
}

// Submit re-checks the recipient and policy, builds and signs the V-chain
// transaction, issues it and waits for the network's decision. Submitting
// an already-submitted payment returns it unchanged: a payment is sent at
// most once.
func (s *Service) Submit(r *http.Request, args *PaymentArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.submitLimiter)
	if err != nil {
		return err
	}
	p, d, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}

	d.lock.Lock()
	defer d.lock.Unlock()
	p, _ = s.store.get(p.ID)
	if p, err = s.expireIfStale(p); err != nil {
		return err
	}

	switch p.Status {
	case StatusSubmitted, StatusAccepted, StatusRejected:
		*reply = PaymentReply{Envelope: envelope, Payment: p}
		return nil
	case StatusPrepared:
		return apiError(CodePolicy, ReasonApprovalRequired, "", "the payment needs the user's approval before it can be sent")
	case StatusExpired:
		return apiError(CodeStale, ReasonPaymentExpired, "", "the payment expired before it was submitted; prepare it again")
	case StatusApproved:
	default:
		return apiError(CodeConflict, ReasonPaymentState, strings.ToLower(p.Status), "the payment is %s", strings.ToLower(p.Status))
	}
	autoApproved := strings.HasPrefix(p.ApprovedBy, "policy:")
	if client.Role != RoleApprover && !autoApproved {
		return apiError(CodeUnauthorized, ReasonForbidden, "", "this client may only submit payments approved by policy")
	}
	if err := s.checkDelegationLive(d); err != nil {
		return s.fail(client, p, err)
	}

	ctx := r.Context()
	// The name may have changed since preparation. Refuse rather than pay a
	// recipient the user didn't approve.
	if p.To.Record != nil {
		current, _, err := s.resolveRecipient(ctx, p.To.Name)
		if err != nil {
			if ErrorReason(err) == ReasonNetworkUnavailable {
				return err
			}
			return s.fail(client, p, apiError(CodeRecipient, ReasonRecipientChanged, ErrorReason(err),
				"%q can no longer be paid: %s", p.To.Name, err.Error()))
		}
		if current.Address != p.To.Address ||
			current.Record.RecordID != p.To.Record.RecordID ||
			current.Record.Generation != p.To.Record.Generation ||
			current.Record.Version != p.To.Record.Version {
			return s.fail(client, p, apiError(CodeRecipient, ReasonRecipientChanged, "",
				"%q's registration changed after the payment was prepared; prepare and approve it again", p.To.Name))
		}
	}
	amount := uint64(p.Amount.Units)
	fee := uint64(p.Fee.Units)
	if err := s.checkLimits(d, amount, fee); err != nil {
		return s.fail(client, p, err)
	}

	tx, inputs, err := s.buildPayment(ctx, d, p)
	if err != nil {
		if ErrorReason(err) == ReasonInsufficientFunds {
			return s.fail(client, p, err)
		}
		return err
	}

	txID, err := s.network.IssueTx(ctx, tx.Bytes())
	if err != nil {
		var jsonErr *json2.Error
		if errors.As(err, &jsonErr) {
			return s.fail(client, p, apiError(CodeRejected, ReasonTxRejected, "", "the network rejected the transaction: %s", jsonErr.Message))
		}
		return networkError(err)
	}

	now := s.now().UTC()
	p.Status = StatusSubmitted
	p.TxID = txID
	p.SubmittedAt = &now
	p.Inputs = inputs
	if err := s.audit.Append(AuditEntry{
		Event:      "submitted",
		Client:     client.Name,
		Delegation: p.Delegation,
		PaymentID:  p.ID,
		Fields: map[string]string{
			"txID":   txID.String(),
			"to":     p.To.Address,
			"amount": p.Amount.Display,
		},
	}); err != nil {
		s.log.Printf("failed to audit submission of %s: %v", txID, err)
	}
	if err := s.store.put(p); err != nil {
		return s.internal("store", err)
	}

	// Wait for the decision while still holding the allowance lock, so the
	// next payment spends the change of this one.
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(s.config.DecisionTimeout))
	defer cancel()
	p = s.awaitDecision(waitCtx, p)
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

// buildPayment builds and signs the payment's transaction from the
// allowance's unreserved UTXOs.
func (s *Service) buildPayment(ctx context.Context, d *delegation, p *Payment) (*avmTx, []ids.ID, error) {
	utxos, err := s.allowanceUTXOs(ctx, d)
	if err != nil {
		return nil, nil, networkError(err)
	}
	var available uint64
	for _, utxo := range utxos {
		available += utxo.Out.(*secp256k1fx.TransferOutput).Amt
	}
	amount := uint64(p.Amount.Units)
	fee := uint64(p.Fee.Units)
	if available < amount+fee {
		return nil, nil, apiError(CodeInsufficientFund, ReasonInsufficientFunds, "",
			"the allowance holds %s %s, but the payment needs %s %s including the fee",
			FormatAmount(available, d.asset.Denomination), d.asset.Symbol,
			FormatAmount(amount+fee, d.asset.Denomination), d.asset.Symbol)
	}

	xCtx := s.network.Context()
	chainID := xCtx.BlockchainID()
	chainUTXOs := primary.NewChainUTXOs(chainID, primary.NewUTXOs())
	for _, utxo := range utxos {
		if err := chainUTXOs.AddUTXO(ctx, chainID, utxo); err != nil {
			return nil, nil, s.internal("build", err)
		}
	}
	backend := x.NewBackend(xCtx, chainID, chainUTXOs)
	builder := x.NewBuilder(setOf(d.agent), backend)
	signer := x.NewSigner(secp256k1fx.NewKeychain(d.agentKey), backend)

	target, err := parseVAddress(s.network.HRP(), p.To.Address)
	if err != nil {
		return nil, nil, s.internal("build", err)
	}
	out := &djtx.TransferableOutput{
		Asset: djtx.Asset{ID: p.Asset.ID},
		Out: &secp256k1fx.TransferOutput{
			Amt: amount,
			OutputOwners: secp256k1fx.OutputOwners{
				Threshold: 1,
				Addrs:     []ids.ShortID{target},
			},
		},
	}
	utx, err := builder.NewBaseTx(
		[]*djtx.TransferableOutput{out},
		common.WithChangeOwner(d.owner),
		common.WithMemo([]byte(p.Memo)),
	)
	if err != nil {
		return nil, nil, s.internal("build", err)
	}
	tx, err := signer.SignUnsigned(ctx, utx)
	if err != nil {
		return nil, nil, s.internal("sign", err)
	}
	if err := checkPaymentTx(tx, d, p, target, xCtx.DJTXAssetID()); err != nil {
		return nil, nil, s.internal("self-check", err)
	}
	inputs := make([]ids.ID, len(utx.Ins))
	for i, in := range utx.Ins {
		inputs[i] = in.InputID()
	}
	return tx, inputs, nil
}

// allowanceUTXOs returns the delegation's spendable UTXOs: outputs of its
// asset owned exactly by the allowance owner set, not reserved by a
// submitted payment.
func (s *Service) allowanceUTXOs(ctx context.Context, d *delegation) ([]*djtx.UTXO, error) {
	all, err := s.network.UTXOs(ctx, d.agent)
	if err != nil {
		return nil, err
	}
	reserved := s.store.reserved(d.config.ID)
	var utxos []*djtx.UTXO
	for _, utxo := range all {
		out, ok := utxo.Out.(*secp256k1fx.TransferOutput)
		if !ok || utxo.AssetID() != d.asset.ID || !out.OutputOwners.Equals(d.owner) || reserved.Contains(utxo.InputID()) {
			continue
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

func (s *Service) awaitDecision(ctx context.Context, p *Payment) *Payment {
	ticker := time.NewTicker(decisionPolling)
	defer ticker.Stop()
	for {
		updated, err := s.refreshStatus(ctx, p)
		if err == nil {
			p = updated
		}
		if p.Final() {
			return p
		}
		select {
		case <-ctx.Done():
			return p
		case <-ticker.C:
		}
	}
}

// refreshStatus asks the network for a submitted payment's status.
func (s *Service) refreshStatus(ctx context.Context, p *Payment) (*Payment, error) {
	if p.Status != StatusSubmitted {
		return p, nil
	}
	status, err := s.network.TxStatus(ctx, p.TxID)
	if err != nil {
		return p, err
	}
	switch status {
	case choices.Accepted:
		p.Status = StatusAccepted
	case choices.Rejected:
		p.Status = StatusRejected
	default:
		return p, nil
	}
	now := s.now().UTC()
	p.DecidedAt = &now
	_ = s.audit.Append(AuditEntry{
		Event:      "decided",
		Delegation: p.Delegation,
		PaymentID:  p.ID,
		Fields:     map[string]string{"txID": p.TxID.String(), "status": p.Status},
	})
	return p, s.store.put(p)
}

// Status refreshes and returns a payment.
func (s *Service) Status(r *http.Request, args *PaymentArgs, reply *PaymentReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	p, _, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}
	if p, err = s.expireIfStale(p); err != nil {
		return err
	}
	if p, err = s.refreshStatus(r.Context(), p); err != nil {
		return networkError(err)
	}
	*reply = PaymentReply{Envelope: envelope, Payment: p}
	return nil
}

// Receipt returns the verified result of an accepted payment, read back
// from the network.
func (s *Service) Receipt(r *http.Request, args *PaymentArgs, reply *Receipt) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	p, _, err := s.paymentFor(client, args.PaymentID)
	if err != nil {
		return err
	}
	if p, err = s.refreshStatus(r.Context(), p); err != nil {
		return networkError(err)
	}
	if p.Status != StatusAccepted {
		return apiError(CodeConflict, ReasonNotFinal, strings.ToLower(p.Status), "the payment is %s, not accepted", strings.ToLower(p.Status))
	}
	view, err := s.describe(r.Context(), p.TxID)
	if err != nil {
		return err
	}
	*reply = Receipt{
		Envelope:    envelope,
		Payment:     p,
		Transaction: view,
		ExplorerURL: s.explorerURL(p.TxID),
	}
	return nil
}

func (s *Service) explorerURL(txID ids.ID) string {
	if s.config.ExplorerTxURL == "" {
		return ""
	}
	return strings.ReplaceAll(s.config.ExplorerTxURL, "{txID}", txID.String())
}

// Balance returns the allowance balance and the remaining daily limit.
func (s *Service) Balance(r *http.Request, args *DelegationArgs, reply *BalanceReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	d, err := s.delegationFor(client, args.Delegation)
	if err != nil {
		return err
	}
	all, err := s.network.UTXOs(r.Context(), d.agent)
	if err != nil {
		return networkError(err)
	}
	reserved := s.store.reserved(d.config.ID)
	var available, held uint64
	for _, utxo := range all {
		out, ok := utxo.Out.(*secp256k1fx.TransferOutput)
		if !ok || utxo.AssetID() != d.asset.ID || !out.OutputOwners.Equals(d.owner) {
			continue
		}
		if reserved.Contains(utxo.InputID()) {
			held += out.Amt
		} else {
			available += out.Amt
		}
	}
	denomination := d.asset.Denomination
	*reply = BalanceReply{
		Envelope:   envelope,
		Delegation: d.config.ID,
		From:       s.allowance(d),
		Asset:      d.asset,
		Available:  newAmount(available, denomination),
		Reserved:   newAmount(held, denomination),
		SpentToday: newAmount(s.store.spentSince(d.config.ID, s.now().Add(-24*time.Hour)), denomination),
		Policy:     d.policy(),
		Revoked:    s.store.isRevoked(d.config.ID),
	}
	return nil
}

// History returns a delegation's payments, newest first.
func (s *Service) History(r *http.Request, args *HistoryArgs, reply *HistoryReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	d, err := s.delegationFor(client, args.Delegation)
	if err != nil {
		return err
	}
	limit := args.Limit
	if limit <= 0 || limit > maxHistory {
		limit = 20
	}
	payments := s.store.list(d.config.ID)
	if len(payments) > limit {
		payments = payments[:limit]
	}
	*reply = HistoryReply{Envelope: envelope, Delegation: d.config.ID, Payments: payments}
	return nil
}

// DescribeTx returns the human-readable view of any V-chain transaction.
func (s *Service) DescribeTx(r *http.Request, args *DescribeTxArgs, reply *TxView) error {
	if _, err := s.begin(r, s.readLimiter); err != nil {
		return err
	}
	view, err := s.describe(r.Context(), args.TxID)
	if err != nil {
		return err
	}
	*reply = *view
	return nil
}

// RevokeDelegation stops all further payments of a delegation and cancels
// those not yet submitted. Funds in the allowance stay where they are: the
// user sweeps them back with their own key.
func (s *Service) RevokeDelegation(r *http.Request, args *DelegationArgs, reply *RevokeReply) error {
	client, err := s.begin(r, s.submitLimiter)
	if err != nil {
		return err
	}
	if client.Role != RoleApprover {
		return apiError(CodeUnauthorized, ReasonForbidden, "", "this client may not revoke delegations")
	}
	d, err := s.delegationFor(client, args.Delegation)
	if err != nil {
		return err
	}
	d.lock.Lock()
	defer d.lock.Unlock()

	if err := s.audit.Append(AuditEntry{Event: "delegation_revoked", Client: client.Name, Delegation: d.config.ID}); err != nil {
		return s.internal("audit", err)
	}
	if err := s.store.revoke(d.config.ID, s.now().UTC()); err != nil {
		return s.internal("store", err)
	}
	*reply = RevokeReply{Envelope: envelope, Delegation: d.config.ID, Revoked: true, Cancelled: []string{}}
	for _, p := range s.store.list(d.config.ID) {
		if p.Status != StatusPrepared && p.Status != StatusApproved {
			continue
		}
		p.Status = StatusCancelled
		p.FailureReason = ReasonDelegationRevoked
		if err := s.store.put(p); err != nil {
			return s.internal("store", err)
		}
		reply.Cancelled = append(reply.Cancelled, p.ID)
	}
	return nil
}

// Info describes the calling client and the delegations it may use.
func (s *Service) Info(r *http.Request, _ *struct{}, reply *InfoReply) error {
	client, err := s.begin(r, s.readLimiter)
	if err != nil {
		return err
	}
	*reply = InfoReply{
		Envelope:  envelope,
		Client:    client.Name,
		Role:      client.Role,
		NetworkID: jsonUint32(s.network.Context().NetworkID()),
	}
	for _, id := range client.Delegations {
		d := s.delegations[id]
		reply.Asset = d.asset
		reply.Delegations = append(reply.Delegations, DelegationSummary{
			ID:      id,
			From:    s.allowance(d),
			Policy:  d.policy(),
			Revoked: s.store.isRevoked(id),
		})
	}
	return nil
}
