package mail

import (
	"context"
	"fmt"

	"github.com/mailgun/mailgun-go/v4"
)

// MailgunMailer is the production Mailer backed by github.com/mailgun/mailgun-go/v4.
type MailgunMailer struct {
	mg   *mailgun.MailgunImpl
	from string
}

// MailgunConfig captures only the fields this package needs from the
// global config. Taking a small, packaged struct keeps internal/mail
// independent of internal/config (avoids the import cycle that would
// otherwise appear once config grows mail-related helpers).
type MailgunConfig struct {
	Domain  string
	APIKey  string
	From    string
	APIBase string // empty = SDK default (US)
}

// NewMailgunMailer builds a MailgunMailer. The DESIGN.md contract is
// that callers (huck serve) have already validated that Domain,
// APIKey, and From are non-empty (config.ValidateServe), so we panic
// here only in the obviously-broken case.
func NewMailgunMailer(cfg MailgunConfig) (*MailgunMailer, error) {
	if cfg.Domain == "" || cfg.APIKey == "" || cfg.From == "" {
		return nil, fmt.Errorf("mail: mailgun config is incomplete (domain/api-key/from required)")
	}
	mg := mailgun.NewMailgun(cfg.Domain, cfg.APIKey)
	if cfg.APIBase != "" {
		mg.SetAPIBase(cfg.APIBase)
	}
	return &MailgunMailer{mg: mg, from: cfg.From}, nil
}

// Send dispatches an HTML message synchronously. A non-nil return means
// Mailgun rejected the request; callers (T6's POST /admin/invites) use
// this to roll back the invite row so the operator can retry.
func (m *MailgunMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	msg := mailgun.NewMessage(m.from, subject, "", to)
	msg.SetHTML(htmlBody)
	if _, _, err := m.mg.Send(ctx, msg); err != nil {
		return fmt.Errorf("mail: mailgun send: %w", err)
	}
	return nil
}
