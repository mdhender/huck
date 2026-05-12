package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/mail"
	"github.com/mdhender/huck/internal/server"
	"github.com/mdhender/huck/internal/users"
)

// newLoginFixture seeds a non-admin user "bob" with password "hunter2hunter2"
// and returns the test server + store so individual tests can read or mutate
// suspension/last-login state directly.
func newLoginFixture(t *testing.T) (*httptest.Server, *http.Client, *users.Store, users.User) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "huck.db")
	if err := db.Create(dbPath); err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	pool, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	store := users.NewStore(pool)
	hash, err := auth.Hash("hunter2hunter2")
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	bob, err := store.Create(context.Background(), users.NewUser{
		Handle:       "bob",
		Email:        "bob@example.com",
		PasswordHash: hash,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	cfg := &config.Config{
		DB:           dbPath,
		JWTSecret:    strings.Repeat("k", 32),
		CookieSecure: false,
	}
	srv, err := server.New(cfg, pool, store, invites.NewStore(pool), mail.NewFakeMailer(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Echo())
	t.Cleanup(ts.Close)

	client := &http.Client{
		Jar: newJar(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return ts, client, store, bob
}

// TestLoginRecordsLastLoginAt covers Sprint 5 T3.1: a successful login
// stamps last_login_at on the user row.
func TestLoginRecordsLastLoginAt(t *testing.T) {
	t.Parallel()
	ts, client, store, bob := newLoginFixture(t)

	if !bob.LastLoginAt.IsZero() {
		t.Fatalf("seeded user should have zero last_login_at, got %v", bob.LastLoginAt)
	}

	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"handle":   {"bob"},
		"password": {"hunter2hunter2"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status %d, want 303", resp.StatusCode)
	}

	got, err := store.GetByID(context.Background(), bob.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LastLoginAt.IsZero() {
		t.Fatalf("last_login_at should be set after successful login")
	}
}

// TestLoginRefusesSuspendedUser covers Sprint 5 T3.1: a suspended user
// who supplies the correct password is denied a JWT cookie with a 403
// and a user-facing message; last_login_at is not advanced.
func TestLoginRefusesSuspendedUser(t *testing.T) {
	t.Parallel()
	ts, client, store, bob := newLoginFixture(t)

	if err := store.Suspend(context.Background(), bob.ID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"handle":   {"bob"},
		"password": {"hunter2hunter2"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" && c.MaxAge >= 0 {
			t.Errorf("auth cookie should not be set for suspended login, got %+v", c)
		}
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "suspended") {
		t.Errorf("body should mention suspension, got: %s", trim(string(body)))
	}

	got, err := store.GetByID(context.Background(), bob.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.LastLoginAt.IsZero() {
		t.Fatalf("last_login_at should be unchanged for refused login, got %v", got.LastLoginAt)
	}
}

// TestLoginSuspendedWrongPasswordHidesSuspension covers Sprint 5 T3.1:
// a suspended user supplying a wrong password sees the same generic
// 401 path as any other bad-credentials attempt — the suspension fact
// must not leak before password verify.
func TestLoginSuspendedWrongPasswordHidesSuspension(t *testing.T) {
	t.Parallel()
	ts, client, store, bob := newLoginFixture(t)
	if err := store.Suspend(context.Background(), bob.ID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}

	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"handle":   {"bob"},
		"password": {"wrong"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "suspended") {
		t.Errorf("wrong-password 401 must not reveal suspension, got: %s", trim(string(body)))
	}
	if !strings.Contains(string(body), "Unknown handle or wrong password") {
		t.Errorf("expected generic credentials error, got: %s", trim(string(body)))
	}
}

// TestLoginAllowsReactivatedUser covers Sprint 5 T3.1: reactivating a
// suspended user lets them log in on the next attempt.
func TestLoginAllowsReactivatedUser(t *testing.T) {
	t.Parallel()
	ts, client, store, bob := newLoginFixture(t)
	if err := store.Suspend(context.Background(), bob.ID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := store.Reactivate(context.Background(), bob.ID); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}

	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"handle":   {"bob"},
		"password": {"hunter2hunter2"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Errorf("auth cookie should be set for reactivated user")
	}
}
