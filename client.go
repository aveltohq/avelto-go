package avelto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the API origin used when neither [WithBaseURL] nor the
// AVELTO_BASE_URL environment variable is set.
const DefaultBaseURL = "https://api.avelto.dev"

// Default settings. Retries: three attempts in all, 300 ms then 600 ms,
// capped at 5 s per wait.
const (
	DefaultTimeout     = 30 * time.Second
	DefaultMaxAttempts = 3
	DefaultBaseDelay   = 300 * time.Millisecond
	DefaultMaxDelay    = 5 * time.Second
)

// retryAfterCeiling is the least a Retry-After wait is capped at, whatever
// the retry's max delay.
const retryAfterCeiling = 30 * time.Second

// Client is an Avelto API client. It is safe for concurrent use by multiple
// goroutines. Create one with [NewClient] and reuse it.
type Client struct {
	// Emails sends, schedules, fetches, lists, cancels and validates emails.
	Emails *EmailsService
	// Domains adds, lists, verifies and removes sending domains.
	Domains *DomainsService
	// Webhooks manages webhook endpoints and their deliveries.
	Webhooks *WebhooksService
	// Templates manages stored templates and their version history.
	Templates *TemplatesService
	// Suppressions manages the account's suppression list.
	Suppressions *SuppressionsService
	// Account reads what the key can do and what the account has set up, and
	// changes the account name and alert settings.
	Account *AccountService
	// APIKeys lists, creates and revokes keys, from a key with keys:manage.
	APIKeys *APIKeysService

	apiKey      string
	baseURL     string
	httpClient  *http.Client
	timeout     time.Duration
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration

	// Seams for tests: waiting between attempts, jitter, the clock and key generation.
	sleep  func(ctx context.Context, d time.Duration) error
	random func() float64
	now    func() time.Time
	newKey func() string
}

// Option configures a [Client]. Pass options to [NewClient].
type Option func(*Client)

// WithBaseURL sets the API origin. It wins over the AVELTO_BASE_URL
// environment variable. Trailing slashes are trimmed. An empty string leaves
// the default in place.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if s := strings.TrimSpace(baseURL); s != "" {
			c.baseURL = s
		}
	}
}

// WithHTTPClient sets the [http.Client] requests are sent with, for a custom
// transport, proxy or instrumentation. Nil keeps the default. The client is
// used through a copy that never follows redirects (see [NewClient]); the
// client passed in is not changed.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithTimeout sets the limit on each attempt of a request. Default 30 s.
// Zero turns the SDK's own limit off, leaving only the context's deadline.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) { c.timeout = timeout }
}

// WithRetry configures automatic retries.
//
// maxAttempts is the total number of attempts including the first (default 3;
// pass 1 to turn retries off). Waits between attempts grow exponentially from
// baseDelay (default 300 ms), capped at maxDelay (default 5 s), with jitter of
// up to half the wait. A 429 with a Retry-After header waits that long instead,
// capped at the larger of maxDelay and 30 s.
//
// 429, 502 and 503 mean the server did not process the request, so they are
// retried for every request. A network error, a timeout or a 504 leaves it
// unknown whether the request was processed, so those are retried only when
// repeating the request is harmless: GET, DELETE, a request carrying an
// Idempotency-Key (every send), and Emails.Validate. Other POSTs and every
// PATCH are not; a template update saves a version per call, so a repeat
// would leave a duplicate in the history.
func WithRetry(maxAttempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		c.maxAttempts = maxAttempts
		c.baseDelay = baseDelay
		c.maxDelay = maxDelay
	}
}

