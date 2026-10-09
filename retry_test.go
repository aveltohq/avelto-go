package avelto

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestRetryDefaults(t *testing.T) {
	c, err := NewClient("k")
	if err != nil {
		t.Fatal(err)
	}
	if c.maxAttempts != 3 || c.baseDelay != 300*time.Millisecond || c.maxDelay != 5*time.Second || c.timeout != 30*time.Second {
		t.Errorf("defaults: attempts %d base %v max %v timeout %v", c.maxAttempts, c.baseDelay, c.maxDelay, c.timeout)
	}
}

func TestRetries503502And429ThenSucceeds(t *testing.T) {
	f := newFakeAPI(t, textStep(503, "down"), textStep(502, "bad gateway"), jsonStep(200, `{"id":"e1"}`))
	e, err := f.client().Emails.Get(context.Background(), "e1")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "e1" || len(f.Calls()) != 3 {
		t.Errorf("id %q after %d calls", e.ID, len(f.Calls()))
	}

	f = newFakeAPI(t, jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`), jsonStep(200, `{"id":"e1"}`))
	if _, err := f.client().Emails.Get(context.Background(), "e1"); err != nil {
		t.Fatal(err)
	}
	if n := len(f.Calls()); n != 2 {
		t.Errorf("429: %d calls", n)
	}
}

func TestGivesUpAfterMaxAttemptsWithTheLastError(t *testing.T) {
	f := newFakeAPI(t, textStep(503, "a"), textStep(503, "b"), textStep(503, "c"), jsonStep(200, `{"id":"never"}`))
	_, err := f.client().Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.Status != 503 || e.Details != "c" {
		t.Errorf("got %+v", e)
	}
	if n := len(f.Calls()); n != 3 {
		t.Errorf("%d calls, want 3", n)
	}
}

func TestHonoursMaxAttempts(t *testing.T) {
	cases := []struct {
		attempts int
		steps    []step
		wantErr  bool
		calls    int
	}{
		{5, []step{textStep(503, "a"), textStep(503, "b"), textStep(503, "c"), textStep(503, "d"), jsonStep(200, `{"id":"e1"}`)}, false, 5},
		{1, []step{textStep(503, "a"), jsonStep(200, `{"id":"e1"}`)}, true, 1},
		{2, []step{textStep(503, "a"), textStep(503, "b"), jsonStep(200, `{"id":"e1"}`)}, true, 2},
	}
	for _, tc := range cases {
		f := newFakeAPI(t, tc.steps...)
		_, err := f.client(WithRetry(tc.attempts, 0, 0)).Emails.Get(context.Background(), "e1")
		if (err != nil) != tc.wantErr {
			t.Errorf("attempts %d: err = %v", tc.attempts, err)
		}
		if n := len(f.Calls()); n != tc.calls {
			t.Errorf("attempts %d: %d calls, want %d", tc.attempts, n, tc.calls)
		}
	}
}

func TestDoesNotRetry4xxOtherThan429(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422} {
		f := newFakeAPI(t, jsonStep(status, `{"error":{"code":"x","message":"no"}}`), jsonStep(200, `{"id":"e1"}`))
		_, err := f.client().Emails.Get(context.Background(), "e1")
		asError(t, err)
		if n := len(f.Calls()); n != 1 {
			t.Errorf("%d: %d calls, want 1", status, n)
		}
		f.mu.Lock()
		f.steps = nil // the unused success
		f.mu.Unlock()
	}
}

func TestRetriesASendAfterANetworkErrorWithTheSameKey(t *testing.T) {
	f := newFakeAPI(t, netErrStep("fetch failed"), netErrStep("socket hang up"), jsonStep(201, `{"id":"e1"}`))
	res, err := f.client().Emails.Send(context.Background(), sendReq())
	if err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if res.ID != "e1" || len(calls) != 3 {
		t.Fatalf("id %q after %d calls", res.ID, len(calls))
	}
	key := calls[0].Header.Get("Idempotency-Key")
	if key == "" {
		t.Fatal("no Idempotency-Key")
	}
	for i, c := range calls {
		if got := c.Header.Get("Idempotency-Key"); got != key {
			t.Errorf("attempt %d key %q, want %q", i+1, got, key)
		}
		if string(c.Body) != string(calls[0].Body) {
			t.Errorf("attempt %d body differs", i+1)
		}
	}
}

// Which requests are repeated after a failure that may have reached the server.
func TestRetryOnlyWhenSafeToRepeat(t *testing.T) {
	type op func(c *Client) error
	get := func(c *Client) error { _, err := c.Emails.Get(context.Background(), "e1"); return err }
	del := func(c *Client) error { return c.Domains.Delete(context.Background(), "d1") }
	send := func(c *Client) error { _, err := c.Emails.Send(context.Background(), sendReq()); return err }
	batch := func(c *Client) error {
		_, err := c.Emails.SendBatch(context.Background(), &BatchSendRequest{Messages: []SendEmailRequest{*sendReq()}})
		return err
	}
	validate := func(c *Client) error {
		_, err := c.Emails.Validate(context.Background(), "jane@example.com")
		return err
	}
	create := func(c *Client) error {
		_, err := c.Domains.Create(context.Background(), &CreateDomainRequest{Name: "mail.example.com"})
		return err
	}
	cancel := func(c *Client) error { _, err := c.Emails.Cancel(context.Background(), "e1"); return err }
	patch := func(c *Client) error {
		_, err := c.Templates.Update(context.Background(), "t1", &UpdateTemplateRequest{Subject: "x"})
		return err
	}

	createKey := func(c *Client) error {
		_, err := c.APIKeys.Create(context.Background(), &CreateAPIKeyRequest{Name: "x", Mode: ModeTest, Scopes: []APIKeyScope{ScopeEmailsSend}})
		return err
	}
	patchWebhook := func(c *Client) error {
		_, err := c.Webhooks.Update(context.Background(), "w1", &UpdateWebhookRequest{Enabled: Bool(true)})
		return err
	}

	ok := jsonStep(200, `{"id":"x"}`)
	cases := []struct {
		name    string
		do      op
		first   step
		success step
		retried bool
	}{
		{"GET after network error", get, netErrStep("fetch failed"), ok, true},
		{"GET after 504", get, textStep(504, "gw"), ok, true},
		{"DELETE after network error", del, netErrStep("fetch failed"), step{status: 204}, true},
		{"DELETE after 504", del, textStep(504, "gw"), step{status: 204}, true},
		{"send after 504", send, textStep(504, "gw"), ok, true},
		{"batch after network error", batch, netErrStep("fetch failed"), jsonStep(200, `{"results":[],"accepted":0,"failed":0}`), true},
		{"validate after network error", validate, netErrStep("fetch failed"), ok, true},
		{"validate after 504", validate, textStep(504, "gw"), ok, true},
		{"other POST after network error", create, netErrStep("fetch failed"), ok, false},
		{"other POST after 504", create, textStep(504, "gw"), ok, false},
		{"cancel after network error", cancel, netErrStep("fetch failed"), ok, false},
		{"PATCH after network error", patch, netErrStep("fetch failed"), ok, false},
		{"PATCH after 504", patch, textStep(504, "gw"), ok, false},
		{"key creation after network error", createKey, netErrStep("fetch failed"), ok, false},
		{"webhook PATCH after network error", patchWebhook, netErrStep("fetch failed"), ok, false},
		// 429/502/503 mean the request was not processed: retried for every method.
		{"other POST after 503", create, textStep(503, "down"), ok, true},
		{"other POST after 502", create, textStep(502, "bad"), ok, true},
		{"other POST after 429", create, jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`), ok, true},
		{"PATCH after 503", patch, textStep(503, "down"), ok, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, tc.first, tc.success)
			err := tc.do(f.client())
			calls := len(f.Calls())
			if tc.retried {
				if err != nil || calls != 2 {
					t.Errorf("want a retry and success; err %v, %d calls", err, calls)
				}
				return
			}
			e := asError(t, err)
			if calls != 1 {
				t.Errorf("want no retry; %d calls", calls)
			}
			if tc.first.netErr != nil && e.Code != CodeNetworkError {
				t.Errorf("code = %q", e.Code)
			}
			if tc.first.status != 0 && e.Status != tc.first.status {
				t.Errorf("status = %d", e.Status)
			}
		})
	}
}

