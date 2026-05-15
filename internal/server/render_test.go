package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/users"
)

// TestRendererDispatch pins the page-vs-partial dispatch contract. A
// page rendered without HX-Request must go through its registered
// layout (so the output carries the <!doctype html> prelude); the same
// page with HX-Request: true must emit only the page's "content" block;
// a partial template must render alone without any layout.
func TestRendererDispatch(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	cases := []struct {
		name           string
		template       string
		data           any
		hxRequest      bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "page without HX goes through layout",
			template:     "pages/home_public.html",
			data:         homePublicView{},
			hxRequest:    false,
			wantContains: []string{"<!doctype html>", "What is huck?"},
		},
		{
			name:           "page with HX-Request renders content only",
			template:       "pages/home_public.html",
			data:           homePublicView{},
			hxRequest:      true,
			wantContains:   []string{"What is huck?"},
			wantNotContain: []string{"<!doctype html>"},
		},
		{
			name:     "partial renders alone, no layout",
			template: "partials/error.html",
			data: errorView{
				Status:     404,
				StatusText: "Not Found",
				Message:    "no such page",
			},
			hxRequest:      false,
			wantContains:   []string{"<article>", "404", "Not Found", "no such page"},
			wantNotContain: []string{"<!doctype html>"},
		},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.hxRequest {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var buf strings.Builder
			if err := r.Render(c, &buf, tc.template, tc.data); err != nil {
				t.Fatalf("Render: %v", err)
			}
			out := buf.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\n--- output ---\n%s", want, out)
				}
			}
			for _, bad := range tc.wantNotContain {
				if strings.Contains(out, bad) {
					t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
				}
			}
		})
	}
}

// TestAuthShellPagesUseAuthLayout is the Sprint 4 T4.1 contract test:
// every auth-shell page (public home, login, signup, error) must render
// through layout_auth.html. The auth layout's distinguishing marks are
// the centered <main class="container"> wrapper and the absence of any
// app-shell grid chrome (.huck-shell, .huck-sidebar). The page-specific
// content slot must still render so the layout swap did not silently
// drop the page body. This test pins each page rather than only one
// representative so a future re-registration in pageLayouts cannot
// quietly move a page into the app shell.
func TestAuthShellPagesUseAuthLayout(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	cases := []struct {
		name         string
		template     string
		data         any
		wantContains []string // page-specific content slot evidence
	}{
		{
			name:         "home_public",
			template:     "pages/home_public.html",
			data:         homePublicView{},
			wantContains: []string{"What is huck?"},
		},
		{
			name:         "login",
			template:     "pages/login.html",
			data:         loginView{},
			wantContains: []string{"Sign in"},
		},
		{
			name:     "signup",
			template: "pages/signup.html",
			data: signupView{
				Token: "tok-123",
				Email: "alice@example.com",
			},
			wantContains: []string{"Create your account", "alice@example.com"},
		},
		{
			name:     "error",
			template: "pages/error.html",
			data: errorView{
				Status:     404,
				StatusText: "Not Found",
				Message:    "no such page",
			},
			wantContains: []string{"404", "Not Found", "no such page"},
		},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var buf strings.Builder
			if err := r.Render(c, &buf, tc.template, tc.data); err != nil {
				t.Fatalf("Render: %v", err)
			}
			out := buf.String()

			for _, want := range []string{
				`<!doctype html>`,
				`<main class="container">`,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("auth-shell page %s missing %q\n--- output ---\n%s", tc.name, want, out)
				}
			}
			for _, bad := range []string{
				`huck-shell`,
				`huck-sidebar`,
				`huck-topbar`,
				`huck-breadcrumbs`,
			} {
				if strings.Contains(out, bad) {
					t.Errorf("auth-shell page %s should not contain app-shell marker %q\n--- output ---\n%s", tc.name, bad, out)
				}
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("auth-shell page %s missing page-specific content %q\n--- output ---\n%s", tc.name, want, out)
				}
			}
		})
	}
}

