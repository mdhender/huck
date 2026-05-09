// sendtest is a throwaway one-shot for verifying that the
// internal/mail.MailgunMailer can talk to Mailgun with the credentials
// in .env.development.local. Delete after use.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mdhender/huck/internal/dotenv"
	"github.com/mdhender/huck/internal/mail"
)

func main() {
	if err := dotenv.Load("development"); err != nil {
		log.Fatalf("dotenv: %v", err)
	}
	cfg := mail.MailgunConfig{
		Domain:  os.Getenv("HUCK_MAILGUN_DOMAIN"),
		APIKey:  os.Getenv("HUCK_MAILGUN_API_KEY"),
		From:    os.Getenv("HUCK_MAILGUN_FROM"),
		APIBase: os.Getenv("HUCK_MAILGUN_API_BASE"),
	}
	if cfg.Domain == "" || cfg.APIKey == "" || cfg.From == "" {
		log.Fatalf("missing HUCK_MAILGUN_DOMAIN/API_KEY/FROM after dotenv load")
	}
	to := os.Getenv("HUCK_MAILGUN_SANDBOX_TO_ADDR")
	if len(os.Args) > 1 {
		to = os.Args[1]
	}
	if to == "" {
		log.Fatalf("missing recipient: set HUCK_MAILGUN_SANDBOX_TO_ADDR or pass an address as argv[1]")
	}

	mg, err := mail.NewMailgunMailer(cfg)
	if err != nil {
		log.Fatalf("NewMailgunMailer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	subject := "huck sendtest — " + time.Now().UTC().Format(time.RFC3339)
	body := `<!doctype html>
<html><body>
<p>Hello from huck's <code>internal/mail</code> package.</p>
<p>If you're reading this, the Mailgun sandbox credentials in
<code>.env.development.local</code> work end-to-end against
<code>api.mailgun.net</code>.</p>
</body></html>`

	fmt.Printf("sending to %s via domain %s ... ", to, cfg.Domain)
	if err := mg.Send(ctx, to, subject, body); err != nil {
		fmt.Println("FAIL")
		log.Fatalf("Send: %v", err)
	}
	fmt.Println("queued")
}
