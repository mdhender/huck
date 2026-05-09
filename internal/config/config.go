// Package config holds the typed configuration shared by every huck
// subcommand. Field shape mirrors docs/DESIGN.md §6 in full so future sprints
// can populate values without reshaping the struct.
package config

import (
	"errors"
	"fmt"
)

// Config is the typed configuration for huck. Every flag in DESIGN.md §6 has
// a field here. Per-subcommand validation lives next to each subcommand;
// this struct intentionally does not validate itself.
type Config struct {
	// Globals (every subcommand sees these).
	ConfigFile string // --config (optional path to a plain-format file)
	LogLevel   string // --log-level (debug|info|warn|error)

	// Database.
	DB string // --db (path to the SQLite file)

	// HTTP server.
	Addr         string // --addr
	BaseURL      string // --base-url   (parsed but unused in Sprint 1)
	JWTSecret    string // --jwt-secret (HS256 key, ≥32 bytes)
	CookieSecure bool   // --cookie-secure
	CookieDomain string // --cookie-domain

	// Mail (Mailgun). Parsed but unused in Sprint 1.
	MailgunDomain string // --mailgun-domain
	MailgunAPIKey string // --mailgun-api-key
	MailgunFrom   string // --mailgun-from

	// admin create.
	AdminHandle string // --handle
	AdminEmail  string // --email
}

// MinJWTSecretLen is the minimum acceptable length, in bytes, of the HS256
// secret. DESIGN.md §6 requires ≥32.
const MinJWTSecretLen = 32

// ValidateServe is invoked by `huck serve` once flags are parsed.
// Sprint 1 only requires --db and a sufficiently long --jwt-secret;
// Mailgun and BaseURL are tolerated empty.
func (c *Config) ValidateServe() error {
	var missing []string
	if c.DB == "" {
		missing = append(missing, "--db")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "--jwt-secret")
	}
	if len(missing) > 0 {
		return fmt.Errorf("serve: missing required flag(s): %v", missing)
	}
	if len(c.JWTSecret) < MinJWTSecretLen {
		return fmt.Errorf("serve: --jwt-secret must be at least %d bytes (got %d)", MinJWTSecretLen, len(c.JWTSecret))
	}
	return nil
}

// ValidateDB is invoked by `huck db create` and `huck db migrate`.
func (c *Config) ValidateDB() error {
	if c.DB == "" {
		return errors.New("missing required flag: --db")
	}
	return nil
}

// ValidateAdminCreate is invoked by `huck admin create`.
func (c *Config) ValidateAdminCreate() error {
	var missing []string
	if c.DB == "" {
		missing = append(missing, "--db")
	}
	if c.AdminHandle == "" {
		missing = append(missing, "--handle")
	}
	if c.AdminEmail == "" {
		missing = append(missing, "--email")
	}
	if len(missing) > 0 {
		return fmt.Errorf("admin create: missing required flag(s): %v", missing)
	}
	return nil
}
