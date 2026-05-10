package mail

// This file contains Huck's tiny, stdlib-only Mailgun client, folded
// directly into the application-facing `internal/mail` package so there
// is exactly one Mailer abstraction (per AGENTS.md).
//
// What it does:
//   - POST /v3/{domain}/messages
//   - Basic Auth with Mailgun API keys
//   - form fields for `from`, `to`, `subject`, and `html`
//
// What it intentionally does not do (extend deliberately, and update
// DESIGN.md first, before adding any of these — Huck originally used
// `github.com/mailgun/mailgun-go/v5` and the replacement targets were
// "stdlib only / no Chi / no SDK surface / easy to read, test, and
// replace"):
//   - attachments
//   - templates
//   - webhooks
//   - mailing lists
//   - event APIs
//   - retries/backoff
//   - MIME composition
//   - broader Mailgun SDK compatibility

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mailgunDefaultBaseURL = "https://api.mailgun.net"

var defaultMailgunHTTPClient = &http.Client{Timeout: 30 * time.Second}

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

// MailgunMailer is the production Mailer backed by the Mailgun HTTP API.
type MailgunMailer struct {
	apiKey     string
	domain     string
	from       string
	baseURL    string
	httpClient *http.Client
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
		apiKey:  cfg.APIKey,
		domain:  cfg.Domain,
		from:    cfg.From,
		baseURL: cfg.APIBase,
	}, nil
}

// Send dispatches an HTML message synchronously. A non-nil return means
// Mailgun rejected the request; callers use this to roll back the
// invite row so the operator can retry.
func (m *MailgunMailer) Send(ctx context.Context, to, subject, htmlBody string) error {
	if err := m.sendMailgun(ctx, to, subject, htmlBody); err != nil {
		return fmt.Errorf("mail: mailgun send: %w", err)
	}
	return nil
}

func (m *MailgunMailer) apiURL() (string, error) {
	baseURL := m.baseURL
	if baseURL == "" {
		baseURL = mailgunDefaultBaseURL
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("parse base url: missing scheme or host")
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/v3/" + url.PathEscape(m.domain) + "/messages"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func (m *MailgunMailer) client() *http.Client {
	if m.httpClient != nil {
		return m.httpClient
	}
	return defaultMailgunHTTPClient
}

func (m *MailgunMailer) sendMailgun(ctx context.Context, to, subject, htmlBody string) error {
	endpoint, err := m.apiURL()
	if err != nil {
		return fmt.Errorf("mailgun send: %w", err)
	}

	form := url.Values{
		"from":    {m.from},
		"to":      {to},
		"subject": {subject},
		"html":    {htmlBody},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", m.apiKey)

	resp, err := m.client().Do(req)
	if err != nil {
		return fmt.Errorf("mailgun send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return fmt.Errorf("mailgun send: status=%d read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("mailgun send: status=%d body=%q", resp.StatusCode, string(body))
	}

	var payload struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
