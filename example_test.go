package avelto_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/aveltohq/avelto-go"
)

// fakeAPI stands in for the API so the examples run offline. In real code,
// leave out WithBaseURL and the client talks to the public API.
func fakeAPI() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/emails":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.HasSuffix(fmt.Sprint(body["from"]), "@unverified.example>") {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":{"code":"domain_not_verified","message":"Domain unverified.example is not verified"}}`)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"5b3d8c1e-0000-4000-8000-000000000001"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/emails":
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"data":[{"id":"e1","subject":"Receipt #1042","status":"bounced","to":["jane@example.com"]}],"next_cursor":"c2"}`)
			} else {
				_, _ = io.WriteString(w, `{"data":[{"id":"e2","subject":"Receipt #1043","status":"bounced","to":["sam@example.com"]}],"next_cursor":null}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"not_found","message":"Not found"}}`)
		}
	}))
}

// The quickstart: create a client and send an email.
func Example() {
	srv := fakeAPI()
	defer srv.Close()

	client, err := avelto.NewClient("av_test_your_key", avelto.WithBaseURL(srv.URL))
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
	fmt.Println(email.ID)
	// Output: 5b3d8c1e-0000-4000-8000-000000000001
}

// Pass your own idempotency key to make a retry at your application's level
// safe: a replay returns the email already created instead of sending again.
func ExampleEmailsService_Send_idempotencyKey() {
	srv := fakeAPI()
	defer srv.Close()
	client, _ := avelto.NewClient("av_test_your_key", avelto.WithBaseURL(srv.URL))

	email, err := client.Emails.Send(context.Background(), &avelto.SendEmailRequest{
		From:    "billing@mail.acme.com",
		To:      []string{"jane@example.com"},
		Subject: "Receipt #1042",
		Text:    "Thanks for your order.",
	}, avelto.WithIdempotencyKey("receipt-1042"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(email.ID != "")
	// Output: true
}

// Page through every bounced email with the cursor.
func ExampleEmailsService_List() {
	srv := fakeAPI()
	defer srv.Close()
	client, _ := avelto.NewClient("av_test_your_key", avelto.WithBaseURL(srv.URL))

	params := &avelto.ListEmailsParams{Status: avelto.EmailBounced, Limit: 100}
	for {
		page, err := client.Emails.List(context.Background(), params)
		if err != nil {
			log.Fatal(err)
		}
		for _, e := range page.Data {
			fmt.Println(e.ID, e.To[0], e.Subject)
		}
		if page.NextCursor == nil {
			break
		}
		params.Cursor = *page.NextCursor
	}
	// Output:
	// e1 jane@example.com Receipt #1042
	// e2 sam@example.com Receipt #1043
}

// Every failure is an *avelto.Error; read its code with errors.As.
func ExampleError() {
	srv := fakeAPI()
	defer srv.Close()
	client, _ := avelto.NewClient("av_test_your_key", avelto.WithBaseURL(srv.URL))

	_, err := client.Emails.Send(context.Background(), &avelto.SendEmailRequest{
		From:    "Acme <hello@unverified.example>",
		To:      []string{"jane@example.com"},
		Subject: "Hi",
		Text:    "Hi",
	})
	var apiErr *avelto.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code == avelto.CodeDomainNotVerified:
			fmt.Println(apiErr.Status, apiErr.Code)
		case apiErr.IsNetworkError():
			fmt.Println("no response:", errors.Unwrap(apiErr))
		default:
			fmt.Println(apiErr.Status, apiErr.Code, apiErr.Message)
		}
	}
	// Output: 403 domain_not_verified
}

// Verify the Avelto-Signature header against the raw body before trusting a
// webhook payload.
func ExampleVerifyWebhookSignature() {
	secret := "whsec_example"
	body := []byte(`{"id":"evt_1","type":"email.delivered","data":{"email_id":"e1"}}`)

	// What Avelto sends: t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<body>">.
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	header := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))

	fmt.Println(avelto.VerifyWebhookSignature(secret, body, header))
	fmt.Println(avelto.VerifyWebhookSignature(secret, append(body, ' '), header))
	// Output:
	// true
	// false
}

// A net/http handler for webhook deliveries.
func ExampleVerifyWebhookSignature_handler() {
	secret := "whsec_example" // os.Getenv("AVELTO_WEBHOOK_SECRET")

	http.HandleFunc("/hooks/avelto", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body) // the raw bytes, before any decoding
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		if !avelto.VerifyWebhookSignature(secret, body, r.Header.Get(avelto.WebhookSignatureHeader)) {
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
}

// Clear a template's plain-text part while changing its subject. Fields left
// at their zero value are not sent, so the server keeps them.
func ExampleTemplatesService_Update() {
	req := &avelto.UpdateTemplateRequest{
		Subject: "Hello {{name}}",
		Text:    avelto.Null[string](),
	}
	b, _ := json.Marshal(req)
	fmt.Println(string(b))
	// Output: {"subject":"Hello {{name}}","text":null}
}