// NewClient returns a client authenticated with apiKey (av_live_… or av_test_…).
//
// The API origin is the [WithBaseURL] option, then the AVELTO_BASE_URL
// environment variable, then [DefaultBaseURL]. It returns an error when the key
// is empty or an option is out of range.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, errors.New("avelto: an API key is required (av_live_… or av_test_…)")
	}
	c := &Client{
		apiKey:      key,
		httpClient:  &http.Client{},
		timeout:     DefaultTimeout,
		maxAttempts: DefaultMaxAttempts,
		baseDelay:   DefaultBaseDelay,
		maxDelay:    DefaultMaxDelay,
		sleep:       sleepContext,
		random:      mathrand.Float64,
		now:         time.Now,
		newKey:      newUUID,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.baseURL == "" {
		c.baseURL = strings.TrimSpace(os.Getenv("AVELTO_BASE_URL"))
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	// Never follow a redirect. net/http re-sends the Authorization header to the
	// same host or a subdomain, even when the redirect goes from https to http, so
	// following one could put the API key on the wire in clear text. The API does
	// not redirect; a 3xx comes back as an *Error. A copy, so a client the caller
	// shares with other code keeps its own redirect policy.
	noRedirect := *c.httpClient
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.httpClient = &noRedirect
	if c.maxAttempts < 1 {
		return nil, errors.New("avelto: retry maxAttempts must be at least 1")
	}
	if c.baseDelay < 0 || c.maxDelay < 0 {
		return nil, errors.New("avelto: retry delays must not be negative")
	}
	if c.timeout < 0 {
		return nil, errors.New("avelto: timeout must not be negative")
	}

	c.Emails = &EmailsService{c: c}
	c.Domains = &DomainsService{c: c}
	c.Webhooks = &WebhooksService{c: c}
	c.Templates = &TemplatesService{c: c}
	c.Suppressions = &SuppressionsService{c: c}
	c.Account = &AccountService{c: c}
	c.APIKeys = &APIKeysService{c: c}
	return c, nil
}

// BaseURL returns the API origin this client sends requests to.
func (c *Client) BaseURL() string { return c.baseURL }

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

// queryParam is one query string pair. Pairs keep their order; empty values
// are skipped.
type queryParam struct{ key, value string }

type request struct {
	method  string
	path    string
	query   []queryParam
	body    any
	headers map[string]string
	// readOnly marks a POST that changes nothing on the server, so a retry
	// after a timeout is harmless.
	readOnly bool
}

// safeToRepeat reports whether the request may be sent again when it is
// unknown whether the last attempt reached the server. GET and DELETE are safe
// by nature. POST is safe when it carries an idempotency key, which every send
// does, or when it is a question rather than a change (Emails.Validate). PATCH
// is excluded even though HTTP calls it idempotent: ours saves a template
// version per call, so a retried PATCH would leave a duplicate entry in a
// history the customer reads.
func (r *request) safeToRepeat() bool {
	if r.method == http.MethodGet || r.method == http.MethodDelete || r.readOnly {
		return true
	}
	for k, v := range r.headers {
		if strings.EqualFold(k, "Idempotency-Key") && v != "" {
			return true
		}
	}
	return false
}

// shouldRetry decides whether a failed attempt may be repeated.
//   - 429, 502 and 503 mean the server did not process the request: always retry.
//   - Network errors, timeouts and 504 leave it unknown whether the request was
//     processed: retry only when repeating it is harmless.
func shouldRetry(err *Error, safeToRepeat bool) bool {
	switch err.Status {
	case 429, 502, 503:
		return true
	case 0, 504:
		return safeToRepeat
	}
	return false
}

// retryDelay is exponential backoff with jitter; a Retry-After on a 429 wins
// when the server sent one.
func (c *Client) retryDelay(attempt int, err *Error) time.Duration {
	if err.hasRetryAfter {
		ceiling := c.maxDelay
		if ceiling < retryAfterCeiling {
			ceiling = retryAfterCeiling
		}
		if err.RetryAfter > ceiling {
			return ceiling
		}
		return err.RetryAfter
	}
	exp := math.Min(float64(c.maxDelay), float64(c.baseDelay)*math.Pow(2, float64(attempt-1)))
	return time.Duration(exp * (0.5 + c.random()*0.5))
}

// do sends req, retrying as the policy allows, and decodes a successful JSON
// response into out (which may be nil).
func (c *Client) do(ctx context.Context, req *request, out any) error {
	var body []byte
	if req.body != nil {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(req.body); err != nil {
			return fmt.Errorf("avelto: encoding the request body: %w", err)
		}
		body = bytes.TrimRight(buf.Bytes(), "\n")
	}
	safe := req.safeToRepeat()
	for attempt := 1; ; attempt++ {
		err := c.attempt(ctx, req, body, out)
		if err == nil {
			return nil
		}
		var apiErr *Error
		if !errors.As(err, &apiErr) || attempt >= c.maxAttempts || !shouldRetry(apiErr, safe) || ctx.Err() != nil {
			return err
		}
		if serr := c.sleep(ctx, c.retryDelay(attempt, apiErr)); serr != nil {
			return &Error{
				Code:    CodeNetworkError,
				Message: fmt.Sprintf("Request to %s %s cancelled while waiting to retry (last error: %s)", req.method, req.path, apiErr.Message),
				cause:   serr,
			}
		}
	}
}

func (c *Client) buildURL(path string, query []queryParam) string {
	u := c.baseURL + path
	sep := "?"
	for _, p := range query {
		if p.value == "" {
			continue
		}
		u += sep + url.QueryEscape(p.key) + "=" + url.QueryEscape(p.value)
		sep = "&"
	}
	return u
}

func (c *Client) attempt(ctx context.Context, req *request, body []byte, out any) error {
	actx, cancel := ctx, context.CancelFunc(func() {})
	if c.timeout > 0 {
		actx, cancel = context.WithTimeout(ctx, c.timeout)
	}
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(actx, req.method, c.buildURL(req.path, req.query), reader)
	if err != nil {
		return fmt.Errorf("avelto: building the request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", UserAgent)
	for k, v := range req.headers {
		httpReq.Header.Set(k, v)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	res, err := c.httpClient.Do(httpReq)
	if err == nil {
		defer res.Body.Close()
		var text []byte
		text, err = io.ReadAll(res.Body)
		if err == nil {
			return c.handleResponse(req, res, text, out)
		}
	}
	return c.networkError(ctx, actx, req, err)
}

func (c *Client) networkError(ctx, actx context.Context, req *request, cause error) *Error {
	var netErr interface{ Timeout() bool }
	timedOut := ctx.Err() == nil && (errors.Is(actx.Err(), context.DeadlineExceeded) || (errors.As(cause, &netErr) && netErr.Timeout()))
	var msg string
	switch {
	case timedOut && c.timeout > 0 && actx.Err() != nil:
		msg = fmt.Sprintf("Request to %s %s timed out after %s", req.method, req.path, c.timeout)
	case timedOut:
		msg = fmt.Sprintf("Request to %s %s timed out: %v", req.method, req.path, cause)
	default:
		msg = fmt.Sprintf("Request to %s %s failed: %v", req.method, req.path, cause)
	}
	return &Error{Code: CodeNetworkError, Message: msg, cause: cause}
}

func (c *Client) handleResponse(req *request, res *http.Response, text []byte, out any) error {
	requestID := res.Header.Get("x-request-id")
	var retryAfter time.Duration
	var hasRetryAfter bool
	if res.StatusCode == http.StatusTooManyRequests {
		retryAfter, hasRetryAfter = parseRetryAfter(res.Header.Get("retry-after"), c.now())
	}

	if res.StatusCode == http.StatusNoContent {
		return nil
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		if out == nil || len(bytes.TrimSpace(text)) == 0 {
			return nil
		}
		if err := json.Unmarshal(text, out); err != nil {
			return &Error{
				Status:    res.StatusCode,
				Code:      CodeInternalError,
				Message:   fmt.Sprintf("Request to %s %s returned a response that could not be decoded: %v", req.method, req.path, err),
				Details:   string(text),
				RequestID: requestID,
				cause:     err,
			}
		}
		return nil
	}

	apiErr := &Error{Status: res.StatusCode, RequestID: requestID, RetryAfter: retryAfter, hasRetryAfter: hasRetryAfter}
	var envelope struct {
		Error map[string]any `json:"error"`
	}
	if len(text) > 0 && json.Unmarshal(text, &envelope) == nil && envelope.Error != nil {
		if code, ok := envelope.Error["code"].(string); ok {
			apiErr.Code = code
			switch m := envelope.Error["message"].(type) {
			case string:
				apiErr.Message = m
			case nil:
			default:
				apiErr.Message = fmt.Sprint(m)
			}
			apiErr.Details = envelope.Error["details"]
			return apiErr
		}
	}
	apiErr.Code = codeForStatus(res.StatusCode)
	apiErr.Message = fmt.Sprintf("Request to %s %s failed with HTTP %d", req.method, req.path, res.StatusCode)
	if len(text) > 0 {
		apiErr.Details = string(text)
	}
	return apiErr
}

// parseRetryAfter reads a Retry-After header: a number of seconds, or an HTTP
// date. The second result is false when the header is missing or unreadable.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(v, 64); err == nil {
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds * float64(time.Second)), true
	}
	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := at.Sub(now)
	if d < 0 {
		d = 0
	}
	return d, true
}

