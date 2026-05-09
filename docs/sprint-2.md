# Sprint 2 — Implementation Plan

Status: **Ready to start — design locked 2026-05-09.**

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
  using `github.com/mailgun/mailgun-go/v4`. A no-op/fake `Mailer` for
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
  (e.g. `https://api.eu.mailgun.net/v3`); empty value means "use the
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

## References

- [docs/DESIGN.md](DESIGN.md) §6 (config), §7.4 (schema), §8.7 (password
  policy), §8.8 (handle policy), §9 (invite flow), §10 (routes),
  §11 (templates and HTMX), §14 (logging).
- [docs/sprint-1.md](sprint-1.md) §3 for the original deferral list.
