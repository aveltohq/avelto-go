# avelto-go

The official Go SDK for [Avelto](https://avelto.dev), the email API for developers who want it to just work.

- Go 1.21+
- Standard library only, no dependencies
- Typed: every request and response is a Go struct with the API's snake_case JSON tags
- Every method takes a `context.Context`; the client is safe for concurrent use

## Install

```sh
go get github.com/aveltohq/avelto-go
```

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aveltohq/avelto-go"
)

func main() {
	client, err := avelto.NewClient(os.Getenv("AVELTO_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	email, err := client.Emails.Send(context.Background(), &avelto.SendEmailRequest{
		From:    "Acme <hello@mail.acme.com>",
		To:      []string{"jane@example.com"},
		Subject: "Welcome to Acme",
		HTML:    "<p>Thanks for signing up.</p>",
		Text:    "Thanks for signing up.",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(email.ID) // "5b3d…"
}
```

`To`, `Cc` and `Bcc` take one address or several. Addresses can be `a@b.com` or `Name <a@b.com>`. Provide `HTML`, `Text`, or both. You can also set `ReplyTo`, `Headers`, `Tags` (up to 10), `Attachments` (each with a `Filename` and base64 `Content` or an http(s) `URL`, up to 10 files and 7 MB in total), `ScheduledAt` and `UnsubscribeURL`. To send a stored template instead of a body, set `TemplateID` or `TemplateSlug` with `Variables` and leave `Subject`, `HTML` and `Text` empty.

Fields left at their zero value are not sent. Response fields the API may return as `null` are pointers, so `email.ReplyTo == nil` means there is none.

### Options

```go
client, err := avelto.NewClient(apiKey,
	avelto.WithBaseURL("https://api.avelto.dev"),                    // also read from AVELTO_BASE_URL
	avelto.WithTimeout(30*time.Second),                              // per attempt; 0 leaves only the context's deadline
	avelto.WithRetry(3, 300*time.Millisecond, 5*time.Second),        // the retry defaults
	avelto.WithHTTPClient(&http.Client{Transport: myTransport}),     // a custom transport, proxy or instrumentation
)
```

`NewClient` returns an error when the key is empty. Create one client and share it; it holds no per-request state.

## Idempotency

Every `Emails.Send` and `Emails.SendBatch` carries an `Idempotency-Key` header: a fresh random UUID per call by default, so the SDK's own retries can never create two emails. Pass your own key (an order id, say) to make retries at your level safe too. A replay returns the id of the email that was already created and does not send again.

```go
email, err := client.Emails.Send(ctx, &avelto.SendEmailRequest{
	From:    "hello@mail.acme.com",
	To:      []string{"jane@example.com"},
	Subject: "Receipt #1042",
	Text:    "…",
}, avelto.WithIdempotencyKey("receipt-1042"))
```

## Retries

The SDK makes up to three attempts at a failed request (the call plus two retries) with exponential backoff and jitter; a `429` waits for the server's `Retry-After` instead, capped at 30 seconds or the maximum delay, whichever is larger. `429`, `502` and `503` are retried for every request. Network errors, timeouts and `504` (where the request may have been processed) are retried only for `GET`, `DELETE`, `Emails.Send`, `Emails.SendBatch` and `Emails.Validate`, never for the other `POST`s or for `PATCH`. Pass `avelto.WithRetry(1, 0, 0)` to turn retries off. After the last attempt the `*avelto.Error` is returned; on `429` it carries `RetryAfter`.

A cancelled context stops the retries at once, including while the SDK is waiting between attempts.

## Scheduled send and cancel

`ScheduledAt` is a `*time.Time`, in the future and at most 30 days ahead. A scheduled email can be cancelled until it is sent.

```go
at := time.Now().Add(24 * time.Hour)
email, err := client.Emails.Send(ctx, &avelto.SendEmailRequest{
	From:        "hello@mail.acme.com",
	To:          []string{"jane@example.com"},
	Subject:     "Your trial ends tomorrow",
	Text:        "…",
	ScheduledAt: &at,
})

cancelled, err := client.Emails.Cancel(ctx, email.ID) // cancelled.Status == avelto.EmailCancelled
```

Cancelling an email that is no longer `scheduled` returns an `*avelto.Error` with code `not_scheduled`.

## Validate an address

Ask whether an address is worth a send before you spend one on it, for example on every submit of a signup form. Nothing is sent or stored.

```go
check, err := client.Emails.Validate(ctx, "jane@gmial.com")
check.Result      // avelto.Deliverable | avelto.Risky | avelto.Undeliverable
check.Reason      // nil, or "invalid_syntax" | "suppressed" | "no_mail_server" | "disposable" | "possible_typo" | "role_address"
check.Suggestion  // "jane@gmail.com"
check.Normalized  // the lower-cased address to store, or nil when the syntax is invalid
check.Checks      // Syntax, MailServer, Disposable, Role, FreeProvider, Suppressed
```

`undeliverable` means a send would fail or be refused: bad syntax, a domain with no mail server, or an address on your own suppression list. `risky` means it may well deliver but deserves a look: a throwaway domain, a role address such as `info@`, or a domain within a typo of a well-known one, in which case `Suggestion` is the corrected address to offer the user. It cannot tell you whether the mailbox exists; nobody can without sending.

## Batch send

`Emails.SendBatch` sends up to 100 messages in one call. Each message is a `SendEmailRequest`. Each is accepted or refused on its own, so read `Results` rather than assuming the call either worked or did not; `Results[i]` lines up with `Messages[i]`.

```go
batch, err := client.Emails.SendBatch(ctx, &avelto.BatchSendRequest{
	Messages: []avelto.SendEmailRequest{
		{From: "billing@mail.acme.com", To: []string{"jane@example.com"}, Subject: "Receipt #1042", Text: "…"},
		{From: "billing@mail.acme.com", To: []string{"sam@example.com"}, Subject: "Receipt #1043", Text: "…"},
	},
}, avelto.WithIdempotencyKey("receipts-2026-09-24"))
if err != nil {
	log.Fatal(err)
}

for _, r := range batch.Results {
	if r.OK {
		fmt.Println(r.Index, r.ID)
	} else {
		fmt.Println(r.Index, r.Error.Code, r.Error.Message)
	}
}
```

## Fetch and list emails

```go
email, err := client.Emails.Get(ctx, emailID)
fmt.Println(email.Status) // "queued" | "scheduled" | "sent" | "delivered" | "bounced" | "complained" | "failed" | "cancelled"
for _, e := range email.Events {
	fmt.Println(e.Type) // "email.queued", "email.sent", "email.delivered"
}
```

Lists are newest first and cursor-paginated. Filter by `Status`, `Tag`, `Mode` and `Q`, a substring search over recipient, subject and message id. A live key defaults to live and may ask for test; a test key only ever sees test emails, whatever `Mode` says, and a live email is a `404` to it.

```go
params := &avelto.ListEmailsParams{Status: avelto.EmailBounced, Tag: "onboarding", Limit: 100}
for {
	page, err := client.Emails.List(ctx, params)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range page.Data {
		fmt.Println(e.ID, e.To, e.Subject)
	}
	if page.NextCursor == nil {
		break
	}
	params.Cursor = *page.NextCursor
}
```

## Domains

Add a domain, publish the DNS records it returns, then poll `Get` until it is verified. `Domains.Get` re-checks verification on every call.

```go
domain, err := client.Domains.Create(ctx, &avelto.CreateDomainRequest{Name: "mail.acme.com"})
if err != nil {
	log.Fatal(err)
}
for _, r := range domain.DNSRecords {
	fmt.Printf("%s\t%s\t%s\t(%s)\n", r.Type, r.Name, r.Value, r.Purpose)
}

// Publish the records, then:
for domain.Status == avelto.DomainPending {
	time.Sleep(30 * time.Second)
	if domain, err = client.Domains.Get(ctx, domain.ID); err != nil {
		log.Fatal(err)
	}
}
fmt.Println(domain.Status) // "verified" (or "failed")

client.Domains.List(ctx)
client.Domains.Delete(ctx, domain.ID)
```

## Templates

```go
tpl, err := client.Templates.Create(ctx, &avelto.CreateTemplateRequest{
	Name:    "Welcome",
	Subject: "Hi {{name}}",
	HTML:    "<p>Welcome, {{name}}.</p>",
})

client.Emails.Send(ctx, &avelto.SendEmailRequest{
	From:         "hello@mail.acme.com",
	To:           []string{"jane@example.com"},
	TemplateSlug: tpl.Slug,
	Variables:    map[string]any{"name": "Jane"},
})

client.Templates.Update(ctx, tpl.ID, &avelto.UpdateTemplateRequest{Subject: "Hello {{name}}"}) // saves a new version
client.Templates.Update(ctx, tpl.ID, &avelto.UpdateTemplateRequest{Text: avelto.Null[string]()}) // clears a body part

versions, err := client.Templates.Versions(ctx, tpl.ID)
client.Templates.Restore(ctx, tpl.ID, versions.Data[len(versions.Data)-1].Version)
```

`UpdateTemplateRequest` changes only what you set. `Name`, `Slug` and `Subject` are left alone when empty. `HTML` and `Text` are `avelto.Nullable[string]`, because clearing a body part is not the same as leaving it: the zero value keeps it, `avelto.Value("…")` replaces it and `avelto.Null[string]()` clears it.

## Webhooks

Create an endpoint. The signing `Secret` is returned once, on creation. Store it.

```go
endpoint, err := client.Webhooks.Create(ctx, &avelto.CreateWebhookRequest{
	URL:    "https://acme.com/hooks/avelto",
	Events: []avelto.EmailEventType{avelto.EventDelivered, avelto.EventBounced, avelto.EventComplained}, // optional; defaults to all eight
})
fmt.Println(endpoint.Secret)
```

Every delivery is a JSON POST with an `Avelto-Signature` header of the form `t=<unix seconds>,v1=<hex>`. Verify it against the raw body before you trust the payload. Use the event `ID` to de-duplicate.

### net/http

```go
http.HandleFunc("/hooks/avelto", func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body) // the raw bytes, before any decoding
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	if !avelto.VerifyWebhookSignature(os.Getenv("AVELTO_WEBHOOK_SECRET"), body, r.Header.Get(avelto.WebhookSignatureHeader)) {
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}
	var event avelto.WebhookPayload
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if event.Type == avelto.EventBounced {
		log.Println("bounced", event.Data.EmailID, event.Data.Details)
	}
	w.WriteHeader(http.StatusOK)
})
```

Signatures older than five minutes are rejected; pass `avelto.WithTolerance(d)` to change that. The comparison is constant-time.

### Deliveries

```go
endpoints, err := client.Webhooks.List(ctx)
endpoint, err := client.Webhooks.Get(ctx, endpointID)

