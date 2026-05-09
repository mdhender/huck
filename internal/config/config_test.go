package config

import (
	"strings"
	"testing"
)

func TestValidateServe(t *testing.T) {
	t.Parallel()

	complete := Config{
		DB:            "x.db",
		JWTSecret:     strings.Repeat("a", 32),
		BaseURL:       "https://huck.example",
		MailgunDomain: "mg.example.com",
		MailgunAPIKey: "key-123",
		MailgunFrom:   "huck <noreply@example.com>",
	}

	cases := []struct {
		name        string
		cfg         Config
		wantErr     string
		wantMissing []string
	}{
		{
			name:    "empty config names every required flag",
			cfg:     Config{},
			wantErr: "missing required flag",
			wantMissing: []string{
				"--db", "--jwt-secret", "--base-url",
				"--mailgun-domain", "--mailgun-api-key", "--mailgun-from",
			},
		},
		{
			name: "missing only base-url",
			cfg: func() Config {
				c := complete
				c.BaseURL = ""
				return c
			}(),
			wantErr:     "missing required flag",
			wantMissing: []string{"--base-url"},
		},
		{
			name: "missing the mailgun trio",
			cfg: func() Config {
				c := complete
				c.MailgunDomain = ""
				c.MailgunAPIKey = ""
				c.MailgunFrom = ""
				return c
			}(),
			wantErr:     "missing required flag",
			wantMissing: []string{"--mailgun-domain", "--mailgun-api-key", "--mailgun-from"},
		},
		{
			name: "short jwt secret reported after presence checks",
			cfg: func() Config {
				c := complete
				c.JWTSecret = "short"
				return c
			}(),
			wantErr: "at least 32 bytes",
		},
		{
			name: "ok when every required flag is set",
			cfg:  complete,
		},
		{
			name: "ok with mailgun-api-base left empty (optional)",
			cfg:  complete,
		},
		{
			name: "ok with mailgun-api-base set (EU host)",
			cfg: func() Config {
				c := complete
				c.MailgunAPIBase = "https://api.eu.mailgun.net"
				return c
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateServe()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil err, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want err containing %q, got %v", tc.wantErr, err)
			}
			for _, flag := range tc.wantMissing {
				if !strings.Contains(err.Error(), flag) {
					t.Errorf("want err to mention %q, got %v", flag, err)
				}
			}
		})
	}
}

func TestValidateAdminCreate(t *testing.T) {
	t.Parallel()
	if err := (&Config{}).ValidateAdminCreate(); err == nil {
		t.Fatal("expected error on empty config")
	}
	cfg := &Config{DB: "x.db", AdminHandle: "alice", AdminEmail: "a@example.com"}
	if err := cfg.ValidateAdminCreate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
