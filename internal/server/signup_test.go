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
	"sync"
	"testing"
	"time"

	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/mail"
	"github.com/mdhender/huck/internal/server"
	"github.com/mdhender/huck/internal/users"
)

// signupFixture wires a real Echo + DB plus seeded admin so each
// signup test can issue invites and exercise the GET/POST handlers
// without re-stating the boilerplate.
type signupFixture struct {
	ts           *httptest.Server
	pool         *sqlitex.Pool
	usersStore   *users.Store
	invitesStore *invites.Store
	admin        users.User
}

func newSignupFixture(t *testing.T) *signupFixture {
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

	hash, err := auth.Hash("hunter2hunter2")
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	admin, err := us.Create(context.Background(), users.NewUser{
		Handle:       "admin",
		Email:        "admin@example.com",
		PasswordHash: hash,
		IsAdmin:      true,
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	cfg := &config.Config{
		DB:           dbPath,
		JWTSecret:    strings.Repeat("k", 32),
		CookieSecure: false,
	}
	srv, err := server.New(cfg, pool, us, is, mail.NewFakeMailer(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Echo())
	t.Cleanup(ts.Close)

	return &signupFixture{
		ts:           ts,
		pool:         pool,
		usersStore:   us,
		invitesStore: is,
		admin:        admin,
	}
}

// createInvite issues a new invite for the given email via the store
// (skipping the admin HTTP path, which T6 will add).
func (f *signupFixture) createInvite(t *testing.T, email string) invites.Invite {
	t.Helper()
	inv, err := f.invitesStore.Create(context.Background(), email, f.admin.ID)
	if err != nil {
		t.Fatalf("invitesStore.Create: %v", err)
	}
	return inv
}

// backdateExpiry rewrites the stored expires_at for a token to the past
// so the "expired" branches can be exercised without sleeping.
func (f *signupFixture) backdateExpiry(t *testing.T, tok invites.Token) {
	t.Helper()
	conn, err := f.pool.Take(context.Background())
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET expires_at = ? WHERE token = ?;`,
		&sqlitex.ExecOptions{Args: []any{past, tok.String()}}); err != nil {
		t.Fatalf("backdate: %v", err)
	}
}

// markConsumed flips consumed_at on a token, simulating a prior signup.
func (f *signupFixture) markConsumed(t *testing.T, tok invites.Token) {
	t.Helper()
	conn, err := f.pool.Take(context.Background())
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET consumed_at = ? WHERE token = ?;`,
		&sqlitex.ExecOptions{Args: []any{now, tok.String()}}); err != nil {
		t.Fatalf("mark consumed: %v", err)
	}
}

// signupClient builds an isolated http.Client + cookie jar for a single
// signup attempt.
func (f *signupFixture) signupClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Jar: newJar(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestSignupGoldenPath(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "alice@example.com")

	client := f.signupClient(t)
	body := getBody(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("signup form should pre-fill the invite email; got: %s", trim(body))
	}

	resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
		"email":    {"alice@example.com"},
		"handle":   {"alice"},
		"password": {"correcthorsebattery"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}

	var authed bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" && c.HttpOnly {
			authed = true
		}
	}
	if !authed {
		t.Fatal("auth cookie should be set after successful signup")
	}

	// New user landed in the DB, with is_admin = 0 and the invite consumed.
	u, err := f.usersStore.GetByHandle(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetByHandle: %v", err)
	}
	if u.IsAdmin {
		t.Error("signup user should not be admin")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("user email = %q, want alice@example.com", u.Email)
	}

	got, err := f.invitesStore.GetByToken(context.Background(), inv.Token)
	if err != nil {
		t.Fatalf("invites.GetByToken: %v", err)
	}
	if !got.Consumed() {
		t.Error("invite should be marked consumed after signup")
	}
}

func TestSignupGetMissingToken(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	client := f.signupClient(t)

	resp, err := client.Get(f.ts.URL + "/signup/no-such-token")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestSignupGetExpiredToken(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "expired@example.com")
	f.backdateExpiry(t, inv.Token)

	client := f.signupClient(t)
	resp, err := client.Get(f.ts.URL + "/signup/" + inv.Token.String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("status: got %d, want 410 (Gone)", resp.StatusCode)
	}
}

