# Sprint 6 — Implementation Plan

Status: **Draft 2026-05-12.** Pending Sprint 5 close.

> **Note:** This is a draft prepared while Sprint 5 was still being
> planned. Do **not** start work until Sprint 5 closes and this plan
> has been reviewed against the actual Sprint-5-landed code. Likely
> updates after Sprint 5 closes:
>
> - Reconcile any schema or store API drift introduced by Sprint 5
>   (especially in `internal/users` — the bridge table joins users
>   and the new `suspended_at` / `last_login_at` columns may affect
>   the Players/Gamemasters count queries).
> - Pin the partial-index pattern Sprint 5 used for soft-deletes
>   (the `archived_at` column on `games` and the `is_active` column
>   on `game_users` follow the same shape).
> - Update the entry checklist with the actual Sprint-5 closing
>   commit hash and any contracts that moved.
> - Re-confirm the sidebar slot ordering ("Games" between "Users"
>   and the future "Server" entry).

Sprint 6 is the second of three "admin tasks" sprints driven by
Miko's design notes in [`docs/admin-tasks-design.md`](admin-tasks-design.md).
It introduces the **Games** metadata domain and the admin surface
for managing it. The Server console (Sprint 7) and the in-game player
experience (driven by the eventual game engine) are explicitly
out of scope.

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

Before starting T1.1, confirm:

- Sprint 5 is closed and `main` carries: the `0002_user_status.sql`
  and `0003_invite_status.sql` migrations; the `RecordLogin` /
  `Suspend` / `Reactivate` user-store methods; soft-revoked invites;
  the "Administration" / "Invitations" copy rename.
- Schema is at versions 1, 2, 3; this sprint adds 4 and 5.
- The admin pages render through `layout_app.html` with the Sprint-4
  shell; sidebar uses `SectionAdmin*` constants.
- No `internal/games` package exists yet.
- The "Games" sidebar slot is not present (Sprint 5 deliberately did
  not stub it — no dead links, per `front-end-plan.md` §2).

---

## Design decisions (settled with the human before drafting)

1. **Two concepts of "game".** Huck owns game **metadata** (id,
   name, status, slug, current turn, membership). The future game
   **engine** owns game state, orders, turn artifacts. Sprint 6
   ships only the metadata domain.
2. **Membership is a bridge table.** `game_users (user_id, game_id,
   is_gamemaster, is_active, …)`. `is_active=0` is the soft-delete
   equivalent for membership; we never hard-delete a row.
3. **A game cannot have zero active gamemasters.** Removing or
   demoting the last GM is refused at the store layer with an
   explicit sentinel error and surfaced as a 422 by the handler.
4. **Soft delete everywhere.** Archived games stay in the table with
   `archived_at` set and remain visible in the list (with a clear
   Archived badge), mirroring the Sprint-5 suspended-user pattern.
5. **Status taxonomy.** Per Miko: `Setup`, `Active`, `Paused`,
   `Archived`. Stored as TEXT for human-grepability. Validated in
   Go before insert.
6. **Slug.** Lowercase letters, digits, and `-`. 3–32 characters.
   Globally unique. Used in URLs going forward; for Sprint 6 the
   admin URLs still use numeric IDs, but the slug exists so the
   future game-engine pages can mount under `/games/<slug>/…`.
7. **Last activity** for the list page is `updated_at` for now — any
   write to the games or `game_users` rows bumps it. The future
   game engine will write turn-processing timestamps directly into
   this column.
8. **Sidebar.** A new "Games" entry lands under the Administration
   section (between Users and the eventual Server entry from
   Sprint 7). No game-scoped sidebar yet — that ships when the
   game-engine domain mounts URLs under `/games/<slug>/…`.
9. **No game-engine routes, no player-facing surfaces.** Sprint 6
   is admin-only.

---

## In scope

