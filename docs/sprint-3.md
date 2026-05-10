# Sprint 3 — Implementation Plan

Status: **Ready 2026-05-10.**

Sprint 3 is a clean-up and consolidation sprint driven by the code-smell
review of the post–Sprint 2 codebase. Almost every task either removes
duplication, deletes dead code, or swaps a hand-rolled mechanism for a
stdlib equivalent. The only new user-visible surface is T11's minimal
`/admin` and `/account` pages, added to settle navigation contracts
before the layout sprint.

The detailed contracts (schema, password/handle policies, invite flow,
routes, security headers) continue to live in `docs/DESIGN.md`. This
file is a sprint plan — when a task changes a contract, DESIGN.md is
the document to update first.

## Agent Execution Rules

Work exactly one task row at a time.

Task selection:

1. Select the first task with `Status = TODO`
2. If a dependency is not satisfied, mark the task `BLOCKED`, explain
   why in `Notes`, and stop
3. Do not start a later task unless instructed by a human

Workflow for one task:

1. Mark the selected task `IN_PROGRESS`
2. Implement only that task and any directly required tests/docs
3. Run the repo verification required by `AGENTS.md`:
   `go build ./...`, `go test ./...`, and `go vet ./...`
4. If narrower validation is intentionally used, record why in `Notes`
5. Update the sprint row:
   - `Status = DONE` if implementation and verification completed
   - `Status = BLOCKED` if the task cannot proceed
   - `Status = REVIEW` only when code is complete but human
     confirmation is explicitly needed before marking the task done
   - `Notes` should briefly name important implementation choices,
     skipped checks, or blockers
6. Commit all task changes, including the sprint table update, as one
   focused atomic commit
7. Report the commit hash in the final response
8. Stop and wait for further instruction

Allowed status values:

- `TODO` — not started
- `IN_PROGRESS` — actively being implemented
- `BLOCKED` — cannot proceed without intervention
- `REVIEW` — complete enough for human verification, not marked done
- `DONE` — implemented, verified, and committed

Sprint task table format:

| Task | Status | Commit | Notes |
|------|--------|--------|-------|

The `Commit` column records the landed task commit hash for traceability,
matching the hash convention used in earlier sprint docs. A task commit
cannot contain its own hash, so agents should report the hash after
committing. Fill the column only if explicitly asked to make a separate
bookkeeping update, or if a human updates it afterward.

Rules:

- Never work on multiple task rows simultaneously
- Never modify unrelated files
- Never skip updating the sprint table
- Never continue automatically after completing a task
- Prefer small, isolated, reversible commits
- Record blockers explicitly in `Notes`
- Preserve the sprint plan as the source of truth for progress

---

## In scope tasks

| Task | Status | Commit | Notes |
|------|--------|--------|-------|
| T1   | TODO   |        |       |
| T2   | TODO   |        |       |
| T3.1 | TODO   |        |       |
| T3.2 | TODO   |        |       |
| T4   | TODO   |        |       |
| T5   | TODO   |        |       |
| T6   | TODO   |        |       |
| T7   | TODO   |        |       |
| T8   | TODO   |        |       |
| T9   | TODO   |        |       |
| T10  | TODO   |        |       |
| T11  | TODO   |        |       |
| T12  | TODO   |        |       |
| T13  | TODO   |        |       |
| T14  | TODO   |        |       |
| T15  | TODO   |        |       |

Task order encodes the known dependencies: T2 follows the mail package
collapse in T1, T3.2 follows the CrossOriginProtection install in
T3.1, T14 waits for the CSRF, redirect, and app-route cleanup baseline,
and T15 closes the Sprint 4 docs once that code baseline is settled.

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

### T3.1 — Install `http.CrossOriginProtection`

Go 1.25 added [`net/http.CrossOriginProtection`][] (see Go 1.25 release
notes, `net/http`). It implements CSRF / cross-origin request
protection using the browser's `Sec-Fetch-Site` and `Origin` headers
instead of double-submit cookies.

