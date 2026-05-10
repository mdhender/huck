# huck

`huck` is a small, self-hosted Go web server intended for fewer than 50
people. It serves server-rendered HTML augmented with HTMX and AlpineJS,
and uses SQLite for persistence. Authentication is invite-only and uses
JWT in an HttpOnly cookie.

The full design lives in [docs/DESIGN.md](docs/DESIGN.md). Sprint plans
live in [docs/sprint-1.md](docs/sprint-1.md),
[docs/sprint-2.md](docs/sprint-2.md), and
[docs/sprint-3.md](docs/sprint-3.md).

Email delivery uses Mailgun through a small internal stdlib-only client
in `internal/email`, fronted by the `mail.Mailer` interface in
`internal/mail`. See [docs/mailgun-lite.md](docs/mailgun-lite.md) for
the rationale and verification notes.

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
#    Passwords must be 12–128 characters of printable Unicode.
HUCK_ADMIN_PASSWORD='change-me-please' \
  ./huck admin create --db ./huck.db --handle alice --email alice@example.com

# 4. Start the server. As of Sprint 2, the Mailgun trio and --base-url
#    are required (invites are emailed synchronously). The JWT secret
#    must be at least 32 bytes. Set --mailgun-api-base only for EU
#    Mailgun customers (e.g. https://api.eu.mailgun.net).
HUCK_JWT_SECRET="$(openssl rand -base64 48)" \
HUCK_COOKIE_SECURE=false \
  ./huck serve \
    --db ./huck.db \
    --addr :8080 \
    --base-url http://localhost:8080 \
    --mailgun-domain mg.example.com \
    --mailgun-api-key "$HUCK_MAILGUN_API_KEY" \
    --mailgun-from 'Huck <noreply@mg.example.com>'
```

Open <http://localhost:8080>, click **Sign in**, and log in as `alice`.
From the admin nav, visit `/admin/invites` to issue an invite — the
recipient gets an email containing
`http://localhost:8080/signup/<token>?email=<urlencoded>`. They follow
the link, pick a handle and password (subject to the §8.7/§8.8
policies), and land logged in. Admins can also resend or revoke
pending invites, and edit or delete users from `/admin/users`.

> `HUCK_COOKIE_SECURE=false` is only appropriate for local development
> over plain HTTP. In production, leave it at the default (`true`) and
> serve huck behind TLS.

For local development, secrets like `HUCK_MAILGUN_API_KEY` are
typically set in `.env.development.local` (gitignored) and picked up
automatically by `internal/dotenv` when `HUCK_ENV` is unset or
`development`.

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


### Coding Agent Prompts

The following prompt seems to work well with Amp, Claude, and Codex.

```text
Implement the next unchecked/TODO task in docs/sprint-NNNN.md. Follow the Agent Execution Rules in the sprint document. When complete:
1. update the sprint table
2. run the required verification
3. commit the task as one focused commit
4. report the commit hash
5. stop
```

After the task completes, clear the context and repeat. Once the sprint has finished, validate and then ask the agent to update the sprint document as complete and commit. Optionally, if you have tokens to burn, you can ask the agent to update the commit column in the sprint table.

## Project layout

```
cmd/huck/            # entry point + ff/v4 command tree
internal/config/     # flag/env/file binding
internal/db/         # zombiezen open / create / migrate
internal/mail/       # Mailer interface + Mailgun adapter, fake for tests
internal/auth/       # bcrypt, JWT, validators, login/logout handlers
internal/users/      # user store
internal/invites/    # invite tokens, signup transaction
internal/server/     # Echo wiring, renderer, middleware, admin pages
migrations/          # NNNN_*.sql, embedded
web/                 # templates + static assets, embedded
docs/                # design + sprint plans
```

## Contributors

Project contributors include:

* Michael D Henderson
* OpenAI Codex
* Anthropic Claude
* Sourcegraph Amp