| Task | Status | Commit | Notes |
|------|--------|--------|-------|
| T1.1 | TODO   |        |       |
| T1.2 | TODO   |        |       |
| T2.1 | TODO   |        |       |
| T2.2 | TODO   |        |       |
| T3.1 | TODO   |        |       |
| T3.2 | TODO   |        |       |
| T3.3 | TODO   |        |       |
| T3.4 | TODO   |        |       |
| T3.5 | TODO   |        |       |
| T3.6 | TODO   |        |       |
| T4   | TODO   |        |       |
| T5   | TODO   |        |       |

Task order: schema migrations land first (T1.x) so the package
compiles; the `games` package + bridge methods (T2.x) come next;
admin handlers + pages (T3.x) consume both store layers; sidebar
plumbing (T4) and DESIGN.md (T5) close the sprint.

### T1.1 — Migration `0004_games.sql`

Add the games metadata table.

- Create `migrations/0004_games.sql` with:

    ```sql
    -- 0004_games.sql -- Games metadata (Sprint 6).
    CREATE TABLE games (
        id              INTEGER PRIMARY KEY,
        name            TEXT    NOT NULL,
        slug            TEXT    NOT NULL UNIQUE, -- lowercased in Go
        description     TEXT    NOT NULL DEFAULT '',
        status          TEXT    NOT NULL,        -- one of: setup|active|paused|archived
        current_turn    INTEGER NOT NULL DEFAULT 0,
        archived_at     TEXT,                    -- NULL until archived
        last_activity_at TEXT   NOT NULL,        -- bumped on any games or game_users write
        created_at      TEXT    NOT NULL,
        updated_at      TEXT    NOT NULL
    );
    ```

- The `slug` is the source of truth for URL identity going forward.
  We keep the numeric `id` for admin URLs in Sprint 6 to mirror
  `/admin/users/:id`; the game-engine domain will mount under
  `/games/<slug>/…`.
- No application-code changes in this task; the package update lands
  in T2.1.

### T1.2 — Migration `0005_game_users.sql`

Add the membership bridge table.

- Create `migrations/0005_game_users.sql` with:

    ```sql
    -- 0005_game_users.sql -- Game membership (Sprint 6).
    CREATE TABLE game_users (
        game_id        INTEGER NOT NULL REFERENCES games(id) ON DELETE RESTRICT,
        user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
        is_gamemaster  INTEGER NOT NULL DEFAULT 0,
        is_active      INTEGER NOT NULL DEFAULT 1,  -- 0 = removed (soft delete)
        created_at     TEXT    NOT NULL,
        updated_at     TEXT    NOT NULL,
        PRIMARY KEY (game_id, user_id)
    );

    CREATE INDEX game_users_user_id ON game_users(user_id);
    ```

- One membership row per `(game_id, user_id)`; toggling
  `is_gamemaster` flips the role. Re-adding a previously removed
  member is a `is_active = 1` toggle, not an insert.
- `ON DELETE RESTRICT` on both FKs is defence-in-depth: the user
  store no longer hard-deletes (Sprint 5), and we don't hard-delete
  games either (T2.1), so this should never fire — but it loud-fails
  if someone introduces a hard-delete path later.

### T2.1 — `internal/games` package: model + games store

Create the package and ship the games-only CRUD surface.

- `internal/games/games.go` — `Game` model with all `0004_games.sql`
  columns plus derived helpers: `func (g Game) Archived() bool`,
  `func (g Game) Status() Status`. `Status` is a typed string with
  constants `StatusSetup`, `StatusActive`, `StatusPaused`,
  `StatusArchived` and a `Validate(s string) (Status, error)` helper
  used by the create/edit handlers.
- Sentinel errors: `ErrNotFound`, `ErrSlugTaken`, `ErrInvalidStatus`,
  `ErrInvalidSlug`. Slug rule: `^[a-z0-9-]{3,32}$` plus first char
  must be alphanumeric. Mirror the lowercasing-in-Go pattern from
  `users.Normalise`.
