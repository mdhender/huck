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
| Email           | `github.com/mailgun/mailgun-go/v4`            |
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
| `--mailgun-api-base`| `HUCK_MAILGUN_API_BASE` | no       | —          | Mailgun API base URL. Empty = SDK default (US). Set to `https://api.eu.mailgun.net/v3` for EU. |
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

### 7.3 Migrations

Migrations live in `migrations/NNNN_<name>.sql` and are embedded with
`//go:embed *.sql`. They are applied in lexical order in a single
transaction each, recording the version in `schema_migrations`.

Migrations are **append-only**. To change a released schema, write a new
migration.

`huck db migrate` runs them explicitly; `huck serve` runs them on startup.

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

### 8.5 CSRF

Echo's `middleware.CSRF()` is mounted with the double-submit cookie
pattern. A small AlpineJS snippet hooks `htmx:configRequest` and copies
the CSRF cookie value into the `X-CSRF-Token` header on every HTMX
request, so handlers do not need per-form tokens.

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

1. **Create.** `POST /admin/invites {email}`. Server lowercases the email,
   generates a 32-byte token (base64url), inserts a row with
   `expires_at = now+7d`, and sends an email whose body links to
   `${BASE_URL}/signup/<token>?email=<urlencoded-email>`. The
   `email` query string is a UX aid only — it pre-fills the read-only
   field on the signup form so the recipient can see which address the
   invite is bound to. The token alone is the security boundary; the
   email is re-validated server-side against the invite row.
   The unique partial index `invites_email_active` enforces "one
   active invite per email"; if an unconsumed invite exists for the
   same address, the create endpoint returns a 409 and the admin must
   resend or revoke the existing one.
2. **Resend.** `POST /admin/invites/:token/resend`. Allowed only if the
   invite exists and is not consumed. Updates `expires_at = now+7d`
   (regardless of prior expiry) and re-sends the same link.
3. **Revoke.** `POST /admin/invites/:token/revoke`. Deletes the row.
4. **Landing.** `GET /signup/:token` validates that the token exists, is
   not consumed, and is not expired. Renders a form whose fields are:
   - `email` — pre-filled from the invite row and rendered as a
     **visible, `readonly`** input. The user can see (and copy) the
     address the invite is bound to, but cannot edit it. The field
     submits with the form, giving the server a defence-in-depth value
     to re-check against the invite row.
   - `handle` — validated against the [§8.8 handle policy](#88-handle-policy)
     and against handle-uniqueness.
   - `password` — validated against the [§8.7 password policy](#87-password-policy).
5. **Submit.** `POST /signup/:token` performs everything inside **one
   SQLite transaction**:
   1. Re-validate the token (exists, not consumed, not expired).
   2. Re-validate that the submitted email equals the invite email
      (case-insensitively, defence against form tampering).
   3. Validate handle and password against §8.7 / §8.8.
   4. `INSERT INTO users (..., is_admin = 0)`.
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
| GET      | `/admin/invites`                      | admin  | List + create form.                          |
| POST     | `/admin/invites`                      | admin  | Create + send.                               |
| POST     | `/admin/invites/:token/resend`        | admin  | Refresh expiry, re-send.                     |
| POST     | `/admin/invites/:token/revoke`        | admin  | Delete invite.                               |
| GET      | `/admin/users`                        | admin  | List users.                                  |
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
