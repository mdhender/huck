package mail_test

import (
	"os"
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
			Domain: "d", APIKey: "k", From: "f", APIBase: "https://api.eu.mailgun.net/v3",
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