page, err := client.Webhooks.ListDeliveries(ctx, endpointID, &avelto.ListWebhookDeliveriesParams{Limit: 50})
for _, d := range page.Data {
	if d.Status == avelto.DeliveryFailed {
		client.Webhooks.RetryDelivery(ctx, endpointID, d.ID)
		break
	}
}

// Change which events it receives, or switch it off and on. The URL cannot change.
client.Webhooks.Update(ctx, endpointID, &avelto.UpdateWebhookRequest{
	Events:  []avelto.EmailEventType{avelto.EventBounced, avelto.EventComplained},
	Enabled: avelto.Bool(true),
})

client.Webhooks.Delete(ctx, endpointID)
```

## Suppressions

Addresses that hard-bounce or complain are suppressed automatically and sends to them are rejected. You can manage the list yourself.

```go
page, err := client.Suppressions.List(ctx, &avelto.ListSuppressionsParams{Limit: 100})
for _, s := range page.Data {
	if s.EmailAddress != nil {
		fmt.Println(*s.EmailAddress, s.Reason) // "bounce" | "complaint" | "manual"
	}
}

client.Suppressions.Create(ctx, &avelto.CreateSuppressionRequest{EmailAddress: "unsubscribed@example.com"})
client.Suppressions.Delete(ctx, "unsubscribed@example.com")
```

`EmailAddress` is `nil` once a recipient has been erased; the entry still blocks sending.

## Account

`Account.Get` returns the key's mode and scopes, the sandbox domain with every recipient a sandbox send may reach, and the domains, webhook endpoints and templates that exist. Read it first when wiring up a new project.

```go
summary, err := client.Account.Get(ctx)
fmt.Println(summary.Key.Mode, summary.Sandbox.Domain, summary.Sandbox.Recipients)
```

With the `account:settings` scope, read and change the account's name and its alert settings. Changing them needs a live key. A new notify email only receives alerts once it confirms by the link sent to it; `avelto.Null[string]()` sends them back to the owner.

```go
settings, err := client.Account.GetSettings(ctx)
settings, err = client.Account.UpdateSettings(ctx, &avelto.UpdateAccountSettingsRequest{
	Name:   "Acme",
	Alerts: &avelto.UpdateAlertSettingsRequest{NotifyEmail: avelto.Value("ops@acme.com"), BounceRate: avelto.Bool(true)},
})
```

## API keys

With the `keys:manage` scope (part of a "Key for an AI agent", which only the account owner can create), a key can list keys, create narrower ones and revoke the ones it made. A created key's scopes must be a subset of the caller's and never include `account:settings` or `keys:manage`; a test key can only create test keys.

```go
created, err := client.APIKeys.Create(ctx, &avelto.CreateAPIKeyRequest{
	Name:   "Production server",
	Mode:   avelto.ModeLive,
	Scopes: []avelto.APIKeyScope{avelto.ScopeEmailsSend},
})
// created.Key is shown only here: put it in the application's environment.