[`net/http.CrossOriginProtection`]: https://pkg.go.dev/net/http#CrossOriginProtection

- Bump `go.mod` (already on `go 1.26.2`, so the API is available; no
  toolchain change needed) and confirm `go build` succeeds against
  `http.NewCrossOriginProtection`.
- Wrap the Echo handler returned by `Server.Echo()` with
  `http.NewCrossOriginProtection().Handler(...)` in `cmd/huck`'s
  `runServe`, *or* mount the protection inside `installMiddleware` as
  an Echo middleware adapter — pick whichever sits more naturally with
  Echo v5's `StartConfig` flow. Document the choice next to the call.
- Remove `middleware.CSRFWithConfig` and the `_csrf` cookie config.
- Keep this task focused on server-side request protection. If removing
  the old middleware leaves compile-time references to `csrfToken` or
  `CSRF` view fields, make the smallest temporary compatibility change
  needed and leave the full template/view cleanup to T3.2.
- Add a single integration test that POSTs to `/login` with a
  cross-site `Sec-Fetch-Site: cross-site` header and asserts a 403,
  and one that POSTs the same request as `same-origin` and asserts
  the existing happy path.
- Verify `Sec-Fetch-Site` / `Origin` reach the server in our local
  dev setup: HTMX issues normal `fetch`-shaped requests, so the
  browser will set those headers automatically. The middleware
  defaults are fine for a same-origin app like Huck; we should not
  need `AddTrustedOrigin` until / unless we ever serve the frontend
  from a different host.

### T3.2 — Remove double-submit CSRF token plumbing

With T3.1 in place, delete the old token mechanism end-to-end:

- Remove the `csrfToken(c)` helper and the `c.Get("csrf")` reflective
  lookup it does (the silent `string`-cast fallback was the original
  code smell that prompted this task).
- Remove the `CSRF` field on every page view struct (`homeView`,
  `loginView`, `signupView`, `adminInvitesView`, `adminUsersView`,
  `adminUserView`, `inviteRowView`).
- Update the templates to drop the hidden `_csrf` form fields and the
  `htmx.config.headers` mirror, if any.
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
- Update existing form/HTMX tests so they no longer fetch or submit
  `_csrf` values. Keep the T3.1 cross-origin tests green.

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

### T6 — Single shared time-format helper / template FuncMap

The `"2006-01-02 15:04 UTC"` literal plus its `time.RFC3339Nano` ISO
sibling are duplicated across `admin_invites.go` (`rowViewAt`),
`admin_users.go` (list rows + `newAdminUserView`).

