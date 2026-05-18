# Sprint 5 — Implementation Plan

Status: **Ready 2026-05-12.**

Sprint 5 is the first of three "admin tasks" sprints driven by Miko's
design notes in [`docs/admin-tasks-design.md`](admin-tasks-design.md).
It covers the **Users** and **Invitations** sections plus the matching
sidebar/copy rename. The remaining two sections from Miko's design —
**Games** (a new metadata domain with a `users ↔ games` bridge table)
and **Server** (the operational status + graceful-shutdown console) —
are deliberately deferred:

- Sprint 6 will introduce the games metadata model, the bridge table,
  and the admin Games surface.
- Sprint 7 will introduce the Server status dashboard and the
  graceful-shutdown action (later wired to a systemd-touched marker
  file at deploy time).

The contracts that matter (schema, password/handle policies, invite
flow, routes, security headers) live in `docs/DESIGN.md`. This file is
a sprint plan — when a task changes a contract, DESIGN.md is the
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
matching the hash convention used in earlier sprint docs. A task commit
cannot contain its own hash, so agents should report the hash after
committing. Fill the column only if explicitly asked to make a separate
bookkeeping update, or if a human updates it afterward.

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

- Sprint 4 is closed and `main` carries the app shell + Phase-2
  CSS primitives.
- Schema is at `0001_init.sql` only; no `0002_*.sql` exists yet.
- `internal/users/users.go` exposes `Store.Create*`, `GetByID`,
  `GetByHandle`, `ListAll`, `SetAdmin`, `SetPassword`, `Delete`,
  `AdminExists`. Sprint 5 removes `Delete` and adds `RecordLogin`,
  `Suspend`, `Reactivate`; the `SetPassword` method stays but loses
  its admin-edit-form caller.
- `internal/invites/invites.go` exposes `Store.Create`, `GetByToken*`,
  `List`, `Resend`, `Revoke` (currently a `DELETE`), `Consume`. Sprint 5
  changes `Revoke` to a soft-delete (`UPDATE … SET revoked_at = now`)
  and teaches the package a `Status()` derivation.
- The admin pages (`admin.html`, `admin_users.html`,
  `admin_user_view.html`, `admin_user_edit.html`, `admin_invites.html`)
  render inside `layout_app.html` and the `.huck-content` width
  contract from Sprint 4.
- The `invites_email_active` partial index (from `0001_init.sql`) is
  the only uniqueness on `invites.email`.
- `cmd/huck/main.go` startup runs `db.Migrate` on every `huck serve`
  (Sprint 3 T4 documented that contract).

---

## Design decisions (settled with the human before drafting)

These are the answers that drive the task list. They are recorded
here so reviewers don't re-litigate them mid-sprint.

1. **Soft delete everywhere.** Suspended users and revoked invites stay
   in the table and the list. Hard `DELETE` of users is removed from
   the codebase entirely; `DELETE` of invites becomes
   `UPDATE … SET revoked_at = now`.
2. **Suspended users.** Cannot get a new JWT. Any existing JWT keeps
   working until expiry. Project policy is to rotate `--jwt-secret` if
   a token must die early — there is no per-user revocation list.
3. **No admin-set passwords.** The password field on
   `/admin/users/:id/edit` is removed. In alpha the recovery story is
   "admin reaches the user out of band (Discord, etc.)"; a real
   password-reset email flow is an exit-beta criterion, **out of scope
   for this sprint**.
4. **Invite role is a server-side fact.** The invite row carries
   `is_admin`. Signup ignores any role hint in the form and reads
   `invites.is_admin` from the row. Form tampering cannot promote.
5. **Two-step admin invite confirmation.** A single endpoint
   (`POST /admin/invites`) branches on `is_admin && !confirm`. The
   first POST renders an interstitial with the normalised values
   re-posted as hidden fields; the confirm POST creates and sends.
6. **No new sidebar entries yet.** Games and Server entries land with
   their respective sprints (no dead links — see
   `docs/front-end-plan.md` §2). Sprint 5 only renames "Admin" →
   "Administration" and "Invites" → "Invitations" everywhere
   user-visible.
7. **No new CSS primitives.** Sprint 5 reuses `.huck-shell`,
   `.huck-page-header`, `.huck-form-stack`, etc. from Sprint 4.
   `docs/front-end-plan.md` §4 is the closed vocabulary.

---

## In scope

