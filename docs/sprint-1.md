# Sprint 1 — Implementation Plan

Status: **Done — 2026-05-09.**
Owner: TBD.
Target outcome: a single operator can run, in order,

```
huck db create   --db ./huck.db
huck admin create --db ./huck.db --handle alice --email alice@example.com
huck serve       --db ./huck.db --jwt-secret <32+ bytes>
```

…then open the listen address in a browser, see the **public** "what is
huck" landing page, click **Sign in**, log in as `alice`, and land on the
**authenticated** "welcome to huck" home page.

Anything not on the path above is out of scope for this sprint.

---

## 1. In scope

### 1.1 Repository bootstrap

- `go.mod` initialised with module path `github.com/mdhender/huck`,
  Go 1.23 (or current stable).
- Top-level files: `README.md` (one-paragraph stub + quickstart),
  `.gitignore` (Go + `*.db`, `*.db-shm`, `*.db-wal`).
- Empty directory skeleton matching `docs/DESIGN.md` §4.
- Vendored frontend assets dropped into `web/static/`:
  - `htmx.min.js` (latest 2.x)
  - `alpine.min.js` (latest 3.x)
  - `pico.min.css` (Pico v2)
  - `app.css` (empty file with a TODO header)

### 1.2 `internal/config`

- `Config` struct with **all** fields from DESIGN.md §6 so future sprints
  do not have to reshape it.
- `Parse(args []string)` builds the `ff.FlagSet`, applies
  `ff.WithEnvVarPrefix("HUCK")` and optional `ff.WithConfigFile`/
  `ff.PlainParser`.
- **Per-subcommand validation**, not global. Sprint 1 actively requires:
  `--db` (all subcommands), `--jwt-secret` (`serve`),
  `--handle`/`--email` (`admin create`).
  `--mailgun-*` and `--base-url` are **parsed but not validated** in
  Sprint 1 — leaving them empty must not block `serve`.
- Reject `--jwt-secret` < 32 bytes at boot.

### 1.3 `internal/db`

- `Open(path string) (*sqlitex.Pool, error)` — opens an **existing** file
  only, with `sqlite.OpenReadWrite` (no `OpenCreate`). Missing file →
  return a sentinel `ErrMissing`; `huck serve` turns it into a fatal
  error pointing at `huck db create`.
- On every connection: `journal_mode=WAL`, `synchronous=NORMAL`,
  `foreign_keys=ON`, `busy_timeout=5000`.
- `Create(path string) error` — fails if the file exists; otherwise opens
  with `OpenCreate`, sets PRAGMAs, runs migrations, closes.
- `Migrate(pool *sqlitex.Pool) error` — applies every embedded migration
  whose version is not yet recorded in `schema_migrations`, each in its
  own transaction.
- **Bootstrap hazard:** the very first migration creates
  `schema_migrations` itself. The migrator MUST tolerate the table not
  existing on its first run: probe with `SELECT name FROM sqlite_master
  WHERE type='table' AND name='schema_migrations'` and treat absence as
  "applied versions = ∅". This is the one piece of glue worth a unit
  test on its own.

### 1.4 `migrations/0001_init.sql`

- Full schema from DESIGN.md §7.4 — `users`, `invites`,
  `schema_migrations`. Ship the `invites` table now even though Sprint 1
  does not touch it; migrations are append-only.

### 1.5 `cmd/huck/main.go`

- `ff/v4` command tree:
  ```
  huck
  ├── serve         --addr --db --jwt-secret --cookie-secure --cookie-domain
  │                 --base-url --mailgun-* --log-level --config
  ├── db
  │   ├── create    --db
  │   └── migrate   --db
  └── admin
      └── create    --db --handle --email
                    (password from $HUCK_ADMIN_PASSWORD or stdin)
  ```
- Shared global flags resolved once, then per-subcommand validation as
  described in 1.2.
- `slog` set up at start of `main` based on `--log-level`; default text
  handler to stderr.
- **Graceful shutdown** in `serve`: capture `SIGINT`/`SIGTERM`, call
  `e.Shutdown(ctx)` with a 10s deadline, then close the SQLite pool.
- **Sane HTTP timeouts** on the underlying `http.Server`
  (`ReadHeaderTimeout=5s`, `ReadTimeout=15s`, `WriteTimeout=30s`,
  `IdleTimeout=120s`). Echo v5 lets you set these via the
  `e.Server` struct before `e.Start`.

