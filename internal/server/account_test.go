package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mdhender/huck/internal/auth"
)

func TestAccountAnonymousRedirected(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(f.ts.URL + "/account")
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

func TestAccountSignedInUserRenders(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.userClient(t)

	body := getBody(t, client, f.ts.URL+"/account", http.StatusOK)
	for _, want := range []string{
		"<h1>Account</h1>",
		"alice@example.com",
		"alice",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account page missing %q; body=%s", want, trim(body))
		}
	}
	// A non-admin user must not see the admin link in their own account
	// nav — only admins do.
	if strings.Contains(body, `href="/admin"`) {
		t.Errorf("non-admin account page should not link to /admin; body=%s", trim(body))
	}
}

func TestAccountAdminUserShowsAdminLink(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	body := getBody(t, client, f.ts.URL+"/account", http.StatusOK)
	if !strings.Contains(body, `href="/admin"`) {
		t.Errorf("admin account page should link to /admin; body=%s", trim(body))
	}
	if !strings.Contains(body, "admin@example.com") {
		t.Errorf("admin account page missing own email; body=%s", trim(body))
	}
}

// TestAccountDeletedUserClearsCookie covers the corner case where a JWT
// outlives the user record (admin deleted them between requests).
func TestAccountDeletedUserClearsCookie(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.userClient(t)

	if err := f.usersStore.Delete(context.Background(), f.user.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	resp, err := client.Get(f.ts.URL + "/account")
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
	// The cookie must be cleared so the dangling JWT is rejected on
	// the next request too.
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("auth cookie should be cleared when user no longer exists")
	}
}