| Task | Status | Commit | Notes |
|------|--------|--------|-------|
| T1.1 | DONE   | e13bfc7 | Migration applied; second `db migrate` is a no-op; schema_migrations holds versions 1 and 2. |
| T1.2 | DONE   | 3a2f91c | Added `LastLoginAt`/`SuspendedAt` + `IsSuspended()`; new `RecordLogin`/`Suspend`/`Reactivate`; removed `Store.Delete`. Suspend uses an existence pre-check so an idempotent re-suspend still returns nil while a missing id returns `ErrNotFound`. Verified with narrower scope (`go build/vet/test ./internal/users/...`) — full-tree build break in `internal/server/admin_users.go:242` (`s.users.Delete`) and the matching `account_test.go` caller is the intentional signal called out in this task; T4.2 removes the handler+route. |
| T2.1 | DONE   | f2bb09e | Migration adds `revoked_at` + `is_admin`; rebuilds `invites_email_active` to predicate on `consumed_at IS NULL AND revoked_at IS NULL`. Verified `db.Create` + second `db.Migrate` is a no-op with `schema_migrations` holding versions 1, 2, 3. Narrower verification scope (`internal/db/...`) because the full-tree build is still broken at `internal/server/admin_users.go:242` (`s.users.Delete`) — the intentional signal called out in T1.2 that T4.2 will remove. |
| T2.2 | DONE   | 4c97fd9 | Adds `RevokedAt`/`IsAdmin` + `Revoked()`/`Status()`; introduces `NewInvite` input struct; `Create`/`CreateOnConn` now take it (signup_test + admin_invites_test + admin_invites.go callers updated); `Revoke` soft-deletes via `UPDATE … SET revoked_at = now`; `Resend`/`Consume` refuse revoked invites with new `ErrRevoked`; `activeInviteExists` predicate now mirrors the partial index (`consumed_at IS NULL AND revoked_at IS NULL`). Narrower verification (`internal/invites/...`) because the full-tree build is still broken at `internal/server/admin_users.go:242` (the intentional T1.2 → T4.2 signal). Central `ErrRevoked` error mapping is deferred to T3.2 per the sprint plan. |
| T3.1 | DONE   | fea6b17 | Login handler now checks `user.IsSuspended()` *after* password verify (so wrong-password attempts can't probe suspension state) and refuses with 403 + "This account has been suspended." instead of issuing a JWT. Successful logins call `users.RecordLogin`; ErrNotFound there falls through to the generic credentials failure (race with the user vanishing). Suspended message uses an inline 403 render, not a new sentinel — keeps the auth package decoupled. New `login_suspend_test.go` covers the four cases from the task spec. Verified the test suite by temporarily stubbing the T1.2-signal `handleAdminUsersDelete` callers (admin_users.go and account_test.go), running `go build/vet/test ./...` clean (only `TestAdminUsersDelete` failed under the stub, which T4.2 removes), then reverted the stub so the intentional build break stays in place. Committed-state verification is `go build/vet/test` of `internal/{users,invites,db,auth}/...` — clean — plus the unchanged `internal/server/admin_users.go:242` break that T4.2 closes. |
| T3.2 | DONE   | 1cb977a | Signup form (GET) refuses revoked invites with `invites.ErrRevoked`; submit (POST) re-checks `Revoked()` inside the same transaction as `Consumed()`/`Expired(now)`; `users.CreateOnConn` now passes `IsAdmin: inv.IsAdmin` so the role is the server-side invite fact (form tampering cannot promote — DESIGN.md §9). `internal/server/errors.go` maps `invites.ErrRevoked` to 410 Gone with "This invitation has been revoked." (mirrors `ErrConsumed`); `renderSignupFailure` propagates `ErrRevoked` to the central handler. `signupErrorMessage` left untouched — `ErrRevoked` always reaches the central handler before the form-render path. New tests in `signup_test.go`: GET revoked → 410 + "revoked" copy, POST revoked → 410, admin invite → `IsAdmin = true`, regular invite → `IsAdmin = false`. Verified the full `./internal/server/...` suite by stubbing the T1.2 → T4.2 signal (`s.users.Delete` in `admin_users.go:242` and `account_test.go:77`), running `go build/vet/test ./...` (only `TestAdminUsersDelete`/`TestAdminUsersDeleteMissing`/`TestAccountDeletedUserClearsCookie` failed — the three tests T4.2 removes), then reverted the stub so the intentional break remains. Committed-state verification is `go build/vet/test` of `internal/{invites,users,db,auth}/...` — clean — plus the unchanged `admin_users.go:242` break that T4.2 closes. |
| T4.1 | DONE   | 4470b0c | Reshapes admin users list: column order Name, Email, Role, Status, Last login, Created, Actions; `userRowView` gains `Role`/`Status`/`LastLogin`/`LastLoginISO`; never-logged-in rows render `—` and omit `<time>`. List page already had only View/Edit actions (no Delete to remove). Tests pin the new headers, the em-dash, the absence of Delete, and populated-LastLogin via `RecordLogin`+`Suspend` (covers Active/Suspended Status and the populated/em-dash LastLogin branches by counting `<time>` elements). Verified `./internal/server/...` by stubbing the T1.2 → T4.2 signal (`s.users.Delete` in `admin_users.go:262` + `account_test.go:77`), running `go build/vet/test ./...` (only `TestAdminUsersDelete`/`TestAdminUsersDeleteMissing` failed — the two tests T4.2 removes; `TestAccountDeletedUserClearsCookie` passed via `t.Skip`), then reverted the stub so the intentional break remains. Committed-state verification is `go build/vet/test` of `internal/{users,invites,db,auth}/...` — clean — plus the unchanged `admin_users.go:262` break that T4.2 closes. |
| T4.2 | DONE   | 264d8a9, 0329ae9 | Detail page gains an **Account status** section (Active/Suspended + suspended-at) and a single Suspend/Reactivate button gated by the Alpine `@click.prevent` confirm pattern. Removed `handleAdminUsersDelete` + the `POST /users/:id/delete` route; replaced with `handleAdminUsersSuspend` (self-guard: 403 "You cannot suspend yourself…") and `handleAdminUsersReactivate` (no self-guard — a user cannot suspend themselves so cannot reactivate themselves either). Both handlers use `hxRedirect` for HTMX parity and target `/admin/users/:id`. `adminUserView` gains `Status`/`SuspendedAt`/`SuspendedAtISO`; `newAdminUserView` derives them from `User.IsSuspended()`. Dropped the now-orphan delete form on `admin_user_edit.html` since the route is gone. Tests: replaced the three `TestAdminUsersDelete*` tests with `TestAdminUsersViewShowsSuspendForActive`, `TestAdminUsersViewShowsReactivateForSuspended`, `TestAdminUsersSuspend`/`SuspendRefusesSelf`/`SuspendMissing`, `TestAdminUsersReactivate`/`ReactivateMissing`. Also removed `TestAccountDeletedUserClearsCookie` (its only path to a deleted user was `Store.Delete`, removed in T1.2); the defensive `ErrNotFound` branch in `handleAccount` is left in place — out of scope for this task. Closes the intentional T1.2 → T4.2 build break — full `go build/vet/test ./...` now clean. |
| T4.3 | DONE   | e628f1c | Edit form drops the password fieldset; `handleAdminUsersEditSubmit` no longer reads/validates/hashes/saves a password (the `is_admin` toggle + self-demote 403 guard stay). Removed the now-orphan `adminUserPasswordErrorMessage` helper and its `TestAdminUserPasswordErrorMessageDelegates` test; `passwordErrMsg` stays because signup still uses it (`TestPasswordErrMsg` + `TestSignupErrorMessageDelegates` cover that path). `users.Store.SetPassword` is left in place per the sprint plan — a real password-reset flow can adopt it later. Tests: dropped `TestAdminUsersEditResetPassword`/`TestAdminUsersEditWeakPassword`; flipped `TestAdminUsersEditForm` and `TestAdminUsersEditRendersAppShell` to assert the absence of `name="password"`; added `TestAdminUsersEditIgnoresPassword` that POSTs a `password` form value and confirms the stored hash is unchanged (template drift can't silently re-enable the feature). Full `go build/vet/test ./...` clean. |
| T5.1 | DONE   | b5bd0c3 | Admin invites list reshape: header order Email/Role/Status/Sent/Expires/Actions; `inviteRowView` gains `Role` + `CanRevoke`; `rowViewAt` now derives Status via `Invite.Status(now)` (Pending/Accepted/Expired/Revoked) and actionability via `status == Pending \|\| status == Expired` — Accepted and Revoked rows render with no Resend/Revoke. Page H1 → "Invitations"; breadcrumb final crumb `Label: "Invites"` → `"Invitations"`. Sidebar/topbar `Title:` strings and the page `<h2>Existing invites</h2>` deliberately untouched (T6 owns the rest of the copy pass). New `TestAdminInvitesListColumnsAndRows` pins the header order, every-status-row presence, the per-status Role/Status text, and the action-gating; updated `TestAdminInvitesRendersAppShell` for the new H1. Full `go build/vet/test ./...` clean. |
| T5.2 | DONE   | 3c0b274 | Create form gains a `<fieldset>` of `User`/`Admin` radios (`name="role"`, User default-checked); `handleAdminInvitesCreate` reads `role` (defaulting to `"user"` for any value other than `"admin"`) and `confirm := c.FormValue("confirm") == "true"`. `role=admin && !confirm` short-circuits to `pages/admin_invite_confirm.html` (new template, layout `layout_app.html`) which re-POSTs `email`/`role=admin`/`confirm=true` as hidden fields — no DB write, no mail. Otherwise the existing create path runs with `IsAdmin: role == "admin"` threaded into `invites.NewInvite`. `renderAdminInvitesError` grew a `role` parameter so the radio selection survives validation/duplicate-error re-renders (`FormRole` echoed on `adminInvitesView`). New `adminInviteConfirmView{Email,Role}`. Tests: existing `TestAdminInvitesCreate` posts `role=user` and asserts `IsAdmin = false` on the row; new `TestAdminInvitesCreateAdminNeedsConfirm` pins the interstitial copy + hidden fields + zero side-effects; new `TestAdminInvitesCreateAdminConfirmed` pins the `IsAdmin = true` row + mail send; new `TestAdminInvitesCreateRoleSelector` pins the form's `<legend>Role</legend>` + both radios + User-default. Form-tampering safety (signup reading `invites.is_admin` server-side) is already covered by T3.2's `TestSignupAdminInvite`/`TestSignupRegularInviteNotAdmin`. Full `go build/vet/test ./...` clean. |
| T5.3 | DONE   | 46d7c96 | `inviteRowView` gains `Link` + `CanCopy`; `rowView`/`rowViewAt` now take a `link string` and the two callers in `admin_invites.go` thread `s.signupURL(inv)` through (`loadInviteRows` for the list, `handleAdminInvitesResend` for the HTMX swap). `handleAdminInvitesRevoke` now calls `Revoke` → `GetByToken` → renders `partials/invite_row.html` on HTMX (so the row stays visible with Status=Revoked and no Resend/Revoke/Copy actions); non-HTMX redirects to `/admin/invites`. `partials/invite_row.html` gains a `{{ if .CanCopy }}` `<button data-invite-url="{{ .Link }}">Copy invite link</button>` gated on the same `actionable` predicate as Resend/Revoke (Pending or Expired). `web/static/app.js` grows a CSP-clean `[data-invite-url]` click handler that `navigator.clipboard.writeText`s the URL and briefly flips the button text to "Copied" (1.5s) — the sprint plan's Alpine `x-text`+`setTimeout` snippet is not viable under our `script-src 'self'` CSP (no `unsafe-eval`, which Alpine's directive evaluator needs), so this follows the same `data-*` + `app.js` precedent T4.2 set for `data-confirm`. Tests: `TestAdminInvitesRevoke` now asserts the HTMX response carries the revoked row markup (id, `<td>Revoked</td>`, no Resend/Revoke/Copy); new `TestAdminInvitesCopyLink` pins `data-invite-url="http://huck.test/signup/<token>?email=<urlencoded-email>"` on a Pending row; new `TestAdminInvitesRevokedRowOmitsCopy` pins that a revoked row in the list page has Status=Revoked and no action buttons. Full `go build/vet/test ./...` clean. |
| T6   | DONE   | 740df56 | Sidebar `<h2>Admin</h2>` → `<h2>Administration</h2>`; sidebar Invites link/current → Invitations. All breadcrumb `Label: "Admin"` and `Label: "Invites"` in `internal/server` flipped to "Administration"/"Invitations" — `server.go` (admin index), `admin_invites.go` (invitesShell), `admin_users.go` (usersShell/userDetailShell/userEditShell). Topbar Title strings: admin index "Admin" → "Administration"; invites list "Invites" → "Invitations". Page H1s: `pages/admin.html` → "Administration" (plus the in-page prose "Admin dashboard"/"administration section" + the `<a href="/admin/invites">` link copy → "Invitations"); `pages/admin_invites.html` H2 "Existing invites" → "Existing invitations" (the H1 was already "Invitations" from T5.1). Role-value `"Admin"` (admin_invites.go:305, admin_users.go:136) and definition-list `<dt>Admin</dt>` on account/admin_user_view pages deliberately untouched — those are user-attribute labels, not section headers. Updated copy-pinning tests: `sidebar_test.go` (banned-`Invites`/`<h2>Admin</h2>` for non-admin → `Invitations`/`<h2>Administration</h2>`; admin-shows-section want strings; aria-current Invitations link), `topbar_test.go` ("Invitations" title + ordering check), `breadcrumbs_test.go` (Crumb labels, partial assertion strings, the ordering `strings.Index` lookups), `render_test.go` (every admin shell crumb/topbar/H1 assertion across the four admin pages), `admin_index_test.go` (`<h1>Administration</h1>`). `admin_invites_test.go` line 281 "Admin" wantRole left as-is. Full `go build/vet/test ./...` clean. |
| T7   | DONE   | 64c425b | DESIGN.md surgical updates: §7.4 adds a "Sprint 5 columns" block for `users.last_login_at`/`users.suspended_at`/`invites.revoked_at`/`invites.is_admin` + the recreated `invites_email_active` predicate (the released `0001_init.sql` snippet is left alone as a historical record); new §8.9 "Suspended users" pins the login-after-password-verify refusal + 403 copy + the "existing JWTs survive until exp, rotate `--jwt-secret` to mass-invalidate" wording cross-referencing the §2 non-goal; §9 numbered steps rewritten — Create branches on `role` with the admin two-step interstitial (no DB write/no mail on the first POST), Revoke is `UPDATE … SET revoked_at = now` (soft-delete; row stays for audit), Resend refuses revoked, Landing refuses revoked, Submit reads `invites.is_admin` server-side ("the signup form has no role field"); §10 routes table swaps `POST /admin/users/:id/delete` for `…/suspend` + `…/reactivate` and reworded the `…/edit` note to drop admin-set passwords; §11.2 grows a "User-visible labels" paragraph clarifying that "Administration"/"Invitations" are copy-only (URLs `/admin`, `/admin/invites` and Go identifiers unchanged); §17 grows a 2026-05-15 Sprint-5 entry summarising all of the above. Doc-only change — full `go build/vet/test ./...` clean (no code touched). Closes Sprint 5. |

Task order encodes the known dependencies: schema migrations land first
(T1.1, T2.1) so the store updates (T1.2, T2.2) compile against real
columns; auth-flow integration (T3.x) follows the store contracts;
admin UI (T4.x, T5.x) is last because it consumes both store layers; the
copy pass (T6) and DESIGN.md update (T7) close the sprint.

### T1.1 — Migration `0002_user_status.sql`

Add the soft-status columns to `users`. Keep the migration small and
idempotent on a fresh DB.

- Create `migrations/0002_user_status.sql` with:

    ```sql
    -- 0002_user_status.sql -- Add login tracking + soft-suspend.
    ALTER TABLE users ADD COLUMN last_login_at TEXT;
    ALTER TABLE users ADD COLUMN suspended_at  TEXT;
    ```

- Both columns are nullable; existing rows take `NULL` (= never logged
  in / not suspended).
- No application-code changes in this task. The store update lands in
  T1.2 to keep the migration commit reviewable on its own.
- Verify: `huck db create --db /tmp/huck-s5.db` followed by
  `huck db migrate --db /tmp/huck-s5.db` is a no-op the second time
  (the existing `TestMigrateAfterCreateIsNoOp` covers this contract;
  re-run it).

### T1.2 — `users` package: scan + status methods

Update `internal/users/users.go` so the new columns flow through the
model and the store.

- Add `LastLoginAt time.Time` and `SuspendedAt time.Time` to
  `users.User`. Use the same zero-time-on-NULL pattern the package
  already uses for timestamps. Add a derived helper
  `func (u User) IsSuspended() bool { return !u.SuspendedAt.IsZero() }`.
- Update every `SELECT` in the package to read the two new columns
  (the column list, the `ResultFunc` decoders in `ListAll` and
  `getOneOnConn`, and the `getOne` callsites).
- Add three new store methods:
  - `RecordLogin(ctx, id) error` — `UPDATE users SET last_login_at = ?,
    updated_at = ? WHERE id = ?`. Returns `ErrNotFound` on zero rows.
  - `Suspend(ctx, id) error` — sets `suspended_at = now()` and bumps
    `updated_at`. No-op if already suspended (still returns nil).
  - `Reactivate(ctx, id) error` — clears `suspended_at = NULL` and
    bumps `updated_at`.
- **Remove** `Store.Delete`. The hard-delete escape hatch goes away in
  this sprint (Suspend/Reactivate replaces it). The handler/route
  removal lands in T4.2; this task removes only the store method, so
  the compile failure in `admin_users.go` is intentional and signals
  that T4.2 must follow.
- Update `internal/users/users_test.go`:
  - extend the "create + read back" test to assert `LastLoginAt`,
    `SuspendedAt` are zero on a fresh row;
  - add tests for `RecordLogin`, `Suspend`, `Reactivate` (round-trip
    through `GetByID`);
  - drop the `Delete` test rows.

### T2.1 — Migration `0003_invite_status.sql`

Add the soft-revoke + role columns to `invites` and rebuild the
partial index so revoked rows no longer block re-invite.

- Create `migrations/0003_invite_status.sql` with:

    ```sql
    -- 0003_invite_status.sql -- Add soft-revoke + invite role.
    ALTER TABLE invites ADD COLUMN revoked_at TEXT;
    ALTER TABLE invites ADD COLUMN is_admin   INTEGER NOT NULL DEFAULT 0;

    -- "One active invite per email" must now also exclude revoked
    -- rows. Drop+create plainly; the index is guaranteed to exist
    -- from 0001_init.sql, and a "missing index" failure here is a
    -- legitimate signal of schema drift.
    DROP INDEX invites_email_active;
    CREATE UNIQUE INDEX invites_email_active
        ON invites(email)
        WHERE consumed_at IS NULL AND revoked_at IS NULL;
    ```

- Do **not** use `IF EXISTS` / `IF NOT EXISTS` on the index — the index
  exists in every released DB from 0001, and silent masking would hide
  a real schema-drift bug.
- No application-code changes in this task. The store update lands in
  T2.2.
- Verify: same idempotency check as T1.1 (run T1.1's migration, then
  this one, then re-run all migrations and confirm both
  `schema_migrations` rows survive).

### T2.2 — `invites` package: soft revoke + role

Update `internal/invites/invites.go` so the new columns flow through
the model, the store changes from hard-delete to soft-revoke, and the
new partial index predicate is reflected in classification logic.

- Add `RevokedAt time.Time` and `IsAdmin bool` to `invites.Invite`.
  Derived helpers: `func (i Invite) Revoked() bool { return !i.RevokedAt.IsZero() }`.
- Add an exported `Status` derivation that returns one of
  `"Pending"`, `"Accepted"`, `"Expired"`, `"Revoked"`. Order of
  precedence (highest first):
  1. `Revoked()`
  2. `Consumed()`
  3. `Expired(now)`
  4. otherwise `"Pending"`
  Place this on the `Invite` value: `func (i Invite) Status(now time.Time) string`.
- Update every `SELECT` in the package to read the two new columns
  (column lists, `ResultFunc` decoders, `getOne*` callsites).
- Add `IsAdmin bool` to `NewInvite`. Pass it through `Create` and
  `CreateOnConn` to the `INSERT` statement.
- Switch `Revoke` from `DELETE FROM invites WHERE token = ?` to
  `UPDATE invites SET revoked_at = ? WHERE token = ? AND revoked_at IS NULL`.
  Returns `ErrNotFound` when zero rows match (so revoking a
  not-found token, or an already-revoked token, both surface as
  `ErrNotFound`).
- Update `Resend` to refuse revoked invites (it already refuses
  consumed ones); add a `Revoked()` check that returns a new
  sentinel `ErrRevoked = errors.New("invite has been revoked")`.
- Update `classifyInsertErr` (and any same-connection `SELECT 1`
  helper it relies on) to mirror the new partial-index predicate:
  `WHERE email = ? AND consumed_at IS NULL AND revoked_at IS NULL`.
  Without this, a duplicate-active-invite collision would be
  classified incorrectly after a revoke + re-create cycle.
- Update `Consume` to additionally refuse revoked invites — the
  signup flow re-checks `Consumed()` and `Expired(now)` today; add a
  `Revoked()` check returning `ErrRevoked`.
- Extend `internal/invites/invites_test.go`:
  - new test: `Revoke` is idempotent (second call returns `ErrNotFound`);
  - new test: revoking an invite for `foo@example.com` allows a fresh
    `Create` for the same email (the partial-index predicate fix);
  - new test: `Resend` refuses revoked invites with `ErrRevoked`;
  - new test: `Status(now)` returns `Pending`/`Accepted`/`Expired`/
    `Revoked` for the four cases;
  - new test: `Create` round-trips `IsAdmin = true`.

### T3.1 — Login records `last_login_at`; refuses suspended users

Wire the new user columns into the auth path. Order of operations
matters — see the rationale comment.

- In `handleLoginSubmit` (`internal/server/server.go`), after a
  successful password verify, **before** issuing the JWT:
  1. If `user.IsSuspended()` → render the login page with
     `Error: "This account has been suspended. Contact an administrator."`
     and HTTP 403; do **not** issue a cookie. Do **not** record
     `last_login_at`.
  2. Otherwise call `s.users.RecordLogin(ctx, user.ID)`. Treat any
     non-`ErrNotFound` error as a 500.
  3. Then issue the JWT and set the cookie.
- The check order (find user → verify password → check suspended →
  record login → issue JWT) is deliberate:
  - Suspension is checked **after** password verify so a wrong-password
    attempt cannot probe whether an account is suspended.
  - `last_login_at` is updated **only** for fully successful logins.
- Do not introduce a new `auth.ErrSuspended` sentinel — the inline
  handler message + 403 status is sufficient and avoids cross-package
  coupling.
- Tests in `internal/server/server_test.go` (or a new
  `login_suspend_test.go` if it grows the file too much):
  - successful login bumps `last_login_at` (read back via
    `users.GetByID`);
  - suspended user with correct password is refused with 403, no
    auth cookie, and `last_login_at` is unchanged;
  - suspended user with wrong password gets the same generic
    "Unknown handle or wrong password" 401 path (no leak);
  - reactivated user can log in again on the next attempt.

### T3.2 — Signup honours invite role + revoked state

Wire the new invite columns into the signup path.

- In `handleSignupForm` (GET), after `GetByToken` succeeds, additionally
  refuse revoked invites: surface `invites.ErrRevoked`, mapped by the
  central error handler the same way `ErrConsumed` and `ErrExpired`
  already are.
- In `handleSignupSubmit` (POST), inside the existing transaction:
  - `if inv.Revoked() { return invites.ErrRevoked }` immediately after
    the existing `Consumed()` and `Expired(now)` checks.
  - When calling `users.CreateOnConn`, pass `IsAdmin: inv.IsAdmin`.
    The signup form does **not** carry a role field; the server reads
    the invite row.
- Update `internal/server/errors.go` to map `invites.ErrRevoked` to a
  friendly status (treat it like `ErrConsumed` — `410 Gone` with a
  message like "This invitation has been revoked.").
- Update `signupErrorMessage` to include the new sentinel where
  appropriate (or rely on the central handler — pick the one that
  matches the existing `ErrConsumed` treatment exactly).
- Update `signup_test.go`:
  - signup against a revoked invite renders the revoked error (GET
    and POST paths both covered);
  - signup against an admin invite produces a user with `IsAdmin = true`;
  - signup against a regular invite still produces `IsAdmin = false`.

### T4.1 — Admin users list page

Reshape `pages/admin_users.html` and `handleAdminUsersList` to match
Miko's design. No new sidebar entries; this is a page-content change.

- Columns (in this order): **Name** (handle), **Email**, **Role**
  (Admin/User), **Status** (Active/Suspended), **Last login**,
  **Created**, **Actions**.
- "Last login" renders the same `display + ISO datetime` pair the
  existing `fmtUTC` helper produces. For a never-logged-in user
  (`LastLoginAt.IsZero()`), render `—` (em dash) in the display
  column and omit the `<time>` element.
- "Status" shows `Active` for normal rows and `Suspended` for rows
  with `SuspendedAt` set. Suspended rows stay in the list (no filter,
  no opacity tricks beyond Pico's defaults).
- "Actions" column carries links/buttons for **View**
  (`/admin/users/:id`) and **Edit** (`/admin/users/:id/edit`).
  **Remove the existing Delete action.** Suspend/Reactivate live on
  the detail page (T4.2), not in this row.
- `userRowView` (`internal/server/admin_users.go`) gains
  `Role string`, `Status string`, `LastLogin string`,
  `LastLoginISO string`. Build them in `handleAdminUsersList`.
- Update `admin_users_test.go` list assertions to pin the new column
  set, the never-logged-in em-dash, and the absence of any Delete
  link/button.

### T4.2 — Admin user detail + Suspend/Reactivate

The detail page gains a Status section per Miko's design and acquires
the only entry points for Suspend/Reactivate.

- In `pages/admin_user_view.html`:
  - Add an **Account status** section under the existing detail block
    showing "Active" or "Suspended" (matching `fmtUTC`-style
    `display + ISO` for the suspension timestamp when present).
  - Add a single button:
    - "Suspend account" if `Active` → `POST /admin/users/:id/suspend`
    - "Reactivate account" if `Suspended` → `POST /admin/users/:id/reactivate`
  - Use the small Alpine `confirm()` pattern Sprint 4 already permits
    (`@click.prevent="…confirm…"`); this is the minimal-friction
    confirm Miko prescribed.
  - **Remove the existing "Delete account" button/form.** The hard-delete
    capability is gone.
- Add two handlers in `internal/server/admin_users.go`:
  - `handleAdminUsersSuspend` — looks up the user, refuses self
    (mirror the existing self-demote 403 message: "You cannot suspend
    yourself. Ask another admin to make this change."), calls
    `s.users.Suspend(ctx, id)`, redirects to
    `/admin/users/:id`. Uses `hxRedirect` for HTMX parity.
  - `handleAdminUsersReactivate` — same shape; no self-guard needed
    (a user cannot suspend themselves so cannot reactivate themselves
    either).
- **Remove** `handleAdminUsersDelete` and the
  `admin.POST("/users/:id/delete", …)` route registration.
- Add the new routes inside the existing `admin` group in
  `installRoutes`:
  - `admin.POST("/users/:id/suspend",    s.handleAdminUsersSuspend)`
  - `admin.POST("/users/:id/reactivate", s.handleAdminUsersReactivate)`
- `adminUserView` gains `Status string` and the suspended-at fields
  if the template needs them.
- Tests in `admin_users_test.go`:
  - GET detail of an active user shows "Suspend account" and not
    "Reactivate";
  - GET detail of a suspended user shows "Reactivate" and not
    "Suspend";
  - POST suspend on self returns 403 with the self-guard message;
  - POST suspend on another user redirects to detail and the user is
    suspended in the DB;
  - POST reactivate clears `suspended_at`;
  - The previously-tested `POST /admin/users/:id/delete` route is gone
    — drop those assertions (no replacement).

### T4.3 — Admin user edit page (drop password field)

Per Miko + the human's confirmation, admins can no longer change
passwords. The edit form shrinks to just the role toggle.

- In `pages/admin_user_edit.html`:
  - Remove the `<input type="password" name="password" …>` field, its
    label, and the surrounding rhythm.
  - Keep the `is_admin` toggle and the existing self-demote disabled
    state.
- In `handleAdminUsersEditSubmit` (`internal/server/admin_users.go`):
  - Remove the `password := c.FormValue("password")` block, the
    `auth.ValidatePassword` call, the `auth.Hash` call, and the
    `s.users.SetPassword` call.
  - Keep the existing self-demote 403 guard.
- `users.Store.SetPassword` stays in the package — Sprint 7+ may add
  a real password-reset flow that uses it. Just no admin caller.
- Drop `adminUserPasswordErrorMessage` (the only caller goes away).
  If `passwordErrMsg` becomes unused after this drop, **leave it** —
  signup still uses it. Confirm with `go vet` + `go build` before
  removing anything.
- Update `admin_users_test.go`:
  - Drop the password-set / password-validation tests for the admin
    edit endpoint.
  - Add a test that `POST /admin/users/:id/edit` with a `password`
    form value present **ignores** it (does not change the hash).
    This pins that future template drift can't silently re-enable the
    feature.

### T5.1 — Admin invitations list

Reshape `pages/admin_invites.html` and `handleAdminInvitesList` to
show the full audit history with status and role.

- Columns: **Email**, **Role** (Admin/User), **Status**
  (Pending/Accepted/Expired/Revoked), **Sent** (created_at),
  **Expires**, **Actions**.
- All invites render — pending, accepted, expired, and revoked. No
  filtering toggles. The list orders by `created_at DESC` so the most
  recent activity is at the top.
- Status uses `Invite.Status(now)` from T2.2.
- Actions per row depend on status:
  - **Pending** (not yet expired): Resend, Revoke, Copy link (T5.3).
  - **Pending** (expired): Resend (extends expiry), Revoke, Copy link.
  - **Accepted**: no actions (the invite is consumed).
  - **Revoked**: no actions.
- Page heading reads **Invitations** (not "Invites"). The breadcrumb
  trail helper in `admin_invites.go` updates from `Label: "Invites"` to
  `Label: "Invitations"` (sidebar matches in T6).
- Update `admin_invites_test.go` for the new column set and the
  presence of revoked/accepted rows in the table body.

### T5.2 — Admin invite create + admin two-step confirmation

Add the Role selector to the create form and the interstitial
confirmation for admin invites.

- In `pages/admin_invites.html` create form:
  - Add a Role control: a `<fieldset>` with two radios, `User`
    (default) and `Admin`. Use `name="role"`, values `"user"` /
    `"admin"`.
- In `handleAdminInvitesCreate` (`internal/server/admin_invites.go`):
  - Read `role := c.FormValue("role")` (default `"user"`).
  - Read `confirm := c.FormValue("confirm") == "true"`.
  - Branch:
    - If `role == "admin"` and `!confirm` → render
      `pages/admin_invite_confirm.html` (new template) with the
      already-normalised email, the role, and a hidden
      `confirm=true` input. **Do not** create the invite or send
      mail. The confirmation page presents:
      - "You are about to send an invitation that will create an
        **administrator** account for `<email>`."
      - "Cancel" → links back to `/admin/invites`.
      - "Send admin invitation" → POSTs back to `/admin/invites` with
        `email`, `role=admin`, `confirm=true`.
    - Otherwise → existing create-and-send path, with
      `IsAdmin: role == "admin"` passed into `invites.Store.Create`.
- The confirmation page renders inside the app shell with the
  existing `[Home, Admin, Invitations]` breadcrumb trail
  (no extra crumb — it's a transient step on the same logical page).
- `pages/admin_invite_confirm.html` is a tiny template:
  `.huck-page-header` + a `<form>` carrying the three hidden fields
  and the Cancel/Send buttons.
- The signup flow (already updated in T3.2) reads
  `invite.IsAdmin` from the row, so a tampered POST that sets
  `role=admin` without the prior interstitial **cannot** promote — the
  invite was created with `IsAdmin: false`.
- Tests in `admin_invites_test.go`:
  - POST create with `role=user` creates a regular invite immediately
    (existing happy path; assert `is_admin = 0` on the row).
  - POST create with `role=admin` and no `confirm` returns 200 with
    the confirmation page (no DB write, no mail).
  - POST create with `role=admin` and `confirm=true` creates an
    admin invite (`is_admin = 1`) and sends mail.
  - POST signup against an admin invite (extending T3.2's coverage,
    cross-referenced) yields a user with `IsAdmin = true`.

### T5.3 — Soft-revoke + Copy invite link

Switch the revoke handler to soft-delete and replace the HTMX row
removal with a row re-render. Add the Copy-link button.

- `handleAdminInvitesRevoke` already calls `s.invites.Revoke` — that
  store call now performs `UPDATE … SET revoked_at = now` (T2.2). The
  handler change is in the **response shape**:
  - On a non-HTMX request: redirect back to `/admin/invites` (the row
    will reappear with status Revoked on the next render).
  - On an HTMX request (the existing path): re-render
    `partials/invite_row.html` with the revoked row instead of
    returning an empty 200. The row must show Status=Revoked and the
    no-actions cell (matching T5.1 rules).
- Update `partials/invite_row.html` to render the same column layout
  T5.1 establishes for the table body, including the Status column
  and the actions-by-status branching.
- Add a **Copy link** button to each row that has actionable status
  (pending). Implementation:
  - The button uses an Alpine snippet that copies
    `${BASE_URL}/signup/<token>?email=<urlencoded-email>` to the
    clipboard via `navigator.clipboard.writeText(...)`. The full URL
    is rendered into a `data-invite-url="…"` attribute on the button
    so no JSON serialization is needed.
  - The button's accessible label is "Copy invite link"; on success,
    the visible text briefly toggles to "Copied" via Alpine's
    `x-text` + `setTimeout`. Keep the snippet tiny and inline; do
    not introduce a new JS file.
  - The full URL is built server-side in
    `handleAdminInvitesList` (use `s.cfg.BaseURL`) and passed into
    `inviteRowView` as `Link string`.
- Tests in `admin_invites_test.go`:
  - `Revoke` returns the revoked row markup on HTMX, not an empty
    body.
  - The list page renders revoked rows with no Resend/Revoke/Copy
    buttons.
  - The Copy-link button's `data-invite-url` attribute matches
    `${BASE_URL}/signup/<token>?email=<urlencoded-email>`.
  - The unique partial-index test from T2.2 (revoke-then-recreate) is
    cross-linked here for reviewer attention but lives in the invites
    package.

### T6 — User-visible copy pass: "Administration" + "Invitations"

A targeted rename across every place a human reads the words "Admin"
(as a section header) or "Invites" (as a section/page label).

- `web/templates/partials/sidebar.html`: header text "Admin" →
  "Administration". The sidebar entry "Invites" → "Invitations".
- All breadcrumb construction sites (search for `Label: "Admin"` and
  `Label: "Invites"` in `internal/server`):
  - "Admin" → "Administration"
  - "Invites" → "Invitations"
- All topbar `Title:` strings:
  - "Admin" (the dashboard topbar title) → "Administration"
  - "Invites" → "Invitations"
- Page H1 / page-header strings inside the templates affected:
  - `pages/admin.html` H1
  - `pages/admin_invites.html` H1
- Do **not** rename:
  - URL paths (`/admin`, `/admin/invites`) — these are stable.
  - Go identifiers (`SectionAdminInvites`, `handleAdminInvitesList`,
    `usersShell`, etc.) — these are internal.
  - Existing comments and doc strings unless they describe
    user-facing copy.
- Update the affected tests in `internal/server` that pin specific
  copy ("Invites", "Admin" header, etc.) to the new text. Use
  `rg -l 'Invites|>Admin<'` from `internal/server/` to find them.
- Sidebar/breadcrumb structural tests (T1.3 / T1.4 from Sprint 4)
  should keep passing without change beyond the label strings.

### T7 — DESIGN.md updates

Document the contract changes. Keep edits surgical; don't rewrite
sections that didn't change.

- §7.4 (Initial schema): add a follow-up paragraph or a small
  "Sprint 5 columns" note describing the four new columns and the
  recreated partial index. Do **not** edit the released
  `0001_init.sql` snippet — it is a historical record of the initial
  schema.
- §8 (Authentication): one paragraph noting that login refuses
  suspended users (no JWT issued; existing JWTs survive until expiry;
  rotate `--jwt-secret` to mass-invalidate). Reference the
  "no per-user revocation list" non-goal in §2.
- §9 (Invite flow): update the numbered steps:
  - **Create**: invites carry an `is_admin` flag; admin invites are
    confirmed via a one-step interstitial before the row is created.
  - **Revoke**: now `UPDATE … SET revoked_at = now`, not `DELETE`.
    The row remains for audit.
  - **Submit**: signup reads `invites.is_admin` and creates the user
    with that role; refuses revoked invites.
- §10 (Routes): replace the
  `POST /admin/users/:id/delete` row with two rows:
  - `POST /admin/users/:id/suspend` — admin — Suspend (soft).
  - `POST /admin/users/:id/reactivate` — admin — Clear suspension.
- Add a sentence to §11.2 noting that "Administration" / "Invitations"
  are the user-visible labels for the admin section / invitations
  page; URL paths remain `/admin` / `/admin/invites`.
- Add a Sprint-5 entry to §17 (Change log).

---

## Out of scope

- **Games metadata** — table, bridge to users, admin UI.
  Deferred to Sprint 6 with its own design pass.
- **Server status dashboard + graceful shutdown** — read-only ops
  console + the systemd-marker mechanism. Deferred to Sprint 7.
- **Self-service password reset.** Out-of-band recovery (admin reaches
  the user via Discord) is the alpha story. A real
  token + email + landing flow lands when the project exits alpha.
- **Per-user JWT revocation / refresh tokens.** Project-level policy
  is to rotate `--jwt-secret` if a token must die early. See
  DESIGN.md §2 non-goals.
- **New CSS primitives, design tokens, mobile collapse, light/dark
  toggle.** All deferred per `docs/front-end-plan.md` §8.
- **Sidebar entries for Games or Server.** Sidebar reflects facts
  that are true now (no dead links — `front-end-plan.md` §2). They
  ship with their respective sprints.

---

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including new tests for `RecordLogin`,
  `Suspend`/`Reactivate`, the soft-revoke + recreate-after-revoke
  partial-index behaviour, the four `Invite.Status` cases, the
  suspended-login refusal, the admin-invite confirmation step, and
  the password-form-ignored guard on `/admin/users/:id/edit`).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint5.db` succeeds on a fresh
  path; `huck db migrate --db /tmp/huck-sprint5.db` is a no-op the
  second time. The `schema_migrations` table contains rows for
  versions 1, 2, 3.
- Manual smoke-test of `huck serve`:
  - **Users:** create a user invite → land on signup → log in →
    `last_login_at` populates. Admin suspends the user from the
    detail page → user fails to log in with the suspended message
    (still 403). Admin reactivates → user logs in again. The Delete
    button no longer exists anywhere.
  - **Invitations:** create a user invite (no confirmation step).
    Create an admin invite → land on the confirmation interstitial →
    confirm → signup → resulting user has `is_admin = 1`.
  - **Soft-revoke:** revoke an active invite → it remains in the
    list with Status=Revoked and no Resend/Revoke/Copy actions.
    Create a fresh invite for the same email → succeeds (the
    partial index allows it).
  - **Copy link:** click "Copy invite link" → clipboard contains
    `${BASE_URL}/signup/<token>?email=<urlencoded-email>`. Pasting
    that URL into a new browser tab opens the signup form.
  - **Copy pass:** sidebar header reads "Administration", invitations
    entry reads "Invitations", page H1 / breadcrumbs / topbar titles
    match.
  - Browser dev tools confirm no HTMX swap replaces anything outside
    `.huck-content` (Sprint 4 invariant still holds; the soft-revoke
    re-render targets the row inside the table inside `.huck-content`).

---

## Change log

- **2026-05-12** — Drafted from `docs/admin-tasks-design.md` (Miko)
  and the Sprint 5 planning thread.
