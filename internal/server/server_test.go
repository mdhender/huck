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

// TestLoginLogoutFlow is the integration test described in
// docs/sprint-1.md §1.10 — the canonical guard for CSRF wiring,
// cookie attributes, and the public-vs-authed root handler.
func TestLoginLogoutFlow(t *testing.T) {
	t.Parallel()

	// 1. Build a real Echo + DB with the bootstrap admin pre-inserted.
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
	hash, err := auth.Hash("hunter2")
	if err != nil {
		t.Fatalf("auth.Hash: %v", err)
	}
	if _, err := store.Create(context.Background(), users.NewUser{
		Handle:       "alice",
		Email:        "alice@example.com",
		PasswordHash: hash,
		IsAdmin:      true,
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	cfg := &config.Config{
		DB:           dbPath,
		JWTSecret:    strings.Repeat("k", 32),
		CookieSecure: false, // httptest is plain HTTP
	}
	srv, err := server.New(cfg, pool, store, invites.NewStore(pool), mail.NewFakeMailer(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(srv.Echo())
	t.Cleanup(ts.Close)

	jar := newJar()
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 2. GET /login. The _csrf cookie is gone after T3.1 swapped Echo's
	// double-submit middleware for http.CrossOriginProtection; T3.2 will
	// strip the form-field plumbing entirely.
	mustGet(t, client, ts.URL+"/login", http.StatusOK)
	csrf := jar.value("_csrf")

	// 3. POST /login with cookie + token + credentials.
	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"_csrf":    {csrf},
		"handle":   {"alice"},
		"password": {"hunter2"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: status %d, want 303", resp.StatusCode)
	}

	// 4. The auth cookie must be set with HttpOnly. (Secure is gated on
	// cfg.CookieSecure, which we deliberately turned off for httptest.)
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			found = true
			if !c.HttpOnly {
				t.Errorf("auth cookie should be HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("auth cookie should be SameSite=Lax, got %v", c.SameSite)
			}
			if c.Value == "" {
				t.Errorf("auth cookie has empty value")
			}
		}
	}
	if !found {
		t.Fatal("Set-Cookie: auth=... not present after login")
	}
	resp.Body.Close()

	// 5. GET / with the cookie → body contains "welcome".
	body := getBody(t, client, ts.URL+"/", http.StatusOK)
	if !strings.Contains(body, "welcome to huck") {
		t.Fatalf("authed home should contain 'welcome to huck', got: %s", trim(body))
	}

	// 6. POST /logout → auth cookie cleared.
	resp = mustPost(t, client, ts.URL+"/logout", url.Values{"_csrf": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout: status %d, want 303", resp.StatusCode)
	}
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("auth cookie should be cleared by logout")
	}
	resp.Body.Close()

	// 7. GET / again → body contains "what is huck".
	body = getBody(t, client, ts.URL+"/", http.StatusOK)
	if !strings.Contains(body, "what is huck") {
		t.Fatalf("public home should contain 'what is huck', got: %s", trim(body))
	}
}

// TestLoginRejectsBadPassword verifies that a wrong password returns 401
// and does not set the auth cookie.
func TestLoginRejectsBadPassword(t *testing.T) {
	t.Parallel()
	ts, jar, client := newTestServer(t)
	t.Cleanup(ts.Close)

	mustGet(t, client, ts.URL+"/login", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client, ts.URL+"/login", url.Values{
		"_csrf":    {csrf},
		"handle":   {"alice"},
		"password": {"wrong"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" && c.MaxAge >= 0 {
			t.Errorf("auth cookie should not be set on failed login, got %+v", c)
		}
	}
}

// TestCrossOriginRejected exercises the http.CrossOriginProtection
// middleware installed by installMiddleware: a state-changing POST
// flagged by the browser as cross-site must return 403.
func TestCrossOriginRejected(t *testing.T) {
	t.Parallel()
	ts, _, client := newTestServer(t)
	t.Cleanup(ts.Close)

	resp := postWithFetchSite(t, client, ts.URL+"/login", "cross-site", url.Values{
		"handle":   {"alice"},
		"password": {"hunter2"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site POST should be rejected with 403, got %d", resp.StatusCode)
	}
}

// TestSameOriginAllowed is the matched happy-path counterpart: the
// same login request marked Sec-Fetch-Site: same-origin must succeed.
func TestSameOriginAllowed(t *testing.T) {
	t.Parallel()
	ts, _, client := newTestServer(t)
	t.Cleanup(ts.Close)

	resp := postWithFetchSite(t, client, ts.URL+"/login", "same-origin", url.Values{
		"handle":   {"alice"},
		"password": {"hunter2"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("same-origin login should succeed with 303, got %d", resp.StatusCode)
	}
}

func postWithFetchSite(t *testing.T, c *http.Client, u, site string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", site)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", u, err)
	}
	return resp
}

// --- helpers -------------------------------------------------------------

func newTestServer(t *testing.T) (*httptest.Server, *jarHelper, *http.Client) {
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
	hash, _ := auth.Hash("hunter2")
	if _, err := store.Create(context.Background(), users.NewUser{
		Handle: "alice", Email: "alice@example.com", PasswordHash: hash, IsAdmin: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
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
	jar := newJar()
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return ts, jar, client
}

func mustGet(t *testing.T, c *http.Client, u string, want int) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s: status %d, want %d", u, resp.StatusCode, want)
	}
}

func getBody(t *testing.T, c *http.Client, u string, want int) string {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s: status %d, want %d", u, resp.StatusCode, want)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func mustPost(t *testing.T, c *http.Client, u string, form url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", form.Get("_csrf"))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", u, err)
	}
	return resp
}

func trim(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// jarHelper is a tiny http.CookieJar that also lets us peek at a value by
// name. Stripped down to only what the test needs.
type jarHelper struct {
	cookies map[string]*http.Cookie
}

func newJar() *jarHelper { return &jarHelper{cookies: map[string]*http.Cookie{}} }

func (j *jarHelper) SetCookies(_ *url.URL, cs []*http.Cookie) {
	for _, c := range cs {
		j.cookies[c.Name] = c
	}
}
func (j *jarHelper) Cookies(_ *url.URL) []*http.Cookie {
	out := make([]*http.Cookie, 0, len(j.cookies))
	for _, c := range j.cookies {
		out = append(out, c)
	}
	return out
}
func (j *jarHelper) value(name string) string {
	if c, ok := j.cookies[name]; ok {
		return c.Value
	}
	return ""
}
