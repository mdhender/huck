# Sprint 3 — Implementation Plan

Status: **Draft 2026-05-09.**

Sprint 3 is a clean-up and consolidation sprint driven by the code-smell
review of the post–Sprint 2 codebase. There is no new user-facing
feature; every task either removes duplication, deletes dead code, or
swaps a hand-rolled mechanism for a stdlib equivalent.

The detailed contracts (schema, password/handle policies, invite flow,
routes, security headers) continue to live in `docs/DESIGN.md`. This
file is a sprint plan — when a task changes a contract, DESIGN.md is
the document to update first.

## In scope

| Task | Status | Commit |
|------|--------|--------|
| T1   | TODO   |        |
| T2   | TODO   |        |
| T3   | TODO   |        |
| T4   | TODO   |        |
| T5   | TODO   |        |
| T6   | TODO   |        |
| T7   | TODO   |        |
| T8   | TODO   |        |
| T9   | TODO   |        |
| T10  | TODO   |        |
| T11  | TODO   |        |
| T12  | TODO   |        |
| T13  | TODO   |        |

### T1 — Collapse `internal/email` into `internal/mail`

`internal/mail/mailgun.go` currently wraps `internal/email/mailgun.go`
by translating `mail.MailgunConfig` → `email.Config` and forwarding
`Send`. AGENTS.md mandates "one `Mailer` interface in `internal/mail`";
keeping a second mail-shaped package on the side undercuts that.

- Move the stdlib-only Mailgun HTTP client (today in
  `internal/email/mailgun.go`) into `internal/mail`, e.g. as an
  unexported `sendMailgun` helper or a small `mailgunClient` struct
  used only by `MailgunMailer`.
- Fold `internal/email/mailgun_test.go` into `internal/mail` and drop
  any duplication with `internal/mail/mailgun_test.go`.
- Delete `internal/email/` once nothing imports it. Migrate the
  rationale paragraphs from `internal/email/README.md` into a doc
  comment at the top of the merged file (the "what / what not"
  enumeration is still useful future-reader context).

### T2 — Repurpose `cmd/sendtest` as the documented Mailgun smoke-test

`cmd/sendtest/main.go` is currently labelled "throwaway one-shot …
delete after use," but it is actually the most convenient way for an
operator to confirm that their Mailgun sandbox credentials in
`.env.development.local` work end-to-end. We want to keep that
ergonomics; we just need to stop pretending it's disposable.

- Rewrite the package doc comment to describe `sendtest` as
  Huck's Mailgun configuration validator — its purpose is to send a
  single test email using the same `internal/mail.MailgunMailer` the
  server uses, so the operator can prove that their domain, API key,
  `From:` address, and (optional) `--mailgun-api-base` are all valid
  before running `huck serve`.
- Switch from ad-hoc `os.Getenv("HUCK_MAILGUN_*")` reads to the
  standard `internal/config` flag/env path:
  - Build a `*config.Config` and run the same `ff.Parse` pipeline that
    `cmd/huck` uses, with `ff.WithEnvVarPrefix("HUCK")` so the same
    `HUCK_MAILGUN_*` variables work without code changes.
  - Reuse the relevant slice of `cfg.ValidateServe()` (Mailgun trio
    required) so the failure messages match the server's.
  - Keep the `--to` / argv recipient override, but document it via a
    real flag rather than positional args.
- Add a short `cmd/sendtest/README.md` (or expand the doc comment)
  explaining: when to use it, what env vars / `.env.development.local`
  values it expects, and the expected output for success and failure.
- Confirm `go build ./...` and `go test ./...` still pass after the
  rewrite; the binary should remain installable via
  `go install ./cmd/sendtest`.

### T3 — Replace Echo's CSRF middleware with `http.CrossOriginProtection`

Go 1.25 added [`net/http.CrossOriginProtection`][] (see Go 1.25 release
notes, `net/http`). It implements CSRF / cross-origin request
protection using the browser's `Sec-Fetch-Site` and `Origin` headers
instead of double-submit cookies, which removes the need for:

[`net/http.CrossOriginProtection`]: https://pkg.go.dev/net/http#CrossOriginProtection

- the `middleware.CSRFWithConfig` block in
  `internal/server/server.go`,
- the `_csrf` cookie,
- the `csrfToken(c)` helper and the `c.Get("csrf")` reflective lookup
  it does (the silent `string`-cast fallback was the original code
  smell that prompted this task),
- and the `CSRF` field on every page view struct (`homeView`,
  `loginView`, `signupView`, `adminInvitesView`, `adminUsersView`,
  `adminUserView`, `inviteRowView`).

Subtasks:

- Bump `go.mod` (already on `go 1.26.2`, so the API is available; no
  toolchain change needed) and confirm `go build` succeeds against
  `http.NewCrossOriginProtection`.
- Wrap the Echo handler returned by `Server.Echo()` with
  `http.NewCrossOriginProtection().Handler(...)` in `cmd/huck`'s
  `runServe`, *or* mount the protection inside `installMiddleware` as
  an Echo middleware adapter — pick whichever sits more naturally with
  Echo v5's `StartConfig` flow. Document the choice next to the call.
