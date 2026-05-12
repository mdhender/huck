# Sprint 7 — Implementation Plan

Status: **Draft 2026-05-12.** Pending Sprints 5 and 6 close.

> **Note:** This is a draft prepared while Sprint 5 was still being
> planned. Do **not** start work until Sprints 5 and 6 close and this
> plan has been reviewed against the actual landed code. Likely
> updates after Sprint 5 / 6 close:
>
> - Reconcile schema-version count (this sprint's status dashboard
>   reads from `schema_migrations`; Sprint 5 lands versions 2 and 3,
>   Sprint 6 lands 4 and 5, so the dashboard at start of Sprint 7
>   should show "current schema: 5").
> - Pin the actual sidebar slot ordering finalised by Sprint 6 so
>   the new "Server" entry can land beneath "Games".
> - Confirm the graceful-shutdown wiring against the Sprint-3 T4
>   `runServe` doc comment — it already handles SIGTERM/SIGINT;
>   this sprint just needs to trigger that path from a handler.
> - Re-confirm the build-time version injection is still the
>   simplest route (vs. Go 1.18+ `runtime/debug.ReadBuildInfo`).

Sprint 7 is the third of three "admin tasks" sprints driven by
Miko's design notes in [`docs/admin-tasks-design.md`](admin-tasks-design.md).
It introduces the **Server** ops console: a read-only Server Status
dashboard plus a single Server Action — graceful shutdown ("Stop
services") with type-`STOP` friction. The remaining ops actions
Miko sketched (Restart, Maintenance mode) are deliberately deferred
until they are actually needed.

The contracts that matter (schema, password/handle policies, invite
flow, routes, security headers) live in `docs/DESIGN.md`. This file
is a sprint plan — when a task changes a contract, DESIGN.md is the
document to update first.

---

## Agent Execution Rules

Work exactly one task row at a time.

Task selection:

1. Select the first task with `Status = TODO`
2. If the entry checklist or a dependency is not satisfied, mark the
   task `BLOCKED`, explain why in `Notes`, and stop
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
matching the hash convention used in earlier sprint docs. Fill the
column only if explicitly asked to make a separate bookkeeping update,
or if a human updates it afterward.

Rules:

- Never work on multiple task rows simultaneously
- Never modify unrelated files
- Never skip updating the sprint table
- Never continue automatically after completing a task
- Prefer small, isolated, reversible commits
- Record blockers explicitly in `Notes`
- Preserve the sprint plan as the source of truth for progress

---

## Entry checklist

Before starting T1, confirm:

- Sprints 5 and 6 are closed and `main` carries: the Sprint-5
  user/invite columns and the Sprint-6 games metadata + bridge.
- Schema is at versions 1–5; this sprint adds **no new migrations**.
- The admin sidebar carries Dashboard / Users / Invitations / Games
  under the Administration section. The new "Server" entry slots
  in beneath Games.
- `cmd/huck/main.go` `runServe` still wires
  `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` and
  calls `e.Shutdown(ctx)` on cancellation (Sprint 3 T4 contract).
- The Pico-based confirmation pattern from Sprint 5 (Suspend /
  Reactivate / Archive) is in place; Sprint 7 follows the same
  "no modal, dedicated confirmation page for the dangerous one"
  shape.

---

## Design decisions (settled with the human before drafting)

1. **Read-only first.** The Status dashboard is read-only and
   contains nothing destructive. It can ship before any Server
   Action handler exists.
2. **One Action: Stop services.** Restart and Maintenance Mode are
   deferred until real operational need surfaces. Stop is the
   minimum viable ops action.
3. **Stop = graceful shutdown.** The handler triggers the existing
   SIGTERM path in `runServe` so shutdown follows the same code
   path as `kill -TERM <pid>`. No duplicate shutdown plumbing.
4. **Future systemd handshake** (touch a marker file so the unit
   does not auto-restart) is out of scope until Huck has a real
   deploy story. The handler leaves a clean seam (a small
   `Shutdown` interface on the Server) for a future
   `cmd/huck/main.go` to wire the marker-file step in front of
   the SIGTERM trigger.
5. **Friction.** Miko prescribes "type STOP". Sprint 7 implements
   that as a dedicated confirmation page (not a modal) with a
   text input that must equal `STOP` before the destructive
   button is enabled. The button is the secondary visual style;
   Cancel is primary.
6. **Server section in the sidebar.** A new "Server" entry under
   Administration. The dashboard is the section's only page in
   this sprint; the Stop confirmation page reuses the same section
   (it's the same Server surface, not a separate area).
7. **No new schema.** All facts on the dashboard are read from
   process state, file metadata, or existing tables.

---

## In scope

| Task | Status | Commit | Notes |
|------|--------|--------|-------|
| T1   | TODO   |        |       |
| T2   | TODO   |        |       |
| T3   | TODO   |        |       |
| T4   | TODO   |        |       |
| T5   | TODO   |        |       |
| T6   | TODO   |        |       |
| T7   | TODO   |        |       |

Task order: build-time version plumbing first (T1) so the dashboard
can render real data; the read-only ServerInfo gatherer (T2)
follows; the Status page (T3) consumes T2; the Stop friction page
(T4) and Stop handler (T5) are independent of the dashboard but
benefit from the same shell scaffolding; sidebar/copy (T6) and
DESIGN.md (T7) close the sprint.

### T1 — Build-time version injection

Pin a real version string into the binary so the dashboard can
display it.

- Add `var Version = "dev"` near the top of `cmd/huck/main.go` (or
  in a new `cmd/huck/version.go` if that keeps the file tidier).
- Update the build invocation documented in `README.md` to use
  `-ldflags "-X 'main.Version=$(git describe --tags --always --dirty)'"`.
  No build script change is forced — the default `go build ./...`
  still produces a working binary with `Version = "dev"`, which is
  the right behaviour for `go test` and local dev.
- Expose the version through the `Server` struct as a constructor
  parameter: `New(cfg, pool, ..., logger, version string)`. Update
  the single caller in `cmd/huck/main.go` to pass `Version`.
  Tests pass `"test"` (a small constant) so the dashboard render
  test has a stable string to assert.
- Briefly note in `cmd/huck/version.go` that the project does not
  use `runtime/debug.ReadBuildInfo` — `-ldflags` is a single
  source of truth and works the same in CI / local / packaged
  builds.

### T2 — `internal/server/serverinfo.go`: gather facts

Single struct, single gather function, no destructive code.

- New file `internal/server/serverinfo.go` defining:

    ```go
    type ServerInfo struct {
        Version        string    // build-time injected
        SchemaVersion  int       // max(version) from schema_migrations
        DBPath         string    // cfg.DBPath
        DBSizeBytes    int64     // os.Stat(cfg.DBPath).Size()
        DBSizeDisplay  string    // human-readable, e.g. "12.4 MiB"
        ActiveUsers    int       // suspended_at IS NULL
        TotalUsers     int       // including suspended
        Uptime         time.Duration
        UptimeDisplay  string    // e.g. "3h 41m"
        StartedAt      time.Time // process start
        Environment    string    // cfg.Environment (existing) or HUCK_ENV
        GoVersion      string    // runtime.Version()
    }

    func (s *Server) gatherInfo(ctx context.Context) (ServerInfo, error)
    ```

- "Active users" definition: `users` rows with `suspended_at IS NULL`.
  Add `users.Store.CountActive(ctx) (int, int, error)` returning
  `(active, total, error)` — a single query with two `SUM(...)`
  expressions. Reuse it from the dashboard.
- The gatherer is a pure function of `cfg`, `pool`, and process
  state. It does **not** mutate anything. Never returns an error
  for "DB stat failed" — log and substitute zero so a missing file
  does not make the dashboard 500. (The DB file is the runtime;
  it is always present when `huck serve` is running.)
- Tests cover: schema-version read; counts honour the
  Sprint-5 suspended column; uptime display formats correctly;
  DB size display picks the right unit (B / KiB / MiB / GiB).

### T3 — Admin server status page

The dashboard. Read-only. No actions on this page beyond the entry
into T4.

- Routes:
  - `admin.GET("/server", s.handleAdminServerStatus)`
- `pages/admin_server.html` renders a card-per-fact layout (Pico
  `<article>` per row group, no new CSS primitives). Group facts
  per Miko's outline:
  - **Build:** version, Go version.
  - **Database:** schema version, path, size.
  - **Users:** active / total.
  - **Process:** uptime, started, environment.
- Place the **Operational Actions** block visually separated at
  the bottom: a `<section>` with its own heading and a single
  link "Stop services" pointing at `/admin/server/stop` (T4).
  Per Miko: dangerous actions never sit next to harmless cards.
- Shell helper `serverShell(claims)` mirrors `usersShell` etc.:
  `[Home, Administration, Server]` breadcrumbs;
  `Section: SectionAdminServer`; topbar title "Server".
- Tests assert: each fact label appears; the version/uptime/db
  size strings render through to HTML; the Stop link is present
  but the destructive button is not (it lives on T4).

### T4 — Stop services confirmation page

The friction page Miko prescribed.

- Routes:
  - `admin.GET("/server/stop", s.handleAdminServerStopForm)`
- `pages/admin_server_stop.html`:
  - H1: "Stop services?"
  - Body: "Active users may be disconnected. Uploads or turn
    processing may be interrupted." (Miko's exact copy.)
  - A `<form method="post" action="/admin/server/stop">` with:
    - A required text input `name="confirm"` labelled "Type
      `STOP` to confirm." The input must contain `STOP`
      (case-sensitive) for the submit button to be enabled.
      Use a tiny inline Alpine snippet (`x-data` / `x-bind`) to
      gate the button — small, no new file, no new dependency.
    - Two buttons in this order, left-to-right:
      1. **Cancel** — primary visual style. `<a href="/admin/server">`.
      2. **Stop services** — secondary destructive style.
         `<button type="submit">`.
      The destructive button is **never** the first button, per
      Miko.
- Shell: same `serverShell` from T3 (no new breadcrumb crumb —
  this is a transient confirmation page on the Server section).
- The form re-posts to the same path; the GET handler renders the
  form, the POST handler is T5.
- Tests: the page renders; the input gate is wired; the
  destructive button is the second button.

### T5 — Stop services handler (graceful shutdown)

Trigger the existing SIGTERM path; do not duplicate shutdown
plumbing.

- Routes:
  - `admin.POST("/server/stop", s.handleAdminServerStop)`
- Handler shape:
  - Validate `c.FormValue("confirm") == "STOP"` server-side too
    (defence in depth). On mismatch, re-render the T4 page with
    a banner ("Type STOP to confirm.") and a 422.
  - On success:
    1. Render a small acknowledgement page or fragment ("Server
       is shutting down. This page will become unreachable in a
       moment.") and flush the response.
    2. From a goroutine with a small delay (e.g. 250ms — long
       enough for the response to flush), send `SIGTERM` to the
       process: `proc, _ := os.FindProcess(os.Getpid()); proc.Signal(syscall.SIGTERM)`.
       Wrap in a tiny helper `triggerShutdown()` so future
       sprints can swap in the systemd-marker step in front.
    3. Log an `slog.Info("admin shutdown requested",
       "actor", claims.Handle)` with no token / cookie material.
- Self-guard not needed (an admin shutting down the service is
  the intended use case; suspending oneself was the prevented
  case in Sprint 5).
- Tests:
  - POST with no `confirm` returns 422 and does not signal the
    process. (Use a fake `triggerShutdown` injected on the
    `Server` struct so the test does not actually kill the test
    binary; production wiring stays the real signal.)
  - POST with `confirm=STOP` calls the injected hook exactly once
    and renders the shutting-down acknowledgement.
- The actual shutdown path (Echo's `Shutdown(ctx)` with the
  graceful timeout from Sprint 3 T4) is exercised by the existing
  signal-handling code in `runServe` and is not duplicated here.

### T6 — Sidebar entry + section constant + breadcrumbs

Wire the new "Server" admin entry into the shell.

- Add `SectionAdminServer = "admin-server"` to
  `internal/server/breadcrumbs.go`.
- Update `web/templates/partials/sidebar.html`: under the
  Administration section, add a "Server" link to `/admin/server`
  beneath "Games" (the slot ordering is Dashboard / Users /
  Invitations / Games / Server).
- Update sidebar tests to assert the new entry appears for admins
  and is hidden for non-admins, and that the section is `current`
  on `/admin/server` and `/admin/server/stop`.
- All Server handlers populate
  `Sidebar.Section = SectionAdminServer`.

### T7 — DESIGN.md updates

Document the contract changes.

- §10 (Routes): add the three new admin routes (`GET /admin/server`,
  `GET /admin/server/stop`, `POST /admin/server/stop`).
- Add a new short section (e.g. §15 "Operations" or appended to an
  existing operations-flavoured section) describing the dashboard
  facts and the graceful-shutdown contract: handler triggers
  `SIGTERM` via `os.Process.Signal`, which the existing
  `runServe` signal handler turns into `e.Shutdown(ctx)` with the
  Sprint-3 graceful timeout. Note the deferred systemd-marker
  handshake.
- Add a Sprint-7 entry to §17 (Change log).
- The build-time `Version` and the `-ldflags` snippet land in
  `README.md` (T1 already updated it); a one-line cross-reference
  from DESIGN.md is enough.

---

## Out of scope

- **Restart services** as an in-app action. A real restart needs
  process supervision (systemd, etc.) and a clear story for
  reconnecting active sessions. Not justified for the alpha
  user-base; revisit when there is a deploy contract.
- **Maintenance mode.** A DB flag + middleware that returns 503
  to non-admin requests. Worthwhile but not yet needed; not
  needed for graceful shutdown either, since the SIGTERM path
  already drains in-flight requests via Echo's `Shutdown`.
- **systemd handshake / marker-file** for "stop and stay
  stopped". Lands when there is an actual systemd unit to
  coordinate with. T5 leaves the seam (`triggerShutdown`).
- **Live metrics** (request rate, error rate, DB latency). Wait
  for real operational pain.
- **Last-backup timestamp.** No backup story exists yet; adding
  the field with a permanent `—` is dead weight.
- **Audit log of admin actions** (who triggered the shutdown).
  T5 logs to slog; a queryable audit table is a separate sprint
  if it ever becomes needed.

---

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new `serverinfo`
  formatting tests, the active-users count test, and the
  Stop-services confirmation tests using the injected
  `triggerShutdown` hook).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint7.db` succeeds; `huck db migrate`
  is a no-op the second time. No new migrations — `schema_migrations`
  contains rows 1–5 (or whatever Sprint 5/6 finalised).
- A locally built `huck serve` binary built with the README's
  `-ldflags` snippet shows the real version on `/admin/server`.
- Manual smoke-test of `huck serve`:
  - `/admin/server` renders for admins; non-admins are redirected
    by the existing admin guard. The dashboard shows version,
    Go version, schema version, DB size, active/total user counts,
    uptime, environment, and a separated "Operational Actions"
    section with one link.
  - Clicking "Stop services" lands on the confirmation page.
    The destructive button is disabled until the user types
    `STOP`. Cancel returns to `/admin/server`.
  - Submitting with the wrong text returns 422 and the form.
  - Submitting with `STOP` renders the acknowledgement and the
    process exits cleanly within the Sprint-3 graceful timeout.
    `huck` re-launched manually serves the same DB without
    migration drift.
  - Sidebar shows "Server" only for admins; section is current
    on both `/admin/server` and `/admin/server/stop`.

---

## Change log

- **2026-05-12** — Drafted from Miko's design + the Sprint 5
  planning thread; pending Sprints 5 and 6 close. Will be
  re-read against Sprint-5/6-landed code before Sprint 7 starts.
