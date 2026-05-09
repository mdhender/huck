// Package mail provides the Mailer interface used by handlers that need
// to send transactional email (e.g. invite delivery), along with a
// Mailgun-backed implementation and an in-memory fake for tests.
//
// The package intentionally exposes a tiny surface — Send takes the
// HTML body only — because the only message type Sprint 2 needs is
// the invite email (DESIGN.md §9, sprint-2.md §"In scope"). Plain-text
// multipart can be added later without breaking callers.
package mail

import "context"

// Mailer is the abstraction over a real mail provider. Handlers depend
// on this interface rather than on a concrete client so tests can
// substitute a FakeMailer.
type Mailer interface {
	// Send queues an HTML message for delivery. Implementations should
	// honour ctx cancellation. A nil error means the provider has
	// accepted the message; it does not guarantee final delivery.
	Send(ctx context.Context, to, subject, htmlBody string) error
}
