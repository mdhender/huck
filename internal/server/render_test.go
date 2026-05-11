package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
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
