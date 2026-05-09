package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/users"
)

func TestHashAndVerify(t *testing.T) {
	t.Parallel()

	h, err := auth.Hash("hunter2")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if h == "hunter2" || !strings.HasPrefix(h, "$2") {
		t.Fatalf("Hash output looks wrong: %q", h)
	}
	if err := auth.Verify(h, "hunter2"); err != nil {
		t.Fatalf("Verify(matching): %v", err)
	}
	if err := auth.Verify(h, "wrong"); !errors.Is(err, auth.ErrBadPassword) {
		t.Fatalf("Verify(wrong): want ErrBadPassword, got %v", err)
	}
}

func TestHashRejectsEmpty(t *testing.T) {
	t.Parallel()
	if _, err := auth.Hash(""); err == nil {
		t.Fatal("Hash(\"\") should fail")
	}
}

// TestIssueParseRoundtrip is the silent-cast guard called out in
// docs/sprint-1.md §1.10. It must round-trip into the *exact* *Claims
// type, not the generic jwt.MapClaims that echo-jwt would silently fall
// back to if NewClaimsFunc were wired wrong.
func TestIssueParseRoundtrip(t *testing.T) {
	t.Parallel()

	key := []byte(strings.Repeat("k", 32))
	u := users.User{ID: 42, Handle: "alice", IsAdmin: true}

	tok, err := auth.Issue(u, key, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	claims, err := auth.Parse(tok, key)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Type-assert to the concrete type so a downgrade to MapClaims
	// would be a compile error here, not a silent miss at runtime.
	var typed *auth.Claims = claims
	if typed.Handle != "alice" {
		t.Errorf("Handle: got %q, want alice", typed.Handle)
	}
	if !typed.Admin {
		t.Error("Admin: got false, want true")
	}
	if typed.Subject != "42" {
		t.Errorf("Subject: got %q, want 42", typed.Subject)
	}
}

func TestParseRejectsBadKey(t *testing.T) {
	t.Parallel()
	key := []byte(strings.Repeat("k", 32))
	tok, _ := auth.Issue(users.User{ID: 1, Handle: "x"}, key, time.Minute)
	if _, err := auth.Parse(tok, []byte(strings.Repeat("z", 32))); err == nil {
		t.Fatal("Parse should fail with the wrong key")
	}
}
