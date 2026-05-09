# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Authoritative docs (read first)

- `AGENTS.md` — hard rules, stack pins, conventions. **Re-read before any non-trivial change.** The "Hard rules" section is non-negotiable.
- `docs/DESIGN.md` — full architectural design. Schema/flag/policy decisions live here. If you need to deviate, update DESIGN.md first, then AGENTS.md, then write code.
- `docs/sprint-1.md`, `docs/sprint-2.md` — implementation plans for the in-progress sprints.

## Build, test, run

```sh
# Build (no CGO required — pure-Go SQLite)
CGO_ENABLED=0 go build -o huck ./cmd/huck

# Standard verification (AGENTS.md "Verification before saying done")
go build ./...
go vet ./...
go test ./...

# Single package / single test
go test ./internal/server/...
go test ./internal/auth/ -run TestVerify -v
```

The DB lifecycle is split intentionally — **`huck serve` will not create the DB file**:

```sh
./huck db create  --db ./huck.db                                 # create + migrate
./huck db migrate --db ./huck.db                                 # idempotent
HUCK_ADMIN_PASSWORD=… ./huck admin create --db ./huck.db --handle alice --email alice@example.com
HUCK_JWT_SECRET="$(openssl rand -base64 48)" HUCK_COOKIE_SECURE=false \
  ./huck serve --db ./huck.db --addr :8080
```

For schema changes, the verification loop is: `huck db create` on a fresh path succeeds, **and** `huck db migrate` on the same DB is a no-op the second time.

## Configuration plumbing

Every flag is also `HUCK_<UPPER_SNAKE>` env var, also readable from a plain-text `--config` file. Resolution: CLI > env var > config file > default. Wired via `ff.WithEnvVarPrefix("HUCK")` + `ff.WithConfigFileFlag("config")` in `cmd/huck/main.go`.

If `HUCK_ENV` is set, `internal/dotenv` loads files in this priority order before flag parsing: `.env.{HUCK_ENV}.local` > `.env.local` > `.env.{HUCK_ENV}` > `.env`. The `.local` files are gitignored and may contain secrets; the non-`.local` files must not.

Adding a new flag means: field on `internal/config/Config`, registration on the right `ff.FlagSet` (long name + env mapping), and an entry in DESIGN.md §6 + README.md.

## Architecture (the parts that span files)

**Command tree** — `cmd/huck/main.go` builds the entire `ff.Command` tree (`huck serve`, `huck db {create,migrate}`, `huck admin create`). Each subcommand has its own `*Validate*` method on `Config` rather than self-validating in one place; validation lives next to the command that needs it.

**DB ownership** — `internal/db` is the *only* package that calls `sqlitex.NewPool` / `sqlite.OpenConn`. Stores (`internal/users`, `internal/invites`) take a `*sqlitex.Pool` (or `*sqlite.Conn`) injected at construction time. There are no package-level DB handles.

**Server wiring** — `internal/server.New` builds a `Server` struct that bundles `cfg`, `*echo.Echo`, stores, logger, and the JWT key. Handlers are methods on `*Server`, not closures over `c.Get(...)` lookups. The `cmd` package owns the lifecycle (`echo.StartConfig` + graceful shutdown); `Server.Echo()` is the seam for `httptest` in `internal/server/server_test.go`.

**Renderer (`internal/server/render.go`)** is the one place that knows about HTMX. It dispatches by template name + `HX-Request`:
- `partials/foo.html` → render the partial alone, no layout.
- `pages/foo.html` + `HX-Request: true` (and not `HX-Boosted`) → render only the page's `"content"` block.
- `pages/foo.html` otherwise → render `layout.html` with the page's `title`/`content`/`scripts` blocks.

Handlers must **not** branch on `HX-Request` themselves — they always call `c.Render("pages/foo.html", …)` and let the renderer decide. (Login/logout are the documented exception: they return `HX-Redirect` headers for HTMX form submits.)

**Templates and embedding** — `web/embed.go` embeds both `web/templates/**` and `web/static/**`. Migrations are similarly embedded via `migrations/embed.go`. The renderer parses `layout.html` once and clones it per page so per-page block redefinitions don't leak.

**Auth flow** — JWT in an HttpOnly cookie (name `auth.CookieName`, TTL `auth.DefaultTokenTTL`, ~24h). `Server.bestEffortClaims` parses the cookie on every render — a missing/invalid/expired cookie just means anonymous, never 401. There is no refresh token, no server-side session table, no per-user revocation list; rotating `--jwt-secret` invalidates everyone.

**Normalisation invariant** — handles and emails are lowercased in Go (`users.Normalise`) **before insert**. Schema uniqueness enforces it; do not depend on `COLLATE NOCASE` to do this for you.

**Times** — stored as ISO-8601 UTC text via `strftime('%Y-%m-%dT%H:%M:%fZ','now')`; parsed back with `time.RFC3339Nano`.

## Frontend

Pico.css is the only CSS framework — it's classless, so write semantic HTML and let Pico style it. Project overrides go in `web/static/app.css`, loaded after `pico.min.css`. Don't introduce Tailwind/Bootstrap/etc. without updating DESIGN.md first.

## Things that look missing but are intentional

Refresh tokens, a server-side session/revocation table, a `roles` table, public self-signup, and any non-Go SQLite driver are all explicitly out of scope. See AGENTS.md "Things that are intentionally NOT here" before adding any of them.
