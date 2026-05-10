package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestHXRedirect(t *testing.T) {
	cases := []struct {
		name        string
		hxRequest   bool
		path        string
		wantStatus  int
		wantHXLoc   string
		wantHTTPLoc string
	}{
		{
			name:       "htmx request gets 204 + HX-Redirect header",
			hxRequest:  true,
			path:       "/",
			wantStatus: http.StatusNoContent,
			wantHXLoc:  "/",
		},
		{
			name:        "non-htmx request gets 303 + Location header",
			hxRequest:   false,
			path:        "/login",
			wantStatus:  http.StatusSeeOther,
			wantHTTPLoc: "/login",
		},
	}

	e := echo.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/anywhere", nil)
			if tc.hxRequest {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := hxRedirect(c, tc.path); err != nil {
				t.Fatalf("hxRedirect: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("HX-Redirect"); got != tc.wantHXLoc {
				t.Errorf("HX-Redirect: got %q, want %q", got, tc.wantHXLoc)
			}
			if got := rec.Header().Get("Location"); got != tc.wantHTTPLoc {
				t.Errorf("Location: got %q, want %q", got, tc.wantHTTPLoc)
			}
		})
	}
}