// sleepContext waits for d, or until ctx is done.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// newUUID returns a random (version 4) UUID from crypto/rand.
func newUUID() string {
	var b [16]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		// crypto/rand does not fail on supported platforms; this keeps the key
		// unique enough for idempotency if it ever does.
		for i := range b {
			b[i] = byte(mathrand.Intn(256))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var s [36]byte
	hex.Encode(s[0:8], b[0:4])
	s[8] = '-'
	hex.Encode(s[9:13], b[4:6])
	s[13] = '-'
	hex.Encode(s[14:18], b[6:8])
	s[18] = '-'
	hex.Encode(s[19:23], b[8:10])
	s[23] = '-'
	hex.Encode(s[24:36], b[10:16])
	return string(s[:])
}

// pathSegment URL-encodes one path segment the way JavaScript's
// encodeURIComponent does: everything except A-Z a-z 0-9 - _ . ! ~ * ' ( )
// is percent-encoded, so "/", "+" and "@" are too. url.PathEscape leaves "+"
// and "@" alone, which would let an address like a+b@example.com be read
// back as "a b@example.com".
func pathSegment(s string) string {
	const upperhex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || ('0' <= ch && ch <= '9') ||
			strings.IndexByte("-_.!~*'()", ch) >= 0 {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperhex[ch>>4])
		b.WriteByte(upperhex[ch&15])
	}
	return b.String()
}
