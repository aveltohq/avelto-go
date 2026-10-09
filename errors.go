package avelto

import (
	"fmt"
	"strings"
	"time"
)

// Error codes the API returns in its error envelope, plus [CodeNetworkError],
// which the SDK uses when no response was received. Compare them with
// [Error.Code].
const (
	// CodeValidationError: the request body or query failed validation. Details lists the issues.
	CodeValidationError = "validation_error"
	// CodeUnauthorized: missing or invalid API key.
	CodeUnauthorized = "unauthorized"
	// CodeForbidden: the key is not allowed to do this.
	CodeForbidden = "forbidden"
	// CodeLiveKeyRequired: a test key on an action that would change live sending, such as managing webhooks or templates.
	CodeLiveKeyRequired = "live_key_required"
	// CodeNotFound: no such email, domain, webhook, template or suppression.
	CodeNotFound = "not_found"
	// CodeConflict: the resource already exists (for example the domain).
	CodeConflict = "conflict"
	// CodeRateLimited: too many requests for this key; honour [Error.RetryAfter].
	CodeRateLimited = "rate_limited"
	// CodeInternalError: something went wrong on the server.
	CodeInternalError = "internal_error"
	// CodeSandboxRecipientNotAllowed: a send from the sandbox domain to an address it may not reach.
	CodeSandboxRecipientNotAllowed = "sandbox_recipient_not_allowed"
	// CodeDomainNotVerified: the sender's domain is not added and verified on the account.
	CodeDomainNotVerified = "domain_not_verified"
	// CodeDomainUnavailable: the domain cannot be added (part of another account's
	// domain, reserved, or already set up elsewhere).
	CodeDomainUnavailable = "domain_unavailable"
	// CodeRecipientSuppressed: a recipient is on the account's suppression list.
	CodeRecipientSuppressed = "recipient_suppressed"
	// CodeNotScheduled: Emails.Cancel on an email that is not scheduled.
	CodeNotScheduled = "not_scheduled"
	// CodePlanLimit: a sending cap was reached, or the plan does not allow another domain.
	CodePlanLimit = "plan_limit"
	// CodeAccountPaused: sending is paused on the account.
	CodeAccountPaused = "account_paused"
	// CodeAccountSuspended: the account is suspended; the message says why.
	CodeAccountSuspended = "account_suspended"
	// CodeNetworkError: the request never got a response (DNS, TCP, TLS, timeout, cancellation).
	CodeNetworkError = "network_error"
)

// Error is returned for every failed request, after the SDK's own retries are
// exhausted.
//
// API errors carry the HTTP Status and the Code from the response envelope
// {"error": {"code", "message", "details"}}. A request that never produced a
// response (DNS, TCP, TLS, timeout, a cancelled context) has Status 0 and Code
// [CodeNetworkError], and wraps the underlying error, so [errors.Is] and
// [errors.Unwrap] reach it.
type Error struct {
	// Status is the HTTP status code, or 0 when no response was received.
	Status int
	// Code is the API error code, such as "domain_not_verified". See the Code constants.
	Code string
	// Message is the human-readable explanation.
	Message string
	// Details is the decoded "details" of the error envelope (for a validation
	// error, the failing fields), or the raw response text when the response was
	// not an error envelope. Nil when there is none.
	Details any
	// RequestID is the x-request-id the API answered with, when it sent one.
	RequestID string
	// RetryAfter is the Retry-After header on a 429 (seconds or an HTTP date),
	// when the server sent one; zero otherwise.
	RetryAfter time.Duration

	hasRetryAfter bool
	cause         error
}

// Error formats the error as "avelto: <message> (status <n>, code <code>)".
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("avelto: ")
	b.WriteString(e.Message)
	fmt.Fprintf(&b, " (status %d, code %s", e.Status, e.Code)
	if e.RequestID != "" {
		fmt.Fprintf(&b, ", request_id %s", e.RequestID)
	}
	b.WriteString(")")
	return b.String()
}

// Unwrap returns the underlying error of a network error, or nil.
func (e *Error) Unwrap() error { return e.cause }

// IsNetworkError reports whether the request never produced a response
// (DNS, TCP, TLS, timeout, a cancelled context).
func (e *Error) IsNetworkError() bool { return e.Code == CodeNetworkError }

// codeForStatus is the code for an error response that did not carry the
// API's error envelope.
func codeForStatus(status int) string {
	switch status {
	case 400:
		return CodeValidationError
	case 401:
		return CodeUnauthorized
	case 403:
		return CodeForbidden
	case 404:
		return CodeNotFound
	case 409:
		return CodeConflict
	case 429:
		return CodeRateLimited
	default:
		return CodeInternalError
	}
}
