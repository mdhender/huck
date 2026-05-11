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
			data:     adminIndexView{Handle: "alice"},
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
				Page: adminIndexView{Handle: "alice"},
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