### 1.6 `internal/users`

- `User` struct: `ID, Handle, Email, PasswordHash, IsAdmin, CreatedAt,
  UpdatedAt`.
- `Store` methods needed this sprint:
  `Create(ctx, NewUser) (User, error)`,
  `GetByHandle(ctx, handle) (User, error)`,
  `GetByID(ctx, id) (User, error)`,
  `AdminExists(ctx) (bool, error)` (used by `admin create`).
- Lowercases handle and email in Go before insert/lookup.
- Sentinel errors: `ErrNotFound`, `ErrHandleTaken`, `ErrEmailTaken`.

### 1.7 `internal/auth`

- `Hash(password) (string, error)` and `Verify(hash, password) error`
  using `bcrypt` cost 12.
- `Claims` struct embedding `jwt.RegisteredClaims` plus `Handle string`
  and `Admin bool`.
- `Issue(user, key, ttl) (string, error)` and the `echo-jwt` config:
  ```
  TokenLookup:   "cookie:auth,header:Authorization:Bearer "
  NewClaimsFunc: func(c) jwt.Claims { return new(Claims) }
  SigningKey:    cfg.JWTSecret
  ```
- `RequireAuth` is the `echo-jwt` middleware itself.
- `RequireAdmin(next)` reads `*Claims` from context and 403s when
  `Admin` is false.
- Cookie helpers: `SetAuthCookie(c, token)` and `ClearAuthCookie(c)`
  applying the attributes from DESIGN.md §8.1.
- Handlers:
  - `GET  /login`  — render `pages/login.html` (or 302 to `/` when
    already authenticated).
  - `POST /login`  — verify credentials, issue JWT, set cookie, redirect
    to `/`. On HTMX, return `HX-Redirect: /` instead of 302.
  - `POST /logout` — clear cookie, redirect to `/`.

### 1.8 `internal/server`

- `New(cfg, deps) *echo.Echo` wires middleware in this order:
  request logger (slog) → recovery → security headers → CSRF →
  static file server (`/static/*`).
- Global error handler maps sentinel errors and `echo.HTTPError` to
  `pages/error.html` (or a partial when `HX-Request: true`).
- `Renderer` (in `render.go`):
  - Loads templates once at construction via `template.ParseFS`.
  - On `Render(w, name, data, c)`:
    - Name starts with `partials/` → execute that template alone.
    - `HX-Request: true` and `HX-Boosted` empty → execute only the
      `content` block of the named page template.
    - Otherwise → execute `layout.html` with the named page's blocks.
- CSRF: Echo's `middleware.CSRF()` with the cookie variant. The login
  form must include the `_csrf` hidden input; the integration test
  (1.10) covers this end-to-end so wiring mistakes show up immediately.
- Security headers per DESIGN.md §12 — start with the documented CSP and
  loosen only if Alpine triggers a violation; verify in the browser
  before declaring sprint-done.

### 1.9 Templates

- `web/templates/layout.html` — defines `title`, `content`, `scripts`
  blocks; loads `/static/pico.min.css`, `/static/app.css`,
  `/static/htmx.min.js`, `/static/alpine.min.js`, and the small
  `htmx:configRequest` snippet that copies the CSRF cookie to
  `X-CSRF-Token`.
- `pages/home_public.html` — "what is huck" content + prominent **Sign
  in** button linking to `/login`. No registration link.
- `pages/home_authed.html` — "welcome to huck, {{ .Handle }}" plus
  navigation. If `.Admin`, show an `Admin` link (the page it points to
  arrives in Sprint 2; for now it can render a 404/placeholder).
- `pages/login.html` — handle + password form posting to `/login`,
  CSRF token included.
- `pages/error.html` — minimal status + message.

### 1.10 Tests

- `internal/users` store tests using `file::memory:?cache=shared` with
  the embedded migrations applied — exercise unique constraints and
  lowercasing.
- `internal/auth` unit tests: bcrypt roundtrip, JWT issue/verify
  roundtrip with the typed `*Claims` (this is the
  silent-cast guard that `echo-jwt` README warns about).
- `internal/db` migration test: apply twice, second run is a no-op;
  also exercises the bootstrap-table-missing branch.
