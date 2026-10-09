package avelto

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// Every public route, with the method, path and body the SDK sends.
func TestHitsTheDocumentedRoutes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		resp   step
		call   func(c *Client) error
		method string
		path   string
		body   string // JSON, compared semantically; "" means no body
	}{
		{"emails.send", jsonStep(201, `{"id":"e1"}`), func(c *Client) error { _, err := c.Emails.Send(ctx, sendReq()); return err },
			"POST", "/v1/emails", `{"from":"a@b.com","to":["c@d.com"],"subject":"Hi","text":"Hello"}`},
		{"emails.sendBatch", jsonStep(200, `{"results":[],"accepted":0,"failed":0}`), func(c *Client) error {
			_, err := c.Emails.SendBatch(ctx, &BatchSendRequest{Messages: []SendEmailRequest{*sendReq()}})
			return err
		}, "POST", "/v1/emails/batch", `{"messages":[{"from":"a@b.com","to":["c@d.com"],"subject":"Hi","text":"Hello"}]}`},
		{"emails.get", jsonStep(200, `{"id":"e1"}`), func(c *Client) error { _, err := c.Emails.Get(ctx, "e1"); return err },
			"GET", "/v1/emails/e1", ""},
		{"emails.list", jsonStep(200, `{"data":[],"next_cursor":null}`), func(c *Client) error { _, err := c.Emails.List(ctx, nil); return err },
			"GET", "/v1/emails", ""},
		{"emails.cancel", jsonStep(200, `{"id":"e1","status":"cancelled"}`), func(c *Client) error { _, err := c.Emails.Cancel(ctx, "e1"); return err },
			"POST", "/v1/emails/e1/cancel", ""},
		{"emails.validate", jsonStep(200, `{"email":"jane@example.com","normalized":"jane@example.com","result":"deliverable","reason":null,"checks":{"syntax":true,"mail_server":true,"disposable":false,"role":false,"free_provider":false,"suppressed":false},"suggestion":null}`),
			func(c *Client) error { _, err := c.Emails.Validate(ctx, "jane@example.com"); return err },
			"POST", "/v1/emails/validate", `{"email":"jane@example.com"}`},
		{"domains.create", jsonStep(201, `{"id":"d1"}`), func(c *Client) error {
			_, err := c.Domains.Create(ctx, &CreateDomainRequest{Name: "mail.example.com"})
			return err
		}, "POST", "/v1/domains", `{"name":"mail.example.com"}`},
		{"domains.list", jsonStep(200, `{"data":[]}`), func(c *Client) error { _, err := c.Domains.List(ctx); return err },
			"GET", "/v1/domains", ""},
		{"domains.get", jsonStep(200, `{"id":"d1"}`), func(c *Client) error { _, err := c.Domains.Get(ctx, "d1"); return err },
			"GET", "/v1/domains/d1", ""},
		{"domains.delete", step{status: 204}, func(c *Client) error { return c.Domains.Delete(ctx, "d1") },
			"DELETE", "/v1/domains/d1", ""},
		{"webhooks.create", jsonStep(201, `{"id":"w1","secret":"s"}`), func(c *Client) error {
			_, err := c.Webhooks.Create(ctx, &CreateWebhookRequest{URL: "https://example.com/hook", Events: []EmailEventType{EventDelivered}})
			return err
		}, "POST", "/v1/webhooks", `{"url":"https://example.com/hook","events":["email.delivered"]}`},
		{"webhooks.create default events", jsonStep(201, `{"id":"w1","secret":"s"}`), func(c *Client) error {
			_, err := c.Webhooks.Create(ctx, &CreateWebhookRequest{URL: "https://example.com/hook"})
			return err
		}, "POST", "/v1/webhooks", `{"url":"https://example.com/hook"}`},
		{"webhooks.list", jsonStep(200, `{"data":[]}`), func(c *Client) error { _, err := c.Webhooks.List(ctx); return err },
			"GET", "/v1/webhooks", ""},
		{"webhooks.get", jsonStep(200, `{"id":"w1"}`), func(c *Client) error { _, err := c.Webhooks.Get(ctx, "w1"); return err },
			"GET", "/v1/webhooks/w1", ""},
		{"webhooks.delete", step{status: 204}, func(c *Client) error { return c.Webhooks.Delete(ctx, "w1") },
			"DELETE", "/v1/webhooks/w1", ""},
		{"webhooks.listDeliveries", jsonStep(200, `{"data":[],"next_cursor":null}`), func(c *Client) error {
			_, err := c.Webhooks.ListDeliveries(ctx, "w1", &ListWebhookDeliveriesParams{Limit: 10, Cursor: "abc"})
			return err
		}, "GET", "/v1/webhooks/w1/deliveries?limit=10&cursor=abc", ""},
		{"webhooks.retryDelivery", jsonStep(200, `{"id":"del1"}`), func(c *Client) error { _, err := c.Webhooks.RetryDelivery(ctx, "w1", "del1"); return err },
			"POST", "/v1/webhooks/w1/deliveries/del1/retry", ""},
		{"templates.list", jsonStep(200, `{"data":[]}`), func(c *Client) error { _, err := c.Templates.List(ctx); return err },
			"GET", "/v1/templates", ""},
		{"templates.create", jsonStep(201, `{"id":"t1"}`), func(c *Client) error {
			_, err := c.Templates.Create(ctx, &CreateTemplateRequest{Name: "Welcome", Subject: "Hi {{name}}", HTML: "<p>Welcome, {{name}}.</p>"})
			return err
		}, "POST", "/v1/templates", `{"name":"Welcome","subject":"Hi {{name}}","html":"<p>Welcome, {{name}}.</p>"}`},
		{"templates.get", jsonStep(200, `{"id":"t1"}`), func(c *Client) error { _, err := c.Templates.Get(ctx, "t1"); return err },
			"GET", "/v1/templates/t1", ""},
		{"templates.update", jsonStep(200, `{"id":"t1"}`), func(c *Client) error {
			_, err := c.Templates.Update(ctx, "t1", &UpdateTemplateRequest{Subject: "Hello {{name}}"})
			return err
		}, "PATCH", "/v1/templates/t1", `{"subject":"Hello {{name}}"}`},
		{"templates.delete", step{status: 204}, func(c *Client) error { return c.Templates.Delete(ctx, "t1") },
			"DELETE", "/v1/templates/t1", ""},
		{"templates.versions", jsonStep(200, `{"data":[{"version":1,"name":"Welcome","subject":"Hi","html":null,"text":"Hi","created_by":null,"created_at":"2026-09-01T10:00:00Z"}]}`),
			func(c *Client) error { _, err := c.Templates.Versions(ctx, "t1"); return err },
			"GET", "/v1/templates/t1/versions", ""},
		{"templates.restore", jsonStep(200, `{"id":"t1"}`), func(c *Client) error { _, err := c.Templates.Restore(ctx, "t1", 3); return err },
			"POST", "/v1/templates/t1/restore", `{"version":3}`},
		{"suppressions.list", jsonStep(200, `{"data":[],"next_cursor":null}`), func(c *Client) error { _, err := c.Suppressions.List(ctx, nil); return err },
			"GET", "/v1/suppressions", ""},
		{"suppressions.create", jsonStep(201, `{"id":"s1"}`), func(c *Client) error {
			_, err := c.Suppressions.Create(ctx, &CreateSuppressionRequest{EmailAddress: "x@example.com"})
			return err
		}, "POST", "/v1/suppressions", `{"email_address":"x@example.com"}`},
		{"suppressions.delete", step{status: 204}, func(c *Client) error { return c.Suppressions.Delete(ctx, "x@example.com") },
			"DELETE", "/v1/suppressions/x%40example.com", ""},
		{"account.get", jsonStep(200, `{"account":{"id":"a1"}}`), func(c *Client) error { _, err := c.Account.Get(ctx); return err },
			"GET", "/v1/account", ""},
		{"account.getSettings", jsonStep(200, `{"name":"Acme","alerts":{"notify_email":null,"notify_email_verified":false,"domain_health":true,"bounce_rate":true,"complaint_rate":true,"usage_80":true,"api_key_created":true}}`),
			func(c *Client) error { _, err := c.Account.GetSettings(ctx); return err },
			"GET", "/v1/account/settings", ""},
		{"account.updateSettings", jsonStep(200, `{"name":"Acme Ltd","alerts":{}}`), func(c *Client) error {
			_, err := c.Account.UpdateSettings(ctx, &UpdateAccountSettingsRequest{
				Name:   "Acme Ltd",
				Alerts: &UpdateAlertSettingsRequest{NotifyEmail: Null[string](), BounceRate: Bool(false)},
			})
			return err
		}, "PATCH", "/v1/account/settings", `{"name":"Acme Ltd","alerts":{"notify_email":null,"bounce_rate":false}}`},
		{"webhooks.update", jsonStep(200, `{"id":"w1","enabled":false}`), func(c *Client) error {
			_, err := c.Webhooks.Update(ctx, "w1", &UpdateWebhookRequest{Enabled: Bool(false)})
			return err
		}, "PATCH", "/v1/webhooks/w1", `{"enabled":false}`},
		{"webhooks.update events", jsonStep(200, `{"id":"w1"}`), func(c *Client) error {
			_, err := c.Webhooks.Update(ctx, "w1", &UpdateWebhookRequest{Events: []EmailEventType{EventBounced}})
			return err
		}, "PATCH", "/v1/webhooks/w1", `{"events":["email.bounced"]}`},
		{"apiKeys.list", jsonStep(200, `{"data":[]}`), func(c *Client) error { _, err := c.APIKeys.List(ctx); return err },
			"GET", "/v1/api-keys", ""},
		{"apiKeys.create", jsonStep(201, `{"id":"k1","key":"av_live_x","scopes":["emails:send"],"created_by_key_id":"k0","last_used_at":null,"created_at":"2026-10-01T10:00:00Z","revoked_at":null}`), func(c *Client) error {
			k, err := c.APIKeys.Create(ctx, &CreateAPIKeyRequest{Name: "Production server", Mode: ModeLive, Scopes: []APIKeyScope{ScopeEmailsSend}})
			if err == nil && (k.Key != "av_live_x" || k.CreatedByKeyID == nil || *k.CreatedByKeyID != "k0") {
				t.Errorf("decoded %+v", k)
			}
			return err
		}, "POST", "/v1/api-keys", `{"name":"Production server","mode":"live","scopes":["emails:send"]}`},
		{"apiKeys.revoke", step{status: 204}, func(c *Client) error { return c.APIKeys.Revoke(ctx, "k/1") },
			"DELETE", "/v1/api-keys/k%2F1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, tc.resp)
			if err := tc.call(f.client()); err != nil {
				t.Fatal(err)
			}
			c := f.last()
			if c.Method != tc.method || c.URL != tc.path {
				t.Errorf("got %s %s, want %s %s", c.Method, c.URL, tc.method, tc.path)
			}
			if tc.body == "" {
				if len(c.Body) != 0 {
					t.Errorf("unexpected body %s", c.Body)
				}
				if ct := c.Header.Get("Content-Type"); ct != "" {
					t.Errorf("Content-Type %q without a body", ct)
				}
				return
			}
			if !jsonEqual(t, c.Body, []byte(tc.body)) {
				t.Errorf("body = %s, want %s", c.Body, tc.body)
			}
			if ct := c.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatalf("invalid JSON %s: %v", a, err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatalf("invalid JSON %s: %v", b, err)
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

func TestTemplateUpdateBody(t *testing.T) {
	cases := []struct {
		name string
		req  *UpdateTemplateRequest
		want string
	}{
		{"nothing set", &UpdateTemplateRequest{}, `{}`},
		{"nil request", nil, `{}`},
		{"scalar fields", &UpdateTemplateRequest{Name: "Welcome", Slug: "welcome", Subject: "Hi"}, `{"name":"Welcome","slug":"welcome","subject":"Hi"}`},
		{"set a body part", &UpdateTemplateRequest{HTML: Value("<p>Hi & bye</p>")}, `{"html":"<p>Hi & bye</p>"}`},
		{"clear a body part", &UpdateTemplateRequest{Text: Null[string]()}, `{"text":null}`},
		{"set one, clear the other", &UpdateTemplateRequest{HTML: Value("<p>x</p>"), Text: Null[string]()}, `{"html":"<p>x</p>","text":null}`},
		{"set to empty string is not null", &UpdateTemplateRequest{Text: Value("")}, `{"text":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeAPI(t, jsonStep(200, `{"id":"t1"}`))
			if _, err := f.client().Templates.Update(context.Background(), "t1", tc.req); err != nil {
				t.Fatal(err)
			}
			c := f.last()
			if c.Method != http.MethodPatch {
				t.Errorf("method %s", c.Method)
			}
			// Exact bytes: key order is stable and HTML is not escaped.
			if string(c.Body) != tc.want {
				t.Errorf("body = %s, want %s", c.Body, tc.want)
			}
		})
	}
}

func TestNullable(t *testing.T) {
	var unset Nullable[string]
	if unset.IsSet() || unset.IsNull() {
		t.Error("zero value is set")
	}
	if _, ok := unset.Get(); ok {
		t.Error("zero value has a value")
	}
	v := Value("x")
	if got, ok := v.Get(); !ok || got != "x" || v.IsNull() || !v.IsSet() {
		t.Errorf("Value: %+v", v)
	}
	n := Null[string]()
	if _, ok := n.Get(); ok || !n.IsNull() || !n.IsSet() {
		t.Errorf("Null: %+v", n)
	}
	for _, tc := range []struct {
		in   Nullable[string]
		want string
	}{{unset, "null"}, {n, "null"}, {v, `"x"`}} {
		b, err := json.Marshal(tc.in)
		if err != nil || string(b) != tc.want {
			t.Errorf("Marshal = %s, %v; want %s", b, err, tc.want)
		}
	}
	var got struct {
		A Nullable[string] `json:"a"`
		B Nullable[string] `json:"b"`
		C Nullable[string] `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"a":"x","b":null}`), &got); err != nil {
		t.Fatal(err)
	}
	if s, ok := got.A.Get(); !ok || s != "x" {
		t.Errorf("a = %+v", got.A)
	}
	if !got.B.IsNull() {
		t.Errorf("b = %+v", got.B)
	}
	if got.C.IsSet() {
		t.Errorf("c = %+v", got.C)
	}
}
