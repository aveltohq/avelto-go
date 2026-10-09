package avelto

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func sendReq() *SendEmailRequest {
	return &SendEmailRequest{From: "a@b.com", To: []string{"c@d.com"}, Subject: "Hi", Text: "Hello"}
}

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (%q)", err, b)
	}
	return m
}

func TestNewClientRequiresAnAPIKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if _, err := NewClient(key); err == nil {
			t.Errorf("NewClient(%q): want an error", key)
		}
	}
}

func TestNewClientRejectsBadOptions(t *testing.T) {
	cases := map[string]Option{
		"zero attempts":  WithRetry(0, DefaultBaseDelay, DefaultMaxDelay),
		"negative delay": WithRetry(3, -time.Second, DefaultMaxDelay),
		"negative limit": WithTimeout(-time.Second),
	}
	for name, opt := range cases {
		if _, err := NewClient("k", opt); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestSendsBearerAuthUserAgentAndJSON(t *testing.T) {
	f := newFakeAPI(t, jsonStep(201, `{"id":"e1"}`))
	res, err := f.client().Emails.Send(context.Background(), sendReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "e1" {
		t.Errorf("ID = %q", res.ID)
	}
	c := f.last()
	want := map[string]string{
		"Authorization": "Bearer av_test_abc",
		"User-Agent":    "avelto-go/" + Version,
		"Accept":        "application/json",
		"Content-Type":  "application/json",
	}
	for k, v := range want {
		if got := c.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if c.Method != http.MethodPost || c.URL != "/v1/emails" {
		t.Errorf("got %s %s", c.Method, c.URL)
	}
	body := decodeBody(t, c.Body)
	wantBody := map[string]any{"from": "a@b.com", "to": []any{"c@d.com"}, "subject": "Hi", "text": "Hello"}
	if !reflect.DeepEqual(body, wantBody) {
		t.Errorf("body = %v, want %v", body, wantBody)
	}
}

func TestGetHasNoBodyOrContentType(t *testing.T) {
	f := newFakeAPI(t, jsonStep(200, `{"id":"e1"}`))
	if _, err := f.client().Emails.Get(context.Background(), "e1"); err != nil {
		t.Fatal(err)
	}
	c := f.last()
	if len(c.Body) != 0 || c.Header.Get("Content-Type") != "" {
		t.Errorf("GET sent a body (%q) or content-type (%q)", c.Body, c.Header.Get("Content-Type"))
	}
}

func TestSendCarriesEveryOptionalField(t *testing.T) {
	f := newFakeAPI(t, jsonStep(201, `{"id":"e1"}`))
	at := time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)
	req := &SendEmailRequest{
		From:           "Acme <hello@mail.acme.com>",
		To:             []string{"a@example.com", "b@example.com"},
		Cc:             []string{"c@example.com"},
		Bcc:            []string{"d@example.com"},
		ReplyTo:        "support@acme.com",
		Subject:        "Hi",
		HTML:           "<p>Hi & welcome</p>",
		Text:           "Hi",
		Headers:        map[string]string{"X-Entity-Ref-ID": "42"},
		Tags:           []string{"welcome"},
		Attachments:    []Attachment{{Filename: "a.txt", Content: "aGk="}, {Filename: "b.pdf", URL: "https://example.com/b.pdf", ContentType: "application/pdf"}},
		Variables:      map[string]any{"name": "Jane"},
		ScheduledAt:    &at,
		UnsubscribeURL: "https://acme.com/unsub",
	}
	if _, err := f.client().Emails.Send(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	raw := string(f.last().Body)
	if !strings.Contains(raw, `"html":"<p>Hi & welcome</p>"`) {
		t.Errorf("HTML was escaped: %s", raw)
	}
	body := decodeBody(t, f.last().Body)
	want := map[string]any{
		"from":            "Acme <hello@mail.acme.com>",
		"to":              []any{"a@example.com", "b@example.com"},
		"cc":              []any{"c@example.com"},
		"bcc":             []any{"d@example.com"},
		"reply_to":        "support@acme.com",
		"subject":         "Hi",
		"html":            "<p>Hi & welcome</p>",
		"text":            "Hi",
		"headers":         map[string]any{"X-Entity-Ref-ID": "42"},
		"tags":            []any{"welcome"},
		"attachments":     []any{map[string]any{"filename": "a.txt", "content": "aGk="}, map[string]any{"filename": "b.pdf", "url": "https://example.com/b.pdf", "content_type": "application/pdf"}},
		"variables":       map[string]any{"name": "Jane"},
		"scheduled_at":    "2026-10-01T09:30:00Z",
		"unsubscribe_url": "https://acme.com/unsub",
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body =\n%v\nwant\n%v", body, want)
	}
}

func TestSendsAnIdempotencyKey(t *testing.T) {
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	f := newFakeAPI(t, jsonStep(201, `{"id":"e1"}`), jsonStep(201, `{"id":"e2"}`), jsonStep(201, `{"id":"e3"}`), jsonStep(200, `{"results":[],"accepted":0,"failed":0}`), jsonStep(200, `{"results":[],"accepted":0,"failed":0}`))
	c := f.client()
	ctx := context.Background()

	if _, err := c.Emails.Send(ctx, sendReq(), WithIdempotencyKey("order-42")); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Header.Get("Idempotency-Key"); got != "order-42" {
		t.Errorf("given key: got %q", got)
	}

	if _, err := c.Emails.Send(ctx, sendReq()); err != nil {
		t.Fatal(err)
	}
	auto1 := f.last().Header.Get("Idempotency-Key")
	if !uuidRe.MatchString(auto1) {
		t.Errorf("fresh key %q is not a UUIDv4", auto1)
	}
	if _, err := c.Emails.Send(ctx, sendReq()); err != nil {
		t.Fatal(err)
	}
	if auto2 := f.last().Header.Get("Idempotency-Key"); auto2 == auto1 || !uuidRe.MatchString(auto2) {
		t.Errorf("second fresh key %q (first %q)", auto2, auto1)
	}

	if _, err := c.Emails.SendBatch(ctx, &BatchSendRequest{Messages: []SendEmailRequest{*sendReq()}}, WithIdempotencyKey("batch-1")); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Header.Get("Idempotency-Key"); got != "batch-1" {
		t.Errorf("batch given key: got %q", got)
	}
	if _, err := c.Emails.SendBatch(ctx, &BatchSendRequest{Messages: []SendEmailRequest{*sendReq()}}); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Header.Get("Idempotency-Key"); !uuidRe.MatchString(got) {
		t.Errorf("batch fresh key %q is not a UUIDv4", got)
	}
}

func TestQueryString(t *testing.T) {
	cases := []struct {
		name string
		call func(c *Client) error
		want string
	}{
		{"emails with filters, unset skipped", func(c *Client) error {
			_, err := c.Emails.List(context.Background(), &ListEmailsParams{Limit: 5, Status: EmailDelivered, Tag: "welcome:v2"})
			return err
		}, "/v1/emails?status=delivered&tag=welcome%3Av2&limit=5"},
		{"emails every filter", func(c *Client) error {
			_, err := c.Emails.List(context.Background(), &ListEmailsParams{Status: EmailBounced, Tag: "t", Mode: ModeTest, Q: "jane doe", Limit: 100, Cursor: "abc"})
			return err
		}, "/v1/emails?status=bounced&tag=t&mode=test&q=jane+doe&limit=100&cursor=abc"},
		{"emails nil params", func(c *Client) error {
			_, err := c.Emails.List(context.Background(), nil)
			return err
		}, "/v1/emails"},
		{"emails empty params", func(c *Client) error {
			_, err := c.Emails.List(context.Background(), &ListEmailsParams{})
			return err
		}, "/v1/emails"},
		{"suppressions no query", func(c *Client) error {
			_, err := c.Suppressions.List(context.Background(), nil)
			return err
		}, "/v1/suppressions"},
		{"suppressions paginated", func(c *Client) error {
			_, err := c.Suppressions.List(context.Background(), &ListSuppressionsParams{Limit: 100, Cursor: "c1"})
			return err
		}, "/v1/suppressions?limit=100&cursor=c1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, jsonStep(200, `{"data":[],"next_cursor":null}`))
			if err := tc.call(f.client()); err != nil {
				t.Fatal(err)
			}
			c := f.last()
			if c.URL != tc.want {
				t.Errorf("URL = %s, want %s", c.URL, tc.want)
			}
			if c.Method != http.MethodGet || len(c.Body) != 0 {
				t.Errorf("method %s, body %q", c.Method, c.Body)
			}
		})
	}
}

func TestReturnsNilOn204(t *testing.T) {
	f := newFakeAPI(t, step{status: 204})
	if err := f.client().Domains.Delete(context.Background(), "d1"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if c := f.last(); c.Method != http.MethodDelete || c.URL != "/v1/domains/d1" {
		t.Errorf("got %s %s", c.Method, c.URL)
	}
}

func TestMapsErrorEnvelope(t *testing.T) {
	f := newFakeAPI(t, jsonStep(403, `{"error":{"code":"domain_not_verified","message":"Domain example.com is not verified","details":{"domain":"example.com"}}}`, "x-request-id", "req_123"))
	_, err := f.client().Emails.Send(context.Background(), &SendEmailRequest{From: "a@example.com", To: []string{"c@d.com"}, Subject: "x", Text: "y"})
	e := asError(t, err)
	if e.Status != 403 || e.Code != CodeDomainNotVerified || e.Message != "Domain example.com is not verified" || e.RequestID != "req_123" {
		t.Errorf("got %+v", e)
	}
	if !reflect.DeepEqual(e.Details, map[string]any{"domain": "example.com"}) {
		t.Errorf("Details = %#v", e.Details)
	}
	if e.IsNetworkError() {
		t.Error("IsNetworkError() = true")
	}
	if e.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v", e.RetryAfter)
	}
	if errors.Unwrap(err) != nil {
		t.Errorf("an API error wraps nothing, got %v", errors.Unwrap(err))
	}
	if !strings.Contains(e.Error(), "Domain example.com is not verified") || !strings.Contains(e.Error(), "domain_not_verified") {
		t.Errorf("Error() = %q", e.Error())
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("403 was retried: %d calls", n)
	}
}

func TestMapsNonJSONErrorsByStatus(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
	}{
		{400, "bad", CodeValidationError},
		{401, "nope", CodeUnauthorized},
		{403, "no", CodeForbidden},
		{404, "gone", CodeNotFound},
		{409, "dup", CodeConflict},
		{429, "slow", CodeRateLimited},
		{500, "boom", CodeInternalError},
		{502, "Bad Gateway", CodeInternalError},
		{422, `{"message":"not an envelope"}`, CodeInternalError},
	}
	for _, tc := range cases {
		f := newFakeAPI(t, textStep(tc.status, tc.body))
		_, err := f.client(WithRetry(1, 0, 0)).Emails.Get(context.Background(), "e1")
		e := asError(t, err)
		if e.Status != tc.status || e.Code != tc.code {
			t.Errorf("%d: got status %d code %q, want %q", tc.status, e.Status, e.Code, tc.code)
		}
		if e.Details != tc.body {
			t.Errorf("%d: Details = %#v, want the raw text", tc.status, e.Details)
		}
		if want := "Request to GET /v1/emails/e1 failed with HTTP"; !strings.HasPrefix(e.Message, want) {
			t.Errorf("%d: Message = %q", tc.status, e.Message)
		}
	}
}

func TestEmptyErrorBodyHasNilDetails(t *testing.T) {
	f := newFakeAPI(t, textStep(404, ""))
	e := asError(t, f.client().Domains.Delete(context.Background(), "d1"))
	if e.Code != CodeNotFound || e.Details != nil {
		t.Errorf("got %+v", e)
	}
}

func TestWrapsNetworkFailures(t *testing.T) {
	f := newFakeAPI(t, netErrStep("fetch failed"))
	_, err := f.client(WithRetry(1, 0, 0)).Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.Status != 0 || e.Code != CodeNetworkError || !e.IsNetworkError() {
		t.Errorf("got %+v", e)
	}
	if !strings.Contains(e.Message, "fetch failed") || !strings.HasPrefix(e.Message, "Request to GET /v1/emails/e1 failed") {
		t.Errorf("Message = %q", e.Message)
	}
	if errors.Unwrap(err) == nil || !strings.Contains(errors.Unwrap(err).Error(), "fetch failed") {
		t.Errorf("Unwrap = %v", errors.Unwrap(err))
	}
}

func TestReportsATimeout(t *testing.T) {
	f := newFakeAPI(t, step{hang: true})
	start := time.Now()
	_, err := f.client(WithTimeout(10*time.Millisecond), WithRetry(1, 0, 0)).Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.Status != 0 || e.Code != CodeNetworkError {
		t.Errorf("got %+v", e)
	}
	if !strings.Contains(e.Message, "timed out after 10ms") {
		t.Errorf("Message = %q", e.Message)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; cause %v", errors.Unwrap(err))
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("timeout took %v", time.Since(start))
	}
}

func TestCallerCancellationIsANetworkError(t *testing.T) {
	f := newFakeAPI(t, step{hang: true})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	_, err := f.client().Emails.Get(ctx, "e1")
	e := asError(t, err)
	if e.Code != CodeNetworkError || !errors.Is(err, context.Canceled) {
		t.Errorf("got %+v (cause %v)", e, errors.Unwrap(err))
	}
	if strings.Contains(e.Message, "timed out") {
		t.Errorf("a cancellation is not a timeout: %q", e.Message)
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("a cancelled request was retried: %d calls", n)
	}
}

func TestUndecodableSuccessIsAnError(t *testing.T) {
	f := newFakeAPI(t, textStep(200, "<html>proxy page</html>"))
	_, err := f.client().Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.Status != 200 || e.Code != CodeInternalError || e.Details != "<html>proxy page</html>" {
		t.Errorf("got %+v", e)
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("retried: %d calls", n)
	}
}

func TestPathEncoding(t *testing.T) {
	cases := []struct {
		name string
		call func(c *Client) error
		want string
	}{
		{"suppression address", func(c *Client) error {
			return c.Suppressions.Delete(context.Background(), "first+tag@example.com")
		}, "/v1/suppressions/first%2Btag%40example.com"},
		{"ids with slash and space", func(c *Client) error {
			_, err := c.Webhooks.RetryDelivery(context.Background(), "wh/1", "del 2")
			return err
		}, "/v1/webhooks/wh%2F1/deliveries/del%202/retry"},
		{"reserved characters", func(c *Client) error {
			_, err := c.Emails.Get(context.Background(), "a?b#c&d=e:f;g,h$i")
			return err
		}, "/v1/emails/a%3Fb%23c%26d%3De%3Af%3Bg%2Ch%24i"},
		{"unreserved kept as encodeURIComponent keeps them", func(c *Client) error {
			_, err := c.Emails.Get(context.Background(), "A-z_0.9!~*'()")
			return err
		}, "/v1/emails/A-z_0.9!~*'()"},
		{"non-ASCII as UTF-8", func(c *Client) error {
			_, err := c.Templates.Get(context.Background(), "é")
			return err
		}, "/v1/templates/%C3%A9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, jsonStep(200, `{"id":"x"}`))
			if err := tc.call(f.client()); err != nil {
				t.Fatal(err)
			}
			if got := f.last().URL; got != tc.want {
				t.Errorf("URL = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBaseURLResolution(t *testing.T) {
	cases := []struct {
		name string
		env  string
		opt  string
		want string
	}{
		{"default", "", "", DefaultBaseURL},
		{"env overrides the default", "https://env.example.test/", "", "https://env.example.test"},
		{"option overrides env", "https://env.example.test", "https://opt.example.test", "https://opt.example.test"},
		{"trailing slashes trimmed", "", "https://opt.example.test///", "https://opt.example.test"},
		{"blank env ignored", "   ", "", DefaultBaseURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AVELTO_BASE_URL", tc.env)
			var opts []Option
			if tc.opt != "" {
				opts = append(opts, WithBaseURL(tc.opt))
			}
			c, err := NewClient("av_test_abc", opts...)
			if err != nil {
				t.Fatal(err)
			}
			if c.BaseURL() != tc.want {
				t.Errorf("BaseURL() = %s, want %s", c.BaseURL(), tc.want)
			}
		})
	}
}

func TestBaseURLIsUsedForRequests(t *testing.T) {
	var got string
	c, err := NewClient("av_test_abc", WithBaseURL("https://opt.example.test/"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.URL.String()
		return nil, errors.New("stop")
	})}), WithRetry(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Domains.List(context.Background())
	if got != "https://opt.example.test/v1/domains" {
		t.Errorf("URL = %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDecodesResponses(t *testing.T) {
	email := `{"id":"e1","mode":"test","from":"a@b.com","to":["c@d.com"],"cc":[],"bcc":[],"reply_to":null,"subject":"Hi","tags":["t"],"status":"delivered","scheduled_at":null,"created_at":"2026-09-01T10:00:00.000Z","updated_at":"2026-09-01T10:00:02.000Z",
	"html":null,"text":"Hello","headers":{},"unsubscribe_url":null,"idempotency_key":"k1","attachments":[{"filename":"a.txt","size":2}],"domain_id":null,"ses_message_id":"m1","error":null,
	"events":[{"id":"ev1","type":"email.delivered","payload":{"x":1},"occurred_at":"2026-09-01T10:00:02.000Z"}]}`
	batch := `{"results":[{"ok":true,"index":0,"id":"e1"},{"ok":false,"index":1,"error":{"code":"recipient_suppressed","message":"suppressed","details":{"to":"x"}}}],"accepted":1,"failed":1}`
	domain := `{"id":"d1","name":"mail.acme.com","region":"eu-west-1","status":"pending","dns_records":[{"type":"TXT","name":"_dmarc","value":"v=DMARC1","purpose":"dmarc"}],"health":"unknown","health_changed_at":null,"dns_checks":null,"dns_checked_at":null,"created_at":"2026-09-01T10:00:00Z"}`
	f := newFakeAPI(t, jsonStep(200, email), jsonStep(200, batch), jsonStep(200, domain))
	c := f.client()
	ctx := context.Background()

	e, err := c.Emails.Get(ctx, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "e1" || e.Status != EmailDelivered || e.ReplyTo != nil || *e.Text != "Hello" || e.HTML != nil || *e.IdempotencyKey != "k1" ||
		len(e.Events) != 1 || e.Events[0].Type != EventDelivered || e.Attachments[0].Size != 2 || e.CreatedAt.Year() != 2026 {
		t.Errorf("email decoded as %+v", e)
	}

	b, err := c.Emails.SendBatch(ctx, &BatchSendRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !b.Results[0].OK || b.Results[0].ID != "e1" || b.Results[1].OK || b.Results[1].Error.Code != CodeRecipientSuppressed || b.Accepted != 1 || b.Failed != 1 {
		t.Errorf("batch decoded as %+v", b)
	}

	d, err := c.Domains.Get(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if d.DNSChecks != nil || d.Health != HealthUnknown || d.DNSRecords[0].Purpose != "dmarc" {
		t.Errorf("domain decoded as %+v", d)
	}
}

func TestPathSegmentMatchesEncodeURIComponent(t *testing.T) {
	// Every printable ASCII character, against what encodeURIComponent returns.
	var in strings.Builder
	for ch := 0x20; ch < 0x7f; ch++ {
		in.WriteByte(byte(ch))
	}
	want := "%20!%22%23%24%25%26'()*%2B%2C-.%2F0123456789%3A%3B%3C%3D%3E%3F%40ABCDEFGHIJKLMNOPQRSTUVWXYZ%5B%5C%5D%5E_%60abcdefghijklmnopqrstuvwxyz%7B%7C%7D~"
	if got := pathSegment(in.String()); got != want {
		t.Errorf("pathSegment =\n%s\nwant\n%s", got, want)
	}
	if got, _ := url.PathUnescape(pathSegment("first+tag@example.com")); got != "first+tag@example.com" {
		t.Errorf("round trip = %q", got)
	}
}