- `internal/server` integration test using `httptest`:
  1. Build a real echo + DB with the bootstrap admin pre-inserted.
  2. `GET /login` to grab the CSRF cookie.
  3. `POST /login` with the cookie + `X-CSRF-Token` (or `_csrf` field) +
     credentials.
  4. Assert `Set-Cookie: auth=...; HttpOnly; Secure`-ish.
  5. `GET /` with the cookie → body contains "welcome".
  6. `POST /logout` → `auth` cookie cleared.
  7. `GET /` again → body contains "what is huck".

---

## 2. Build order

The Oracle-recommended order, expanded:

1. **Bootstrap** — module, dirs, vendored static assets, AGENTS-conformant skeleton.
2. **`internal/config`** — flag/env binding, per-command validation, no
   handlers depend on it yet so it can be tested standalone.
3. **`internal/db`** — `Open` (existing-only), `Create`, `Migrate`.
   Includes the schema_migrations bootstrap test.
4. **`internal/users` + `internal/auth` primitives** — store, bcrypt,
   JWT (no Echo yet). Unit-tested in isolation.
5. **`huck db create` and `huck db migrate` subcommands** — first
   user-visible CLI surface; verifies steps 2–3 from the operator path.
6. **`huck admin create` subcommand** — depends on `users` + bcrypt.
   Refuses to run when no DB exists or migrations are pending; emits a
   clear "run `huck db migrate` first" message in that case.
7. **Echo skeleton + Renderer + static + `GET /` (best-effort auth)** —
   render `home_public.html` for everyone for now (no JWT middleware
   wired yet).
8. **JWT middleware, login form, `POST /login`, `POST /logout`** — the
   moment the integration test in 1.10 becomes possible.
9. **CSRF, security headers, slog request logging** — wired last so each
   piece can be added with the integration test as a guard.
10. **Polish**: error handler templates, README quickstart, manual
    smoke-test in a real browser (verify Pico styling + CSP + Alpine).

---

## 3. Out of scope (deferred to Sprint 2)

- `internal/invites` (store, token generation, handlers).
- `internal/mail` (`Mailer` interface + Mailgun implementation).
- `pages/signup.html`, `/signup/:token` routes.
- Admin pages (`/admin/invites`, `/admin/users`).
- Anything that requires `--mailgun-*` or `--base-url` to be set.
- `/health` endpoint (revisit only if a probe actually needs it).

---

## 4. Risks and guardrails

| Risk | Mitigation |
| ---- | ---------- |
| Migrator can't read `schema_migrations` on first run because it doesn't exist yet. | Probe `sqlite_master` first; treat absence as "no versions applied." Unit-tested. |
| `admin create` run before migrations leaves a confusing error. | Detect zero migrations applied → exit with "run `huck db migrate` first" instructions. |
| `GET /` returns 401 for anonymous visitors. | Implement as a plain handler, not under `RequireAuth`; parse the cookie best-effort. |
| CSRF wired wrong, login silently 403s. | The login integration test (1.10) is the canonical guard. |
| `echo-jwt` typed claims silently downgraded to `jwt.MapClaims`. | JWT roundtrip test asserts the exact `*Claims` type. |
| Strict CSP breaks Alpine attribute expressions. | Verify in a real browser before sprint-done; loosen the rule (and document) only if needed. |
| Mailgun config required at boot, blocking development. | Per-command validation: `serve` must NOT require Mailgun flags in Sprint 1. |
| Server hangs forever on shutdown. | `e.Shutdown(ctx)` with 10s deadline + explicit pool close on `SIGINT`/`SIGTERM`. |

---

## 5. Definition of done

- All code paths in §1 implemented and the build order in §2 followed
  (or any deviation noted in `docs/DESIGN.md` change log).
- `go build ./...`, `go vet ./...`, `go test ./...` all clean.
- The operator-path commands at the top of this document succeed on a
  fresh checkout, on macOS and Linux, with no CGO toolchain.
- The integration test in §1.10 passes.
- Manual smoke test in Chrome **and** Firefox: log in, see Pico-styled
  authed home, log out, see Pico-styled public home. No CSP violations
  in the dev-tools console.
- README quickstart works verbatim.
- This document is moved to `docs/sprint-1-DONE.md` (or marked **Done**
  at the top) and `docs/sprint-2.md` is created with the deferred items.
