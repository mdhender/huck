package server

import "testing"

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
