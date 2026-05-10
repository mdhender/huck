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
//
// Config is intentionally one flat bag rather than per-subcommand sub-structs.
// At ~25 fields it would be worth splitting into nested groups (e.g.
// Config.Server, Config.Mail, Config.Admin) so each subcommand only sees its
// slice of the surface; below that threshold the flatness is easier to read
// and matches the DESIGN.md §6 flag table one-for-one.
type Config struct {
	// Globals (every subcommand sees these).
	ConfigFile string // --config (optional path to a plain-format file)
	LogLevel   string // --log-level (debug|info|warn|error)

	// Database.
	DB string // --db (path to the SQLite file)

	// HTTP server.
	Addr         string // --addr
	BaseURL      string // --base-url   (used to build invite links; required by `huck serve` as of Sprint 2)
	JWTSecret    string // --jwt-secret (HS256 key, ≥32 bytes)
	CookieSecure bool   // --cookie-secure
	CookieDomain string // --cookie-domain

	// Mail (Mailgun). Required by `huck serve` as of Sprint 2.
	MailgunDomain  string // --mailgun-domain
	MailgunAPIKey  string // --mailgun-api-key
	MailgunFrom    string // --mailgun-from
	MailgunAPIBase string // --mailgun-api-base (empty = SDK default, US)

	// admin create.
	AdminHandle string // --handle
	AdminEmail  string // --email
}

// MinJWTSecretLen is the minimum acceptable length, in bytes, of the HS256
// secret. DESIGN.md §6 requires ≥32.
const MinJWTSecretLen = 32

// ValidateServe is invoked by `huck serve` once flags are parsed.
// Sprint 2 promotes the Mailgun trio and --base-url to required;
// --mailgun-api-base remains optional (empty means "use the SDK
// default", which is US).
func (c *Config) ValidateServe() error {
	var missing []string
	if c.DB == "" {
		missing = append(missing, "--db")
	}
	if c.JWTSecret == "" {
		missing = append(missing, "--jwt-secret")
	}
	if c.BaseURL == "" {
		missing = append(missing, "--base-url")
	}
	missing = append(missing, c.missingMailerFlags()...)
	if len(missing) > 0 {
		return fmt.Errorf("serve: missing required flag(s): %v", missing)
	}
	if len(c.JWTSecret) < MinJWTSecretLen {
		return fmt.Errorf("serve: --jwt-secret must be at least %d bytes (got %d)", MinJWTSecretLen, len(c.JWTSecret))
	}
	return nil
}

// ValidateMailer checks that the Mailgun trio required by
// [mail.NewMailgunMailer] is set. It is shared between `huck serve` (via
// [Config.ValidateServe]) and `cmd/sendtest` so that the failure
// messages stay in sync. `--mailgun-api-base` remains optional.
func (c *Config) ValidateMailer() error {
	missing := c.missingMailerFlags()
	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %v", missing)
	}
	return nil
}

func (c *Config) missingMailerFlags() []string {
	var missing []string
	if c.MailgunDomain == "" {
		missing = append(missing, "--mailgun-domain")
	}
	if c.MailgunAPIKey == "" {
		missing = append(missing, "--mailgun-api-key")
	}
	if c.MailgunFrom == "" {
		missing = append(missing, "--mailgun-from")
	}
	return missing
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
