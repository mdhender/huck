package invites

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
)

// tokenBytes is the random byte length for an invite token. 32 bytes
// (~256 bits) of entropy is overkill for a 7-day single-use credential
// but matches DESIGN.md §9.
const tokenBytes = 32

// Token is an invite token. Tokens are random secrets that appear in
// signup URLs and email bodies; treat their string value as sensitive.
// The LogValue method returns a redacted form so accidentally passing
// a Token to slog does not leak the secret.
type Token string

// String returns the raw token string. Use sparingly: the value is a
// secret and should never be logged. Prefer passing Token directly so
// LogValue applies.
func (t Token) String() string { return string(t) }

// LogValue implements slog.LogValuer. It redacts the token so a
// stray slog.Info("...", "token", t) does not put the secret in the
// log line. Reviewers still need to catch direct String() / Sprintf
// calls per DESIGN.md §14 — this is a defence in depth, not a filter.
func (t Token) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// Generate returns a fresh invite token: 32 random bytes encoded with
// base64.RawURLEncoding (no padding, URL-safe alphabet) so the value
// can be embedded in a URL path segment without escaping.
func Generate() (Token, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("invites: read random: %w", err)
	}
	return Token(base64.RawURLEncoding.EncodeToString(b)), nil
}