func TestWaitsForRetryAfterOn429AndExposesIt(t *testing.T) {
	f := newFakeAPI(t,
		jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`, "Retry-After", "2"),
		jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`, "Retry-After", "1"),
		jsonStep(200, `{"id":"e1"}`),
	)
	c := f.client(WithRetry(2, 0, 0))
	waits := recordSleeps(c)
	_, err := c.Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.Status != 429 || e.Code != CodeRateLimited || e.RetryAfter != time.Second {
		t.Errorf("got %+v", e)
	}
	if !reflect.DeepEqual(*waits, []time.Duration{2 * time.Second}) {
		t.Errorf("waits = %v, want [2s]", *waits)
	}
	if n := len(f.Calls()); n != 2 {
		t.Errorf("%d calls", n)
	}
}

func TestRetryAfterForms(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		header    string
		maxDelay  time.Duration
		wantWait  time.Duration
		wantAfter time.Duration
	}{
		{"seconds", "3", 5 * time.Second, 3 * time.Second, 3 * time.Second},
		{"fractional seconds", "1.5", 5 * time.Second, 1500 * time.Millisecond, 1500 * time.Millisecond},
		{"zero", "0", 5 * time.Second, 0, 0},
		{"HTTP date", now.Add(4 * time.Second).Format(http.TimeFormat), 5 * time.Second, 4 * time.Second, 4 * time.Second},
		{"HTTP date in the past", now.Add(-time.Minute).Format(http.TimeFormat), 5 * time.Second, 0, 0},
		{"capped at 30s", "120", 5 * time.Second, 30 * time.Second, 120 * time.Second},
		{"capped at max delay when larger", "120", time.Minute, time.Minute, 120 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`, "Retry-After", tc.header), jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`, "Retry-After", tc.header))
			c := f.client(WithRetry(2, time.Hour, tc.maxDelay))
			c.now = func() time.Time { return now }
			waits := recordSleeps(c)
			_, err := c.Emails.Get(context.Background(), "e1")
			e := asError(t, err)
			if e.RetryAfter != tc.wantAfter {
				t.Errorf("RetryAfter = %v, want %v", e.RetryAfter, tc.wantAfter)
			}
			if !reflect.DeepEqual(*waits, []time.Duration{tc.wantWait}) {
				t.Errorf("waits = %v, want [%v]", *waits, tc.wantWait)
			}
		})
	}
}

