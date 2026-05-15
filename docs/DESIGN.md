# huck — Design Document

Status: **Accepted** — 2026-05-09.
Last updated: 2026-05-09.

## 1. Purpose

`huck` is a small, self-hosted Go web server intended for a user base of
fewer than 50 people. It serves server-rendered HTML pages augmented with
[HTMX](https://htmx.org/) and [AlpineJS](https://alpinejs.dev/), backed by
SQLite. Authentication is invite-only and uses JWT in an HttpOnly cookie.

This document captures the agreed design. Code should follow it; deviations
should update this document first.

## 2. Goals and non-goals

### Goals

- Single static binary, no CGO.
- Pure-Go SQLite via `zombiezen.com/go/sqlite`.
- Simple operational story: one process, one SQLite file, env-var config.
- Server-rendered HTML; the browser does HTMX swaps and small Alpine
  interactions, nothing more.
- Invite-only registration, audited via the `invites` table.

### Non-goals

- Horizontal scaling. SQLite + a single process is the deployment model.
- Public sign-up.
- OAuth / SSO / external identity providers.
- A full SPA. No bundler, no React, no build step for the frontend.
- Refresh-token rotation, server-side session storage, or per-user JWT
  revocation lists. The user base is small; rotating `--jwt-secret`
  invalidates everyone if needed.

## 3. Stack

| Concern         | Library                                       |
| --------------- | --------------------------------------------- |
| HTTP framework  | `github.com/labstack/echo/v5` (v5.1.1+)       |
| JWT middleware  | `github.com/labstack/echo-jwt/v5`             |
| JWT primitives  | `github.com/golang-jwt/jwt/v5`                |
| SQLite          | `zombiezen.com/go/sqlite` + `…/sqlitex`       |
| Flags/commands  | `github.com/peterbourgon/ff/v4`               |
| Email           | Mailgun HTTP API via internal stdlib client   |
| Password hash   | `golang.org/x/crypto/bcrypt` (cost 12)        |
| Templates       | stdlib `html/template`                        |
| Frontend assets | HTMX, AlpineJS, Pico.css (vendored in `web/static`) |

Echo v5 and `echo-jwt/v5` are the current stable releases as of May 2026.

## 4. Project layout

```
huck/
├── cmd/huck/main.go                 # ff/v4 root command + subcommand wiring
├── internal/
│   ├── config/config.go             # Config struct, flag/env/file binding
│   ├── db/
│   │   ├── db.go                    # open existing DB only; PRAGMAs
│   │   ├── create.go                # create-new-DB (huck db create)
│   │   └── migrate.go               # apply embedded migrations
│   ├── server/
│   │   ├── server.go                # Echo setup, dependency wiring
│   │   ├── render.go                # html/template Renderer; HX-Request
│   │   ├── middleware.go            # CSRF, security headers, request log
│   │   └── errors.go                # error → HTML/JSON
│   ├── auth/
│   │   ├── password.go              # bcrypt hash/compare
│   │   ├── token.go                 # JWT issue/verify, Claims
│   │   ├── middleware.go            # echo-jwt config; RequireAdmin
│   │   └── handlers.go              # GET/POST /login, POST /logout
│   ├── users/
│   │   ├── model.go
│   │   └── store.go                 # CRUD over zombiezen
│   ├── invites/
│   │   ├── token.go                 # 32-byte crypto/rand, base64url
│   │   ├── store.go                 # CRUD; consume; resend
│   │   └── handlers.go              # admin invite mgmt + public signup
│   └── mail/
│       ├── mail.go                  # Mailer interface
│       └── mailgun.go               # Mailgun implementation
├── migrations/
│   ├── 0001_init.sql
│   └── embed.go                     # //go:embed *.sql
├── web/
│   ├── templates/
│   │   ├── layout.html              # base; "title", "content", "scripts"
│   │   ├── pages/
│   │   │   ├── home_public.html     # "what is huck" landing, anon visitors
│   │   │   ├── home_authed.html     # "welcome to huck" home, signed-in users
│   │   │   ├── login.html
│   │   │   ├── signup.html          # "create your account" (invite landing)
│   │   │   └── admin/
│   │   │       ├── invites.html
│   │   │       └── users.html
│   │   └── partials/
│   │       └── invite_row.html
│   ├── static/
│   │   ├── htmx.min.js
│   │   ├── alpine.min.js
│   │   ├── pico.min.css
│   │   └── app.css                  # project-specific overrides only
│   └── embed.go                     # //go:embed templates static
├── docs/
│   └── DESIGN.md                    # this file
├── AGENTS.md
├── README.md
├── go.mod
└── go.sum
```

All application code lives under `internal/` — nothing in this project is
intended for external consumption.

## 5. Commands (`ff/v4`)

```
huck serve            Start the web server.
huck db create        Create a new SQLite file. Errors if the file exists.
huck db migrate       Apply pending migrations to an existing file.
huck admin create     Create the bootstrap admin user. Errors if any admin
                      already exists.
```

All subcommands accept the same global flags. Flag values resolve in this
order: command-line > environment variable > config file > default.

### 5.1 `huck admin create`

Used once, at install time. Reads:

- `--handle` (required)
- `--email` (required)
- Password from `HUCK_ADMIN_PASSWORD` if set, otherwise from stdin (TTY,
  no echo). Confirmed twice when read interactively.

Inserts a user with `is_admin = 1`, lowercasing handle and email. If a row
with `is_admin = 1` already exists, it exits non-zero with a clear message.
This is the only way the first admin enters the system.

## 6. Configuration

Bound with `ff.WithEnvVarPrefix("HUCK")` and (optionally)
`ff.WithConfigFile`/`ff.PlainParser`. All flags are also valid env vars
(`HUCK_<UPPER_SNAKE>`).

| Flag                | Env                     | Required | Default    | Description                                 |
| ------------------- | ----------------------- | -------- | ---------- | ------------------------------------------- |
| `--config`          | —                       | no       | —          | Optional plain config file path.            |
| `--addr`            | `HUCK_ADDR`             | no       | `:8080`    | Listen address.                             |
| `--db`              | `HUCK_DB`               | yes      | —          | Path to SQLite file. Must already exist on `serve`. |
| `--base-url`        | `HUCK_BASE_URL`         | yes      | —          | Public base URL, used to build invite links. |
| `--jwt-secret`      | `HUCK_JWT_SECRET`       | yes      | —          | HMAC key, ≥32 bytes.                        |
| `--cookie-secure`   | `HUCK_COOKIE_SECURE`    | no       | `true`     | Set `Secure` on the auth cookie.            |
| `--cookie-domain`   | `HUCK_COOKIE_DOMAIN`    | no       | —          | Optional cookie `Domain` attribute.         |
| `--mailgun-domain`  | `HUCK_MAILGUN_DOMAIN`   | yes      | —          | Mailgun sending domain.                     |
| `--mailgun-api-key` | `HUCK_MAILGUN_API_KEY`  | yes      | —          | Mailgun private API key.                    |
| `--mailgun-from`    | `HUCK_MAILGUN_FROM`     | yes      | —          | `From:` address used for invite mail (RFC 5322 string, e.g. `huck <a@b>`). |
| `--mailgun-api-base`| `HUCK_MAILGUN_API_BASE` | no       | —          | Mailgun API base URL (no version suffix; Huck appends `/v3/...`). Empty = US default. Set to `https://api.eu.mailgun.net` for EU. |
| `--log-level`       | `HUCK_LOG_LEVEL`        | no       | `info`     | `debug`/`info`/`warn`/`error`.              |

Required flags missing on `serve` cause a fatal error before the listener
starts.

## 7. Database

### 7.1 Driver

`zombiezen.com/go/sqlite` with a connection pool (`sqlitex.Pool`). Pool
size defaults to a small number (e.g. 8) — the user base is tiny.

On every connection the following PRAGMAs are set:

```
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
```

### 7.2 Existence rule

`huck serve` opens the database file with flags that **forbid creation**.
If the file is missing the process exits with a fatal error pointing the
user at `huck db create`. `huck db create` is the only code path that may
create a new file.

This split is deliberate: `huck serve` will *apply* pending migrations on
startup (see §7.3), but it will *not* bootstrap a missing database. The
operator runs `huck db create` once per deployment; `huck serve` then owns
the run-time lifecycle.

### 7.3 Migrations

Migrations live in `migrations/NNNN_<name>.sql` and are embedded with
`//go:embed *.sql`. They are applied in lexical order in a single
transaction each, recording the version in `schema_migrations`.

Migrations are **append-only**. To change a released schema, write a new
migration.

`huck db migrate` runs them explicitly; `huck serve` runs them on startup.
Both commands call the same `db.Migrate` code path, so running
`huck db migrate` immediately followed by `huck serve` is a no-op the
second time — the operator can keep migration as a separate deploy step
without paying for it twice. The unit test
`TestMigrateAfterCreateIsNoOp` in `internal/db` pins this guarantee.

### 7.4 Initial schema (`0001_init.sql`)

```sql
CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    handle        TEXT    NOT NULL UNIQUE,    -- lowercased in Go
    email         TEXT    NOT NULL UNIQUE,    -- lowercased in Go
    password_hash TEXT    NOT NULL,           -- bcrypt
    is_admin      INTEGER NOT NULL DEFAULT 0, -- 0 = user, 1 = admin
    created_at    TEXT    NOT NULL,           -- ISO-8601 UTC
    updated_at    TEXT    NOT NULL
);

CREATE TABLE invites (
    token        TEXT PRIMARY KEY,            -- 32 bytes, base64url
    email        TEXT NOT NULL,               -- lowercased
    invited_by   INTEGER NOT NULL REFERENCES users(id),
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,               -- = created_at + 7d
    consumed_at  TEXT                          -- NULL until used
);
CREATE UNIQUE INDEX invites_email_active
    ON invites(email) WHERE consumed_at IS NULL;
-- One active invite per email; admin must revoke before re-inviting.

CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);
```

Lowercasing is enforced in Go; uniqueness is enforced in SQL. The DB row
matches what the user sees.

**Sprint 5 columns (migrations `0002_user_status.sql` and
`0003_invite_status.sql`).** The released `0001_init.sql` snippet above
is a historical record; the following columns were added by follow-up
migrations and are present on every live database:

- `users.last_login_at TEXT` — ISO-8601 UTC, NULL until the first
  successful login. Bumped on every login (see §8.9).
- `users.suspended_at  TEXT` — ISO-8601 UTC, NULL for active users.
  Set by `POST /admin/users/:id/suspend`, cleared by
  `POST /admin/users/:id/reactivate`.
- `invites.revoked_at  TEXT` — ISO-8601 UTC, NULL for non-revoked
  invites. Revoke is a soft-delete now (see §9).
- `invites.is_admin    INTEGER NOT NULL DEFAULT 0` — `1` if the invite
  creates an admin account. Read server-side at signup; the signup form
  has no role field.

The partial index `invites_email_active` is **recreated** by
`0003_invite_status.sql` to exclude revoked rows:

```sql
CREATE UNIQUE INDEX invites_email_active
    ON invites(email)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
```

This is what makes "revoke an active invite, then re-invite the same
email" succeed — without the predicate update the previous (revoked)
row would still block the new insert.

## 8. Authentication

### 8.1 Cookie

| attribute | value                          |
| --------- | ------------------------------ |
| name      | `auth`                         |
| value     | signed JWT                     |
| HttpOnly  | true                           |
| Secure    | from `--cookie-secure`         |
| SameSite  | `Lax`                          |
| Path      | `/`                            |
| Max-Age   | 24 h (matches JWT `exp`)       |

### 8.2 JWT

- Algorithm: HS256.
- Key: `--jwt-secret` (≥32 bytes; checked at boot).
- Claims:

  ```
  {
    "sub":    "<user id>",
    "handle": "<lowercased handle>",
    "admin":  true|false,
    "iat":    <unix>,
    "exp":    <unix + 24h>
  }
  ```

- `NewClaimsFunc` returns `*auth.Claims` so handlers get a typed value.

### 8.3 Token extraction

`echo-jwt` is configured with:

```
TokenLookup: "cookie:auth,header:Authorization:Bearer "
```

Cookies are tried first (the normal browser path). The Authorization
header is supported as a fallback so a future CLI or scripted client can
authenticate without depending on a cookie jar.

### 8.4 Logout

`POST /logout` clears the `auth` cookie (sets it with `Max-Age=0` and an
empty value) and redirects (or returns 204 for HTMX). Pure stateless —
this matches the agreed "rotate `--jwt-secret` to mass-invalidate" model.

### 8.5 CSRF / cross-origin request protection

Cross-origin write protection is provided by the stdlib's
[`net/http.CrossOriginProtection`][cop] (Go 1.25+), mounted as an Echo
middleware in `internal/server.installMiddleware`. It rejects
state-changing browser requests whose `Sec-Fetch-Site` or `Origin`
headers identify them as cross-origin; safe methods (GET/HEAD/OPTIONS)
and non-browser requests pass through. Huck is single-origin in
production, so `AddTrustedOrigin` is not used.

[cop]: https://pkg.go.dev/net/http#CrossOriginProtection

The older double-submit `_csrf` cookie / `X-CSRF-Token` header /
hidden form field plumbing is **deliberately removed** (sprint-3 T3.1
and T3.2). Do not reintroduce it: the stdlib check is the single
source of truth, and an extra token mechanism only adds form/template
weight without strengthening the guarantee.

Defence in depth comes from three layers around the `auth` JWT cookie:

1. `SameSite=Lax` on the cookie (set in `Server.setAuthCookie`,
   §8.1), which keeps the cookie off cross-site POST/iframe nav.
2. `http.CrossOriginProtection`, which rejects state-changing
   browser requests that arrive from a different origin even if a
   `SameSite=Lax` exception would otherwise let the cookie ride along.
3. HSTS (§12) once `--cookie-secure` is on, eliminating the
   downgrade-to-HTTP attack on the SameSite/cross-origin checks.

Per Alex Edwards' analysis of `http.CrossOriginProtection`
(<https://www.alexedwards.net/blog/preventing-csrf-in-go>), the
middleware is most effective when the application requires both
**HTTPS** and **TLS 1.3 or later**. The TLS-1.3 floor narrows the
residual CSRF window to roughly "Firefox v60–69 + a small set of
non-major browsers"; older browsers without `Sec-Fetch-Site` /
`Origin` enforcement are refused at the TLS layer.

Huck's deployment contract:

- **Production:** Nginx terminates TLS in front of the Go process and
  is configured with `ssl_protocols TLSv1.3;` (no TLSv1.2 fallback).
  The Go listener only ever speaks plaintext to the local Nginx, so
  no TLS-1.3 enforcement is needed inside the binary. The full proxy
  contract — required headers, sample config, and the operational
  checklist — lives in [`docs/nginx-proxy.md`](./nginx-proxy.md); §12.1
  carries only the TLS-protocol summary.
- **Local dev / `huck serve` direct:** out of scope for the TLS-1.3
  contract. Dev runs on `localhost`, which browsers treat as a Secure
  Context regardless of HTTPS (so `Sec-Fetch-Site` is still emitted);
  the slightly larger residual window is accepted in exchange for an
  HTTP-only dev loop.

The project has decided not to support incompatible legacy clients, so
the production Nginx config pins `ssl_protocols TLSv1.3;` with no
TLSv1.2 fallback. No further mitigations are needed; this resolves the
open question that was tracked against sprint-3 T3.2.

### 8.6 Authorisation guards

- `RequireAuth` — the `echo-jwt` middleware itself.
- `RequireAdmin` — wraps `RequireAuth` and additionally checks
  `claims.Admin == true`, returning 403 otherwise.

### 8.7 Password policy

A single policy applies wherever a password is accepted (`huck admin
create`, `POST /signup/:token`, future password-change endpoints):

- **Length:** ≥12 and ≤128 characters.
- **Character set:** any printable Unicode, including spaces. No
  character-class requirements (no forced digits/symbols/case mix).
- **Storage:** bcrypt only (`golang.org/x/crypto/bcrypt`, cost 12).
  Plaintext is never logged or returned.
- **Login rate limiting:** basic per-handle + per-IP throttle on
  `POST /login` to slow online guessing. (`zxcvbn` strength scoring is
  optional and explicitly **not** an MVP blocker.)

The validator lives in `internal/auth` so the CLI and the HTTP layer
share it; failures return clear messages naming the rule that failed
(too short / too long / non-printable character).

### 8.8 Handle policy

Handles are case-folded to lowercase before validation and storage
(see [§7.4](#74-initial-schema-0001_initsql)). The validator accepts:

```
^[a-z][a-z0-9_,.'-]{2,31}$
```

— a leading lowercase letter, followed by 2–31 characters drawn from
lowercase ASCII letters, digits, and the punctuation set `_ , . ' -`.
Total length 3–32. Anything outside this set is rejected; the rule
deliberately excludes characters that complicate URL encoding or HTML
contexts.

### 8.9 Suspended users

A user with `suspended_at IS NOT NULL` cannot acquire a new JWT.
`POST /login` checks `user.IsSuspended()` **after** password verify (so
a wrong-password attempt cannot probe suspension state) and refuses
with a 403 + "This account has been suspended." message instead of
setting the auth cookie. `last_login_at` is updated only on fully
successful logins, so a refused login leaves it unchanged.

Existing JWTs issued before the suspension stay valid until their
24-hour `exp`. Per the non-goal in §2, the project does not maintain a
per-user revocation list; rotate `--jwt-secret` to mass-invalidate if a
token must die early.

`POST /admin/users/:id/suspend` and `POST /admin/users/:id/reactivate`
are the only entry points (§10). The suspend handler refuses self
("You cannot suspend yourself. Ask another admin to make this
change.") to keep an admin from locking themselves out.

## 9. Invite flow

```diagram
╭───────╮  POST /admin/invites   ╭────────╮  Mailgun  ╭─────────╮
│ Admin │ ─────────────────────▶ │ Server │ ────────▶ │ Invitee │
╰───────╯                        ╰────┬───╯           ╰────┬────╯
                                      │                    │
                                      │   GET /signup/:t   │
                                      │ ◀──────────────────╯
                                      │
                                      │  POST /signup/:t
                                      │  (handle, password)
                                      ▼
                                ╭───────────╮
                                │ users +   │
                                │ invites   │
                                │ updated   │
                                ╰───────────╯
```

1. **Create.** `POST /admin/invites {email, role}`. The form carries a
   Role radio (`user` / `admin`, default `user`). Server lowercases the
   email and branches on role:
   - **User invite** (`role=user`) — proceeds immediately.
   - **Admin invite** (`role=admin`) — first POST renders an
     interstitial (`pages/admin_invite_confirm.html`) with the
     normalised email and a hidden `confirm=true` field. **No DB write,
     no mail** until the operator clicks "Send admin invitation",
     which re-POSTs `/admin/invites` with `role=admin&confirm=true`.
     The role is recorded on the invite row as
     `invites.is_admin = (role == "admin")`.

   Once committed, the server generates a 32-byte token (base64url),
   inserts a row with `expires_at = now+7d`, and sends an email whose
   body links to `${BASE_URL}/signup/<token>?email=<urlencoded-email>`.
   The `email` query string is a UX aid only — it pre-fills the
   read-only field on the signup form so the recipient can see which
   address the invite is bound to. The token alone is the security
   boundary; the email is re-validated server-side against the invite
   row.

   The unique partial index `invites_email_active` enforces "one
   active invite per email" with the predicate
   `consumed_at IS NULL AND revoked_at IS NULL` (§7.4). If an
   unconsumed, unrevoked invite exists for the same address, the
   create endpoint returns a 409 and the admin must resend or revoke
   the existing one. A revoked row does **not** block re-invite.
2. **Resend.** `POST /admin/invites/:token/resend`. Allowed only if the
   invite exists and is neither consumed nor revoked. Updates
   `expires_at = now+7d` (regardless of prior expiry) and re-sends the
   same link.
3. **Revoke.** `POST /admin/invites/:token/revoke`. Soft-delete:
   `UPDATE invites SET revoked_at = now WHERE token = ? AND revoked_at
   IS NULL`. The row remains for audit and is rendered in the admin
   list with Status=Revoked and no actionable buttons. Revoking an
   already-revoked token surfaces as `ErrNotFound` (zero rows matched
   the `WHERE`).
4. **Landing.** `GET /signup/:token` validates that the token exists,
   is not consumed, not expired, and **not revoked**. Renders a form
   whose fields are:
   - `email` — pre-filled from the invite row and rendered as a
     **visible, `readonly`** input. The user can see (and copy) the
     address the invite is bound to, but cannot edit it. The field
     submits with the form, giving the server a defence-in-depth value
     to re-check against the invite row.
   - `handle` — validated against the [§8.8 handle policy](#88-handle-policy)
     and against handle-uniqueness.
   - `password` — validated against the [§8.7 password policy](#87-password-policy).

   The signup form deliberately has **no role field**. The invite's
   `is_admin` is a server-side fact read from the invite row at submit
   time; a tampered form cannot promote.
5. **Submit.** `POST /signup/:token` performs everything inside **one
   SQLite transaction**:
   1. Re-validate the token (exists, not consumed, not expired, not
      revoked).
   2. Re-validate that the submitted email equals the invite email
      (case-insensitively, defence against form tampering).
   3. Validate handle and password against §8.7 / §8.8.
   4. `INSERT INTO users (..., is_admin = invites.is_admin)` — the
      role comes from the invite row, never from the form.
   5. `UPDATE invites SET consumed_at = now WHERE token = ?`.
   6. Commit.

   Then issue the JWT, set the auth cookie, and redirect to `/`. The
   transaction guarantees that two parallel form submits cannot both
   succeed (handle-uniqueness or invite-consumed will fail one of them).

Token entropy: 32 bytes from `crypto/rand` → ~256 bits → not guessable.
Tokens are stored as-is (they are random, not secret in the sense of
needing further hashing for this threat model — but they are treated as
secrets in logs and URLs).

## 10. Routes

| Method   | Path                                  | Guard  | Notes                                        |
| -------- | ------------------------------------- | ------ | -------------------------------------------- |
| GET      | `/`                                   | public | Renders one of two pages based on auth state (see below). |
| GET      | `/login`                              | public | Login form.                                  |
| POST     | `/login`                              | public | Validates, sets cookie, redirects.           |
| POST     | `/logout`                             | auth   | Clears cookie.                               |
| GET      | `/signup/:token`                      | public | Invite landing.                              |
| POST     | `/signup/:token`                      | public | Creates user, consumes invite.               |
| GET      | `/account`                            | auth   | Signed-in user's account detail page.        |
| GET      | `/admin`                              | admin  | Admin dashboard (canonical path).            |
| GET      | `/admin/`                             | public | 301 redirect to `/admin` (canonicalisation runs before the admin guard). |
| GET      | `/admin/invites`                      | admin  | List + create form.                          |
| POST     | `/admin/invites`                      | admin  | Create + send.                               |
| POST     | `/admin/invites/:token/resend`        | admin  | Refresh expiry, re-send.                     |
| POST     | `/admin/invites/:token/revoke`        | admin  | Soft-revoke invite (sets `revoked_at`).      |
| GET      | `/admin/users`                        | admin  | List users.                                  |
| GET      | `/admin/users/:id`                    | admin  | User detail page.                            |
| GET      | `/admin/users/:id/edit`               | admin  | User edit form.                              |
| POST     | `/admin/users/:id/edit`               | admin  | Apply `is_admin` toggle. (Admin-set passwords removed in Sprint 5.) |
| POST     | `/admin/users/:id/suspend`            | admin  | Soft-suspend user (sets `suspended_at`).     |
| POST     | `/admin/users/:id/reactivate`         | admin  | Clear `suspended_at`.                        |
| GET      | `/static/*`                           | public | Embedded assets.                             |

### 10.1 Root route behaviour

`GET /` is mounted as a public route, but its handler inspects the auth
cookie before rendering:

- **Anonymous visitor** → `pages/home_public.html`. A "what is huck"
  landing page describing the project, with a prominent link to
  `/login`. No mention of self-signup (registration is invite-only).
- **Authenticated user** → `pages/home_authed.html`. A "welcome to
  huck" home/dashboard for the signed-in user, with navigation to the
  app's own pages (and `/admin/...` if `claims.Admin` is true).

The handler does **not** issue an HTTP redirect; the URL stays `/` in
both cases so it remains bookmarkable. Auth detection here is "best
effort" — a missing or invalid cookie simply means anonymous, not 401.

The dedicated **"create your account"** page lives at `/signup/:token`
and is only reachable via the link emailed by an invite. There is no
public registration form linked from `home_public.html`.

## 11. Templates, styling, and HTMX

### 11.1 Styling

[Pico.css](https://picocss.com) is the only CSS framework. It is a
classless framework: semantic HTML (`<button>`, `<form>`, `<table>`,
`<nav>`, `<article>`, etc.) is styled directly with no utility classes.
This was chosen because:

- It needs no build step — drop `pico.min.css` into `web/static/` and
  reference it from `layout.html`.
- It pairs naturally with server-rendered HTML and HTMX swaps: the
  server emits semantic markup, Pico styles it.
- It keeps the dependency surface small (no `npm`, no Tailwind CLI, no
  PostCSS) which matches the "single static binary" goal.

Project-specific tweaks live in `web/static/app.css`, which is loaded
**after** `pico.min.css`. Do not introduce additional CSS frameworks
without first updating this document.

### 11.2 Templates

- `web/templates/layout.html` is the only base layout. It defines the
  blocks `title`, `content`, and `scripts`.
- Files under `web/templates/pages/` are full pages and supply those
  blocks.
- Files under `web/templates/partials/` are HTMX fragments — no layout,
  no `<html>` wrapper.

The custom `Renderer` decides which path to take:

- Template name starting with `partials/` → render the named template
  alone.
- Otherwise, if `HX-Request: true` and `HX-Boosted` is empty → render the
  `content` block of the named page alone.
- Otherwise → render the full layout.

Handlers should not branch on `HX-Request` themselves.

AlpineJS handles small client-side interactions (toggles, dropdowns).
HTMX handles partial updates (e.g. updating a single row in the invite
list after resend/revoke).

The Sprint-4 layout split (`layout_auth.html` vs. `layout_app.html`) and
the named Phase-2 CSS primitive vocabulary (`.huck-shell`, `.huck-sidebar`,
`.huck-topbar`, `.huck-breadcrumbs`, `.huck-content`, `.huck-page-header`,
`.huck-form-stack`) are defined in
[`docs/front-end-plan.md`](front-end-plan.md) and are not duplicated here.

**User-visible labels.** "Administration" is the user-visible label for
the admin section; "Invitations" is the user-visible label for the
invitations page. The URL paths remain `/admin` and `/admin/invites`,
and the Go identifiers (`SectionAdminInvites`, `handleAdminInvitesList`,
`usersShell`, etc.) are unchanged — only sidebar, breadcrumb, topbar,
and page-header copy carry the long form.

## 12. Security headers

Set on every response:

- `Strict-Transport-Security: max-age=31536000; includeSubDomains` (only
  when `--cookie-secure` is true).
- `X-Content-Type-Options: nosniff`.
- `Referrer-Policy: strict-origin-when-cross-origin`.
- `Content-Security-Policy: default-src 'self'; script-src 'self';
  style-src 'self' 'unsafe-inline'; img-src 'self' data:; base-uri 'self';
  frame-ancestors 'none'`.

(`'unsafe-inline'` for styles is allowed only because Alpine attribute
expressions can require it; we will tighten this if it proves
unnecessary.)

### 12.1 Production TLS

The Go listener serves plaintext HTTP to a local Nginx; Nginx
terminates TLS to the public internet. Nginx must be configured to
require TLS 1.3 so that the `Sec-Fetch-Site` / `Origin`-based check
in §8.5 has the strongest residual-risk profile. A minimal snippet:

```nginx
ssl_protocols TLSv1.3;
ssl_prefer_server_ciphers off;
```

The full Nginx contract — required headers (notably
`proxy_set_header Host $host;`), a copy-paste-ready sample server
block, and the pre-release operational checklist — lives in
[`docs/nginx-proxy.md`](./nginx-proxy.md). This section deliberately
only owns the TLS-protocol pin; everything else is delegated to that
doc to keep one source of truth for DevOps.

## 13. Error handling

A central `internal/server/errors.go` maps Go errors to HTTP responses:

- Sentinel errors (`users.ErrNotFound`, `invites.ErrExpired`, …) map to
  appropriate status codes.
- For HTML routes, errors render `pages/error.html` with the layout (or
  a fragment for HTMX).
- For JSON routes (none today, but reserved), errors render
  `{"error": "..."}`.

Handlers return `error`; the global error handler does the rendering.

## 14. Logging

Uses `log/slog` with the level controlled by `--log-level`. Echo's
`middleware.RequestLogger` is configured with `LogValuesFunc` that calls
into slog so all logs share a format.

**Sensitive values** — passwords, JWTs, and invite tokens — must not
appear in logs. This is enforced by **discipline and code review**, not
by an automated filter: the threat model assumes operators (and only
operators) read logs, and the worst-case exposure is small (see §1).
Reviewers should reject log statements that pass these values directly,
and types that wrap secrets are encouraged to define a `LogValue() slog.Value`
that returns a redacted representation.

## 15. Testing strategy

- Stores have unit tests against an in-memory SQLite (`file::memory:?cache=shared`)
  using the same migrations.
- `Mailer` has a fake implementation used in handler tests; the Mailgun
  implementation has at least one integration test gated on env vars.
- At least one integration test exercises the full echo-jwt + cookie path
  end-to-end (per `echo-jwt`'s own warning about silent type-assertion
  failures across major versions).

## 16. Open questions

None at the time of this draft. Update this section before extending the
design.

## 17. Change log

- **2026-05-09** — Accepted. Initial design (Pico.css for styling;
  `GET /` renders public vs. authed pages without redirect; `/signup/:token`
  is the only registration entry point).
- **2026-05-10** — §8.5 rewritten for `http.CrossOriginProtection`
  (sprint-3 T3.1 / T3.2): double-submit `_csrf` plumbing removed,
  defence-in-depth now relies on `SameSite=Lax` + cross-origin
  middleware + HSTS, and the production TLS-1.3 / Nginx contract is
  captured in §12.1.
- **2026-05-10** — Nginx reverse-proxy contract extracted to
  [`docs/nginx-proxy.md`](./nginx-proxy.md); §8.5 and §12.1 now link
  to it. Open question on `ssl_protocols TLSv1.3;` resolved: the
  project will not support incompatible legacy clients, so TLS 1.3
  is pinned with no TLSv1.2 fallback.
- **2026-05-15** — Sprint 5 lands the "admin tasks" Users + Invitations
  surface. §7.4 documents the new `users.last_login_at`,
  `users.suspended_at`, `invites.revoked_at`, and `invites.is_admin`
  columns (migrations `0002_user_status.sql` / `0003_invite_status.sql`)
  and the recreated `invites_email_active` partial index that excludes
  revoked rows. New §8.9 records the login-refuses-suspended-users
  contract (no new JWT, existing JWTs survive until `exp`, rotate
  `--jwt-secret` to mass-invalidate). §9 rewritten: invites carry an
  `is_admin` flag confirmed via a two-step interstitial; revoke is now
  a soft-delete; signup reads `invites.is_admin` server-side and
  refuses revoked invites. §10 replaces `POST /admin/users/:id/delete`
  with `…/suspend` + `…/reactivate`; the edit-route note drops the
  admin password-reset capability. §11.2 records the "Administration" /
  "Invitations" user-visible label rename (URL paths and Go
  identifiers unchanged).
