package avelto

import (
	"bytes"
	"encoding/json"
	"time"
)

// Request and response types for the Avelto API. JSON fields are snake_case
// and match the API one to one. Optional request fields are left out of the
// body when they hold their zero value. In responses, a field the API may
// return as null is a pointer (or a nil slice or map).

// ---------------------------------------------------------------------------
// Common
// ---------------------------------------------------------------------------

// KeyMode is the mode of an API key and of the emails it creates.
type KeyMode string

// Key modes.
const (
	ModeLive KeyMode = "live"
	ModeTest KeyMode = "test"
)

// PaginationParams pages through a list. Zero values are left out of the
// query string.
type PaginationParams struct {
	// Limit is the page size, 1-100. The API defaults to 20.
	Limit int
	// Cursor is the NextCursor of the previous page.
	Cursor string
}

func (p *PaginationParams) query() []queryParam {
	if p == nil {
		return nil
	}
	return []queryParam{{"limit", itoa(p.Limit)}, {"cursor", p.Cursor}}
}

// ---------------------------------------------------------------------------
// Emails
// ---------------------------------------------------------------------------

// EmailStatus is where an email is in its life.
type EmailStatus string

// Email statuses.
const (
	EmailQueued     EmailStatus = "queued"
	EmailScheduled  EmailStatus = "scheduled"
	EmailSent       EmailStatus = "sent"
	EmailDelivered  EmailStatus = "delivered"
	EmailBounced    EmailStatus = "bounced"
	EmailComplained EmailStatus = "complained"
	EmailFailed     EmailStatus = "failed"
	EmailCancelled  EmailStatus = "cancelled"
)

// EmailEventType is the type of an email event, and of a webhook delivery.
type EmailEventType string

// Email event types.
const (
	EventQueued          EmailEventType = "email.queued"
	EventScheduled       EmailEventType = "email.scheduled"
	EventSent            EmailEventType = "email.sent"
	EventDelivered       EmailEventType = "email.delivered"
	EventDeliveryDelayed EmailEventType = "email.delivery_delayed"
	EventBounced         EmailEventType = "email.bounced"
	EventComplained      EmailEventType = "email.complained"
	EventRejected        EmailEventType = "email.rejected"
	EventFailed          EmailEventType = "email.failed"
	EventCancelled       EmailEventType = "email.cancelled"
)