- Remove `middleware.CSRFWithConfig` and the `_csrf` cookie config.
- Remove `csrfToken`, `currentClaims`-style `c.Get` lookups for CSRF,
  and the `CSRF` template inputs. Update the templates to drop the
  hidden `_csrf` form fields and the `htmx.config.headers` mirror, if
  any.
- Verify `Sec-Fetch-Site` / `Origin` reach the server in our local
  dev setup: HTMX issues normal `fetch`-shaped requests, so the
  browser will set those headers automatically. The middleware
  defaults are fine for a same-origin app like Huck; we should not
  need `AddTrustedOrigin` until / unless we ever serve the frontend
  from a different host.
- Document the trade-off in `docs/DESIGN.md` §12 (and the new §X for
  CSRF) so a future maintainer doesn't reintroduce a token mechanism:
  Huck deliberately relies on the stdlib's
  `CrossOriginProtection` plus `SameSite=Lax` on the `auth` cookie,
  with HSTS for the HTTP→HTTPS edge case (already shipped). Note the
  small residual risk window (Firefox v60–69 / pre-2020 browsers)
  that motivates keeping `SameSite=Lax`.
- Per Alex Edwards' analysis of `http.CrossOriginProtection`
  (<https://www.alexedwards.net/blog/preventing-csrf-in-go>), the
  middleware is most effective when the application requires both
  **HTTPS** and **TLS 1.3 or later**. The TLS-1.3 floor matters
  because the residual CSRF window narrows to "Firefox v60–69 + a
  small set of non-major browsers" once older TLS clients are
  refused. Huck will:
  - **Production:** terminate TLS at Nginx in front of the Go
    process. Document in `docs/DESIGN.md` §12 that the deployment
    contract is "Nginx terminates TLS and is configured to require
    TLS 1.3+." A sample `nginx.conf` snippet (or just the relevant
    `ssl_protocols TLSv1.3;` line) belongs in the deployment notes
    so an operator can confirm their reverse proxy meets the
    contract. We are *not* enforcing TLS 1.3 at the Go listener
    because the listener only ever speaks plaintext to the local
    Nginx.
  - **Local dev / `huck serve` direct:** out of scope for the
    TLS-1.3 contract, since dev runs on `localhost` (which browsers
    treat as a Secure Context regardless of HTTPS, so
    `Sec-Fetch-Site` is still emitted) and we accept the slightly
    larger residual window in exchange for keeping the dev loop
    HTTP-only.
  - Open question: confirm that the production Nginx config we plan
    to ship sets `ssl_protocols TLSv1.3;` (no TLSv1.2 fallback).
    Capture the answer in `docs/DESIGN.md` §12 before closing the
    sprint; if we cannot enforce TLS 1.3 at Nginx, drop a note in
    DESIGN.md acknowledging the wider residual risk and rely more
    heavily on `SameSite=Lax` + HSTS.
- Add a single integration test that POSTs to `/login` with a
  cross-site `Sec-Fetch-Site: cross-site` header and asserts a 403,
  and one that POSTs the same request as `same-origin` and asserts
  the existing happy path.

### T4 — Document `runServe`'s migrate-but-never-create contract

`runServe` currently calls `db.Migrate(pool)` after `db.Open(cfg.DB)`.
That is intentional and matches `huck db migrate` so the operator does
not need to run a separate command on deploy, but the contract is
implicit. The code-smell review flagged this as easy to misread.

- Add a doc comment on `runServe` (in `cmd/huck/main.go`) stating
  that:
  1. `huck serve` *will* apply pending migrations on startup.
  2. `huck serve` *will not* create the database file if it is
     missing — that remains exclusively `huck db create`'s job, per
     the `db.ErrMissing` check.
  3. The two commands share the same migration code path, so running
     `huck db migrate` immediately followed by `huck serve` is a
     no-op the second time.
- Mirror the same paragraph into `docs/DESIGN.md` §7.2 / §7.3 so the
  design doc is the source of truth.
- Update `AGENTS.md` "Verification before saying 'done'" if the new
  text changes any check (it should not — the existing
  `huck db create` + idempotent `huck db migrate` pair is already
  listed).
- Add a small test (or extend an existing `db_test.go` case) that
  verifies a second `Migrate` call against an already-migrated pool
  returns nil and applies zero new versions.

### T5 — Honour or remove the unused `ctx` on `invites.Store.Consume`

`invites.Store.Consume(ctx, conn, t)` accepts a `context.Context` "for
API symmetry" and never uses it. `sqlitex.Execute` does respect cancel
via `conn.SetInterrupt`. Either:

- Honour the ctx (the signup transaction in `handleSignupSubmit` can
  set the interrupt channel on `conn` once with
  `conn.SetInterrupt(ctx.Done())` so every `sqlitex.Execute` inside
  the transaction is cancellable), **or**
- Drop the parameter from `Consume`'s signature.

Pick the first; it's the same fix the rest of the stores can adopt
later. Update doc comments accordingly. No new public API.

### T6 — Single shared time-format helper / template funcmap

