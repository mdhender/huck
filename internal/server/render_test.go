package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// TestRendererDispatch pins the page-vs-partial dispatch contract before
// Sprint 4 T2.2 introduces per-page layout selection. A page rendered
// without HX-Request must go through layout.html (so the output carries
// the <!doctype html> prelude); the same page with HX-Request: true must
// emit only the page's "content" block; a partial template must render
// alone without the layout.
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
