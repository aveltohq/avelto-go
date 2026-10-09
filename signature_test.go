package avelto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func sign(secret, body string, t int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", t, body)
	return fmt.Sprintf("t=%d,v1=%s", t, hex.EncodeToString(mac.Sum(nil)))
}

func TestWebhookSignatureHeaderName(t *testing.T) {
	if WebhookSignatureHeader != "Avelto-Signature" {
		t.Errorf("WebhookSignatureHeader = %q", WebhookSignatureHeader)
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	const secret = "whsec_test"
	body := `{"id":"evt_1","type":"email.delivered"}`
	const ts int64 = 1_700_000_000
	now := time.Unix(ts, 0)
	header := sign(secret, body, ts)

	cases := []struct {
		name   string
		secret string
		body   string
		header string
		opts   []VerifyOption
		want   bool
	}{
		{"valid", secret, body, header, []VerifyOption{WithNow(now)}, true},
		{"valid with spaces after the comma", secret, body, fmt.Sprintf("t=%d, v1=%s", ts, header[len(fmt.Sprintf("t=%d,v1=", ts)):]), []VerifyOption{WithNow(now)}, true},
		{"valid, fields reversed", secret, body, fmt.Sprintf("v1=%s,t=%d", header[len(fmt.Sprintf("t=%d,v1=", ts)):], ts), []VerifyOption{WithNow(now)}, true},
		{"tampered body", secret, body + " ", header, []VerifyOption{WithNow(now)}, false},
		{"wrong secret", "wrong", body, header, []VerifyOption{WithNow(now)}, false},
		{"missing header", secret, body, "", []VerifyOption{WithNow(now)}, false},
		{"garbage header", secret, body, "garbage", []VerifyOption{WithNow(now)}, false},
		{"no v1", secret, body, fmt.Sprintf("t=%d", ts), []VerifyOption{WithNow(now)}, false},
		{"no t", secret, body, "v1=abc", []VerifyOption{WithNow(now)}, false},
		{"non-numeric t", secret, body, "t=abc,v1=abc", []VerifyOption{WithNow(now)}, false},
		{"truncated signature", secret, body, header[:len(header)-2], []VerifyOption{WithNow(now)}, false},
		{"upper-case hex", secret, body, fmt.Sprintf("t=%d,v1=%s", ts, upper(header[len(fmt.Sprintf("t=%d,v1=", ts)):])), []VerifyOption{WithNow(now)}, false},
		{"at the edge of the tolerance", secret, body, header, []VerifyOption{WithNow(now.Add(300 * time.Second))}, true},
		{"stale", secret, body, header, []VerifyOption{WithNow(now.Add(301 * time.Second))}, false},
		{"from the future", secret, body, header, []VerifyOption{WithNow(now.Add(-301 * time.Second))}, false},
		{"stale but within a wider tolerance", secret, body, header, []VerifyOption{WithNow(now.Add(301 * time.Second)), WithTolerance(600 * time.Second)}, true},
		{"within a narrower tolerance", secret, body, header, []VerifyOption{WithNow(now.Add(10 * time.Second)), WithTolerance(5 * time.Second)}, false},
		{"sub-second clock does not count", secret, body, header, []VerifyOption{WithNow(now.Add(300*time.Second + 900*time.Millisecond))}, true},
		{"absurd timestamp", secret, body, "t=-9223372036854775808,v1=abc", []VerifyOption{WithNow(now)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyWebhookSignature(tc.secret, []byte(tc.body), tc.header, tc.opts...); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVerifyWebhookSignatureUsesTheClock(t *testing.T) {
	ts := time.Now().Unix()
	if !VerifyWebhookSignature("s", []byte("{}"), sign("s", "{}", ts)) {
		t.Error("a fresh signature was rejected")
	}
	if VerifyWebhookSignature("s", []byte("{}"), sign("s", "{}", ts-3600)) {
		t.Error("an hour-old signature was accepted")
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 32
		}
	}
	return string(b)
}
