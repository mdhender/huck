package mail

import (
	"context"
	"fmt"

	"github.com/mdhender/huck/internal/email"
)

// MailgunMailer is the production Mailer backed by the Mailgun HTTP API.
type MailgunMailer struct {
	cfg email.Config
}

// MailgunConfig captures only the fields this package needs from the
// global config. Taking a small, packaged struct keeps internal/mail
// independent of internal/config (avoids the import cycle that would
// otherwise appear once config grows mail-related helpers).
type MailgunConfig struct {
	Domain  string
	APIKey  string
	From    string
	APIBase string // empty = Mailgun US default. The client appends /v3/... itself.
}

// NewMailgunMailer builds a MailgunMailer. The DESIGN.md contract is that
// callers (huck serve) have already validated that Domain, APIKey, and From
// are non-empty (config.ValidateServe), so this constructor only rejects an
// obviously incomplete config.
func NewMailgunMailer(cfg MailgunConfig) (*MailgunMailer, error) {
	if cfg.Domain == "" || cfg.APIKey == "" || cfg.From == "" {
		return nil, fmt.Errorf("mail: mailgun config is incomplete (domain/api-key/from required)")
	}
	return &MailgunMailer{
		cfg: email.Config{
			APIKey:        cfg.APIKey,
			SendingDomain: cfg.Domain,
			FromAddress:   cfg.From,
			BaseURL:       cfg.APIBase,
		},
	}, nil
}

// Send dispatches an HTML message synchronously. A non-nil return means
// Mailgun rejected the request; callers (T6's POST /admin/invites) use
// this to roll back the invite row so the operator can retry.
func (m *MailgunMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	if err := email.Send(ctx, m.cfg, to, subject, htmlBody); err != nil {
		return fmt.Errorf("mail: mailgun send: %w", err)
	}
	return nil
}
