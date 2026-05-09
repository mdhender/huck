package mail_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mdhender/huck/internal/mail"
)

func TestFakeMailerRoundTrip(t *testing.T) {
	t.Parallel()
	f := mail.NewFakeMailer()

	if err := f.Send(context.Background(), "alice@example.com", "Welcome to Huck!", "<p>hi</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := f.Send(context.Background(), "bob@example.com", "Welcome to Huck!", "<p>hi bob</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := f.Sent()
	if len(got) != 2 {
		t.Fatalf("Sent(): got %d messages, want 2", len(got))
	}
	if got[0].To != "alice@example.com" || got[0].Subject != "Welcome to Huck!" || got[0].HTMLBody != "<p>hi</p>" {
		t.Errorf("first message: got %+v", got[0])
	}
	if got[1].To != "bob@example.com" {
		t.Errorf("second message recipient: got %q, want bob@example.com", got[1].To)
	}

	// Sent() returns a copy; mutating it must not affect the next read.
	got[0].To = "mallory@example.com"
	if again := f.Sent(); again[0].To != "alice@example.com" {
		t.Errorf("Sent() returned a shared slice: %q", again[0].To)
	}
}

func TestFakeMailerSendErr(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("simulated mailgun outage")
	f := &mail.FakeMailer{SendErr: wantErr}

	err := f.Send(context.Background(), "alice@example.com", "subject", "body")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Send: got %v, want %v", err, wantErr)
	}
	if len(f.Sent()) != 0 {
		t.Errorf("failed Send must not record the message")
	}
}

func TestFakeMailerReset(t *testing.T) {
	t.Parallel()
	f := mail.NewFakeMailer()
	_ = f.Send(context.Background(), "a@b.c", "s", "b")
	f.Reset()
	if got := f.Sent(); len(got) != 0 {
		t.Errorf("Reset(): %d messages remained", len(got))
	}
}

// Compile-time check that FakeMailer satisfies the Mailer interface.
var _ mail.Mailer = (*mail.FakeMailer)(nil)
