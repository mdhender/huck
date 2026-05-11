package server

import (
	"strings"
	"testing"
)

// TestCrumbCurrent pins the contract that the last crumb in a trail
// (URL == "") reports itself as the current page. The breadcrumbs
// partial (Sprint 4 T1.2) and any handler-side assertions depend on
// this.
func TestCrumbCurrent(t *testing.T) {
	cases := []struct {
		name string
		c    Crumb
		want bool
	}{
		{name: "linked crumb is not current", c: Crumb{Label: "Home", URL: "/"}, want: false},
		{name: "empty-URL crumb is current", c: Crumb{Label: "Invites"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.Current(); got != tc.want {
				t.Errorf("Current() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAppPageWrapping documents the shape the renderer (Sprint 4 T2.2)
// uses to pass a page view + shell context to layout_app.html: page
// templates still see their own view via .Page; shell partials see
// .Shell.*.
func TestAppPageWrapping(t *testing.T) {
	page := adminIndexView{Handle: "alice"}
	shell := ShellView{
		Sidebar: SidebarView{Handle: "alice", IsAdmin: true, Section: SectionAdminInvites},
		Topbar:  TopbarView{Handle: "alice", Title: "Invites"},
		Crumbs: []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Admin", URL: "/admin"},
			{Label: "Invites"},
		},
	}

	wrapped := AppPage{Page: page, Shell: shell}

	gotPage, ok := wrapped.Page.(adminIndexView)
	if !ok {
		t.Fatalf("Page = %T, want adminIndexView", wrapped.Page)
	}
	if gotPage.Handle != "alice" {
		t.Errorf("Page.Handle = %q, want %q", gotPage.Handle, "alice")
	}

	if wrapped.Shell.Sidebar.Section != SectionAdminInvites {
		t.Errorf("Shell.Sidebar.Section = %q, want %q", wrapped.Shell.Sidebar.Section, SectionAdminInvites)
	}
	if !wrapped.Shell.Sidebar.IsAdmin {
		t.Error("Shell.Sidebar.IsAdmin = false, want true")
	}
	if got := len(wrapped.Shell.Crumbs); got != 3 {
		t.Fatalf("len(Shell.Crumbs) = %d, want 3", got)
	}
	if !wrapped.Shell.Crumbs[2].Current() {
		t.Error("last crumb should be current (URL == \"\")")
	}
	if wrapped.Shell.Crumbs[0].Current() {
		t.Error("first crumb should not be current")
	}
}

// TestBreadcrumbsPartial pins the rendered shape of
// partials/breadcrumbs.html (Sprint 4 T1.2): empty slice renders nothing
// (no empty <nav>), and a trail whose last crumb has no URL renders the
// final entry as <span aria-current="page"> rather than as a link.
func TestBreadcrumbsPartial(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	t.Run("empty slice renders nothing", func(t *testing.T) {
		var buf strings.Builder
		if err := r.partials.ExecuteTemplate(&buf, "partials/breadcrumbs.html", []Crumb(nil)); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "" {
			t.Errorf("empty slice produced output: %q", got)
		}

		buf.Reset()
		if err := r.partials.ExecuteTemplate(&buf, "partials/breadcrumbs.html", []Crumb{}); err != nil {
			t.Fatalf("execute (zero-length): %v", err)
		}
		if got := strings.TrimSpace(buf.String()); got != "" {
			t.Errorf("zero-length slice produced output: %q", got)
		}
	})

	t.Run("three crumbs with current last", func(t *testing.T) {
		crumbs := []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Admin", URL: "/admin"},
			{Label: "Invites"},
		}
		var buf strings.Builder
		if err := r.partials.ExecuteTemplate(&buf, "partials/breadcrumbs.html", crumbs); err != nil {
			t.Fatalf("execute: %v", err)
		}
		out := buf.String()

		for _, want := range []string{
			`<nav aria-label="Breadcrumb"`,
			`<ol>`,
			`<a href="/">Home</a>`,
			`<a href="/admin">Admin</a>`,
			`<span aria-current="page">Invites</span>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("output missing %q\n--- output ---\n%s", want, out)
			}
		}

		// The current-page crumb must not be wrapped in an <a>.
		if strings.Contains(out, `href`) && strings.Count(out, `href`) != 2 {
			t.Errorf("expected exactly 2 hrefs (linked crumbs), got %d\n--- output ---\n%s",
				strings.Count(out, `href`), out)
		}
		// Ordering: Home before Admin before Invites.
		homeIdx := strings.Index(out, "Home")
		adminIdx := strings.Index(out, "Admin")
		invitesIdx := strings.Index(out, "Invites")
		if !(homeIdx < adminIdx && adminIdx < invitesIdx) {
			t.Errorf("crumbs out of order: home=%d admin=%d invites=%d\n--- output ---\n%s",
				homeIdx, adminIdx, invitesIdx, out)
		}
	})
}
