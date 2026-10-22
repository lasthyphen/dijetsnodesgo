package txservice

import (
	"time"

	"github.com/lasthyphen/dijetsnodesgo/ids"
	"github.com/lasthyphen/dijetsnodesgo/utils/json"
)

const (
	// ServiceName is the JSON-RPC service name: methods are
	// "payments.<method>".
	ServiceName = "payments"
	// Endpoint is the path the service is served at.
	Endpoint = "/ext/payments"

	APIVersion    = "v1"
	SchemaVersion = 1
)

// Envelope versions every reply.
type Envelope struct {
	APIVersion    string `json:"apiVersion"`
	SchemaVersion int    `json:"schemaVersion"`
}

var envelope = Envelope{APIVersion: APIVersion, SchemaVersion: SchemaVersion}

// Payment statuses.
const (
	StatusPrepared  = "PREPARED"
	StatusApproved  = "APPROVED"
	StatusSubmitted = "SUBMITTED"
	StatusAccepted  = "ACCEPTED"
	StatusRejected  = "REJECTED"
	StatusFailed    = "FAILED"
	StatusCancelled = "CANCELLED"
	StatusExpired   = "EXPIRED"
)

// Asset identifies the asset of a payment.
type Asset struct {
	ID           ids.ID `json:"id"`
	Symbol       string `json:"symbol"`
	Denomination int    `json:"denomination"`
}

// Amount is an amount of an asset, both in base units and as a decimal.
type Amount struct {
	Units   json.Uint64 `json:"units"`
	Display string      `json:"display"`
}

// RecordPin identifies the exact name record version a payment was
// prepared against. Submission is refused if the record has changed.
type RecordPin struct {
	RecordID   ids.ID      `json:"recordID"`
	Generation json.Uint64 `json:"generation"`
	Version    json.Uint64 `json:"version"`
	Expiry     json.Uint64 `json:"expiry"`
	// The accepted M-chain block the name was resolved at.
	BlockID ids.ID      `json:"blockID"`
	Height  json.Uint64 `json:"height"`
}

// Party is a sender or recipient.
type Party struct {
	// Name is the canonical registered name, when one is reliably known.
	Name string `json:"name,omitempty"`
	// Address is the V-chain address funds are sent to or spent from.
	Address string `json:"address"`
	// Record pins the name record a recipient was resolved from.
	Record *RecordPin `json:"record,omitempty"`
}

// Allowance describes the delegated funds a payment is spent from: outputs
// owned 1-of-2 by the user and the agent, so that the user can always sweep
// them back.
type Allowance struct {
	Owner      Party    `json:"owner"`
	Agent      Party    `json:"agent"`
	Threshold  int      `json:"threshold"`
	Addresses  []string `json:"addresses"`
	Delegation string   `json:"delegation"`
}

// Payment is a payment's full lifecycle record.
type Payment struct {
	ID             string    `json:"id"`
	Delegation     string    `json:"delegation"`
	Status         string    `json:"status"`
	From           Allowance `json:"from"`
	To             Party     `json:"to"`
	RecipientInput string    `json:"recipientInput"`
	Asset          Asset     `json:"asset"`
	Amount         Amount    `json:"amount"`
	// Fee is the fee the transaction will burn. The actual fee of the
	// accepted transaction is in the receipt.
	Fee  Amount `json:"fee"`
	Memo string `json:"memo"`
	// Note is free text from the requesting client, kept for the audit log.
	Note string `json:"note,omitempty"`

	// Digest commits to every field the approver sees. Approval must quote
	// it.
	Digest           string     `json:"digest"`
	ApprovalRequired bool       `json:"approvalRequired"`
	ApprovedBy       string     `json:"approvedBy,omitempty"`
	ApprovedAt       *time.Time `json:"approvedAt,omitempty"`

	TxID          ids.ID     `json:"txID,omitempty"`
	SubmittedAt   *time.Time `json:"submittedAt,omitempty"`
	DecidedAt     *time.Time `json:"decidedAt,omitempty"`
	FailureReason string     `json:"failureReason,omitempty"`

	PreparedBy     string    `json:"preparedBy"`
	IdempotencyKey string    `json:"idempotencyKey,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`

	// inputs consumed by the submitted tx, reserved until it is decided.
	Inputs []ids.ID `json:"inputs,omitempty"`
}

