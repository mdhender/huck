package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/mail"
	"github.com/mdhender/huck/internal/server"
	"github.com/mdhender/huck/internal/users"
)

const adminPassword = "correcthorsebattery"

// adminFixture wires a real Echo + DB seeded with one admin and one
// regular user, plus a FakeMailer the tests can inspect or break.
type adminFixture struct {
	ts           *httptest.Server
	pool         *sqlitex.Pool
	usersStore   *users.Store
	invitesStore *invites.Store
	mailer       *mail.FakeMailer
	admin        users.User
	user         users.User
}

func newAdminFixture(t *testing.T) *adminFixture {
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

	us := users.NewStore(pool)
	is := invites.NewStore(pool)
	mailer := mail.NewFakeMailer()

	hash, err := auth.Hash(adminPassword)
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	admin, err := us.Create(context.Background(), users.NewUser{
		Handle: "admin", Email: "admin@example.com",
		PasswordHash: hash, IsAdmin: true,
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	user, err := us.Create(context.Background(), users.NewUser{
		Handle: "alice", Email: "alice@example.com",
		PasswordHash: hash, IsAdmin: false,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	cfg := &config.Config{
		DB:           dbPath,
		JWTSecret:    strings.Repeat("k", 32),
		CookieSecure: false,
		BaseURL:      "http://huck.test",
	}
	srv, err := server.New(cfg, pool, us, is, mailer, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Echo())
	t.Cleanup(ts.Close)

	return &adminFixture{
		ts:           ts,
		pool:         pool,
		usersStore:   us,
		invitesStore: is,
		mailer:       mailer,
		admin:        admin,
		user:         user,
	}
}

// signIn logs in via POST /login and returns a client whose cookie jar
// holds the auth cookie. Used by both the admin and non-admin client
// variants below.
func (f *adminFixture) signIn(t *testing.T, handle string) *http.Client {
	t.Helper()
	client := &http.Client{
		Jar: newJar(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	mustGet(t, client, f.ts.URL+"/login", http.StatusOK)
	resp := mustPost(t, client, f.ts.URL+"/login", url.Values{
		"handle":   {handle},
		"password": {adminPassword},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %s: status %d, want 303", handle, resp.StatusCode)
	}
	return client
}

// adminClient is a signed-in client.
func (f *adminFixture) adminClient(t *testing.T) *http.Client {
	return f.signIn(t, "admin")
}

// userClient is the regular-user equivalent.
func (f *adminFixture) userClient(t *testing.T) *http.Client {
	return f.signIn(t, "alice")
}

func TestAdminInvitesAnonymousRedirected(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(f.ts.URL + "/admin/invites")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location: got %q, want /login", loc)
	}
}

func TestAdminInvitesNonAdminForbidden(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.userClient(t)

	resp, err := client.Get(f.ts.URL + "/admin/invites")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}
}

func TestAdminInvitesListEmpty(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	body := getBody(t, client, f.ts.URL+"/admin/invites", http.StatusOK)
	if !strings.Contains(body, "No invites yet") {
		t.Errorf("expected empty-state copy, got: %s", trim(body))
	}
}

func TestAdminInvitesCreate(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/admin/invites", url.Values{
		"email": {"  Newcomer@Example.COM "},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, trim(string(body)))
	}

	// Stored row is normalised + visible in the list.
	got, err := f.invitesStore.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("invite count = %d, want 1", len(got))
	}
	if got[0].Email != "newcomer@example.com" {
		t.Errorf("email = %q, want lowercased", got[0].Email)
	}

	// Mail captured by the fake mailer.
	sent := f.mailer.Sent()
	if len(sent) != 1 {
		t.Fatalf("messages sent = %d, want 1", len(sent))
	}
	if sent[0].To != "newcomer@example.com" {
		t.Errorf("To = %q, want newcomer@example.com", sent[0].To)
	}
	if sent[0].Subject != "Welcome to Huck!" {
		t.Errorf("Subject = %q, want %q", sent[0].Subject, "Welcome to Huck!")
	}
	if !strings.Contains(sent[0].HTMLBody, "/signup/"+got[0].Token.String()) {
		t.Errorf("body missing signup link with token; body=%s", trim(sent[0].HTMLBody))
	}
	if !strings.Contains(sent[0].HTMLBody, "newcomer%40example.com") {
		t.Errorf("body missing url-encoded email; body=%s", trim(sent[0].HTMLBody))
	}
}

func TestAdminInvitesCreateMissingEmail(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)
	resp := mustPost(t, client, f.ts.URL+"/admin/invites", url.Values{
		"email": {"   "},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", resp.StatusCode)
	}
}

func TestAdminInvitesCreateDuplicate409(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	if _, err := f.invitesStore.Create(context.Background(), "dup@example.com", f.admin.ID); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	client := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/admin/invites", url.Values{
		"email": {"dup@example.com"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 409; body=%s", resp.StatusCode, trim(string(body)))
	}
}

func TestAdminInvitesCreateMailgunFailureRollsBack(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	f.mailer.SendErr = errors.New("mailgun is down")

	client := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/admin/invites", url.Values{
		"email": {"willfail@example.com"},
	})
	defer resp.Body.Close()
	if resp.StatusCode < 500 || resp.StatusCode >= 600 {
		t.Fatalf("status: got %d, want 5xx", resp.StatusCode)
	}

	// The transaction must have rolled back: no invite row.
	got, err := f.invitesStore.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("invites after mail failure = %d, want 0 (rollback)", len(got))
	}
}

func TestAdminInvitesResend(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	inv, err := f.invitesStore.Create(context.Background(), "resend@example.com", f.admin.ID)
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	client := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)

	req, err := http.NewRequest("POST",
		f.ts.URL+"/admin/invites/"+inv.Token.String()+"/resend",
		strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, trim(string(body)))
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invite-"+inv.Token.String()) {
		t.Errorf("HTMX swap should include row id; got: %s", trim(string(body)))
	}

	if len(f.mailer.Sent()) != 1 {
		t.Errorf("messages sent = %d, want 1 (resend)", len(f.mailer.Sent()))
	}
}

func TestAdminInvitesRevoke(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	inv, err := f.invitesStore.Create(context.Background(), "revoke@example.com", f.admin.ID)
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	client := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/invites", http.StatusOK)

	req, err := http.NewRequest("POST",
		f.ts.URL+"/admin/invites/"+inv.Token.String()+"/revoke",
		strings.NewReader(""))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}

	if _, err := f.invitesStore.GetByToken(context.Background(), inv.Token); !errors.Is(err, invites.ErrNotFound) {
		t.Errorf("after revoke want ErrNotFound, got %v", err)
	}
}

func TestAdminInvitesRevokeNonAdminForbidden(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	inv, err := f.invitesStore.Create(context.Background(), "guarded@example.com", f.admin.ID)
	if err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	client := f.userClient(t)

	resp := mustPost(t, client, f.ts.URL+"/admin/invites/"+inv.Token.String()+"/revoke",
		nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}

	// Invite row remains.
	if _, err := f.invitesStore.GetByToken(context.Background(), inv.Token); err != nil {
		t.Errorf("invite should still exist after non-admin revoke attempt: %v", err)
	}
}
