# AGENTS.md

Guidance for AI agents (and humans) working in this repository.

## Project

`huck` is a small, self-hosted Go web server. It serves server-rendered HTML
augmented with HTMX and AlpineJS, and uses SQLite for persistence.

The full architectural design lives in [docs/DESIGN.md](docs/DESIGN.md).
**Read it before making non-trivial changes.**

## Hard rules

These are non-negotiable. Do not "fix" or work around them without explicit
approval.

1. **No CGO.** SQLite access goes through `zombiezen.com/go/sqlite` (pure Go).
   Do not add `mattn/go-sqlite3`, `modernc.org/sqlite`, or any other driver.
2. **Echo v5, not v4.** Use `github.com/labstack/echo/v5` and
   `github.com/labstack/echo-jwt/v5`. If you find yourself reading v4 docs,
   stop and re-check.
3. **`ff/v4`** (`github.com/peterbourgon/ff/v4`) is the only flag/command
   library. Do not introduce `cobra`, `kingpin`, `urfave/cli`, etc.
4. **The server never creates the database file.** A missing DB file on
   `huck serve` is a fatal error. New databases are created exclusively via
   `huck db create`.
5. **Handles and emails are stored lowercased.** Lowercasing happens in Go
   before insert; uniqueness is enforced by the schema. Do not rely on
   `COLLATE NOCASE` as the source of truth.
6. **Passwords use bcrypt** (`golang.org/x/crypto/bcrypt`, cost 12). Never
   store, log, or return password material. Never add a second hashing scheme
   without a migration plan.
7. **Registration is invite-only.** There is no public sign-up route. Invites
   are admin-issued, time-limited (7 days), single-use, and emailed via
   Mailgun.
8. **Secrets come from env vars or `--config <file>`, never from a file
   committed to the repo.** Use `ff.WithEnvVars()` and the `HUCK_` prefix.

## Stack at a glance

| Concern        | Choice                                        |
| -------------- | --------------------------------------------- |
| HTTP framework | `github.com/labstack/echo/v5`                 |
| JWT middleware | `github.com/labstack/echo-jwt/v5`             |
| JWT lib        | `github.com/golang-jwt/jwt/v5`                |
| SQLite         | `zombiezen.com/go/sqlite` (+ `sqlitex`)       |
| Flags/commands | `github.com/peterbourgon/ff/v4`               |
| Email          | Mailgun HTTP API via internal stdlib client   |
| Password hash  | `golang.org/x/crypto/bcrypt`                  |
| Templates      | stdlib `html/template`                        |
| Frontend       | HTMX + AlpineJS + Pico.css (served from `web/static`) |

## Layout

```
cmd/huck/            # entry point, ff/v4 command tree
cmd/sendtest/        # Mailgun smoke-test binary (operator tool)
internal/config/     # flag + env + file binding
internal/cerrs/      # constant sentinel errors
internal/db/         # open, create, migrate (zombiezen)
internal/dotenv/     # .env loader (HUCK_ENV-aware)
internal/server/     # Echo wiring, renderer, middleware
internal/auth/       # bcrypt, JWT, login/logout, guards
internal/users/      # user store
internal/invites/    # invite tokens, signup flow
internal/mail/       # Mailer interface + stdlib-only Mailgun adapter
migrations/          # NNNN_*.sql, embedded
web/templates/       # layout.html, pages/, partials/, email/
web/static/          # htmx, alpine, pico.min.css, app.css
docs/                # DESIGN.md, sprint plans, front-end notes
```

## Conventions

- All packages under `internal/` — nothing in this project is intended to be
  imported by external code.
- One `Mailer` interface in `internal/mail`; the Mailgun implementation is
  one of potentially several (a no-op/fake is used in tests).
- Stores (`users`, `invites`) take a `*sqlite.Conn` (or `*sqlitex.Pool`),
  not a global. No package-level DB handles.
- Handlers receive their dependencies via a small `Server` struct, not
  through `c.Get(...)` lookups.
- Times are stored as ISO-8601 UTC text
  (`strftime('%Y-%m-%dT%H:%M:%fZ','now')`); parse with `time.RFC3339Nano`.
- Errors returned to handlers use `echo.NewHTTPError` or sentinel errors
  mapped centrally in `internal/server/errors.go`.

## Templates, styling, and HTMX

- **Pico.css is the only CSS framework.** It is classless — write
  semantic HTML (`<button>`, `<form>`, `<table>`, …) and let Pico style
  it. Do not introduce Tailwind, Bootstrap, DaisyUI, etc. without
  updating `docs/DESIGN.md` first.
- Project-specific overrides go in `web/static/app.css`, loaded after
  `pico.min.css`. Avoid utility-class proliferation; prefer semantic
  selectors.
- One base layout (`web/templates/layout.html`) with `{{ block "title" }}`,
  `{{ block "content" }}`, `{{ block "scripts" }}`.
- Full pages live in `web/templates/pages/` and define those blocks.
- HTMX fragments live in `web/templates/partials/` and render without the
  layout.
- The custom `Renderer` decides which path to take based on the template name
  and the `HX-Request` header. Handlers should not branch on `HX-Request`
  themselves.

## Front-end conventions

The working plan for Huck's front end — the two-shell split, the named
Phase-2 CSS primitives, the breadcrumb data contract, and the explicit
"not yet" list — lives in [docs/front-end-plan.md](docs/front-end-plan.md).
Read it before changing layouts, sidebars, breadcrumbs, or `web/static/app.css`.

Deliberately deferred (see plan §8 — do not add without updating the plan first):

- **No Tailwind** (or Bootstrap, DaisyUI, etc.) — Pico.css only.
- **No utility classes.** Prefer semantic selectors layered on Pico.
- **No design tokens / CSS custom-property system yet.** Use Pico's
  variables; introduce a `--huck-*` variable only when a real pattern
  forces it.
- **No mobile hamburger / sidebar collapse JS.** Narrow viewports get
  the stacked single-column fallback; that is the whole mobile story
  for now.
- **No forced theme.** Do not pin `data-theme="light"` (or dark) on
  `<html>`; follow `prefers-color-scheme`.

## Migrations

- New schema changes go in `migrations/NNNN_short_name.sql`, where `NNNN` is
  the next zero-padded integer.
- Migrations are append-only. Never edit a migration that has been released.
- `huck db migrate` is idempotent; `huck serve` runs it on startup.

## Adding a flag

1. Add the field to `internal/config/Config`.
2. Register it in the `ff.FlagSet` with both long name and env mapping.
3. Document it in `docs/DESIGN.md` (config table) and `README.md`.

## Verification before saying "done"

- `go build ./...` succeeds.
- `go test ./...` passes.
- `go vet ./...` is clean.
- For schema changes: `huck db create` on a fresh path succeeds, and
  `huck db migrate` is a no-op the second time.

## Things that are intentionally NOT here

- Refresh tokens (single 24h JWT; re-login on expiry).
- Server-side session/revocation table (stateless JWT; rotate
  `--jwt-secret` to mass-invalidate).
- A `roles` table (a single `users.is_admin` boolean is enough).
- Public self-signup.
- Any non-Go SQLite driver.

If a future requirement makes one of these necessary, update `DESIGN.md`
first, then this file, then write code.
