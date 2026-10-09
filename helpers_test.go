package avelto

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// step is one queued answer from the fake API: an HTTP response, a network
// error raised before the request reaches the server, or a hang that only ends
// when the request is cancelled.
type step struct {
	status  int
	body    string
	headers map[string]string
	netErr  error
	hang    bool
}

func jsonStep(status int, body string, headers ...string) step {
	h := map[string]string{"Content-Type": "application/json"}
	for i := 0; i+1 < len(headers); i += 2 {
		h[headers[i]] = headers[i+1]
	}
	return step{status: status, body: body, headers: h}
}

func textStep(status int, body string) step { return step{status: status, body: body} }

func netErrStep(msg string) step { return step{netErr: errors.New(msg)} }

// call is one request as the client sent it.
type call struct {
	Method string
	URL    string // path and raw query, as sent
	Header http.Header
	Body   []byte
}

// fakeAPI is an httptest server that answers from a queue of steps. It is
// reached through a recording transport, so every attempt is recorded, even one
// that fails with an injected network error before reaching the server.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu      sync.Mutex
	steps   []step
	calls   []call
	current step
}

func newFakeAPI(t *testing.T, steps ...step) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, steps: steps}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		s := f.current
		f.mu.Unlock()
		if s.hang {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		for k, v := range s.headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(s.status)
		_, _ = io.WriteString(w, s.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	f.mu.Lock()
	f.calls = append(f.calls, call{Method: r.Method, URL: r.URL.RequestURI(), Header: r.Header.Clone(), Body: body})
	if len(f.steps) == 0 {
		f.mu.Unlock()
		f.t.Errorf("fakeAPI: no response queued for %s %s", r.Method, r.URL)
		return nil, errors.New("fakeAPI: no response queued")
	}
	s := f.steps[0]
	f.steps = f.steps[1:]
	f.current = s
	f.mu.Unlock()
	if s.netErr != nil {
		return nil, s.netErr
	}
	return f.srv.Client().Transport.RoundTrip(r)
}

// client returns a Client pointed at the fake API, with instant retries.
func (f *fakeAPI) client(opts ...Option) *Client {
	f.t.Helper()
	all := append([]Option{WithBaseURL(f.srv.URL), WithHTTPClient(&http.Client{Transport: f}), WithRetry(DefaultMaxAttempts, 0, 0)}, opts...)
	c, err := NewClient("av_test_abc", all...)
	if err != nil {
		f.t.Fatalf("NewClient: %v", err)
	}
	c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }
	return c
}

func (f *fakeAPI) Calls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeAPI) last() call {
	f.t.Helper()
	calls := f.Calls()
	if len(calls) == 0 {
		f.t.Fatal("fakeAPI: no calls")
	}
	return calls[len(calls)-1]
}

// asError asserts err is an *Error and returns it.
func asError(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want *avelto.Error, got %T: %v", err, err)
	}
	return e
}

// recordSleeps replaces the client's sleep with one that records each wait.
func recordSleeps(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.sleep = func(ctx context.Context, d time.Duration) error {
		waits = append(waits, d)
		return ctx.Err()
	}
	return &waits
}