func TestSignupGetConsumedToken(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "consumed@example.com")
	f.markConsumed(t, inv.Token)

	client := f.signupClient(t)
	resp, err := client.Get(f.ts.URL + "/signup/" + inv.Token.String())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("status: got %d, want 410 (Gone)", resp.StatusCode)
	}
}

func TestSignupSubmitEmailMismatch(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "bound@example.com")

	client := f.signupClient(t)
	mustGet(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
		"email":    {"tampered@example.com"},
		"handle":   {"newcomer"},
		"password": {"correcthorsebattery"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "does not match") {
		t.Errorf("expected mismatch message, got: %s", trim(string(body)))
	}
	// Invite remains active.
	got, err := f.invitesStore.GetByToken(context.Background(), inv.Token)
	if err != nil {
		t.Fatalf("invites.GetByToken: %v", err)
	}
	if got.Consumed() {
		t.Error("invite should not be consumed when email mismatch rolls back")
	}
}

func TestSignupSubmitHandleTaken(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)

	// Pre-create a user named "alice" so the signup collides.
	hash, err := auth.Hash("hunter2hunter2")
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	if _, err := f.usersStore.Create(context.Background(), users.NewUser{
		Handle:       "alice",
		Email:        "alice2@example.com",
		PasswordHash: hash,
	}); err != nil {
		t.Fatalf("seed alice: %v", err)
	}

	inv := f.createInvite(t, "newalice@example.com")
	client := f.signupClient(t)
	mustGet(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
		"email":    {"newalice@example.com"},
		"handle":   {"alice"},
		"password": {"correcthorsebattery"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "already taken") {
		t.Errorf("expected handle-taken message, got: %s", trim(string(body)))
	}

	// Invite remains active so the operator can re-send / the user can retry.
	got, err := f.invitesStore.GetByToken(context.Background(), inv.Token)
	if err != nil {
		t.Fatalf("invites.GetByToken: %v", err)
	}
	if got.Consumed() {
		t.Error("invite should not be consumed when user-insert rolls back")
	}
}

func TestSignupSubmitWeakPassword(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "weak@example.com")

	client := f.signupClient(t)
	mustGet(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)

	resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
		"email":    {"weak@example.com"},
		"handle":   {"weakling"},
		"password": {"short"}, // < 12 chars
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Password must be at least") {
		t.Errorf("expected password-length message, got: %s", trim(string(body)))
	}
}

func TestSignupSubmitExpiredToken(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "exp@example.com")

	client := f.signupClient(t)
	// GET first so the form renders, mirroring a user who left the page
	// open past the deadline.
	mustGet(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)

	f.backdateExpiry(t, inv.Token)

	resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
		"email":    {"exp@example.com"},
		"handle":   {"latecomer"},
		"password": {"correcthorsebattery"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status: got %d, want 410 (Gone)", resp.StatusCode)
	}
}

func TestSignupParallelSubmits(t *testing.T) {
	t.Parallel()
	f := newSignupFixture(t)
	inv := f.createInvite(t, "race@example.com")

	type result struct {
		status int
		body   string
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []result
		handles = []string{"alphaone", "bravotwo"}
		start   = make(chan struct{})
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := f.signupClient(t)
			mustGet(t, client, f.ts.URL+"/signup/"+inv.Token.String(), http.StatusOK)

			<-start
			resp := mustPost(t, client, f.ts.URL+"/signup/"+inv.Token.String(), url.Values{
				"email":    {"race@example.com"},
				"handle":   {handles[i]},
				"password": {"correcthorsebattery"},
			})
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			mu.Lock()
			results = append(results, result{status: resp.StatusCode, body: string(body)})
			mu.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	successes, failures := 0, 0
	for _, r := range results {
		switch r.status {
		case http.StatusSeeOther:
			successes++
		case http.StatusGone, http.StatusUnprocessableEntity:
			failures++
		default:
			t.Errorf("unexpected status %d, body=%s", r.status, trim(r.body))
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("want 1 success + 1 failure, got %d/%d", successes, failures)
	}
}