- `Store` methods (against `*sqlitex.Pool`):
  - `Create(ctx, NewGame) (Game, error)` — inserts; on
    `SQLITE_CONSTRAINT_UNIQUE` for `slug`, returns `ErrSlugTaken`
    using the same same-conn `SELECT 1` classifier pattern from
    `users.classifyInsertErr`.
  - `GetByID(ctx, id) (Game, error)`
  - `GetBySlug(ctx, slug) (Game, error)`
  - `ListAll(ctx) ([]Game, error)` — orders by `updated_at DESC`.
  - `Update(ctx, id, UpdateGame) error` — partial update for name,
    description, status, current_turn. Bumps `updated_at` and
    `last_activity_at`.
  - `Archive(ctx, id) error` — sets `archived_at = now`,
    `status = 'archived'`, bumps `updated_at` /
    `last_activity_at`. No-op if already archived.
- `internal/games/games_test.go` covers each method against the
  in-memory SQLite pattern Sprint 1+ already uses, including the
  slug-uniqueness classifier and the partial-update Update path.

### T2.2 — `internal/games` package: membership bridge

Add the `game_users` API and the last-active-GM guard.

- `internal/games/members.go` — `Member` model (user_id, game_id,
  is_gamemaster, is_active, timestamps) with derived helpers
  `IsActive`, `IsGamemaster`.
- New sentinel: `ErrLastGamemaster` — returned when an operation
  would leave the game with zero active gamemasters.
- `Store` methods:
  - `ListMembers(ctx, gameID) ([]MemberWithUser, error)` — joins
    against `users` to populate handle/email for display. Returns
    both active and inactive members; the caller filters.
  - `CountActiveGamemasters(ctx, gameID) (int, error)` — used by
    the guard.
  - `AddMember(ctx, gameID, userID, isGM bool) (Member, error)` —
    insert-or-reactivate. If a row exists with `is_active=0`, set
    `is_active=1` and apply the new `is_gamemaster` value. Bumps
    `last_activity_at` on the game.
  - `RemoveMember(ctx, gameID, userID) error` — sets
    `is_active=0`. If the row currently has `is_gamemaster=1`,
    refuse if `CountActiveGamemasters - 1 == 0` → `ErrLastGamemaster`.
  - `SetGamemaster(ctx, gameID, userID, isGM bool) error` — toggles
    role on an active member. If `isGM=false` and the row is
    currently a GM, refuse if it would drop active GM count to
    zero.
- All membership writes happen inside a single transaction with a
  `last_activity_at` bump on the parent game.
- Tests cover the last-GM guard exhaustively: removing the only
  active GM fails; demoting the only active GM fails; removing one
  of two GMs succeeds; reactivating a removed member preserves
  their old `is_gamemaster` value? — no, takes the new value
  passed by the caller (loud, predictable).

### T3.1 — Admin games list page

Bring up the read-only list. Sidebar entry lands in T4.

- Routes (registered inside the existing `admin` group):
  - `admin.GET("/games", s.handleAdminGamesList)`
- `pages/admin_games.html` columns per Miko: **Name**, **Status**
  (badge: Setup/Active/Paused/Archived), **Current turn**,
  **Players** (count of active non-GM members), **Gamemasters**
  (count of active GM members), **Created**, **Last activity**,
  **Actions** (View, Edit, Manage gamemasters, Archive).
- Show all games — archived ones included — with a clear Archived
  status badge. No filtering controls in this sprint.
- Build the per-row counts in a single batched query in the games
  store (e.g. `SELECT game_id, SUM(is_gamemaster) AS gms,
  SUM(1 - is_gamemaster) AS players FROM game_users WHERE is_active = 1
  GROUP BY game_id`) so the list does not N+1.
- `usersShell`-style `gamesShell(claims)` helper in
  `internal/server/admin_games.go` builds the
  `[Home, Administration, Games]` breadcrumb trail and sets the
  topbar/sidebar.

