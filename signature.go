package avelto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
)

// WebhookSignatureHeader is the name of the header carrying the webhook
// signature.
const WebhookSignatureHeader = "Avelto-Signature"

// DefaultWebhookTolerance is how old a webhook signature may be before
// [VerifyWebhookSignature] rejects it.
const DefaultWebhookTolerance = 300 * time.Second

// maxSeconds is the largest number of seconds a time.Duration can hold.
const maxSeconds = int64(math.MaxInt64 / int64(time.Second))

// VerifyOption configures [VerifyWebhookSignature].
type VerifyOption func(*verifyConfig)

type verifyConfig struct {
	tolerance time.Duration
	now       time.Time
}

// WithTolerance sets how old (or how far in the future) a signature's
// timestamp may be. Default five minutes.
func WithTolerance(d time.Duration) VerifyOption {
	return func(c *verifyConfig) { c.tolerance = d }
}

// WithNow sets the current time to check the timestamp against. For tests.
func WithNow(now time.Time) VerifyOption {
	return func(c *verifyConfig) { c.now = now }
}

// VerifyWebhookSignature reports whether header, the value of the
// Avelto-Signature header, is a valid signature of body under the endpoint's
// signing secret.
//
// The header has the form t=<unix seconds>,v1=<hex>, where v1 is HMAC-SHA256
// over "<t>." followed by the body. Pass the body exactly as received, before
// any JSON decoding. Signatures whose timestamp is more than five minutes from
// now are rejected, so a captured delivery cannot be replayed later; use
// [WithTolerance] to change that. The comparison is constant-time.
func VerifyWebhookSignature(secret string, body []byte, header string, opts ...VerifyOption) bool {
	cfg := verifyConfig{tolerance: DefaultWebhookTolerance}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.now.IsZero() {
		cfg.now = time.Now()
	}
	if header == "" {
		return false
	}
	var tRaw, v1 string
	for _, piece := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(piece), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			tRaw = value
		case "v1":
			v1 = value
		}
	}
	if tRaw == "" || v1 == "" {
		return false
	}
	t, err := strconv.ParseInt(tRaw, 10, 64)
	if err != nil {
		return false
	}
	diff := cfg.now.Unix() - t
	if diff < 0 {
		diff = -diff
	}
	if diff < 0 || diff > maxSeconds || time.Duration(diff)*time.Second > cfg.tolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(t, 10) + "."))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(v1))
}
