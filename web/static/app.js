// huck-specific frontend glue. Kept tiny so the strict default CSP
// (script-src 'self') applies cleanly without needing 'unsafe-inline'.
//
// CSRF / cross-origin request protection is now enforced server-side by
// net/http.CrossOriginProtection (see internal/server/middleware.go), so
// no per-request header mirroring is needed here.
