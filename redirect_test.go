package avelto

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// redirectingAPI answers every API call with a redirect to /captured on the same
// host, the case where net/http would re-send the Authorization header, and
// counts what reaches /captured.
func redirectingAPI(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var captured atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/captured" {
			captured.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"e1"}`))
			return
		}
		http.Redirect(w, r, "/captured", status)
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

func TestDoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		srv, captured := redirectingAPI(t, status)
		c, err := NewClient("av_live_secret", WithBaseURL(srv.URL), WithRetry(1, 0, 0))
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Emails.Send(context.Background(), sendReq())
		apiErr := asError(t, err)
		if apiErr.Status != status {
			t.Errorf("%d: Status = %d, want the redirect itself", status, apiErr.Status)
		}
		if n := captured.Load(); n != 0 {
			t.Errorf("%d: the redirect target was requested %d time(s), with the API key", status, n)
		}
	}
}

func TestDoesNotFollowRedirectsWithACallersClient(t *testing.T) {
	srv, captured := redirectingAPI(t, http.StatusFound)
	own := &http.Client{}
	c, err := NewClient("av_live_secret", WithBaseURL(srv.URL), WithHTTPClient(own), WithRetry(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Emails.Send(context.Background(), sendReq())
	if asError(t, err).Status != http.StatusFound {
		t.Errorf("want the 302 back as an error")
	}
	if n := captured.Load(); n != 0 {
		t.Errorf("the redirect target was requested %d time(s), with the API key", n)
	}
	// The caller's own client keeps the policy it had.
	if own.CheckRedirect != nil {
		t.Error("the caller's http.Client was changed")
	}
}
