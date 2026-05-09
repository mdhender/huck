// Package email provides a tiny stdlib-only Mailgun client for Huck's
// transactional email needs.
package email

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

const defaultBaseURL = "https://api.mailgun.net"

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Config contains the minimal Mailgun settings Huck needs.
type Config struct {
	APIKey        string
	SendingDomain string
	FromAddress   string
	FromName      string
	BaseURL       string
	HTTPClient    *http.Client
}

// Configured reports whether the required Mailgun settings are present.
func (c Config) Configured() bool {
	return c.APIKey != "" && c.SendingDomain != "" && c.FromAddress != ""
}

func (c Config) from() string {
	if c.FromName == "" {
		return c.FromAddress
	}
	return fmt.Sprintf("%s <%s>", c.FromName, c.FromAddress)
}

func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

func (c Config) apiURL() (string, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("parse base url: missing scheme or host")
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/v3/" + url.PathEscape(c.SendingDomain) + "/messages"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Send queues a single HTML email through Mailgun.
func Send(ctx context.Context, cfg Config, to string, subject string, htmlBody string) error {
	if !cfg.Configured() {
		return fmt.Errorf("mailgun send: config is incomplete")
	}

	endpoint, err := cfg.apiURL()
	if err != nil {
		return fmt.Errorf("mailgun send: %w", err)
	}

	form := url.Values{
		"from":    {cfg.from()},
		"to":      {to},
		"subject": {subject},
		"html":    {htmlBody},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("api", cfg.APIKey)

	resp, err := cfg.httpClient().Do(req)
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
