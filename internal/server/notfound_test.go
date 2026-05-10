package server_test

import (
	"net/http"
	"testing"
)

// TestUnknownRouteIs404 guards against a regression where Echo v5's
// router miss surfaced as 500 because the central error handler only
// recognised *echo.HTTPError, not v5's unexported *httpError.
func TestUnknownRouteIs404(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	resp, err := http.Get(f.ts.URL + "/this-route-does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
}