The `"2006-01-02 15:04 UTC"` literal plus its `time.RFC3339Nano` ISO
sibling are duplicated across `admin_invites.go` (`rowViewAt`),
`admin_users.go` (list rows + `newAdminUserView`).

- Add a small `fmtUTC(t time.Time) (display, iso string)` helper in
  `internal/server` (or expose it to templates as a `funcmap` entry
  named e.g. `utc`).
- Replace the four hand-formatted call sites.
- One test asserting the format strings, so a future change is
  loud.

### T7 — Single HTMX-redirect helper

The "if `HX-Request: true` → set `HX-Redirect`, return 204; else 303
redirect" snippet appears in `handleLoginSubmit`, `handleLogout`,
`handleSignupSubmit`, and `requireAuth`. The renderer already enforces
"don't branch on `HX-Request` in handlers"; redirects should follow
the same rule.

- Add `func hxRedirect(c *echo.Context, path string) error` next to
  the renderer (or on `Server`).
- Replace the four call sites.
- Cover with a single table-driven test (HTMX vs. non-HTMX).

### T8 — Single password-error-message helper

`signupErrorMessage` (in `server.go`) and `adminUserPasswordErrorMessage`
(in `admin_users.go`) repeat the same `auth.ErrPasswordTooShort /
TooLong / NotPrintable` mapping with the same wording.

- Extract `passwordErrMsg(err error) string` (private to
  `internal/server`) and call it from both. Keep the broader
  signup-specific switch in `signupErrorMessage` and have it delegate
  to `passwordErrMsg` for the password sentinels.

### T9 — Shared `Normalise` for handle/email lowercasing

`internal/users.Normalise` and `internal/invites.normaliseEmail` are
the same function in two packages; the latter exists only to avoid an
import cycle that doesn't actually exist today.

- Either: have `internal/invites` import `internal/users` and call
  `users.Normalise` (simple, current direction of flow), **or**
- Move `Normalise` to a tiny third package (`internal/strs` or
  `internal/textnorm`) imported by both.

Prefer the first; only escalate to a third package if a future cycle
is concretely likely. Add a comment naming the chosen rationale so
the next reviewer doesn't undo it.

### T10 — Robust UNIQUE-constraint error classification

`users.classifyInsertErr` and `invites.classifyInsertErr` both
disambiguate UNIQUE failures by `strings.Contains(err.Error(),
"users.handle")` / `"invites.email"`. That's silently coupled to the
SQLite error-message format and to the column names.

- Switch to `sqlite.ExtendedErrCode` (`SQLITE_CONSTRAINT_UNIQUE`) plus
  an explicit pre-check inside the same transaction
  (`SELECT 1 FROM users WHERE handle = ?`) to disambiguate
  `ErrHandleTaken` vs. `ErrEmailTaken`.
- Add a unit test that exercises both UNIQUE paths and would fail
  loudly if SQLite's wording ever drifted (or the columns ever moved).

### T11 — Drop or fix the `/admin` index redirect

`admin.GET("", s.handleAdminIndex)` is mounted to redirect to
`/admin/invites`. Echo will not match `/admin/` for that route, and
the round-trip is wasted on the operator's most common entry point.

- Drop `handleAdminIndex` and link directly to `/admin/invites` from
  the home view, or
- Keep it but also handle the trailing-slash case with a single
  redirect helper.

Pick the simpler option (drop the handler); the home page already
knows the destination.

### T12 — Tighten exported surface in `internal/auth`

`MinHandleLen`, `MaxHandleLen`, and `DefaultTokenTTL` are exported but
consumed only within `internal/auth` (and tests). Anything we don't
share across packages should be lowercase to keep the public API
honest.

- Walk the package, lower-case unused exports, and verify nothing
  outside the package referenced them.

### T13 — Polish notes (small, batch into one PR)

The following are too small for their own task headings but are part of
the same burndown:

- Drop `inviteRowView.CSRF` if T3 hasn't already removed every CSRF
  template field — verify the partial doesn't reference it.
- Confirm `templates/email/invite.html` uses `{{ .URL }}` only inside
  an `href` attribute (HTML-safe context), not inside JS or CSS.
- Comment in `internal/config/config.go` that the `Config` struct is
  intentionally one bag for now and call out the threshold (~25
  fields) at which we'd split into per-subcommand sub-structs.
- Sweep for lingering "Sprint 1 unused" / "Sprint 2 unused" comments
  that are now obsolete after Sprint 3 lands.

## Out of scope

- Any new feature work. Sprint 3 is tech-debt burndown only.

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new T3 cross-origin tests
  and the new T4 idempotent-migrate test).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint3.db` succeeds on a fresh
  path, and `huck db migrate --db /tmp/huck-sprint3.db` is a no-op
  the second time.
- `cmd/sendtest` (after T2) sends a real message through the
  Mailgun sandbox using only env vars / `.env.development.local`
  and prints a success line.
- Manual smoke-test of `huck serve`: log in, create an invite, sign
  up via the emailed link, and confirm no `_csrf` cookie or hidden
  form field is present in the rendered HTML (T3).

## Change log

- **2026-05-09** — Drafted from the post–Sprint 2 code-smell review.
