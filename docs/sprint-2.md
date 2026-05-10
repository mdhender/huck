# Sprint 2 — Implementation Plan

Status: **Closed 2026-05-09.** All tasks (T1–T8) landed on `main`; manual smoke test passed.

Sprint 1 deferred the following items, all of which Sprint 2 picks up.
The detailed contracts live in `docs/DESIGN.md` (§7.4 schema, §8.7
password policy, §8.8 handle policy, §9 invite flow, §10 routes); this
file covers sprint-specific scope, sequencing, and hazards.

## In scope

- `internal/invites` — `Store`, token generation (32 bytes from
  `crypto/rand`, base64url-encoded), `Create`, `Resend`, `Revoke`,
  `Consume`, sentinel errors. The schema's `UNIQUE` partial index on
  `invites(email) WHERE consumed_at IS NULL` enforces "one active
  invite per email"; `Create` maps the SQLite uniqueness violation to
  `ErrEmailAlreadyInvited` so the admin endpoint can return 409.

- `internal/auth` validators — extract the password policy (§8.7) and
  handle policy (§8.8) into `internal/auth` (or a small `validate`
  helper) and use them in **both** `huck admin create` *and*
  `POST /signup/:token`. Sprint 1's ad-hoc `len >= 8` admin check is
  replaced.

- Bootstrap-DB rebuild. The `invites_email_active` index is being
  promoted from non-unique to unique by editing `migrations/0001_init.sql`
  in place. We are pre-alpha and have not released; operators (i.e. the
  developer) drop the local SQLite file and re-run `huck db create`. The
  password-policy change to `huck admin create` is verified at the same
  time.

- `internal/mail` — `Mailer` interface plus a Mailgun implementation
  using `github.com/mailgun/mailgun-go/v5`. A no-op/fake `Mailer` for
  tests. The Mailgun impl calls `mg.SetAPIBase(cfg.MailgunAPIBase)`
  only when the value is non-empty, so US users (the default) need not
  set it. Calls are **synchronous**: a Mailgun failure on
  `POST /admin/invites` returns 5xx to the admin and the invite row is
  rolled back so the operator can retry.

- `web/templates/email/` — the invite email template lives here.
  Sprint 2 ships a single **HTML** template with subject
  `"Welcome to Huck!"` and a body that contains the
  `${BASE_URL}/signup/<token>?email=<urlencoded>` link. (Plain-text
  multipart can come later.)

- `pages/signup.html` and `GET`/`POST /signup/:token` — the only
  registration entry point. The form has three fields:
    - `email` — pre-filled from the invite row and rendered as a
      **visible, `readonly`** input so the recipient can see which
      address the invite is bound to (and copy it for support
      purposes). Its value submits with the form for a server-side
      defence-in-depth re-check against the invite row.
    - `handle` — validated against §8.8.
    - `password` — validated against §8.7.
  The submit handler runs token re-validation, email re-check, user
  insert, and `consumed_at` update **inside one SQLite transaction**
  (DESIGN.md §9 step 5); zombiezen's `sqlitex.Transaction` is the
  intended primitive.

- Admin pages — both rendered via the existing layout/Pico/HTMX stack:
    - `/admin/invites` — list + create form + per-row resend/revoke
      using `partials/invite_row.html`.
    - `/admin/users` — list / view / edit / delete.
        - **Edit** = toggle `is_admin` and reset password (handle and
          email are immutable post-creation; changing them is risky and
          out of scope).
        - **Delete** = hard delete from `users`.
        - **Create** is *not* a parallel path — admins create users by
          generating an invite at `/admin/invites`. The bootstrap admin
          path remains `huck admin create` on the CLI.
        - The UI prevents an admin from demoting or deleting **themself**
          (best-effort guard against accidentally locking everyone out).

- `internal/server` — route guards `RequireAuth` (echo-jwt) and
  `RequireAdmin` (claims.Admin == true).

- `internal/config` — per-command validation for `serve` now requires
  `--mailgun-domain`, `--mailgun-api-key`, `--mailgun-from`, and
  `--base-url` (the Sprint 1 grace period ends). Add an *optional*
  `--mailgun-api-base` / `HUCK_MAILGUN_API_BASE` flag for EU customers
  (e.g. `https://api.eu.mailgun.net` — mailgun-go/v5 rejects a
  version suffix on the base URL); empty value means "use the
  SDK default", which is US.

## Out of scope (likely Sprint 3+)

- A `/health` endpoint (still only when a probe needs it).
- Refresh tokens. The "rotate `--jwt-secret` to invalidate everyone"
  model is intentional.
- Roles beyond the `users.is_admin` boolean.
- Server-side session/revocation table.
- **Mailgun integration test against the real API.** The fake `Mailer`
  covers handler tests, which is the actual MVP-blocking coverage.
  Add a `t.Skip` plus a `// TODO(sprint-3): wire HUCK_TEST_MAILGUN_*`
  comment in the integration test stub and revisit in Sprint 3 once
  Mailgun's sandbox API key handling is figured out.
- `zxcvbn` password-strength scoring (optional per §8.7; non-blocking).
- Plain-text multipart for the invite email.
- Editing a user's handle or email from `/admin/users`.

