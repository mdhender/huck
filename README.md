# huck

`huck` is a small, self-hosted Go web server intended for fewer than 50
people. It serves server-rendered HTML augmented with HTMX and AlpineJS,
and uses SQLite for persistence. Authentication is invite-only and uses
JWT in an HttpOnly cookie.

The full design lives in [docs/DESIGN.md](docs/DESIGN.md). The Sprint 1
implementation plan lives in [docs/sprint-1.md](docs/sprint-1.md).

Email delivery uses Mailgun through a small internal stdlib-only client
in `internal/email`. See [docs/mailgun-lite.md](docs/mailgun-lite.md)
for the rationale and verification notes.

## Quickstart

Requires Go 1.23+ (any current stable Go works). **No CGO toolchain
required** — SQLite is pure Go via `zombiezen.com/go/sqlite`.

```sh
# 1. Build the binary.
CGO_ENABLED=0 go build -o huck ./cmd/huck

# 2. Create a fresh SQLite database (refuses to overwrite an existing file).
./huck db create --db ./huck.db

# 3. Create the bootstrap admin user. Password is read from
#    HUCK_ADMIN_PASSWORD if set, otherwise prompted on the TTY.
HUCK_ADMIN_PASSWORD=changeme123 \
  ./huck admin create --db ./huck.db --handle alice --email alice@example.com

# 4. Start the server. The JWT secret must be at least 32 bytes.
HUCK_JWT_SECRET="$(openssl rand -base64 48)" \
HUCK_COOKIE_SECURE=false \
  ./huck serve --db ./huck.db --addr :8080
```

Open <http://localhost:8080>, click **Sign in**, log in as `alice`, and
you should land on the authenticated welcome page. Sign out from the nav
bar and you should land back on the public landing page.

> `HUCK_COOKIE_SECURE=false` is only appropriate for local development
> over plain HTTP. In production, leave it at the default (`true`) and
> serve huck behind TLS.

## Configuration

Every flag also reads from a matching `HUCK_<UPPER_SNAKE>` env var, and
optionally from a plain-text file passed via `--config`. Resolution
order: CLI > env var > config file > default. See
[docs/DESIGN.md §6](docs/DESIGN.md) for the full table.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

Hard rules and conventions live in [AGENTS.md](AGENTS.md). Read it
before making non-trivial changes — particularly the "no CGO", "Echo v5
not v4", and "registration is invite-only" rules.

## Project layout

```
cmd/huck/            # entry point + ff/v4 command tree
internal/config/     # flag/env/file binding
internal/db/         # zombiezen open / create / migrate
internal/email/      # tiny stdlib-only Mailgun client
internal/auth/       # bcrypt, JWT, login/logout handlers
internal/users/      # user store
internal/server/     # Echo wiring, renderer, middleware
migrations/          # NNNN_*.sql, embedded
web/                 # templates + static assets, embedded
docs/                # design + sprint plans
```

## Contributors

Project contributors include:

* Michael D Henderson
* OpenAI Codex