- Add a small `fmtUTC(t time.Time) (display, iso string)` helper in
  `internal/server` (or expose it to templates as a `FuncMap` entry
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

### T11 — Add real `/admin` and `/account` pages

`/admin` and `/account` keep coming up in layout reviews because they
are navigation-level destinations but are not real pages yet. Make them
real now so Sprint 4 can wire stable sidebar links without special
cases.

- Replace the `/admin` redirect with an empty admin dashboard page.
  It may contain only a title/placeholder for now; the route exists
  because `/admin` will eventually be the admin dashboard.
- Handle both `/admin` and `/admin/` consistently. Prefer one canonical
  page route plus the smallest redirect needed for the trailing-slash
  form.
- Add `/account` for signed-in users. For now, it shows the same content
  and data shape as the admin `/admin/users/:id` detail page, scoped to
  the current user.
- Wire the signed-in home page to `/admin` for admins and `/account` for
  account navigation. Sprint 4's sidebar should be able to link to both
  without dead-link exceptions.
- Add route/render tests for `/admin`, `/admin/`, and `/account`.

No schema or account-editing behavior is in scope.

### T12 — Tighten exported surface in `internal/auth`

`MinHandleLen`, `MaxHandleLen`, and `DefaultTokenTTL` are exported but
consumed only within `internal/auth` (and tests). Anything we don't
share across packages should be lowercase to keep the public API
honest.

- Walk the package, lowercase unused exports, and verify nothing
  outside the package referenced them.

### T13 — Polish notes (small, batch into one PR)

The following are too small for their own task headings but are part of
the same burndown:

- Confirm `templates/email/invite.html` uses `{{ .URL }}` only inside
  an `href` attribute (HTML-safe context), not inside JS or CSS.
- Comment in `internal/config/config.go` that the `Config` struct is
  intentionally one bag for now and call out the threshold (~25
  fields) at which we'd split into per-subcommand sub-structs.
- Sweep for lingering "Sprint 1 unused" / "Sprint 2 unused" comments
  that are now obsolete after Sprint 3 lands.

### T14 — Pre-Sprint-4 code alignment

Remove the small code/doc mismatches that would otherwise turn into
layout-sprint speed bumps. This should run after T3.2, T7, and T11 so
the renderer and template baseline is already stable.

- Fix the Sprint 4 user-detail route examples to match the actual route
  contract: admin user pages are `/admin/users/:id` and
  `/admin/users/:id/edit`, not `:handle`. Breadcrumb labels should still
  display the user's handle; only the URL parameter is numeric.
- Split `homeView` into separate `homePublicView` and `homeAuthedView`
  structs. `home_public.html` moves to the auth shell in Sprint 4 and
  should not carry app-shell fields; `home_authed.html` moves to the app
  shell and needs the signed-in handle/admin state plus future breadcrumb
  context. Keeping one shared struct makes the shell split harder to
  reason about.
- Add a small renderer smoke test before Sprint 4 changes layout
  dispatch: rendering a page template should go through the current
  layout, and rendering a partial template should render the partial
  alone. Sprint 4 T2.2 can then update that baseline instead of adding
  coverage from scratch while changing the renderer.

### T15 — Front-end contract readiness for Sprint 4

Sprint 4 should begin with the front-end contracts settled, not with
implementation-time ambiguity. This is a doc-only readiness task: do
not build the new shells, templates, partials, or CSS primitives here.
Run this after T14 so the Sprint 4 docs match the actual Sprint 3
baseline.

- Verify the app-shell data contract in `docs/front-end-plan.md` and
  `docs/sprint-4.md` still matches the Sprint 3 baseline: breadcrumbs
  live in typed shell data, and the renderer may compose that shell data
  with each page view once.
- Confirm `docs/front-end-plan.md` and `docs/sprint-4.md` agree that
  `/admin` is a real admin dashboard page and `/account` is a real
  signed-in page before Sprint 4 starts.
- Fix small wording typos while touching the Sprint 4 docs, if any are
  present in the checked-in text.

## Out of scope

- Any feature work beyond T11's minimal `/admin` and `/account` pages.
  Sprint 3 is otherwise tech-debt burndown only.

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new T3.1 cross-origin tests,
  the new T4 idempotent-migrate test, and the new T14 renderer smoke
  test).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint3.db` succeeds on a fresh
  path, and `huck db migrate --db /tmp/huck-sprint3.db` is a no-op
  the second time.
- `cmd/sendtest` (after T2) sends a real message through the
  Mailgun sandbox using only env vars / `.env.development.local`
  and prints a success line.
- Manual smoke-test of `huck serve`: log in, create an invite, sign
  up via the emailed link, and confirm no `_csrf` cookie or hidden
  form field is present in the rendered HTML (T3.2).
- Manual route smoke-test: `/admin`, `/admin/`, and `/account` render
  for authorized users and reject unauthorized users correctly.

## Change log

- **2026-05-09** — Drafted from the post–Sprint 2 code-smell review.
- **2026-05-10** — Made `/admin` and `/account` real Sprint 3 pages,
  trimmed duplicate Sprint 4 checklist work from T15, and refreshed
  stale Sprint 4 task references.
