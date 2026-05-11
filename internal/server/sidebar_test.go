package server

import (
	"strings"
	"testing"
)

// TestSidebarPartial pins the rendered shape of partials/sidebar.html
// (Sprint 4 T1.3): the partial always shows Home and Account; the Admin
// section (Dashboard, Invites, Users) is rendered only when IsAdmin is
// true; the entry matching Section gets aria-current="page" and no
// other entry does.
func TestSidebarPartial(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	t.Run("non-admin hides admin section", func(t *testing.T) {
		v := SidebarView{Handle: "alice", IsAdmin: false, Section: SectionHome}
		var buf strings.Builder
		if err := r.partials.ExecuteTemplate(&buf, "partials/sidebar.html", v); err != nil {
			t.Fatalf("execute: %v", err)
		}
		out := buf.String()

		for _, want := range []string{
			`<nav aria-label="Primary"`,
			`href="/"`,
			`>Home</a>`,
			`href="/account"`,
			`>Account</a>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\n--- output ---\n%s", want, out)
			}
		}

		for _, banned := range []string{
			`href="/admin"`,
			`href="/admin/invites"`,
			`href="/admin/users"`,
			`Dashboard`,
			`Invites`,
			`Users`,
			`<h2>Admin</h2>`,
		} {
			if strings.Contains(out, banned) {
				t.Errorf("non-admin output unexpectedly contains %q\n--- output ---\n%s", banned, out)
			}
		}
	})

	t.Run("admin shows admin section", func(t *testing.T) {
		v := SidebarView{Handle: "penny", IsAdmin: true, Section: SectionAdminInvites}
		var buf strings.Builder
		if err := r.partials.ExecuteTemplate(&buf, "partials/sidebar.html", v); err != nil {
			t.Fatalf("execute: %v", err)
		}
		out := buf.String()

		for _, want := range []string{
			`href="/"`,
			`>Home</a>`,
			`href="/account"`,
			`>Account</a>`,
			`<h2>Admin</h2>`,
			`href="/admin"`,
			`>Dashboard</a>`,
			`href="/admin/invites"`,
			`>Invites</a>`,
			`href="/admin/users"`,
			`>Users</a>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\n--- output ---\n%s", want, out)
			}
		}
	})

	t.Run("active section marked aria-current", func(t *testing.T) {
		cases := []struct {
			name      string
			view      SidebarView
			wantLink  string
			otherUrls []string
		}{
			{
				name:      "home",
				view:      SidebarView{IsAdmin: true, Section: SectionHome},
				wantLink:  `<a href="/" aria-current="page">Home</a>`,
				otherUrls: []string{"/account", "/admin", "/admin/invites", "/admin/users"},
			},
			{
				name:      "account",
				view:      SidebarView{IsAdmin: true, Section: SectionAccount},
				wantLink:  `<a href="/account" aria-current="page">Account</a>`,
				otherUrls: []string{"/", "/admin", "/admin/invites", "/admin/users"},
			},
			{
				name:      "admin dashboard",
				view:      SidebarView{IsAdmin: true, Section: SectionAdminDashboard},
				wantLink:  `<a href="/admin" aria-current="page">Dashboard</a>`,
				otherUrls: []string{"/", "/account", "/admin/invites", "/admin/users"},
			},
			{
				name:      "admin invites",
				view:      SidebarView{IsAdmin: true, Section: SectionAdminInvites},
				wantLink:  `<a href="/admin/invites" aria-current="page">Invites</a>`,
				otherUrls: []string{"/", "/account", "/admin", "/admin/users"},
			},
			{
				name:      "admin users",
				view:      SidebarView{IsAdmin: true, Section: SectionAdminUsers},
				wantLink:  `<a href="/admin/users" aria-current="page">Users</a>`,
				otherUrls: []string{"/", "/account", "/admin", "/admin/invites"},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var buf strings.Builder
				if err := r.partials.ExecuteTemplate(&buf, "partials/sidebar.html", tc.view); err != nil {
					t.Fatalf("execute: %v", err)
				}
				out := buf.String()

				if !strings.Contains(out, tc.wantLink) {
					t.Errorf("missing active link %q\n--- output ---\n%s", tc.wantLink, out)
				}
				if got := strings.Count(out, `aria-current="page"`); got != 1 {
					t.Errorf("aria-current count = %d, want 1\n--- output ---\n%s", got, out)
				}
				for _, u := range tc.otherUrls {
					marked := `<a href="` + u + `" aria-current="page"`
					if strings.Contains(out, marked) {
						t.Errorf("non-active link %q wrongly marked aria-current\n--- output ---\n%s", u, out)
					}
				}
			})
		}
	})
}
