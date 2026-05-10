package mail_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mdhender/huck/internal/mail"
)

// TestMailgunMailerNew makes sure constructor validation rejects an
// obviously-incomplete config and accepts a fully-specified one. It
// does not hit the network.
func TestMailgunMailerNewValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     mail.MailgunConfig
		wantErr bool
	}{
		{"missing domain", mail.MailgunConfig{APIKey: "k", From: "f"}, true},
		{"missing api key", mail.MailgunConfig{Domain: "d", From: "f"}, true},
		{"missing from", mail.MailgunConfig{Domain: "d", APIKey: "k"}, true},
		{"all set", mail.MailgunConfig{Domain: "d", APIKey: "k", From: "f"}, false},
		{"all set + EU base", mail.MailgunConfig{
			Domain: "d", APIKey: "k", From: "f", APIBase: "https://api.eu.mailgun.net",
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := mail.NewMailgunMailer(tc.cfg)
			if tc.wantErr && err == nil {
				t.Errorf("NewMailgunMailer: want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("NewMailgunMailer: unexpected error: %v", err)
			}
		})
	}
}

func TestMailgunMailerSend(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/sandbox.mailgun.org/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v3/sandbox.mailgun.org/messages")
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("content-type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("from"); got != "huck <noreply@example.com>" {
			t.Fatalf("form[from] = %q", got)
		}
		if got := r.PostForm.Get("to"); got != "user@example.com" {
			t.Fatalf("form[to] = %q", got)
		}
		if got := r.PostForm.Get("subject"); got != "subject" {
			t.Fatalf("form[subject] = %q", got)
		}
		if got := r.PostForm.Get("html"); got != "<p>hello</p>" {
			t.Fatalf("form[html] = %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc123","message":"Queued. Thank you."}`))
	}))
	defer server.Close()

	m, err := mail.NewMailgunMailer(mail.MailgunConfig{
		Domain:  "sandbox.mailgun.org",
		APIKey:  "key-test",
		From:    "huck <noreply@example.com>",
		APIBase: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMailgunMailer: %v", err)
	}
	if err := m.Send(context.Background(), "user@example.com", "subject", "<p>hello</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestMailgunMailerSendHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	defer server.Close()

	m, err := mail.NewMailgunMailer(mail.MailgunConfig{
		Domain:  "sandbox.mailgun.org",
		APIKey:  "key-test",
		From:    "huck <noreply@example.com>",
		APIBase: server.URL,
	})
	if err != nil {
		t.Fatalf("NewMailgunMailer: %v", err)
	}

	err = m.Send(context.Background(), "user@example.com", "subject", "<p>hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !strings.Contains(err.Error(), "status=401") {
		t.Fatalf("Send error = %q, want status=401", err)
	}
}

func TestMailgunMailerSendContextCancellation(t *testing.T) {
	t.Parallel()

	m, err := mail.NewMailgunMailer(mail.MailgunConfig{
		Domain:  "sandbox.mailgun.org",
		APIKey:  "key-test",
		From:    "huck <noreply@example.com>",
		APIBase: "https://api.mailgun.net",
	})
	if err != nil {
		t.Fatalf("NewMailgunMailer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = m.Send(ctx, "user@example.com", "subject", "<p>hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
}

func TestMailgunMailerSendInvalidBaseURL(t *testing.T) {
	t.Parallel()

	m, err := mail.NewMailgunMailer(mail.MailgunConfig{
		Domain:  "sandbox.mailgun.org",
		APIKey:  "key-test",
		From:    "huck <noreply@example.com>",
		APIBase: "://bad",
	})
	if err != nil {
		t.Fatalf("NewMailgunMailer: %v", err)
	}

	err = m.Send(context.Background(), "user@example.com", "subject", "<p>hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse base url") {
		t.Fatalf("Send error = %q, want parse base url", err)
	}
}

// TestMailgunIntegration is the live-API smoke test stub. It is
// deliberately skipped — Sprint 2 does not pin the test-key strategy.
//
// TODO(sprint-3): wire HUCK_TEST_MAILGUN_DOMAIN / _API_KEY / _FROM /
// _TO env vars (and an opt-in flag) to actually exercise the API
// against a sandbox domain.
func TestMailgunIntegration(t *testing.T) {
	t.Skip("TODO(sprint-3): wire HUCK_TEST_MAILGUN_*")

	// The body below documents the intended shape so the future commit
	// only has to delete the t.Skip and fill in env-var reads.
	cfg := mail.MailgunConfig{
		Domain:  os.Getenv("HUCK_TEST_MAILGUN_DOMAIN"),
		APIKey:  os.Getenv("HUCK_TEST_MAILGUN_API_KEY"),
		From:    os.Getenv("HUCK_TEST_MAILGUN_FROM"),
		APIBase: os.Getenv("HUCK_TEST_MAILGUN_API_BASE"),
	}
	to := os.Getenv("HUCK_TEST_MAILGUN_TO")
	if cfg.Domain == "" || cfg.APIKey == "" || cfg.From == "" || to == "" {
		t.Skip("HUCK_TEST_MAILGUN_* not set")
	}
	m, err := mail.NewMailgunMailer(cfg)
	if err != nil {
		t.Fatalf("NewMailgunMailer: %v", err)
	}
	_ = m
}
