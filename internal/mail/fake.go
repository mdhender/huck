package mail

import (
	"context"
	"sync"
)

// SentMessage is the captured form of a Send call, used by tests to
// assert what would have been delivered.
type SentMessage struct {
	To       string
	Subject  string
	HTMLBody string
}

// FakeMailer records every Send call in memory. It is safe for
// concurrent use; tests that exercise parallel handler code can read
// the slice via Sent() without races.
type FakeMailer struct {
	mu       sync.Mutex
	messages []SentMessage

	// SendErr, if non-nil, is returned by Send instead of recording the
	// message. Lets tests exercise the Mailgun-error rollback path.
	SendErr error
}

// NewFakeMailer returns a ready-to-use FakeMailer.
func NewFakeMailer() *FakeMailer { return &FakeMailer{} }

// Send records the message (or returns SendErr if it has been set).
func (f *FakeMailer) Send(_ context.Context, to, subject, htmlBody string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SendErr != nil {
		return f.SendErr
	}
	f.messages = append(f.messages, SentMessage{
		To:       to,
		Subject:  subject,
		HTMLBody: htmlBody,
	})
	return nil
}

// Sent returns a copy of the recorded messages in send order.
func (f *FakeMailer) Sent() []SentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentMessage, len(f.messages))
	copy(out, f.messages)
	return out
}

// Reset discards any recorded messages.
func (f *FakeMailer) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = nil
}