// Final reports whether the payment can no longer change.
func (p *Payment) Final() bool {
	switch p.Status {
	case StatusAccepted, StatusRejected, StatusFailed, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

type PaymentReply struct {
	Envelope
	Payment *Payment `json:"payment"`
}

type ResolveArgs struct {
	Name string `json:"name"`
}

type ResolveReply struct {
	Envelope
	Input  string `json:"input"`
	Name   string `json:"name"`
	Status string `json:"status"`
	// Payable reports whether a payment may be prepared to the name now.
	Payable bool `json:"payable"`
	// Reason explains why the name is not payable.
	Reason  string     `json:"reason,omitempty"`
	Address string     `json:"address,omitempty"`
	Record  *RecordPin `json:"record,omitempty"`
}

type PrepareArgs struct {
	Delegation string `json:"delegation"`
	// Recipient is a registered name or a V-chain address.
	Recipient string `json:"recipient"`
	Asset     string `json:"asset"`
	// Amount is a decimal string in whole units of the asset, e.g. "10" or
	// "0.5".
	Amount string `json:"amount"`
	// IdempotencyKey makes retries safe: repeating a prepare with the same
	// key returns the original payment.
	IdempotencyKey string `json:"idempotencyKey"`
	Note           string `json:"note"`
}

type PaymentArgs struct {
	PaymentID string `json:"paymentID"`
}

type ApproveArgs struct {
	PaymentID string `json:"paymentID"`
	// Digest must equal the digest of the payment the approver was shown.
	Digest string `json:"digest"`
}

type DelegationArgs struct {
	Delegation string `json:"delegation"`
}

type BalanceReply struct {
	Envelope
	Delegation string    `json:"delegation"`
	From       Allowance `json:"from"`
	Asset      Asset     `json:"asset"`
	// Available is the accepted allowance balance not reserved by
	// submitted payments.
	Available Amount `json:"available"`
	Reserved  Amount `json:"reserved"`
	// SpentToday is spending in the trailing 24 hours counted against the
	// daily limit.
	SpentToday Amount        `json:"spentToday"`
	Policy     PolicySummary `json:"policy"`
	Revoked    bool          `json:"revoked"`
}

type HistoryArgs struct {
	Delegation string `json:"delegation"`
	Limit      int    `json:"limit"`
}

type HistoryReply struct {
	Envelope
	Delegation string     `json:"delegation"`
	Payments   []*Payment `json:"payments"`
}

type DescribeTxArgs struct {
	TxID ids.ID `json:"txID"`
}

// Receipt is the verified result of an accepted payment.
type Receipt struct {
	Envelope
	Payment     *Payment `json:"payment"`
	Transaction *TxView  `json:"transaction"`
	// ExplorerURL links to the transaction's record.
	ExplorerURL string `json:"explorerURL,omitempty"`
}

type InfoReply struct {
	Envelope
	Client      string              `json:"client"`
	Role        string              `json:"role"`
	NetworkID   json.Uint32         `json:"networkID"`
	Asset       Asset               `json:"asset"`
	Delegations []DelegationSummary `json:"delegations"`
}

type DelegationSummary struct {
	ID      string        `json:"id"`
	From    Allowance     `json:"from"`
	Policy  PolicySummary `json:"policy"`
	Revoked bool          `json:"revoked"`
}

// PolicySummary is the policy a delegation enforces, as shown to clients.
type PolicySummary struct {
	Assets              []string `json:"assets"`
	PerPaymentMax       Amount   `json:"perPaymentMax"`
	DailyMax            Amount   `json:"dailyMax"`
	AutoApproveMax      Amount   `json:"autoApproveMax"`
	NamedRecipientsOnly bool     `json:"namedRecipientsOnly"`
	NotAfter            string   `json:"notAfter,omitempty"`
}

type RevokeReply struct {
	Envelope
	Delegation string `json:"delegation"`
	Revoked    bool   `json:"revoked"`
	// Cancelled lists payments that were cancelled because they were not
	// submitted yet.
	Cancelled []string `json:"cancelled"`
}
