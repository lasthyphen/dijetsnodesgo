package txservice

import (
	"errors"
	"fmt"

	"github.com/gorilla/rpc/v2/json2"
)

// JSON-RPC error codes. They are stable across API versions; clients should
// branch on the reason in the error data.
const (
	CodeInternal         json2.ErrorCode = -32000
	CodeUnauthorized     json2.ErrorCode = -32001
	CodeNotFound         json2.ErrorCode = -32004
	CodeUnavailable      json2.ErrorCode = -32010
	CodeRateLimited      json2.ErrorCode = -32029
	CodeRecipient        json2.ErrorCode = -32030
	CodeStale            json2.ErrorCode = -32031
	CodeConflict         json2.ErrorCode = -32032
	CodePolicy           json2.ErrorCode = -32040
	CodeInsufficientFund json2.ErrorCode = -32041
	CodeRejected         json2.ErrorCode = -32050
	CodeInvalidParams    json2.ErrorCode = -32602
)

// Reasons distinguish failures that share a code.
const (
	ReasonInvalidParams       = "invalid_params"
	ReasonInvalidName         = "invalid_name"
	ReasonInvalidRecipient    = "invalid_recipient"
	ReasonInvalidAmount       = "invalid_amount"
	ReasonUnsupportedAsset    = "unsupported_asset"
	ReasonNameNotFound        = "name_not_found"
	ReasonNameNotResolvable   = "name_not_resolvable"
	ReasonNameExpiring        = "name_expiring"
	ReasonRecipientChanged    = "recipient_changed"
	ReasonSelfPayment         = "self_payment"
	ReasonPaymentNotFound     = "payment_not_found"
	ReasonPaymentExpired      = "payment_expired"
	ReasonPaymentState        = "invalid_payment_state"
	ReasonNotFinal            = "not_final"
	ReasonIdempotencyConflict = "idempotency_conflict"
	ReasonUnauthorized        = "unauthorized"
	ReasonForbidden           = "forbidden"
	ReasonDelegationNotFound  = "delegation_not_found"
	ReasonDelegationRevoked   = "delegation_revoked"
	ReasonPolicyDenied        = "policy_denied"
	ReasonApprovalRequired    = "approval_required"
	ReasonApprovalMismatch    = "approval_mismatch"
	ReasonInsufficientFunds   = "insufficient_funds"
	ReasonRateLimited         = "rate_limited"
	ReasonNetworkUnavailable  = "network_unavailable"
	ReasonTxRejected          = "tx_rejected"
	ReasonInternal            = "internal_error"
)

// ErrorData is attached to every error returned by the service.
type ErrorData struct {
	APIVersion string `json:"apiVersion"`
	Reason     string `json:"reason"`
	// Detail is a machine-readable refinement of the reason, such as the
	// violated naming rule or the policy limit that was hit.
	Detail string `json:"detail,omitempty"`
}

func apiError(code json2.ErrorCode, reason, detail, format string, args ...interface{}) *json2.Error {
	return &json2.Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Data: ErrorData{
			APIVersion: APIVersion,
			Reason:     reason,
			Detail:     detail,
		},
	}
}

// ErrorReason returns the reason of an error returned by the service, or ""
// if [err] is not one. It sees through the wrapping added by RPC clients.
func ErrorReason(err error) string {
	var jsonErr *json2.Error
	if !errors.As(err, &jsonErr) {
		return ""
	}
	switch data := jsonErr.Data.(type) {
	case ErrorData:
		return data.Reason
	case map[string]interface{}:
		reason, _ := data["reason"].(string)
		return reason
	default:
		return ""
	}
}