// Attachment is one file on a send. Give it Content or URL, not both and not
// neither.
//
// A URL is fetched by Avelto when the send is accepted, so a bad one is an
// error on the call that made it rather than a surprise later. It must be
// reachable from the public internet; private addresses are refused.
type Attachment struct {
	Filename string `json:"filename"`
	// Content is the file, base64-encoded.
	Content string `json:"content,omitempty"`
	// URL is an http(s) URL fetched server-side, up to 7 MB.
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// SendEmailRequest is the body of [EmailsService.Send], and one message of
// [EmailsService.SendBatch].
type SendEmailRequest struct {
	// From is "a@b.com" or "Name <a@b.com>".
	From string `json:"from"`
	// To is one or more addresses (at most 50 recipients across To, Cc and Bcc).
	To  []string `json:"to"`
	Cc  []string `json:"cc,omitempty"`
	Bcc []string `json:"bcc,omitempty"`
	// ReplyTo is one address.
	ReplyTo string `json:"reply_to,omitempty"`
	// Subject is required unless you name a template, which supplies it.
	Subject string `json:"subject,omitempty"`
	// HTML and Text: provide one or both, unless you name a template.
	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
	// Headers are extra message headers.
	Headers map[string]string `json:"headers,omitempty"`
	// Tags: up to 10; letters, numbers, _ - : .
	Tags []string `json:"tags,omitempty"`
	// Attachments: up to 10, 7 MB decoded in total.
	Attachments []Attachment `json:"attachments,omitempty"`
	// TemplateID or TemplateSlug sends a stored template instead of a body.
	// Name it by id or by slug, not both, and leave Subject, HTML and Text
	// unset; the template owns those.
	TemplateID   string `json:"template_id,omitempty"`
	TemplateSlug string `json:"template_slug,omitempty"`
	// Variables are the values for the template's {{variables}}. Scalars only
	// (string, number, bool or nil). A variable the template uses and you do
	// not supply is an error, not an empty string.
	Variables map[string]any `json:"variables,omitempty"`
	// ScheduledAt schedules the send: in the future and at most 30 days ahead.
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	// UnsubscribeURL is an https URL that unsubscribes the recipient, for bulk
	// mail. It adds the RFC 8058 List-Unsubscribe headers so a mail client can
	// unsubscribe in one click. The URL must accept an unauthenticated POST.
	// Leave it unset on transactional mail.
	UnsubscribeURL string `json:"unsubscribe_url,omitempty"`
}

// SendEmailResponse is the result of [EmailsService.Send].
type SendEmailResponse struct {
	ID string `json:"id"`
}

// EmailEvent is one entry in an email's event log.
type EmailEvent struct {
	ID         string         `json:"id"`
	Type       EmailEventType `json:"type"`
	Payload    map[string]any `json:"payload"`
	OccurredAt time.Time      `json:"occurred_at"`
}

// EmailMatchField names the field a search term was found in.
type EmailMatchField string

// Search match fields.
const (
	MatchRecipient EmailMatchField = "recipient"
	MatchSubject   EmailMatchField = "subject"
	MatchMessageID EmailMatchField = "message_id"
	MatchEmailID   EmailMatchField = "email_id"
)

// EmailMatch says which field a search term was found in, so a result can say
// why it is one.
type EmailMatch struct {
	Field EmailMatchField `json:"field"`
	// Value is the matching value itself: the recipient, the subject, the message id.
	Value string `json:"value"`
}

// EmailSummary is one row of [EmailsService.List].
type EmailSummary struct {
	ID          string      `json:"id"`
	Mode        KeyMode     `json:"mode"`
	From        string      `json:"from"`
	To          []string    `json:"to"`
	Cc          []string    `json:"cc"`
	Bcc         []string    `json:"bcc"`
	ReplyTo     *string     `json:"reply_to"`
	Subject     string      `json:"subject"`
	Tags        []string    `json:"tags"`
	Status      EmailStatus `json:"status"`
	ScheduledAt *time.Time  `json:"scheduled_at"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	// Match is present only on a search result: which field the term was found in.
	Match *EmailMatch `json:"match,omitempty"`
}

// EmailAttachment describes an attachment on a stored email.
type EmailAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
}

// Email is the full object returned by [EmailsService.Get], with its event log.
type Email struct {
	EmailSummary
	HTML           *string           `json:"html"`
	Text           *string           `json:"text"`
	Headers        map[string]string `json:"headers"`
	UnsubscribeURL *string           `json:"unsubscribe_url"`
	// IdempotencyKey is the Idempotency-Key sent with the request that created this email.
	IdempotencyKey *string           `json:"idempotency_key"`
	Attachments    []EmailAttachment `json:"attachments"`
	DomainID       *string           `json:"domain_id"`
	SESMessageID   *string           `json:"ses_message_id"`
	Error          *string           `json:"error"`
	Events         []EmailEvent      `json:"events"`
}

// ListEmailsParams filters [EmailsService.List]. Zero values are left out.
type ListEmailsParams struct {
	Status EmailStatus
	Tag    string
	// Mode defaults to the API key's mode. A test key only ever sees test emails.
	Mode KeyMode
	// Q is a substring search over recipient, subject and message id, and an
	// exact match on an email id.
	Q string
	// Limit is the page size, 1-100. The API defaults to 20.
	Limit int
	// Cursor is the NextCursor of the previous page.
	Cursor string
}

func (p *ListEmailsParams) query() []queryParam {
	if p == nil {
		return nil
	}
	return []queryParam{
		{"status", string(p.Status)},
		{"tag", p.Tag},
		{"mode", string(p.Mode)},
		{"q", p.Q},
		{"limit", itoa(p.Limit)},
		{"cursor", p.Cursor},
	}
}

// ListEmailsResponse is one page of emails, newest first.
type ListEmailsResponse struct {
	Data []EmailSummary `json:"data"`
	// NextCursor is nil on the last page.
	NextCursor *string `json:"next_cursor"`
}

// CancelEmailResponse is the result of [EmailsService.Cancel].
type CancelEmailResponse struct {
	ID string `json:"id"`
	// Status is always "cancelled".
	Status EmailStatus `json:"status"`
}

// ValidationResult is the verdict of [EmailsService.Validate].
//
// "deliverable": nothing known counts against the address. "risky": it may
// well deliver, but look twice (a throwaway domain, a role address, a likely
// typo). "undeliverable": a send would fail or be refused.
type ValidationResult string

// Validation results.
const (
	Deliverable   ValidationResult = "deliverable"
	Risky         ValidationResult = "risky"
	Undeliverable ValidationResult = "undeliverable"
)

// ValidationReason is the most serious thing [EmailsService.Validate] found.
type ValidationReason string

// Validation reasons.
const (
	ReasonInvalidSyntax ValidationReason = "invalid_syntax"
	ReasonSuppressed    ValidationReason = "suppressed"
	ReasonNoMailServer  ValidationReason = "no_mail_server"
	ReasonDisposable    ValidationReason = "disposable"
	ReasonPossibleTypo  ValidationReason = "possible_typo"
	ReasonRoleAddress   ValidationReason = "role_address"
)

// ValidationChecks are the individual checks behind a validation result.
type ValidationChecks struct {
	// Syntax: well-formed as an address.
	Syntax bool `json:"syntax"`
	// MailServer: the domain publishes MX records (or an address record). Nil
	// when the lookup did not complete in time.
	MailServer *bool `json:"mail_server"`
	// Disposable: a known throwaway-mail domain.
	Disposable bool `json:"disposable"`
	// Role: a functional mailbox such as info@ or noreply@ rather than a person.
	Role bool `json:"role"`
	// FreeProvider: a consumer webmail provider. Informational; it does not affect the result.
	FreeProvider bool `json:"free_provider"`
	// Suppressed: on your suppression list, so a send would be refused with recipient_suppressed.
	Suppressed bool `json:"suppressed"`
}

// ValidateEmailResponse is the result of [EmailsService.Validate].
type ValidateEmailResponse struct {
	// Email is the address as given, trimmed.
	Email string `json:"email"`
	// Normalized is lower-cased, the form to store and compare. Nil when the syntax is invalid.
	Normalized *string          `json:"normalized"`
	Result     ValidationResult `json:"result"`
	// Reason is nil when the result is deliverable.
	Reason *ValidationReason `json:"reason"`
	Checks ValidationChecks  `json:"checks"`
	// Suggestion is a corrected address when the domain looks like a slip of a well-known one.
	Suggestion *string `json:"suggestion"`
}

// BatchSendRequest is the body of [EmailsService.SendBatch]: up to 100
// messages, each validated and accepted on its own.
type BatchSendRequest struct {
	Messages []SendEmailRequest `json:"messages"`
}

// BatchError is why one message of a batch was refused.
type BatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// BatchSendResult is one message's outcome. Index matches the position in the
// request, so Results[i].Index == i. When OK is true, ID is set; otherwise
// Error is.
type BatchSendResult struct {
	OK    bool        `json:"ok"`
	Index int         `json:"index"`
	ID    string      `json:"id,omitempty"`
	Error *BatchError `json:"error,omitempty"`
}

// BatchSendResponse is the result of [EmailsService.SendBatch].
type BatchSendResponse struct {
	Results  []BatchSendResult `json:"results"`
	Accepted int               `json:"accepted"`
	Failed   int               `json:"failed"`
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

// DomainStatus is a domain's verification status.
type DomainStatus string

// Domain statuses.
const (
	DomainPending  DomainStatus = "pending"
	DomainVerified DomainStatus = "verified"
	DomainFailed   DomainStatus = "failed"
)

// DomainHealth is a domain's overall DNS health at the last check.
type DomainHealth string

// Domain health values.
const (
	HealthUnknown      DomainHealth = "unknown"
	HealthHealthy      DomainHealth = "healthy"
	HealthActionNeeded DomainHealth = "action_needed"
)

// DNSRecordState says whether the record we expect is published, and what is
// there instead.
type DNSRecordState string

// DNS record states.
const (
	DNSFound    DNSRecordState = "found"
	DNSMissing  DNSRecordState = "missing"
	DNSMismatch DNSRecordState = "mismatch"
)

// CreateDomainRequest is the body of [DomainsService.Create].
type CreateDomainRequest struct {
	Name string `json:"name"`
}

// DNSRecord is one record to publish for a domain.
type DNSRecord struct {
	// Type is "CNAME", "TXT" or "MX".
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	// Purpose is "ownership", "dkim", "spf" or "dmarc". The ownership record is
	// listed until it is found; the DKIM records appear after that.
	Purpose string `json:"purpose"`
}

// DNSCheck is the result of checking one record.
type DNSCheck struct {
	DNSRecord
	// ID is a stable row id: "ownership", "dkim1".."dkim3", "spf", "dmarc".
	ID    string         `json:"id"`
	State DNSRecordState `json:"state"`
	// Found is what the resolver returned, or nil when the lookup found nothing.
	Found *string `json:"found"`
}

// Domain is a sending domain.
type Domain struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Region string       `json:"region"`
	Status DomainStatus `json:"status"`
	// OwnershipVerified is whether the ownership TXT record has been found. A
	// domain still unproven 7 days after it was added is removed.
	OwnershipVerified bool        `json:"ownership_verified"`
	DNSRecords        []DNSRecord `json:"dns_records"`
	// Health is "unknown" until a check has run.
	Health          DomainHealth `json:"health"`
	HealthChangedAt *time.Time   `json:"health_changed_at"`
	// DNSChecks are the per-record results of the last check; nil until one has run.
	DNSChecks    []DNSCheck `json:"dns_checks"`
	DNSCheckedAt *time.Time `json:"dns_checked_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ListDomainsResponse is the result of [DomainsService.List].
type ListDomainsResponse struct {
	Data []Domain `json:"data"`
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

// CreateWebhookRequest is the body of [WebhooksService.Create].
type CreateWebhookRequest struct {
	// URL is an http(s) URL, at most 2048 characters.
	URL string `json:"url"`
	// Events defaults to all eight: sent, delivered, delivery_delayed, bounced,
	// complained, rejected, failed and cancelled.
	Events []EmailEventType `json:"events,omitempty"`
}

// WebhookEndpoint is a webhook endpoint.
type WebhookEndpoint struct {
	ID        string           `json:"id"`
	URL       string           `json:"url"`
	Events    []EmailEventType `json:"events"`
	Enabled   bool             `json:"enabled"`
	CreatedAt time.Time        `json:"created_at"`
}

// WebhookEndpointWithSecret is returned once, by [WebhooksService.Create]: it
// includes the signing secret. Store it.
type WebhookEndpointWithSecret struct {
	WebhookEndpoint
	Secret string `json:"secret"`
}

// ListWebhooksResponse is the result of [WebhooksService.List].
type ListWebhooksResponse struct {
	Data []WebhookEndpoint `json:"data"`
}

// UpdateWebhookRequest is the body of [WebhooksService.Update]. Set Events,
// Enabled, or both; a field left nil is not changed. The URL cannot change: a
// new URL is a new endpoint, with its own secret.
type UpdateWebhookRequest struct {
	// Events replaces the list of event types the endpoint receives.
	Events []EmailEventType `json:"events,omitempty"`
	// Enabled switches the endpoint off (false) and on (true). Use [Bool].
	Enabled *bool `json:"enabled,omitempty"`
}

// WebhookDeliveryStatus is the status of one delivery.
type WebhookDeliveryStatus string

// Webhook delivery statuses.
const (
	DeliveryPending   WebhookDeliveryStatus = "pending"
	DeliveryDelivered WebhookDeliveryStatus = "delivered"
	DeliveryFailed    WebhookDeliveryStatus = "failed"
)

// WebhookDelivery is one attempt to deliver an event to an endpoint.
type WebhookDelivery struct {
	ID         string `json:"id"`
	EndpointID string `json:"endpoint_id"`
	// EmailEventID is nil for a test delivery, which has no email event behind it.
	EmailEventID *string        `json:"email_event_id"`
	EventType    EmailEventType `json:"event_type"`
	// EmailID is nil for a test delivery, for the same reason.
	EmailID   *string               `json:"email_id"`
	Status    WebhookDeliveryStatus `json:"status"`
	Attempts  int                   `json:"attempts"`
	LastError *string               `json:"last_error"`
	// ResponseCode is what the endpoint answered with, once it has answered.
	ResponseCode *int `json:"response_code"`
	DurationMS   *int `json:"duration_ms"`
	// IsTest is true when this delivery came from "send a test event" rather than real mail.
	IsTest bool `json:"is_test"`
	// ReplayOf is set when this delivery repeats an earlier one.
	ReplayOf  *string   `json:"replay_of"`
	CreatedAt time.Time `json:"created_at"`
}

// ListWebhookDeliveriesParams pages through [WebhooksService.ListDeliveries].
type ListWebhookDeliveriesParams = PaginationParams

// ListWebhookDeliveriesResponse is one page of deliveries.
type ListWebhookDeliveriesResponse struct {
	Data []WebhookDelivery `json:"data"`
	// NextCursor is nil on the last page.
	NextCursor *string `json:"next_cursor"`
}

// WebhookPayload is the body POSTed to your webhook endpoint. It is signed
// with HMAC-SHA256 over "<timestamp>.<body>" and sent in the Avelto-Signature
// header as t=<unix seconds>,v1=<hex>; check it with [VerifyWebhookSignature]
// before decoding.
type WebhookPayload struct {
	// ID is the event id; use it to de-duplicate.
	ID        string             `json:"id"`
	Type      EmailEventType     `json:"type"`
	CreatedAt time.Time          `json:"created_at"`
	Data      WebhookPayloadData `json:"data"`
}

// WebhookPayloadData is the email a webhook event is about.
type WebhookPayloadData struct {
	EmailID string   `json:"email_id"`
	Mode    KeyMode  `json:"mode"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Tags    []string `json:"tags"`
	Status  string   `json:"status"`
	// Details are event-specific (bounce type, diagnostic code, ...).
	Details map[string]any `json:"details"`
}

// ---------------------------------------------------------------------------
// Suppressions
// ---------------------------------------------------------------------------

// SuppressionReason is why an address is suppressed.
type SuppressionReason string

// Suppression reasons.
const (
	SuppressionBounce    SuppressionReason = "bounce"
	SuppressionComplaint SuppressionReason = "complaint"
	SuppressionManual    SuppressionReason = "manual"
)

// Suppression is one entry on the suppression list.
type Suppression struct {
	ID string `json:"id"`
	// EmailAddress is nil once the recipient has been erased; the entry still blocks sending.
	EmailAddress *string           `json:"email_address"`
	Reason       SuppressionReason `json:"reason"`
	CreatedAt    time.Time         `json:"created_at"`
}

// ListSuppressionsParams pages through [SuppressionsService.List].
type ListSuppressionsParams = PaginationParams

// ListSuppressionsResponse is one page of suppressions.
type ListSuppressionsResponse struct {
	Data []Suppression `json:"data"`
	// NextCursor is nil on the last page.
	NextCursor *string `json:"next_cursor"`
}

// CreateSuppressionRequest is the body of [SuppressionsService.Create].
type CreateSuppressionRequest struct {
	EmailAddress string `json:"email_address"`
}

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

// Plan is an account's plan.
type Plan string

// Plans.
const (
	PlanFree       Plan = "free"
	PlanPro        Plan = "pro"
	PlanGrowth     Plan = "growth"
	PlanEnterprise Plan = "enterprise"
)

// AccountSummary is what the key can do and what the account has set up.
// Read it first when integrating; it says which sandbox addresses a send may
// reach and whether a domain, a webhook or a template exists yet.
type AccountSummary struct {
	Account   AccountInfo       `json:"account"`
	Key       KeyInfo           `json:"key"`
	Sandbox   SandboxInfo       `json:"sandbox"`
	Domains   []AccountDomain   `json:"domains"`
	Webhooks  []AccountWebhook  `json:"webhooks"`
	Templates []AccountTemplate `json:"templates"`
	// DocsURL is where the documentation lives.
	DocsURL string `json:"docs_url"`
}

// AccountInfo identifies the account and its plan.
type AccountInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Plan Plan   `json:"plan"`
	// PlanStep is the pricing step inside the plan, or nil for the plan's default step.
	PlanStep *string `json:"plan_step"`
}

// KeyInfo is the calling key's mode and scopes.
type KeyInfo struct {
	Mode   KeyMode  `json:"mode"`
	Scopes []string `json:"scopes"`
}

// SandboxInfo describes the account's sandbox domain.
type SandboxInfo struct {
	// Domain: every account may send from any address on it before verifying its own.
	Domain string `json:"domain"`
	// Recipients is every address a sandbox sender may deliver to: the owner's
	// verified email and the simulator addresses.
	Recipients []string `json:"recipients"`
	// Simulator lists the addresses that produce each outcome.
	Simulator SimulatorAddresses `json:"simulator"`
}

// SimulatorAddresses are the sandbox addresses that each produce the named outcome.
type SimulatorAddresses struct {
	Delivered  string `json:"delivered"`
	Bounced    string `json:"bounced"`
	Complained string `json:"complained"`
}

// AccountDomain is a domain as listed in [AccountSummary].
type AccountDomain struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Status DomainStatus `json:"status"`
}

// AccountWebhook is a webhook endpoint as listed in [AccountSummary].
type AccountWebhook struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// AccountTemplate is a template as listed in [AccountSummary].
type AccountTemplate struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// AccountSettings is the account's name and its alert settings, returned by
// [AccountService.GetSettings]. It needs the account:settings scope.
type AccountSettings struct {
	Name   string        `json:"name"`
	Alerts AlertSettings `json:"alerts"`
}

// AlertSettings says where the account's alerts go and which are on.
type AlertSettings struct {
	// NotifyEmail is where alerts go, or nil for the account owner.
	NotifyEmail *string `json:"notify_email"`
	// NotifyEmailVerified is whether NotifyEmail has confirmed by the link
	// sent to it. Until it has, alerts keep going to the owner.
	NotifyEmailVerified bool `json:"notify_email_verified"`
	DomainHealth        bool `json:"domain_health"`
	BounceRate          bool `json:"bounce_rate"`
	ComplaintRate       bool `json:"complaint_rate"`
	Usage80             bool `json:"usage_80"`
	APIKeyCreated       bool `json:"api_key_created"`
}

// UpdateAccountSettingsRequest is the body of [AccountService.UpdateSettings].
// Set Name, Alerts, or both.
type UpdateAccountSettingsRequest struct {
	Name   string                      `json:"name,omitempty"`
	Alerts *UpdateAlertSettingsRequest `json:"alerts,omitempty"`
}

// UpdateAlertSettingsRequest holds the alert fields to change; the ones left
// unset keep their value. NotifyEmail is a [Nullable]: [Null] sends alerts
// back to the owner, and a new address gets them only once it confirms by the
// link sent to it. Use [Bool] for the switches.
type UpdateAlertSettingsRequest struct {
	NotifyEmail   Nullable[string]
	DomainHealth  *bool
	BounceRate    *bool
	ComplaintRate *bool
	Usage80       *bool
	APIKeyCreated *bool
}

// MarshalJSON writes only the fields that were set, and null for a
// NotifyEmail set with [Null].
func (r UpdateAlertSettingsRequest) MarshalJSON() ([]byte, error) {
	fields := []setField{
		{"notify_email", r.NotifyEmail.IsSet(), r.NotifyEmail},
		{"domain_health", r.DomainHealth != nil, r.DomainHealth},
		{"bounce_rate", r.BounceRate != nil, r.BounceRate},
		{"complaint_rate", r.ComplaintRate != nil, r.ComplaintRate},
		{"usage_80", r.Usage80 != nil, r.Usage80},
		{"api_key_created", r.APIKeyCreated != nil, r.APIKeyCreated},
	}
	return marshalSetFields(fields)
}

// Bool returns a pointer to v, for the optional switches in
// [UpdateWebhookRequest] and [UpdateAlertSettingsRequest].
func Bool(v bool) *bool { return &v }

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

// APIKeyScope is what a key may do.
type APIKeyScope string

// API key scopes. ScopeAccountSettings and ScopeKeysManage are only granted
// by the account owner in the dashboard, never by [APIKeysService.Create].
const (
	ScopeEmailsSend         APIKeyScope = "emails:send"
	ScopeEmailsRead         APIKeyScope = "emails:read"
	ScopeDomainsManage      APIKeyScope = "domains:manage"
	ScopeWebhooksManage     APIKeyScope = "webhooks:manage"
	ScopeSuppressionsManage APIKeyScope = "suppressions:manage"
	ScopeTemplatesManage    APIKeyScope = "templates:manage"
	ScopeAccountManage      APIKeyScope = "account:manage"
	ScopeAccountSettings    APIKeyScope = "account:settings"
	ScopeKeysManage         APIKeyScope = "keys:manage"
)

// APIKey is an API key as listed. The secret is never included after creation.
type APIKey struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Mode KeyMode `json:"mode"`
	// Prefix is enough to recognise the key, never the whole thing.
	Prefix string        `json:"prefix"`
	Scopes []APIKeyScope `json:"scopes"`
	// CreatedByKeyID is the key that created this one through
	// [APIKeysService.Create], or nil when a person made it.
	CreatedByKeyID *string    `json:"created_by_key_id"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	CreatedAt      time.Time  `json:"created_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
}

// APIKeyWithSecret is returned once, by [APIKeysService.Create]: Key is the
// plaintext key. Store it.
type APIKeyWithSecret struct {
	APIKey
	Key string `json:"key"`
}

// ListAPIKeysResponse is the result of [APIKeysService.List].
type ListAPIKeysResponse struct {
	Data []APIKey `json:"data"`
}

// CreateAPIKeyRequest is the body of [APIKeysService.Create]. Scopes are
// required and must be a subset of the calling key's.
type CreateAPIKeyRequest struct {
	Name   string        `json:"name"`
	Mode   KeyMode       `json:"mode"`
	Scopes []APIKeyScope `json:"scopes"`
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

// Template is a stored template.
type Template struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Slug is the stable handle for TemplateSlug on a send.
	Slug    string  `json:"slug"`
	Subject string  `json:"subject"`
	HTML    *string `json:"html"`
	Text    *string `json:"text"`
	// Variables is every {{variable}} the template refers to, sorted.
	Variables []string  `json:"variables"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TemplateVersion is one saved version of a template.
type TemplateVersion struct {
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject"`
	HTML      *string   `json:"html"`
	Text      *string   `json:"text"`
	CreatedBy *string   `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ListTemplatesResponse is the result of [TemplatesService.List].
type ListTemplatesResponse struct {
	Data []Template `json:"data"`
}

// ListTemplateVersionsResponse is the result of [TemplatesService.Versions].
type ListTemplateVersionsResponse struct {
	Data []TemplateVersion `json:"data"`
}

// CreateTemplateRequest is the body of [TemplatesService.Create].
type CreateTemplateRequest struct {
	Name string `json:"name"`
	// Slug is derived from the name when left empty.
	Slug    string `json:"slug,omitempty"`
	Subject string `json:"subject"`
	// HTML and Text: provide one or both.
	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
}

// UpdateTemplateRequest is the body of [TemplatesService.Update]. Only the
// fields you set change.
//
// Name, Slug and Subject are left unchanged when empty. HTML and Text are
// [Nullable]: leave them as the zero value to keep the body part,
// set them with [Value] to change it, or with [Null] to clear it:
//
//	&avelto.UpdateTemplateRequest{
//		Subject: "Hello {{name}}",
//		Text:    avelto.Null[string](), // clear the plain-text part
//	}
type UpdateTemplateRequest struct {
	Name    string
	Slug    string
	Subject string
	HTML    Nullable[string]
	Text    Nullable[string]
}

// MarshalJSON writes only the fields that were set, and null for a Nullable
// set with [Null].
func (r UpdateTemplateRequest) MarshalJSON() ([]byte, error) {
	return marshalSetFields([]setField{
		{"name", r.Name != "", r.Name},
		{"slug", r.Slug != "", r.Slug},
		{"subject", r.Subject != "", r.Subject},
		{"html", r.HTML.IsSet(), r.HTML},
		{"text", r.Text.IsSet(), r.Text},
	})
}

// setField is one field of a partial-update body: written only when set.
type setField struct {
	key string
	set bool
	v   any
}

// marshalSetFields writes a JSON object of the fields that are set, in order,
// without escaping HTML.
func marshalSetFields(fields []setField) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, f := range fields {
		if !f.set {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(f.key)
		buf.Write(k)
		buf.WriteByte(':')
		b, err := marshalNoEscape(f.v)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Nullable
// ---------------------------------------------------------------------------

// Nullable is a request field with three states: not set (the zero value,
// left out of the request, so the server keeps what it has), set to a value,
// or set to null (which clears it). Build one with [Value] or [Null].
type Nullable[T any] struct {
	value T
	set   bool
	null  bool
}

// Value returns a [Nullable] set to v.
func Value[T any](v T) Nullable[T] { return Nullable[T]{value: v, set: true} }

// Null returns a [Nullable] set to null, which clears the field.
func Null[T any]() Nullable[T] { return Nullable[T]{set: true, null: true} }

// IsSet reports whether the field is set, to a value or to null.
func (n Nullable[T]) IsSet() bool { return n.set }

// IsNull reports whether the field is set to null.
func (n Nullable[T]) IsNull() bool { return n.set && n.null }

// Get returns the value and true when the field is set to a value, and the
// zero value and false otherwise.
func (n Nullable[T]) Get() (T, bool) {
	if n.set && !n.null {
		return n.value, true
	}
	var zero T
	return zero, false
}

// MarshalJSON writes the value, or null when the field is null or not set.
// A struct holding a Nullable decides whether to leave an unset one out, as
// [UpdateTemplateRequest] does.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if v, ok := n.Get(); ok {
		return marshalNoEscape(v)
	}
	return []byte("null"), nil
}

// UnmarshalJSON sets the field to null or to the decoded value.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	if string(bytes.TrimSpace(b)) == "null" {
		*n = Null[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = Value(v)
	return nil
}

// marshalNoEscape is json.Marshal without escaping <, > and &, so HTML bodies
// travel as written.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
