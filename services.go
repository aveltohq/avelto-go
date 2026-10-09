package avelto

import (
	"context"
	"net/http"
	"strconv"
)

// SendOption configures one call to [EmailsService.Send] or
// [EmailsService.SendBatch].
type SendOption func(*sendConfig)

type sendConfig struct {
	idempotencyKey string
}

// WithIdempotencyKey sets the Idempotency-Key of a send. A replay with the same
// key returns the original email's id instead of sending again.
//
// Without it, each call gets a fresh random UUID, so the SDK's own retries never
// double-send. Pass your own (an order id, say) to make retries at your
// application's level safe too.
func WithIdempotencyKey(key string) SendOption {
	return func(c *sendConfig) { c.idempotencyKey = key }
}

func (c *Client) idempotencyHeaders(opts []SendOption) map[string]string {
	var cfg sendConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	key := cfg.idempotencyKey
	if key == "" {
		key = c.newKey()
	}
	return map[string]string{"Idempotency-Key": key}
}

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// ---------------------------------------------------------------------------
// Emails
// ---------------------------------------------------------------------------

// EmailsService sends, schedules, fetches, lists, cancels and validates
// emails. Use it through [Client.Emails].
type EmailsService struct{ c *Client }

// Send sends (or schedules) an email and returns its id. It always carries an
// Idempotency-Key: the one given with [WithIdempotencyKey], or a fresh random
// UUID.
func (s *EmailsService) Send(ctx context.Context, req *SendEmailRequest, opts ...SendOption) (*SendEmailResponse, error) {
	var out SendEmailResponse
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/emails", body: req, headers: s.c.idempotencyHeaders(opts)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendBatch sends up to 100 messages in one call.
//
// Each is accepted or refused on its own, so check Results rather than assuming
// the call either worked or did not. Results[i] lines up with Messages[i].
// Passing an idempotency key makes the whole batch replayable: a retry after a
// timeout returns the same ids instead of sending twice.
func (s *EmailsService) SendBatch(ctx context.Context, req *BatchSendRequest, opts ...SendOption) (*BatchSendResponse, error) {
	var out BatchSendResponse
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/emails/batch", body: req, headers: s.c.idempotencyHeaders(opts)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches one email with its full event log.
func (s *EmailsService) Get(ctx context.Context, id string) (*Email, error) {
	var out Email
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/emails/" + pathSegment(id)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List lists emails, newest first, with cursor pagination. params may be nil.
func (s *EmailsService) List(ctx context.Context, params *ListEmailsParams) (*ListEmailsResponse, error) {
	var out ListEmailsResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/emails", query: params.query()}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel cancels a scheduled email. It fails with code not_scheduled once the
// email has been sent.
func (s *EmailsService) Cancel(ctx context.Context, id string) (*CancelEmailResponse, error) {
	var out CancelEmailResponse
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/emails/" + pathSegment(id) + "/cancel"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Validate asks whether an address is worth a send. It checks syntax, that the
// domain has a mail server, throwaway domains, role addresses, likely typos and
// your own suppression list. Nothing is sent or stored, so it is safe to call
// from a signup form on every submit. It cannot tell you whether the mailbox
// exists; nobody can without sending.
func (s *EmailsService) Validate(ctx context.Context, email string) (*ValidateEmailResponse, error) {
	var out ValidateEmailResponse
	body := struct {
		Email string `json:"email"`
	}{email}
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/emails/validate", body: body, readOnly: true}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

// DomainsService adds, lists, verifies and removes sending domains. Use it
// through [Client.Domains].
type DomainsService struct{ c *Client }

// Create adds a sending domain. The response lists the DNS records to publish.
func (s *DomainsService) Create(ctx context.Context, req *CreateDomainRequest) (*Domain, error) {
	var out Domain
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/domains", body: req}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every domain on the account.
func (s *DomainsService) List(ctx context.Context) (*ListDomainsResponse, error) {
	var out ListDomainsResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/domains"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches a domain. Verification is re-checked on every call, so poll this
// after publishing DNS.
func (s *DomainsService) Get(ctx context.Context, id string) (*Domain, error) {
	var out Domain
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/domains/" + pathSegment(id)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a domain.
func (s *DomainsService) Delete(ctx context.Context, id string) error {
	return s.c.do(ctx, &request{method: http.MethodDelete, path: "/v1/domains/" + pathSegment(id)}, nil)
}

// ---------------------------------------------------------------------------
// Webhooks
// ---------------------------------------------------------------------------

// WebhooksService manages webhook endpoints and their deliveries. Use it
// through [Client.Webhooks].
type WebhooksService struct{ c *Client }

// Create creates an endpoint. The signing Secret is only returned here.
func (s *WebhooksService) Create(ctx context.Context, req *CreateWebhookRequest) (*WebhookEndpointWithSecret, error) {
	var out WebhookEndpointWithSecret
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/webhooks", body: req}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns every endpoint on the account.
func (s *WebhooksService) List(ctx context.Context) (*ListWebhooksResponse, error) {
	var out ListWebhooksResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/webhooks"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches one endpoint.
func (s *WebhooksService) Get(ctx context.Context, id string) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/webhooks/" + pathSegment(id)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes which events an endpoint receives, or switches it off and
// on. It needs a live key. Like every PATCH, it is never retried after a
// network error or a 504.
func (s *WebhooksService) Update(ctx context.Context, id string, req *UpdateWebhookRequest) (*WebhookEndpoint, error) {
	var out WebhookEndpoint
	var body any = req
	if req == nil {
		body = UpdateWebhookRequest{}
	}
	if err := s.c.do(ctx, &request{method: http.MethodPatch, path: "/v1/webhooks/" + pathSegment(id), body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes an endpoint.
func (s *WebhooksService) Delete(ctx context.Context, id string) error {
	return s.c.do(ctx, &request{method: http.MethodDelete, path: "/v1/webhooks/" + pathSegment(id)}, nil)
}

// ListDeliveries lists an endpoint's deliveries, newest first, with cursor
// pagination. params may be nil.
func (s *WebhooksService) ListDeliveries(ctx context.Context, id string, params *ListWebhookDeliveriesParams) (*ListWebhookDeliveriesResponse, error) {
	var out ListWebhookDeliveriesResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/webhooks/" + pathSegment(id) + "/deliveries", query: params.query()}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RetryDelivery re-enqueues a delivery.
func (s *WebhooksService) RetryDelivery(ctx context.Context, id, deliveryID string) (*WebhookDelivery, error) {
	var out WebhookDelivery
	path := "/v1/webhooks/" + pathSegment(id) + "/deliveries/" + pathSegment(deliveryID) + "/retry"
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: path}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

// TemplatesService manages stored templates and their version history. Use it
// through [Client.Templates].
type TemplatesService struct{ c *Client }

// List returns every template on the account, most recently edited first.
func (s *TemplatesService) List(ctx context.Context) (*ListTemplatesResponse, error) {
	var out ListTemplatesResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/templates"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create creates a template. The slug is derived from the name unless you set it.
func (s *TemplatesService) Create(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	var out Template
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/templates", body: req}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get fetches one template, by id.
func (s *TemplatesService) Get(ctx context.Context, id string) (*Template, error) {
	var out Template
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/templates/" + pathSegment(id)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update changes the fields you set. Every change saves a new version. See
// [UpdateTemplateRequest] for how to clear a body part. An update is never
// retried after a network error or a 504, because a repeat would save a
// second version.
func (s *TemplatesService) Update(ctx context.Context, id string, req *UpdateTemplateRequest) (*Template, error) {
	var out Template
	var body any = req
	if req == nil {
		body = UpdateTemplateRequest{}
	}
	if err := s.c.do(ctx, &request{method: http.MethodPatch, path: "/v1/templates/" + pathSegment(id), body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a template.
func (s *TemplatesService) Delete(ctx context.Context, id string) error {
	return s.c.do(ctx, &request{method: http.MethodDelete, path: "/v1/templates/" + pathSegment(id)}, nil)
}

// Versions returns every saved version of a template, newest first.
func (s *TemplatesService) Versions(ctx context.Context, id string) (*ListTemplateVersionsResponse, error) {
	var out ListTemplateVersionsResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/templates/" + pathSegment(id) + "/versions"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Restore puts an old version back, as a new version. The history is
// append-only, so a restore is itself something you can undo.
func (s *TemplatesService) Restore(ctx context.Context, id string, version int) (*Template, error) {
	var out Template
	body := struct {
		Version int `json:"version"`
	}{version}
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/templates/" + pathSegment(id) + "/restore", body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// Suppressions
// ---------------------------------------------------------------------------

// SuppressionsService manages the account's suppression list. Use it through
// [Client.Suppressions].
type SuppressionsService struct{ c *Client }

// List lists suppressed addresses with cursor pagination. params may be nil.
func (s *SuppressionsService) List(ctx context.Context, params *ListSuppressionsParams) (*ListSuppressionsResponse, error) {
	var out ListSuppressionsResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/suppressions", query: params.query()}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create suppresses an address manually.
func (s *SuppressionsService) Create(ctx context.Context, req *CreateSuppressionRequest) (*Suppression, error) {
	var out Suppression
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/suppressions", body: req}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes an address from the suppression list. The address is
// URL-encoded, so "+" and "@" arrive intact.
func (s *SuppressionsService) Delete(ctx context.Context, emailAddress string) error {
	return s.c.do(ctx, &request{method: http.MethodDelete, path: "/v1/suppressions/" + pathSegment(emailAddress)}, nil)
}

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

// AccountService reads the account and changes its settings. Use it through
// [Client.Account].
type AccountService struct{ c *Client }

// Get returns the key's mode and scopes, the sandbox and its allowed
// recipients, and what the account has set up.
func (s *AccountService) Get(ctx context.Context) (*AccountSummary, error) {
	var out AccountSummary
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/account"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSettings returns the account's name and alert settings. It needs the
// account:settings scope.
func (s *AccountService) GetSettings(ctx context.Context) (*AccountSettings, error) {
	var out AccountSettings
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/account/settings"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateSettings changes the account's name, its alert settings, or both. It
// needs a live key with the account:settings scope. A new notify email gets
// alerts only once it confirms by the link sent to it.
func (s *AccountService) UpdateSettings(ctx context.Context, req *UpdateAccountSettingsRequest) (*AccountSettings, error) {
	var out AccountSettings
	var body any = req
	if req == nil {
		body = UpdateAccountSettingsRequest{}
	}
	if err := s.c.do(ctx, &request{method: http.MethodPatch, path: "/v1/account/settings", body: body}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

// APIKeysService lists, creates and revokes API keys from a key with the
// keys:manage scope: for a program that sets an account up (an AI agent with
// a "Key for an AI agent", say) and hands a narrower key to the application.
// Use it through [Client.APIKeys].
type APIKeysService struct{ c *Client }

// List returns every key on the account, without secrets. A test key sees
// only test keys.
func (s *APIKeysService) List(ctx context.Context) (*ListAPIKeysResponse, error) {
	var out ListAPIKeysResponse
	if err := s.c.do(ctx, &request{method: http.MethodGet, path: "/v1/api-keys"}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create creates a key. Its scopes must be a subset of the calling key's and
// may not include account:settings or keys:manage; a test key may only create
// test keys. The plaintext Key is only returned here. It is never retried
// after a network error or a 504, since a repeat would create a second key.
func (s *APIKeysService) Create(ctx context.Context, req *CreateAPIKeyRequest) (*APIKeyWithSecret, error) {
	var out APIKeyWithSecret
	if err := s.c.do(ctx, &request{method: http.MethodPost, path: "/v1/api-keys", body: req}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Revoke revokes a key this key created, or this key itself. Any other id
// returns a not_found error.
func (s *APIKeysService) Revoke(ctx context.Context, id string) error {
	return s.c.do(ctx, &request{method: http.MethodDelete, path: "/v1/api-keys/" + pathSegment(id)}, nil)
}
