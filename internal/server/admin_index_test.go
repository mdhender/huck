package server_test

import (
	"net/http"
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

func TestAdminIndexAdminRedirectsToInvites(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client := f.adminClient(t)

	resp, err := client.Get(f.ts.URL + "/admin")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin/invites" {
		t.Errorf("Location: got %q, want /admin/invites", loc)
	}
}