### T3.2 — Admin games create form

Add the create form Miko sketched.

- Routes:
  - `admin.GET("/games/new", s.handleAdminGamesNewForm)`
  - `admin.POST("/games", s.handleAdminGamesCreate)`
- `pages/admin_game_new.html` form fields: **Name** (required),
  **Slug** (required, validated against the `games` package rule;
  show the rule inline), **Description** (textarea, optional),
  **Initial status** (radio: Setup/Active; default Setup),
  **Starting turn number** (numeric, default 0),
  **Assigned gamemasters** (multi-select of all non-suspended users;
  must select at least one).
- Validation: at least one GM required at create-time so the
  last-GM invariant holds from the first row written.
- Create-and-assign happens in a single transaction: `Create` the
  game, then `AddMember(isGM=true)` for every selected user. The
  `last_activity_at` bumps cascade.
- Re-render with field-level errors on validation failure, mirroring
  the Sprint-2/5 admin-edit error pattern.
- Tests cover: happy path; duplicate slug → 422 with
  `ErrSlugTaken`; zero GMs → 422 form error.

### T3.3 — Admin game detail page

Read-only summary + entry points to manage / archive.

- Route:
  - `admin.GET("/games/:id", s.handleAdminGamesView)`
- `pages/admin_game_view.html` shows: name, slug, description,
  status badge, current turn, created/updated/last-activity
  timestamps (using `fmtUTC`), counts of active GMs and players,
  and a small actions block: Edit, Manage gamemasters, Archive.
- Shell: `[Home, Administration, Games, <game name>]`. Topbar
  title is the game name.

### T3.4 — Admin game edit form

Edit name / description / status / current turn.

- Routes:
  - `admin.GET("/games/:id/edit", s.handleAdminGamesEditForm)`
  - `admin.POST("/games/:id/edit", s.handleAdminGamesEditSubmit)`
- Slug is **not** editable in Sprint 6 (it would invalidate any
  bookmarks once the game-engine pages mount under `/games/<slug>/…`).
  Add a comment in the handler citing this.
- Status changes between Setup/Active/Paused are free. Setting
  status=Archived from this form is **not** allowed — Archive is a
  separate action (T3.6) so the language can carry a confirmation
  step. Reject with a form-level error if attempted.
- Tests cover the slug-locked behaviour and the
  archive-via-edit-rejected behaviour.

### T3.5 — Admin game gamemasters panel

Dedicated panel for membership management on the detail page.

- Routes:
  - `admin.GET("/games/:id/gamemasters", s.handleAdminGameGMsForm)`
  - `admin.POST("/games/:id/gamemasters/add", s.handleAdminGameGMsAdd)`
  - `admin.POST("/games/:id/gamemasters/:userId/remove", s.handleAdminGameGMsRemove)`
- `pages/admin_game_gms.html` lists current active GMs with a
  Remove button (Alpine `confirm()` with the text Miko prescribed)
  and an Add form: a select of non-suspended users not currently
  GMs of this game, plus an Add button.
- `Remove` calls `Store.SetGamemaster(gameID, userID, false)`. If
  the package returns `ErrLastGamemaster`, render the panel with a
  banner: "This game must keep at least one active gamemaster.
  Add another gamemaster before removing this one." (422.)
- `Add` calls `Store.AddMember(gameID, userID, true)`. Reactivating
  a previously-removed member is allowed and surfaces with a
  notice: "Reactivated <handle> as a gamemaster."
- Tests cover the last-GM error path, add-new, add-reactivated.
- This sprint does **not** add a player-management panel — players
  are part of the game-engine domain and arrive with that work.

### T3.6 — Admin game archive

Soft-delete / archive a game.

- Route:
  - `admin.POST("/games/:id/archive", s.handleAdminGameArchive)`