keys, err := client.APIKeys.List(ctx)
err = client.APIKeys.Revoke(ctx, created.ID)
```

## Error handling

Every failed request returns an `*avelto.Error` with the HTTP `Status`, the API `Code`, the `Message`, optional `Details`, the `RequestID` the API answered with, and on a `429` the `RetryAfter`. A request that got no response at all has status `0` and code `network_error`, and wraps the underlying error, so `errors.Is(err, context.DeadlineExceeded)` and `errors.Unwrap` work; `IsNetworkError()` is true for exactly those.

```go
_, err := client.Emails.Send(ctx, &avelto.SendEmailRequest{From: "hello@mail.acme.com", To: []string{"jane@example.com"}, Subject: "Hi", Text: "Hi"})

var apiErr *avelto.Error
if errors.As(err, &apiErr) {
	switch {
	case apiErr.Code == avelto.CodeDomainNotVerified: // 403: verify mail.acme.com first
	case apiErr.Code == avelto.CodeSandboxRecipientNotAllowed: // 403: the sandbox sender only delivers to your account's owner email
	case apiErr.Code == avelto.CodeRecipientSuppressed: // 422: address is on your suppression list
	case apiErr.Code == avelto.CodeValidationError: // 400: apiErr.Details lists the failing fields
	case apiErr.IsNetworkError(): // status 0: no response (DNS, TLS, timeout, cancelled context)
	default:
		fmt.Println(apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Details)
	}
}
```

| Code | Status | When |
| --- | --- | --- |
| `validation_error` | 400 | Request body or query failed validation. `Details` lists the issues. |
| `unauthorized` | 401 | Missing or invalid API key. |
| `forbidden` | 403 | Key is not allowed to do this. |
| `live_key_required` | 403 | A test key on a call that needs a live one, such as creating a webhook endpoint or changing a template. |
| `not_found` | 404 | No such email, domain, webhook, template or suppression. |
| `conflict` | 409 | Resource already exists (for example the domain). |
| `domain_unavailable` | 409 | The domain cannot be added: it is part of a domain on another account, is reserved, or is already set up elsewhere. |
| `not_scheduled` | 409 | `Emails.Cancel` on an email that is not scheduled. |
| `sandbox_recipient_not_allowed` | 403 | Sending from the sandbox domain to anyone but your account's owner email. |
| `domain_not_verified` | 403 | `From` uses a domain that is not added and verified on your account. |
| `recipient_suppressed` | 422 | A recipient is on your suppression list. |
| `plan_limit` | 429 / 403 | Monthly or first-week sending cap reached (429), or the plan does not allow another domain (403). |
| `account_paused` | 403 | Sending is paused on the account. Check the dashboard. |
| `account_suspended` | 403 | The account is suspended; the message says why. Nothing sends until it is lifted. |
| `rate_limited` | 429 | Too many requests for this key; honour `RetryAfter`. |
| `internal_error` | 5xx | Something went wrong on our side. |
| `network_error` | 0 | The request never got a response. `errors.Unwrap(err)` returns the underlying error. |

Each code has a constant, such as `avelto.CodeNotScheduled`.

## Test mode

Keys starting with `av_test_` never send real email. They run the full pipeline and emit the same events and webhooks, so you can build against them without touching an inbox.

Use these recipients on your sandbox domain to simulate outcomes, in both live and test mode:

- `delivered@<your sandbox domain>` - delivered
- `bounced@<your sandbox domain>` - hard bounce (never added to suppressions)
- `complained@<your sandbox domain>` - complaint (never added to suppressions)

Your sandbox domain is shown in the dashboard and in `Account.Get`. Sending from it to any other address only works for your account's owner email; verify a domain to send to anyone.

## License

MIT