## Hazards to keep in mind

- **Token storage.** Invite tokens are random and large (≥256 bits),
  but they are secrets in URLs and email bodies. Discipline-based
  redaction per DESIGN.md §14: don't pass tokens to `slog`, and consider
  a typed `invites.Token` with `LogValue() slog.Value` returning a
  redacted form. There is no automated slog filter; reviewers catch
  slips.

- **Mailgun in tests.** Use the fake `Mailer` in the handler tests.
  The real Mailgun integration test is deferred (see Out of scope).

- **Email case.** Lowercase before insert and before `WHERE email = ?`
  comparisons (the `users` package already does this; mirror the
  pattern in `invites`).

- **Transaction boundary on signup.** Token re-validation, email
  re-check, user insert, and invite consumption must all happen in one
  zombiezen transaction. Without it, two parallel form submits can
  both pass validation and race the inserts.

- **HTMX swaps for admin invite list.** Use `partials/invite_row.html`
  for resend/revoke responses so the admin page only redraws the
  affected row.

- **Self-lockout guard.** `/admin/users` must not let an admin demote
  or delete the row whose ID matches `claims.sub`. If an operator
  bypasses the UI and locks themselves out anyway, recovery is
  `huck admin create` after manual SQL.

## Tasks

Worked one-at-a-time with a fresh context per task. See "How to pick up
a task" below. Mark a task with `[x]` when its acceptance criteria
pass and a commit lands on `main`.

- [x] **T1 — Auth validators + admin create.** ✅ `650b599`
  Add `internal/auth/validate.go` (or split into `password.go`
  additions + new `handle.go`) with:
  - `ValidatePassword(pw string) error` enforcing §8.7 (length 12–128,
    printable Unicode + spaces, no character-class rules).
  - `ValidateHandle(h string) error` enforcing §8.8
    (`^[a-z][a-z0-9_,.'-]{2,31}$`, applied **after** lowercasing).
  - Sentinel errors with messages naming the rule that failed.
  Update `cmd/huck/main.go:readAdminPassword` and the `users.NewUser`
  insert path in `huck admin create` to call the new validators (the
  old `len >= 8` check goes away). Add tests covering boundaries and
  representative invalid inputs.
  *Acceptance:* `go test ./internal/auth/... ./internal/users/... ./cmd/...`
  passes; `huck admin create` rejects passwords <12 or >128 with a
  clear message; existing Sprint 1 tests still pass.

- [x] **T2 — Config: promote Mailgun + base-url; add api-base.** ✅ `5503399`
  In `internal/config/config.go`: add `MailgunAPIBase string`. In
  `cmd/huck/main.go:newServeCmd`: register `--mailgun-api-base`. In
  `Config.ValidateServe`: require `--mailgun-domain`,
  `--mailgun-api-key`, `--mailgun-from`, `--base-url`. Update
  `internal/config/config_test.go`.
  *Acceptance:* `go test ./internal/config/` passes; `huck serve`
  without the now-required flags fails fast with a list naming each
  missing flag; `--mailgun-api-base` shows up in `huck serve --help`.

- [x] **T3 — `internal/mail` package.** ✅ `b353b3f`
  `Mailer` interface: `Send(ctx context.Context, to, subject, htmlBody string) error`.
  `FakeMailer` (records sent messages, used by tests).
  `MailgunMailer` impl using `github.com/mailgun/mailgun-go/v5`; calls
  `mg.SetAPIBase(cfg.MailgunAPIBase)` only when non-empty. Wire the
  chosen `Mailer` into `server.New` (interface dependency, not a
  concrete type) so the server-test can inject the fake. Do *not*
  call it from any handler yet — that's T6.
  *Acceptance:* package builds; `go test ./internal/mail/...` passes
  (fake mailer round-trip); the real Mailgun integration test stub
  exists with `t.Skip("TODO(sprint-3): wire HUCK_TEST_MAILGUN_*")`.

- [x] **T4 — `internal/invites` package.** ✅ `ae7556a`
  `Token` type with `Generate() (Token, error)` (32 bytes from
  `crypto/rand`, base64url). `Invite` model. `Store` methods:
  - `Create(ctx, email string, invitedBy int64) (Invite, error)` —
    maps the SQLite `UNIQUE` violation on `invites_email_active` to
    `ErrEmailAlreadyInvited`.
  - `GetByToken(ctx, Token) (Invite, error)` (`ErrNotFound`).
  - `Resend(ctx, Token) (Invite, error)` — `expires_at = now + 7d`;
    rejects consumed invites.
  - `Revoke(ctx, Token) error`.
  - `Consume(ctx, conn *sqlite.Conn, Token) error` — takes an explicit
    `*sqlite.Conn` so callers can run it inside their own transaction
    (the signup handler in T5 needs this).
  Sentinel errors: `ErrNotFound`, `ErrExpired`, `ErrConsumed`,
  `ErrEmailAlreadyInvited`. Tests against in-memory SQLite, including
  the unique-active-invite enforcement and the expired/consumed paths.
  Treat `Token` as a secret in any future logging (typed value with
  `LogValue() slog.Value` is encouraged but not required this sprint).
  *Acceptance:* `go test ./internal/invites/...` passes.

