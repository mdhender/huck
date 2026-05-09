# Sprint 2 — Implementation Plan (draft)

Status: **Not started.**

Sprint 1 deferred the following items, all of which Sprint 2 picks up:

## In scope

- `internal/invites` — `Store`, token generation (32 bytes from
  `crypto/rand`, base64url-encoded), `Create`, `Resend`, `Revoke`,
  `Consume`, sentinel errors.
- `internal/mail` — `Mailer` interface plus a Mailgun implementation
  using `github.com/mailgun/mailgun-go/v4`. A no-op/fake `Mailer` for
  tests.
- `pages/signup.html` and `GET`/`POST /signup/:token` — the only
  registration entry point. Pre-filled, read-only email field.
- Admin pages: `/admin/invites` (list + create + resend + revoke
  via HTMX partials) and `/admin/users` (list).
- `internal/server`: route guards `RequireAuth` (echo-jwt) and
  `RequireAdmin` (claims.Admin == true).
- `internal/config`: per-command validation for `serve` now requires
  `--mailgun-domain`, `--mailgun-api-key`, `--mailgun-from`, and
  `--base-url` (the Sprint 1 grace period ends).

## Out of scope (likely Sprint 3+)

- A `/health` endpoint (still only when a probe needs it).
- Refresh tokens. The "rotate `--jwt-secret` to invalidate everyone"
  model is intentional.
- Roles beyond the `users.is_admin` boolean.
- Server-side session/revocation table.

## Hazards to keep in mind

- **Token storage.** Invite tokens are random and large (≥256 bits),
  but they are still secrets in URLs and email bodies. Never log them;
  treat them like passwords in slog filters.
- **Mailgun in tests.** Use the fake `Mailer` in the handler tests; gate
  the real Mailgun integration test on env vars so CI doesn't need
  credentials.
- **Email case.** Lowercase before insert and before `WHERE email = ?`
  comparisons (the `users` package already does this; mirror the
  pattern in `invites`).
- **HTMX swaps for admin invite list.** Use `partials/invite_row.html`
  for resend/revoke responses so the admin page only redraws the
  affected row.

## References

- [docs/DESIGN.md](DESIGN.md) §9 (invite flow), §10 (routes),
  §11 (templates and HTMX).
- [docs/sprint-1.md](sprint-1.md) §3 for the original deferral list.
