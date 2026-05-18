# Sprint 5 — Post-mortem review

Status: **Draft 2026-05-17.** Written against the Sprint-5-landed code
on `main` (migrations `0002_user_status.sql`, `0003_invite_status.sql`;
`internal/users`, `internal/invites`, `internal/server`). Intended as
input to the Sprint 6 entry checklist, which already calls out
"Pin the partial-index pattern Sprint 5 used for soft-deletes" and
"Reconcile any schema or store API drift introduced by Sprint 5".

These are findings, not blockers. Sprint 5 closed; nothing here
contradicts the design decisions agreed at the top of `docs/sprint-5.md`.
The aim is to surface code smells and missed optimisations *before*
Sprint 6 starts copying patterns into `internal/games` /
`game_users`.

Findings are grouped by severity. Each one names the file + line
range, what's wrong, and the suggested fix (smallest correct change
first).

---

## High — fix before Sprint 6 forks the pattern

**Status update 2026-05-17:** H1 and H2 have landed. H3 is the
documented project policy and is captured in DESIGN.md §8.9 — no
change planned. See per-finding notes below.

### H1. SQLite write transaction held open across the Mailgun HTTP call

`internal/server/admin_invites.go` lines 140–171. The create handler
runs `s.invites.CreateOnConn` and `s.mailer.Send` inside one
`sqlitex.Transaction`. SQLite has **one writer**; while Mailgun is
round-tripping we hold the write lock and block every other write
(suspend, reactivate, edit, future game create/archive). Mailgun
timeout = serialised stall.

The rollback-on-mail-failure semantics are real and we want to keep
them. Two cheap fixes:

1. **Insert outside the txn, revoke on mail failure.** Run
   `Create` (its own short txn), then `Send`. On send error, call
   `Revoke` and surface the same 5xx the user sees today. The
   row stays in the table as Revoked — consistent with Sprint 5's
   "soft delete everywhere" rule, and the partial index lets a
   re-invite for the same email succeed immediately. The audit
   trail gains a Revoked row, which is arguably *more* honest than
   silently rolling back.
2. **Stage the row.** Insert with `status = 'staged'`, send mail,
   then `UPDATE … SET status = 'active'`. Requires a schema
   addition — too much for a follow-up cleanup.

Recommend (1). Sprint 6 *will* introduce the same shape in
`handleAdminGamesCreate` (`Create` + multiple `AddMember` in a txn
— at least that one is all-DB, no HTTP). Lock in the rule now:
**no network I/O inside a SQLite write transaction.**

**Resolved 2026-05-17.** Approach (1) landed:
`handleAdminInvitesCreate` now calls `invites.Store.Create` outside
any transaction, then renders + sends the mail, then on either
render or send failure calls a new `revokeAfterMailFailure` helper
that logs the cause + revokes the row (and logs again if the revoke
itself fails). `sqlitex` is no longer imported by the handler.
`TestAdminInvitesCreateMailgunFailureRollsBack` was renamed to
`TestAdminInvitesCreateMailgunFailureRevokesInvite` and extended to
pin (a) one Revoked row remains after the mail failure, and (b) a
retry against the same email succeeds because the partial unique
index excludes revoked rows.

### H2. `Suspend` is idempotent, `Revoke` is not — both claim to be

`internal/users/users.go:219-242` vs. `internal/invites/invites.go:261-278`.

- `users.Store.Suspend` does a `SELECT 1` pre-check, then
  `UPDATE … WHERE id = ? AND suspended_at IS NULL`. Re-suspending a
  suspended user returns `nil` (idempotent). Missing id returns
  `ErrNotFound`.
- `invites.Store.Revoke` does
  `UPDATE … WHERE token = ? AND revoked_at IS NULL`, then
  `if conn.Changes() == 0 { return ErrNotFound }`. Re-revoking an
  already-revoked invite returns **`ErrNotFound`**, even though the
  invite plainly exists. The doc comment on `Revoke` says
  "idempotent at the caller surface" — that's wrong.

