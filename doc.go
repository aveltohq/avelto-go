// Package avelto is the official Go SDK for Avelto, the email API for developers
// who want it to just work.
//
// Create a client with your API key, then call the resource services on it:
//
//	client, err := avelto.NewClient(os.Getenv("AVELTO_API_KEY"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	email, err := client.Emails.Send(ctx, &avelto.SendEmailRequest{
//		From:    "Acme <hello@mail.acme.com>",
//		To:      []string{"jane@example.com"},
//		Subject: "Welcome to Acme",
//		HTML:    "<p>Thanks for signing up.</p>",
//		Text:    "Thanks for signing up.",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(email.ID)
//
// Every method takes a [context.Context] first and returns a typed response.
// Every failure is an [*Error]; use [errors.As] to read its status and code.
// Failed requests are retried with backoff when that is safe (see [WithRetry]),
// and every send carries an Idempotency-Key so a retry never sends twice.
//
// Incoming webhooks are verified with [VerifyWebhookSignature].
//
// The package uses the standard library only.
package avelto
