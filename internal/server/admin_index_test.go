package server_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminIndexAnonymousRedirected(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(f.ts.URL + "/admin")
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

func TestAdminIndexNonAdminForbidden(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.userClient(t)

	resp, err := client.Get(f.ts.URL + "/admin")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}
}

func TestAdminIndexAdminRendersDashboard(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	body := getBody(t, client, f.ts.URL+"/admin", http.StatusOK)
	// Page heading and the two anchor links Sprint 4's sidebar will
	// surface — proves the canonical dashboard renders, not a redirect.
	for _, want := range []string{
		"<h1>Administration</h1>",
		`href="/admin/invites"`,
		`href="/admin/users"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q; body=%s", want, trim(body))
		}
	}
}

func TestAdminIndexTrailingSlashRedirects(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	// The redirect runs before requireAdmin, so an anonymous client
	// is enough to assert the canonicalisation behaviour.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(f.ts.URL + "/admin/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status: got %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Errorf("Location: got %q, want /admin", loc)
	}
}
