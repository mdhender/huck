package email

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestConfigConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "valid config",
			cfg: Config{
				APIKey:        "key-test",
				SendingDomain: "sandbox.mailgun.org",
				FromAddress:   "sender@example.com",
			},
			want: true,
		},
		{name: "missing api key", cfg: Config{SendingDomain: "sandbox.mailgun.org", FromAddress: "sender@example.com"}},
		{name: "missing domain", cfg: Config{APIKey: "key-test", FromAddress: "sender@example.com"}},
		{name: "missing from address", cfg: Config{APIKey: "key-test", SendingDomain: "sandbox.mailgun.org"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.Configured(); got != tt.want {
				t.Fatalf("Configured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "with from name",
			cfg:  Config{FromName: "Huck", FromAddress: "sender@example.com"},
			want: "Huck <sender@example.com>",
		},
		{
			name: "without from name",
			cfg:  Config{FromAddress: "sender@example.com"},
			want: "sender@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cfg.from(); got != tt.want {
				t.Fatalf("from() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendSuccess(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotMethod string
	var gotUser string
	var gotPass string
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()

		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		gotForm = r.PostForm

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc123","message":"Queued. Thank you."}`))
	}))
	defer server.Close()

	cfg := Config{
		APIKey:        "key-test",
		SendingDomain: "sandbox.mailgun.org",
		FromAddress:   "sender@example.com",
		FromName:      "Huck",
		BaseURL:       server.URL,
		HTTPClient:    server.Client(),
	}

	err := Send(context.Background(), cfg, "dest@example.com", "Test Subject", "<p>Hello</p>")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotPath != "/v3/sandbox.mailgun.org/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v3/sandbox.mailgun.org/messages")
	}
	if gotUser != "api" {
		t.Fatalf("basic auth user = %q, want %q", gotUser, "api")
	}
	if gotPass != "key-test" {
		t.Fatalf("basic auth password = %q, want %q", gotPass, "key-test")
	}
	if gotForm.Get("from") != "Huck <sender@example.com>" {
		t.Fatalf("form[from] = %q", gotForm.Get("from"))
	}
	if gotForm.Get("to") != "dest@example.com" {
		t.Fatalf("form[to] = %q", gotForm.Get("to"))
	}
	if gotForm.Get("subject") != "Test Subject" {
		t.Fatalf("form[subject] = %q", gotForm.Get("subject"))
	}
	if gotForm.Get("html") != "<p>Hello</p>" {
		t.Fatalf("form[html] = %q", gotForm.Get("html"))
	}
}

func TestSendHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := Config{
		APIKey:        "key-test",
		SendingDomain: "sandbox.mailgun.org",
		FromAddress:   "sender@example.com",
		BaseURL:       server.URL,
		HTTPClient:    server.Client(),
	}

	err := Send(context.Background(), cfg, "dest@example.com", "Test Subject", "<p>Hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !strings.Contains(err.Error(), "status=401") {
		t.Fatalf("Send error = %q, want status=401", err)
	}
}

func TestSendContextCancellation(t *testing.T) {
	t.Parallel()

	cfg := Config{
		APIKey:        "key-test",
		SendingDomain: "sandbox.mailgun.org",
		FromAddress:   "sender@example.com",
		BaseURL:       "https://api.mailgun.net",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Send(ctx, cfg, "dest@example.com", "Test Subject", "<p>Hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
}

func TestSendInvalidBaseURL(t *testing.T) {
	t.Parallel()

	cfg := Config{
		APIKey:        "key-test",
		SendingDomain: "sandbox.mailgun.org",
		FromAddress:   "sender@example.com",
		BaseURL:       "://bad",
	}

	err := Send(context.Background(), cfg, "dest@example.com", "Test Subject", "<p>Hello</p>")
	if err == nil {
		t.Fatal("Send: want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse base url") {
		t.Fatalf("Send error = %q, want parse base url", err)
	}
}