- The detail page (T3.3) carries an Archive button that POSTs
  here. Use Miko's prescribed copy on the confirm:
  "Archive game — this will hide the game from active dashboards
  but preserve its records and artifacts." Use the small Alpine
  `confirm()` pattern; no modal. (Type-the-name-to-archive friction
  is **not** required per Miko's "Possibly require…" note.)
- Handler calls `games.Store.Archive(ctx, id)`, redirects to
  `/admin/games/:id` (which now shows status=Archived). Idempotent:
  archiving an already-archived game is a 200 redirect, not an
  error.
- Tests cover the happy path and the idempotency rule.

### T4 — Sidebar entry + section constant + breadcrumbs

Wire the new "Games" admin entry into the shell.

- Add `SectionAdminGames = "admin-games"` to
  `internal/server/breadcrumbs.go`.
- Update `web/templates/partials/sidebar.html`: under the
  Administration section, add a "Games" link to `/admin/games`
  between Users and any future Server entry. The partial picks up
  `aria-current="page"` from the section constant automatically.
- Update sidebar tests to assert the new entry appears for admins
  and is hidden for non-admins.
- All games handlers populate
  `Sidebar.Section = SectionAdminGames`.

### T5 — DESIGN.md updates

Document the new contracts.

- §4 (Project layout): add `internal/games/` and the new templates.
- §7.4 (Initial schema): add a follow-up section listing the
  Sprint-6 tables (`games`, `game_users`) with their column lists.
  Do **not** edit the released `0001_init.sql` snippet.
- §10 (Routes): add the new admin routes (list, new, view, edit,
  gamemasters CRUD, archive) with their guards.
- Add a new §17 entry summarising Sprint 6 (games metadata domain
  introduced; bridge table for membership; archive is soft;
  last-active-GM invariant enforced).
- Optional: add a short §X "Games metadata vs. game engine" note
  clarifying that Sprint 6 owns metadata only and the game engine
  domain will live in a future package.

---

## Out of scope

- **Game-engine state, turn processing, orders upload, downloads.**
  Lives in a future game-engine package, possibly an entirely
  separate repo.
- **Player management UI.** Adding/removing players is a
  game-engine concern; Sprint 6 only manages gamemasters.
- **Game-scoped sidebar.** Per `front-end-plan.md` §2, the game
  scope appears when the URL is scoped to a specific game. Lands
  with the game-engine work, not here.
- **Renaming an existing game's slug.** Slugs are immutable in
  Sprint 6; future sprints can introduce slug-history if real
  product need emerges.
- **Bulk operations** (archive multiple games, batch-add members).
  Not justified for the expected user-base size.
- **Search / filter / sort controls** on the games list. The list
  is small and orders by recent activity; controls add complexity
  without value at this scale.

---

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new `games` package tests
  for slug uniqueness, partial updates, the last-GM guard, archive
  idempotency, and the count-batching list query).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint6.db` succeeds; `huck db migrate`
  is a no-op the second time. `schema_migrations` contains rows
  1–5.
- Manual smoke-test of `huck serve`:
  - Create a game with at least one GM → it appears in
    `/admin/games` with the right status / GM count / player count
    (zero, since players land later).
  - Edit the game name and description; status transitions
    Setup → Active → Paused round-trip cleanly.
  - Open the gamemasters panel; add a second GM; remove the
    original — the second remains. Try to remove the last GM →
    422 with the Miko-prescribed copy.
  - Archive the game from the detail page → it shows status
    Archived in the list and the archive button is gone.
  - Sidebar shows the new "Games" entry only for admins; it
    becomes the current entry on every `/admin/games*` URL.
  - Browser dev tools confirm no HTMX swap replaces anything
    outside `.huck-content` (Sprint 4 invariant still holds).

---

## Change log

- **2026-05-12** — Drafted from Miko's design + the
  Sprint 5 planning thread; pending Sprint 5 close. Will be
  re-read against Sprint-5-landed code before Sprint 6 starts.