Sprint 6 T3.6 explicitly wants `Archive` to be idempotent
("archiving an already-archived game is a 200 redirect, not an
error"). Pick one pattern and use it everywhere:

- **Preferred:** the `Suspend` shape (existence pre-check, then
  conditional update). It's two queries but it disambiguates
  "missing" from "already in target state" cleanly and avoids
  surprising the handler with `ErrNotFound` on a valid id.
- Apply the same shape to `invites.Store.Revoke` (the
  `handleAdminInvitesRevoke` HTMX path currently can't tell whether
  it should re-render the row or 404).
- Plan to apply it to `games.Store.Archive` (T2.1) and
  `members.Store.RemoveMember` (T2.2).

A small helper in a shared place would prevent drift, but see L3
below before extracting prematurely.

**Resolved 2026-05-17.** `invites.Store.Revoke` now mirrors
`users.Store.Suspend`: a `tokenExists` pre-check returns
`ErrNotFound` for genuinely missing tokens, then a conditional
`UPDATE … WHERE token = ? AND revoked_at IS NULL` runs. Calling
Revoke on an already-revoked token returns `nil` (the `revoked_at`
timestamp does not move — pinned by the updated `TestRevoke`). A
new `TestRevokeMissing` pins the ErrNotFound branch. Sprint 6's
`games.Store.Archive` (T2.1) should adopt the same shape; pre-check
is the agreed convention for soft-delete idempotency across stores.

### H3. Suspended users keep full app access until their JWT expires

**Resolved (no code change).** This is documented project policy:
DESIGN.md §8.9 spells out that a user with `suspended_at IS NOT NULL`
cannot acquire a new JWT but existing JWTs survive until `exp`, and
the mass-invalidate path is to rotate `--jwt-secret`. §2 lists
"per-user JWT revocation lists" as an explicit non-goal. No
middleware re-check is planned. Leaving this entry in the review as
a pointer for future contributors who notice the gap.

---

## Medium — worth doing as part of Sprint 6 setup

### M1. `signupURL` recomputes the base on every call; `loadInviteRows` calls it per-row

`internal/server/admin_invites.go:284-288` and `:268-279`. `signupURL`
calls `strings.TrimRight(s.cfg.BaseURL, "/")` once per invocation,
and `loadInviteRows` invokes it once per row. The TrimRight is cheap
but pointless — the base URL is fixed at server construction.

Smallest fix: compute the trimmed base once in `Server.New` and
store it on `*Server` (e.g. `s.baseURL`). `signupURL` becomes
`s.baseURL + "/signup/" + token + "?email=" + url.QueryEscape(email)`.

While there: pass the already-built URL into `rowViewAt` instead of
calling `s.signupURL(inv)` three times in two callsites (`Resend`
calls it twice — once for the email body, once for the rendered row).

### M2. `handleAdminInvitesRevoke` does an extra `GetByToken` after a successful `Revoke`

`internal/server/admin_invites.go:230-244`. On the HTMX path we
`Revoke`, then `GetByToken` to re-read the row, then render. The
read is a wasted round-trip — `Revoke` already has the row state in
the conn (the `UPDATE`'s `now` is what we just wrote; everything
else is what was already there).

Two clean options:

1. Change `invites.Store.Revoke` to return the updated `Invite`
   (mirrors the `Resend` signature). Handler renders the returned
   row.
2. Read the row once *before* the update (same conn), apply the
   change locally, render. Slightly grosser; ignore.

Option 1 matches the existing `Resend` shape and removes one
sqlitex round-trip from the hot HTMX path.

### M3. Two near-identical `HX-Request` branches in `admin_invites.go` bypass the existing helper

`internal/server/admin_invites.go:219` and `:236` both do
`c.Request().Header.Get("HX-Request") == "true"`. `render.go:247`
already exports `isHXFragmentRequest(c)`, which `errors.go` uses
and which **also** rejects `hx-boost` full-page navigations as
false. The two raw checks in `admin_invites.go` are subtly
different from the helper: a boosted page navigation would slip
through and get a row partial instead of a full page.

Fix: replace both raw header checks with `isHXFragmentRequest(c)`.
Add the same lint to the Sprint 6 admin-games handlers.

### M4. `boolToInt` / `parseTime` are duplicated in two packages; about to be three

`internal/users/users.go:329-339` and `internal/invites/invites.go:367-377`
have byte-identical copies. Sprint 6 will add a third copy in
`internal/games`. There's no shared `internal/sqlx` (or similar)
package yet; introducing one *now* is justified by three call-sites,
not two.

Smallest correct change: a new `internal/dbx` (or fold into
`internal/db`) with `BoolToInt`, `ParseTime`, and a `NowISO()`
that returns `time.Now().UTC().Format(time.RFC3339Nano)` — the
last is sprinkled in 6+ places across the two stores.

Don't extract `classifyInsertErr` / `rowExists` yet — see L3.

### M5. Status taxonomies are sprinkled string literals (users) vs. typed constants (invites)

`internal/invites/invites.go:38-43` exposes `StatusPending`,
`StatusAccepted`, `StatusExpired`, `StatusRevoked` as package
constants. The users domain (and the server admin handlers)
sprinkle raw `"Active"` / `"Suspended"` literals through
`admin_users.go:138-141` and the per-row + detail views. Templates
pin the same strings in tests.

Sprint 6 will land a third taxonomy
(`Setup|Active|Paused|Archived`) for games. Repeat the invites
pattern there from the start — `games.StatusSetup` etc. — and
backfill `users.StatusActive`/`StatusSuspended` so the
admin-users handler stops carrying display strings inline. Cheap,
prevents typo-drift between Go and template tests.

### M6. `pages/admin_user_view.html` mixes "Admin" attribute label and the new "Account status" block

`web/templates/pages/admin_user_view.html:14-15` still renders
`<dt>Admin</dt><dd>{{ if .User.IsAdmin }}yes{{ else }}no{{ end }}</dd>`
in the first article, while the second article carries the new
"Account status" `<dl>` with Status + Suspended. This is fine but:

- The first article exposes role only as yes/no, while the list
  page uses the friendlier `"Admin"`/`"User"` (Sprint 5 T4.1).
- The detail page has no role badge.

Sprint 5 didn't promise either of these, but Sprint 6 admin-games
pages will want a role badge for GMs/players. Worth aligning the
two pages on a single "render a yes/no role" convention before
adding a third.

---

## Low — note and revisit if it bites

### L1. `account.go` carries a now-cosmetic dead branch

`internal/server/account.go:25-30`. The `ErrNotFound` branch was
the only way for a signed-in user to see a deleted-row state. Sprint
5 T1.2 removed `users.Store.Delete`, so the only paths that reach
`ErrNotFound` are (a) manual SQL delete, (b) future code we don't
have. The branch is reasonable defensive code — the *removal of the
test that covered it* (T4.2 dropped `TestAccountDeletedUserClearsCookie`)
is the smell.

Either restore a smaller test that drives the branch with a
hand-crafted DB delete, or document why the branch survives the
test's removal. Not urgent.

### L2. `handleAdminUsersEditSubmit` silently drops unknown form fields

`internal/server/admin_users.go:208-236`. The handler reads only
`is_admin`. Sprint 5 T4.3 explicitly tests that a submitted
`password` field is ignored. Good. But there's no positive proof
*at the handler level* that nothing else slips through — Sprint 6
will inevitably add a "transfer ownership" or similar form, and
the per-field tolerance pattern (`c.FormValue(...)` + ignore
everything else) gets harder to audit.

Not a Sprint 5 bug. Worth adopting a small "expected-fields"
convention for new admin forms — even just a comment that
enumerates the fields the handler reads. Sprint 6 admin-games-edit
(T3.4) is a good place to set the precedent.

### L3. Don't extract the `classifyInsertErr` / `rowExists` machinery yet

Tempting after seeing two near-identical copies (`users.go:354-381`,
`invites.go:392-419`). But the two differ on *which* unique
constraints they classify (handle vs. email vs. partial-index on
email), and the wrapped error prefixes differ. A premature shared
helper either becomes parameter-soup or hides the package-specific
classification rule.

Sprint 6 will add a third call-site (`games.classifyInsertErr` for
slug uniqueness). At three, extract — and only the
`rowExists`-shaped half. Leave the package-specific classification
inline so the wrapping prefix stays grep-able.

### L4. `Invite.Status` is computed in Go, not in SQL

`internal/invites/invites.go:70-81`. Fine for a 12-row admin list.
If Sprint 7 grows an "expired invites" cleanup job, the Go-side
status derivation will be wrong for any filter logic that wants
"all expired but not consumed" without loading every row. Note for
Sprint 7+ — not now.

### L5. `pages/admin_invite_confirm.html` doesn't carry its own breadcrumb

`web/templates/pages/admin_invite_confirm.html` reuses
`invitesShell`, whose last crumb is `Invitations`. The interstitial
page has no breadcrumb of its own ("Confirm admin invitation"),
which is technically inconsistent with the Sprint-4 breadcrumb
contract ("the current page is always the last crumb"). It's a
transient form, so debatable. If a future page lands at
`/admin/invites/confirm-admin` directly, the breadcrumb will be
wrong.

### L6. `sqlite.ColumnType(...) != TypeNull` pattern in `invites.go` but not `users.go`

`internal/invites/invites.go:184-189`, `:349-354` checks for SQL
NULL before parsing a timestamp. `internal/users/users.go` doesn't
— it relies on `time.Parse(time.RFC3339Nano, "")` returning the
zero value (and silently ignoring the error). Both work today
(`parseTime` discards the error), but the invites approach is
defensively clearer and survives a future column that uses a
sentinel string rather than NULL.

Not a bug; pattern drift worth aligning. Cheapest fix is to lift
`parseTime` into `internal/dbx` (M4) and have it accept the
column type as well — or just settle on one convention and
document it.

### L7. `inviteRowView.Token` is `string`, not `invites.Token`

`internal/server/admin_invites.go:26-39`. Templates can't reach into
`invites.Token`'s `.String()` method through `html/template`'s dot
access without a custom funcmap, so the conversion at the view
boundary is correct. Just flagging that Sprint 6 admin-games rows
will face the same `Slug` question. Set the precedent now:
**view structs hold display-ready strings**, packages own the
domain types.

---

## Things Sprint 5 got right — keep these

- The partial unique index pattern
  (`WHERE consumed_at IS NULL AND revoked_at IS NULL`) is exactly
  the shape Sprint 6 needs for `game_users (… WHERE is_active = 1)`.
  Mirror the migration shape (drop+create without `IF EXISTS`,
  honest about schema drift).
- `classifyInsertErr` doing a same-conn `SELECT 1` instead of
  parsing the driver's error text — keep this pattern for
  `games.slug` uniqueness in Sprint 6 T2.1.
- `CreateOnConn` / `GetByTokenOnConn` / `Consume(conn)` for the
  signup transaction: the connection-scoped sibling pattern is
  the right shape for Sprint 6's "create game + add gamemasters
  in one transaction" (T3.2) and for "remove-member + bump
  last_activity_at" (T2.2).
- Suspension-checked-after-password (`server.go:259-266`) is the
  correct order to avoid leaking account-state via wrong-password
  probes. Sprint 6 doesn't add any login surface, but if Sprint 7+
  does, the same rule applies.

---

## Recommended sequencing for Sprint 6 preflight

Before T1.1 starts, land a single small "Sprint 5 cleanup" commit
that:

1. Extracts `internal/dbx` with `BoolToInt`, `ParseTime`, `NowISO`
   (M4) and migrates users + invites onto it.
2. Adds `users.StatusActive` / `StatusSuspended` constants and
   replaces the inline strings (M5).
3. Replaces the raw `HX-Request` header checks in
   `admin_invites.go` with `isHXFragmentRequest` (M3).
4. Caches `s.baseURL` on `*Server` (M1).

That's < 150 lines of diff, all reversible, and lets Sprint 6 copy
the right shape from day one.

H1 (Mailgun-in-transaction) and H2 (Revoke idempotency) landed on
2026-05-17 — see the per-finding Resolved notes. H3 is the
documented project policy (DESIGN.md §8.9) and is left as-is.