- [x] **T5 — Email template + signup flow (single transaction).** ✅ `2c41f43`
  `web/templates/email/invite.html` — HTML invite body with subject
  `Welcome to Huck!` and link `${BASE_URL}/signup/<token>?email=<urlencoded>`.
  `web/templates/pages/signup.html` — three fields per §9 step 4:
  visible-readonly `email`, `handle`, `password`. Renderer additions
  to load `templates/email/*.html` separately from the page/partial
  paths (no layout). Handlers in `internal/server`:
  - `GET /signup/:token` — best-effort token lookup, render form (or
    error page on missing/expired/consumed).
  - `POST /signup/:token` — entire pipeline inside one
    `sqlitex.Transaction` per §9 step 5: re-validate token; re-check
    email match; validate handle (T1) + password (T1); insert user
    with `is_admin=0`; `invites.Consume(...)`. After commit, issue
    JWT + set auth cookie + redirect (or `HX-Redirect` for HTMX).
  Add httptest coverage for: golden path, expired token, consumed
  token, email mismatch, handle taken, weak password, two-parallel-
  submits race (one wins, one fails cleanly).
  *Acceptance:* `go test ./internal/server/...` passes; manual run
  with the fake mailer can complete a signup end-to-end.

- [x] **T6 — Route guards + admin invite pages.** ✅ `4206d03`
  `RequireAuth` (the existing `echo-jwt` middleware) and `RequireAdmin`
  (wrap, then check `claims.Admin`, else 403) in `internal/auth` (or
  `internal/server/middleware.go` — match whichever the existing layout
  prefers). `web/templates/pages/admin_invites.html` (list + create
  form, semantic Pico HTML, HTMX `hx-post`/`hx-target` on rows).
  `web/templates/partials/invite_row.html`. Handlers:
  - `GET /admin/invites` — list (most recent first; show status:
    pending / expired / never-been-consumed-because-revoked).
  - `POST /admin/invites` — `Create` + Mailgun `Send`, **synchronous**;
    on Mailgun error, the transaction rolls back and the response is
    5xx so the admin sees the failure.
  - `POST /admin/invites/:token/resend` — `Resend` + send again.
  - `POST /admin/invites/:token/revoke` — `Revoke`, return the row's
    new state via `partials/invite_row.html` for HTMX swap.
  Tests for each route + 409 on duplicate active invite.
  *Acceptance:* `go test ./internal/server/...` passes; admin can
  list/create/resend/revoke through the UI; fake mailer captures
  bodies in tests.

- [x] **T7 — Admin user pages (list/view/edit/delete).** ✅ `7158c82`
  Templates: `pages/admin_users.html` (list), `pages/admin_user_edit.html`
  (toggle `is_admin`, reset password). Handlers:
  - `GET /admin/users` — list (handle, email, is_admin, created_at).
  - `GET /admin/users/:id` — view (read-only summary).
  - `GET /admin/users/:id/edit` — edit form.
  - `POST /admin/users/:id/edit` — apply is_admin toggle and/or
    password reset (validated by T1); refuse if `id == claims.sub`
    and the change would demote.
  - `POST /admin/users/:id/delete` — hard delete; refuse if
    `id == claims.sub`.
  Tests for each route + the two self-lockout guards.
  *Acceptance:* `go test ./internal/server/...` passes; admin can
  walk the four operations against another user.

- [x] **T8 — Manual smoke test + README.** ✅ `33d5bb9`
  Drop and rebuild the dev DB, run `huck admin create`, log in, issue
  an invite (real Mailgun this time, not fake), click the email link,
  sign up as a non-admin, log out, log back in as the new user,
  switch back to admin to exercise resend/revoke and the
  /admin/users edit/delete flows. Update `README.md` quickstart if
  any commands have changed shape. No CSP violations in Chrome or
  Firefox dev-tools.
  *Acceptance:* end-to-end loop works in both browsers; README
  quickstart matches reality.

## How to pick up a task

Each task is meant to be implemented in a fresh Claude Code session.
On entering a session:

1. Read `CLAUDE.md`, then the `AGENTS.md` "Hard rules" section, then
   the `docs/DESIGN.md` sections cited in the task.
2. Read this task block end-to-end. Re-read the "In scope" section
   above for surrounding context if needed.
3. Run `git log --oneline -10` to see the most recent landed work,
   and `git status` to make sure the tree is clean.
4. Implement, run `go build ./... && go vet ./... && go test ./...`,
   and commit with a message naming the task (`T3: internal/mail …`).
5. Update this file: change the task's `[ ]` to `[x]`, add the commit
   hash inline (e.g. `**T1 — …** ✅ `<short-sha>``), and commit that
   doc edit too.

## References

- [docs/DESIGN.md](DESIGN.md) §6 (config), §7.4 (schema), §8.7 (password
  policy), §8.8 (handle policy), §9 (invite flow), §10 (routes),
  §11 (templates and HTMX), §14 (logging).
- [docs/sprint-1.md](sprint-1.md) §3 for the original deferral list.