func TestRetryAfterIgnoredWhenUnreadableOrNot429(t *testing.T) {
	f := newFakeAPI(t, jsonStep(429, `{"error":{"code":"rate_limited","message":"slow"}}`, "Retry-After", "soon"), jsonStep(503, `{"error":{"code":"internal_error","message":"down"}}`, "Retry-After", "9"), jsonStep(503, "", "Retry-After", "9"))
	c := f.client(WithRetry(3, 100*time.Millisecond, time.Second))
	c.random = func() float64 { return 1 }
	waits := recordSleeps(c)
	_, err := c.Emails.Get(context.Background(), "e1")
	e := asError(t, err)
	if e.RetryAfter != 0 {
		t.Errorf("RetryAfter on a 503 = %v", e.RetryAfter)
	}
	if want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}; !reflect.DeepEqual(*waits, want) {
		t.Errorf("waits = %v, want backoff %v", *waits, want)
	}
}

func TestBackoff(t *testing.T) {
	cases := []struct {
		name     string
		random   float64
		attempts int
		base     time.Duration
		max      time.Duration
		want     []time.Duration
	}{
		{"exponential, jitter pinned to its maximum", 1, 3, 100 * time.Millisecond, 10 * time.Second, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}},
		{"jitter of up to half the delay", 0, 2, 100 * time.Millisecond, 10 * time.Second, []time.Duration{50 * time.Millisecond}},
		{"jitter in between", 0.5, 3, 100 * time.Millisecond, 10 * time.Second, []time.Duration{75 * time.Millisecond, 150 * time.Millisecond}},
		{"capped at max delay", 1, 5, time.Second, 1500 * time.Millisecond, []time.Duration{time.Second, 1500 * time.Millisecond, 1500 * time.Millisecond, 1500 * time.Millisecond}},
		{"defaults", 1, 3, DefaultBaseDelay, DefaultMaxDelay, []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps := make([]step, 0, tc.attempts)
			for i := 1; i < tc.attempts; i++ {
				steps = append(steps, textStep(503, "down"))
			}
			steps = append(steps, jsonStep(200, `{"id":"e1"}`))
			f := newFakeAPI(t, steps...)
			c := f.client(WithRetry(tc.attempts, tc.base, tc.max))
			c.random = func() float64 { return tc.random }
			waits := recordSleeps(c)
			if _, err := c.Emails.Get(context.Background(), "e1"); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*waits, tc.want) {
				t.Errorf("waits = %v, want %v", *waits, tc.want)
			}
		})
	}
}

func TestCancellationWhileWaitingToRetry(t *testing.T) {
	f := newFakeAPI(t, textStep(503, "down"), jsonStep(200, `{"id":"e1"}`))
	c := f.client(WithRetry(3, time.Minute, time.Minute))
	c.sleep = sleepContext // the real wait
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := c.Emails.Get(ctx, "e1")
	if time.Since(start) > 5*time.Second {
		t.Fatalf("waited %v despite cancellation", time.Since(start))
	}
	e := asError(t, err)
	if !e.IsNetworkError() || !errors.Is(err, context.Canceled) {
		t.Errorf("got %+v (cause %v)", e, errors.Unwrap(err))
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("%d calls, want 1", n)
	}
}

func TestSleepContext(t *testing.T) {
	if err := sleepContext(context.Background(), time.Millisecond); err != nil {
		t.Errorf("sleep: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled sleep: %v", err)
	}
}