// TestAuthedHomeRendersAppShell is the Sprint 4 T4.2 contract test: the
// signed-in home page renders through layout_app.html with the [Home]
// breadcrumb trail (final crumb current), the sidebar Home entry marked
// current, the topbar carrying the signed-in handle, and the page H1
// wrapped in .huck-page-header. The old in-page <header><nav>…</nav></header>
// strip is gone — the topbar/sidebar own that chrome now.
func TestAuthedHomeRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	data := AppPage{
		Page: homeAuthedView{Handle: "alice"},
		Shell: ShellView{
			Sidebar: SidebarView{Handle: "alice", IsAdmin: true, Section: SectionHome},
			Topbar:  TopbarView{Handle: "alice", Title: "Welcome"},
			Crumbs:  []Crumb{{Label: "Home"}},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/home_authed.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`welcome to huck, alice`,
		// Topbar carries the page title and the signed-in handle.
		`Welcome`,
		`<em>alice</em>`,
		// Breadcrumb: single [Home] entry rendered as the current page.
		`<span aria-current="page">Home</span>`,
		// Sidebar: Home is the current section.
		`<a href="/" aria-current="page">Home</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		// Old in-page brand crumb / nav signals — the topbar owns the
		// strip now, so these must not survive the retrofit.
		`<li><strong>huck</strong></li>`,
		// The legacy logout form carried class="inline"; the topbar form
		// drops that class (per Sprint 4 T3, .huck-topbar form styling
		// replaces the old form.inline utility).
		`class="inline"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestAccountRendersAppShell is the Sprint 4 T4.3 contract test: the
// /account page renders through layout_app.html with the [Home, Account]
// breadcrumb trail (Home as a link, Account as the current page), the
// sidebar Account entry marked current, the topbar carrying the
// signed-in handle, and the H1 wrapped in .huck-page-header. The legacy
// in-page <header><nav>…</nav></header> strip is gone — the topbar and
// sidebar own that chrome now.
func TestAccountRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	claims := &auth.Claims{Handle: "alice", Admin: false}
	page := newAdminUserView(claims, users.User{
		ID:        7,
		Handle:    "alice",
		Email:     "alice@example.com",
		IsAdmin:   false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	data := AppPage{
		Page: page,
		Shell: ShellView{
			Sidebar: SidebarView{Handle: "alice", IsAdmin: false, Section: SectionAccount},
			Topbar:  TopbarView{Handle: "alice", Title: "Account"},
			Crumbs:  []Crumb{{Label: "Home", URL: "/"}, {Label: "Account"}},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/account.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>Account</h1>`,
		// Page body still receives the original adminUserView as dot.
		`alice@example.com`,
		// Topbar carries the title and the signed-in handle.
		`Account`,
		`<em>alice</em>`,
		// Breadcrumb: Home links back, Account is the current page.
		`<a href="/">Home</a>`,
		`<span aria-current="page">Account</span>`,
		// Sidebar: Account is the current section.
		`<a href="/account" aria-current="page">Account</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		// Old in-page brand crumb — the topbar owns this now.
		`<li><strong>huck</strong></li>`,
		// The legacy logout form carried class="inline"; .huck-topbar form
		// styling replaces that utility per Sprint 4 T3.
		`class="inline"`,
		// A non-admin account view must not show the Admin sidebar section.
		`<h2>Admin</h2>`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestRendererPicksLayoutPerPage is the Sprint 4 T2.2 contract test:
// the renderer chooses layout_auth.html for an auth-shell page and
// layout_app.html for an app-shell page, based on its registration in
// pageLayouts. The marker for each layout is a structural element the
// other layout does not emit: the auth shell renders <main class="container">,
// the app shell renders the <div class="huck-shell"> grid wrapper.
func TestRendererPicksLayoutPerPage(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	cases := []struct {
		name           string
		template       string
		data           any
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:     "auth-shell page uses layout_auth.html",
			template: "pages/login.html",
			data:     loginView{},
			wantContains: []string{
				`<!doctype html>`,
				`<main class="container">`,
			},
			wantNotContain: []string{
				`huck-shell`,
				`huck-sidebar`,
			},
		},
		{
			name:     "app-shell page uses layout_app.html",
			template: "pages/admin.html",
			data:     adminIndexView{},
			wantContains: []string{
				`<!doctype html>`,
				`huck-shell`,
				`huck-sidebar`,
				`huck-topbar`,
			},
			wantNotContain: []string{
				`<main class="container">`,
			},
		},
		{
			name:     "app-shell page accepts an AppPage wrapper",
			template: "pages/admin.html",
			data: AppPage{
				Page: adminIndexView{},
				Shell: ShellView{
					Sidebar: SidebarView{Handle: "alice", IsAdmin: true, Section: SectionAdminDashboard},
					Topbar:  TopbarView{Handle: "alice", Title: "Admin"},
					Crumbs:  []Crumb{{Label: "Home", URL: "/"}, {Label: "Admin"}},
				},
			},
			wantContains: []string{
				`huck-shell`,
				`aria-current="page"`, // sidebar marks current section
				`<nav aria-label="Breadcrumb"`,
			},
		},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var buf strings.Builder
			if err := r.Render(c, &buf, tc.template, tc.data); err != nil {
				t.Fatalf("Render: %v", err)
			}
			out := buf.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\n--- output ---\n%s", want, out)
				}
			}
			for _, bad := range tc.wantNotContain {
				if strings.Contains(out, bad) {
					t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
				}
			}
		})
	}
}

// TestAdminIndexRendersAppShell is the Sprint 4 T4.4 contract test for
// the admin dashboard page: layout_app.html, [Home, Admin] breadcrumbs
// (Home as a link, Admin as the current page), the admin-dashboard
// sidebar entry marked current, the topbar carrying the signed-in
// handle, and the H1 wrapped in .huck-page-header. The legacy in-page
// <header><nav>…</nav></header> strip and the form.inline utility class
// must be gone — the topbar and sidebar own that chrome now.
func TestAdminIndexRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	data := AppPage{
		Page: adminIndexView{},
		Shell: ShellView{
			Sidebar: SidebarView{Handle: "admin", IsAdmin: true, Section: SectionAdminDashboard},
			Topbar:  TopbarView{Handle: "admin", Title: "Admin"},
			Crumbs:  []Crumb{{Label: "Home", URL: "/"}, {Label: "Admin"}},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/admin.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>Admin</h1>`,
		// Topbar carries the title and the signed-in handle.
		`<em>admin</em>`,
		// Breadcrumb: Home links back, Admin is the current page.
		`<a href="/">Home</a>`,
		`<span aria-current="page">Admin</span>`,
		// Sidebar: admin-dashboard is the current section.
		`<a href="/admin" aria-current="page">Dashboard</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		// Old in-page brand crumb — the topbar owns this now.
		`<li><strong>huck</strong></li>`,
		// The legacy logout form carried class="inline"; the topbar form
		// replaces that utility per Sprint 4 T3.
		`class="inline"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestAdminUsersListRendersAppShell is the Sprint 4 T4.5 contract test
// for /admin/users: layout_app.html, [Home, Admin, Users] breadcrumbs
// (Home and Admin as links, Users as the current page), the admin-users
// sidebar entry marked current, the topbar carrying the signed-in
// handle, and the H1 wrapped in .huck-page-header. The legacy in-page
// <header><nav>…</nav></header> strip and the form.inline utility class
// must be gone.
func TestAdminUsersListRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	formatted, iso := fmtUTC(now)
	page := adminUsersView{
		Rows: []userRowView{
			{
				ID:           7,
				Handle:       "alice",
				Email:        "alice@example.com",
				IsAdmin:      false,
				IsSelf:       false,
				CreatedAt:    formatted,
				CreatedAtISO: iso,
			},
		},
	}
	data := AppPage{
		Page: page,
		Shell: ShellView{
			Sidebar: SidebarView{Handle: "admin", IsAdmin: true, Section: SectionAdminUsers},
			Topbar:  TopbarView{Handle: "admin", Title: "Users"},
			Crumbs: []Crumb{
				{Label: "Home", URL: "/"},
				{Label: "Admin", URL: "/admin"},
				{Label: "Users"},
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/admin_users.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>Users</h1>`,
		// Page body still receives the original adminUsersView as dot.
		`alice@example.com`,
		// Topbar carries the title and the signed-in handle.
		`<em>admin</em>`,
		// Breadcrumb: Home and Admin link back, Users is current.
		`<a href="/">Home</a>`,
		`<a href="/admin">Admin</a>`,
		`<span aria-current="page">Users</span>`,
		// Sidebar: admin-users is the current section.
		`<a href="/admin/users" aria-current="page">Users</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		`<li><strong>huck</strong></li>`,
		`class="inline"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestAdminUsersViewRendersAppShell is the Sprint 4 T4.5 contract test
// for /admin/users/:id: layout_app.html, [Home, Admin, Users, <handle>]
// breadcrumbs (the first three as links, the handle as the current page),
// the admin-users sidebar entry marked current, the topbar title set to
// the viewed user's handle, and the H1 wrapped in .huck-page-header.
func TestAdminUsersViewRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	claims := &auth.Claims{Handle: "admin", Admin: true}
	u := users.User{
		ID:        7,
		Handle:    "alice",
		Email:     "alice@example.com",
		IsAdmin:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	data := AppPage{
		Page:  newAdminUserView(claims, u),
		Shell: userDetailShell(claims, u),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/users/7", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/admin_user_view.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>alice</h1>`,
		// Page body still receives the original adminUserView as dot.
		`alice@example.com`,
		// Topbar carries the viewed user's handle as the page title.
		`<em>admin</em>`,
		// Breadcrumb: Home / Admin / Users link back; the handle is current.
		`<a href="/">Home</a>`,
		`<a href="/admin">Admin</a>`,
		`<a href="/admin/users">Users</a>`,
		`<span aria-current="page">alice</span>`,
		// Sidebar: admin-users is the current section.
		`<a href="/admin/users" aria-current="page">Users</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		`<li><strong>huck</strong></li>`,
		`class="inline"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestAdminUsersEditRendersAppShell is the Sprint 4 T4.5 contract test
// for /admin/users/:id/edit: layout_app.html, the five-entry
// [Home, Admin, Users, <handle>, Edit] breadcrumbs (the first four as
// links, Edit as the current page), the admin-users sidebar entry marked
// current, the topbar carrying the signed-in handle, the H1 wrapped in
// .huck-page-header, and the edit form carrying .huck-form-stack for
// vertical rhythm. The URL parameter remains numeric in the per-user
// link even though the label uses the handle.
func TestAdminUsersEditRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	claims := &auth.Claims{Handle: "admin", Admin: true}
	u := users.User{
		ID:        7,
		Handle:    "alice",
		Email:     "alice@example.com",
		IsAdmin:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	data := AppPage{
		Page:  newAdminUserView(claims, u),
		Shell: userEditShell(claims, u),
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/users/7/edit", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/admin_user_edit.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>Edit alice</h1>`,
		// The edit form gets .huck-form-stack for vertical rhythm.
		`class="huck-form-stack"`,
		// Page body still receives the original adminUserView as dot.
		`name="is_admin"`,
		// Topbar carries the signed-in handle.
		`<em>admin</em>`,
		// Breadcrumb: Home / Admin / Users / <handle> link back; Edit is current.
		`<a href="/">Home</a>`,
		`<a href="/admin">Admin</a>`,
		`<a href="/admin/users">Users</a>`,
		// The per-user link must keep the numeric id even though the
		// label uses the handle (sprint plan T4.5: "keep URL parameters
		// numeric even when labels use handles").
		`<a href="/admin/users/7">alice</a>`,
		`<span aria-current="page">Edit</span>`,
		// Sidebar: admin-users is the current section.
		`<a href="/admin/users" aria-current="page">Users</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		`<li><strong>huck</strong></li>`,
		`class="inline"`,
		// Sprint 5 T4.3 removed admin-set passwords; the edit form must
		// not carry a password input.
		`name="password"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestAdminInvitesRendersAppShell is the Sprint 4 T4.4 contract test for
// the admin invites page: layout_app.html, [Home, Admin, Invites]
// breadcrumbs (Home and Admin as links, Invites as the current page),
// the admin-invites sidebar entry marked current, the topbar carrying
// the signed-in handle, the H1 wrapped in .huck-page-header, and the
// create form carrying .huck-form-stack for vertical rhythm. The legacy
// in-page <header><nav>…</nav></header> strip and the form.inline utility
// class must be gone.
func TestAdminInvitesRendersAppShell(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	data := AppPage{
		Page: adminInvitesView{},
		Shell: ShellView{
			Sidebar: SidebarView{Handle: "admin", IsAdmin: true, Section: SectionAdminInvites},
			Topbar:  TopbarView{Handle: "admin", Title: "Invites"},
			Crumbs: []Crumb{
				{Label: "Home", URL: "/"},
				{Label: "Admin", URL: "/admin"},
				{Label: "Invites"},
			},
		},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/invites", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var buf strings.Builder
	if err := r.Render(c, &buf, "pages/admin_invites.html", data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`<!doctype html>`,
		`huck-shell`,
		`huck-sidebar`,
		`huck-topbar`,
		`class="huck-page-header"`,
		`<h1>Invitations</h1>`,
		// The create form gets .huck-form-stack for vertical rhythm.
		`class="huck-form-stack"`,
		// Topbar carries the signed-in handle.
		`<em>admin</em>`,
		// Breadcrumb: Home and Admin link back, Invites is current.
		`<a href="/">Home</a>`,
		`<a href="/admin">Admin</a>`,
		`<span aria-current="page">Invites</span>`,
		// Sidebar: admin-invites is the current section.
		`<a href="/admin/invites" aria-current="page">Invites</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	for _, bad := range []string{
		// Old in-page brand crumb — the topbar owns this now.
		`<li><strong>huck</strong></li>`,
		// The legacy logout form carried class="inline"; the topbar form
		// replaces that utility per Sprint 4 T3.
		`class="inline"`,
	} {
		if strings.Contains(out, bad) {
			t.Errorf("output should not contain %q\n--- output ---\n%s", bad, out)
		}
	}
}

// TestHeadTitleRendersThroughBothShells is the Sprint 4 T7 regression test
// for the <head><title>…</title></head> element. Splitting layout.html into
// layout_auth.html and layout_app.html changed what dot is when the title
// block evaluates: auth pages get the page view as dot directly, while app
// pages get the AppPage wrapper and the layout calls the title block with
// .Page so existing page templates keep working. This test pins all three
// combinations so a future refactor cannot silently regress the title:
//   - auth shell, static title (login)
//   - app shell, static title (authed home, wrapped in AppPage)
//   - app shell, dynamic title that reads a page-view field (admin user
//     edit; proves {{ block "title" .Page }} threads the page view through)
func TestHeadTitleRendersThroughBothShells(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	editClaims := &auth.Claims{Handle: "admin", Admin: true}
	editUser := users.User{
		ID:        7,
		Handle:    "alice",
		Email:     "alice@example.com",
		IsAdmin:   false,
		CreatedAt: now,
		UpdatedAt: now,
	}

	cases := []struct {
		name      string
		template  string
		data      any
		wantTitle string
	}{
		{
			name:      "auth shell static title",
			template:  "pages/login.html",
			data:      loginView{},
			wantTitle: "<title>huck — sign in</title>",
		},
		{
			name:     "app shell static title",
			template: "pages/home_authed.html",
			data: AppPage{
				Page: homeAuthedView{Handle: "alice"},
				Shell: ShellView{
					Sidebar: SidebarView{Handle: "alice", Section: SectionHome},
					Topbar:  TopbarView{Handle: "alice", Title: "Welcome"},
					Crumbs:  []Crumb{{Label: "Home"}},
				},
			},
			wantTitle: "<title>huck — welcome</title>",
		},
		{
			name:     "app shell dynamic title reads page-view field",
			template: "pages/admin_user_edit.html",
			data: AppPage{
				Page:  newAdminUserView(editClaims, editUser),
				Shell: userEditShell(editClaims, editUser),
			},
			wantTitle: "<title>huck — edit alice</title>",
		},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var buf strings.Builder
			if err := r.Render(c, &buf, tc.template, tc.data); err != nil {
				t.Fatalf("Render: %v", err)
			}
			out := buf.String()
			if !strings.Contains(out, tc.wantTitle) {
				t.Errorf("missing %q\n--- output ---\n%s", tc.wantTitle, out)
			}
		})
	}
}
