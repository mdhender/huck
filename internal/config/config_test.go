package config

import (
	"strings"
	"testing"
)

func TestValidateServe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing db and jwt",
			cfg:     Config{},
			wantErr: "missing required flag",
		},
		{
			name:    "short jwt secret",
			cfg:     Config{DB: "x.db", JWTSecret: "short"},
			wantErr: "at least 32 bytes",
		},
		{
			name: "ok with empty mailgun",
			cfg:  Config{DB: "x.db", JWTSecret: strings.Repeat("a", 32)},
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
