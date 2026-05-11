# Sprint 4 — Implementation Plan

Status: **Ready 2026-05-10.**

Sprint 4 builds the Layer-2 (layout) front end described in
[`docs/front-end-plan.md`](front-end-plan.md): a persistent app
shell with sidebar, topbar, breadcrumbs, and a width-controlled
content region; a separate auth shell for pre-login pages; and
a small, named set of CSS primitives that future sprints will
build components on top of.

The design-level decisions (role taxonomy, the two-shell split,
the primitive vocabulary, the breadcrumb data contract, the
"not yet" list) live in `docs/front-end-plan.md`. **Read it
first.** This file is the sprint plan — when a task changes a
contract, the plan doc is the document to update first.

No new functional features are added in Sprint 4. Existing pages
are retrofitted onto the new shells; behaviour stays identical.

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

Before starting T1.1, confirm the Sprint 3 front-end readiness work is
complete:

- CSRF tokens, hidden `_csrf` fields, and CSRF view fields are gone.
- The shared `hxRedirect` helper exists and covers HTMX vs. non-HTMX
  redirects.
- `/admin` is a real admin dashboard page and `/account` is a real
  signed-in account page per Sprint 3 T11.
- `homeView` has been split into auth-shell and app-shell view structs
  per Sprint 3 T14.
- A baseline renderer smoke test exists for the current page-vs-partial
  dispatch before T2.2 changes the layout selection logic.
- Admin user detail routes are confirmed as `/admin/users/:id` and
  `/admin/users/:id/edit`; breadcrumb labels may use handles, but the
  URL parameter remains numeric.
- Renderer tests are green before changing the layout dispatch path.
- Existing page templates have been reviewed for duplicated
  nav/header assumptions that the new app shell will own.

---

## In scope

| Task | Status | Commit | Notes |
|------|--------|--------|-------|
| T1.1 | DONE   |        | Added `Crumb`, `SidebarView`, `TopbarView`, `ShellView`, `AppPage` in `internal/server/breadcrumbs.go` plus `Section*` constants and a small contract test. Renderer wrapping and handler retrofits deferred to T2.2 / T4.x per task scope. |
| T1.2 | DONE   |        | Added `web/templates/partials/breadcrumbs.html` rendering `[]Crumb` as `<nav aria-label="Breadcrumb">` + `<ol>` with `/` separators; empty slice → no output; final crumb (URL=="") → `<span aria-current="page">`. Tests in `internal/server/breadcrumbs_test.go` cover both shapes. |
| T1.3 | DONE   |        | Added `web/templates/partials/sidebar.html` rendering the typed `SidebarView`: always-visible Home/Account, admin-only Dashboard/Invites/Users under an `<h2>Admin</h2>`, and `aria-current="page"` on the entry whose link matches `Section`. Tests in `internal/server/sidebar_test.go` cover non-admin (admin section absent), admin (admin section present), and one active-section case per `Section*` constant. |
| T1.4 | DONE   |        | Added `web/templates/partials/topbar.html` rendering the typed `TopbarView` as a `<header class="huck-topbar">` with the page title on the left and the signed-in handle + plain POST `/logout` form on the right. Form carries no `class="inline"` — `.huck-topbar form` styling lands in T3. Test in `internal/server/topbar_test.go` pins the shape and the title-before-handle source order. |
| T2.1 | DONE   |        | Added `web/templates/layout_auth.html` (centered `<main class="container">`, no shell) and `web/templates/layout_app.html` (`.huck-shell` grid wrapping sidebar/topbar/breadcrumbs partials and a `.huck-content` block). App layout uses `{{ block "content" .Page }}` / `{{ block "scripts" .Page }}` so page templates keep receiving the original page view as dot while shell partials read `.Shell.*`. Dropped `data-theme="light"` on both layouts. `layout.html` left in place until T2.2 switches the renderer. |
| T2.2 | DONE   |        | Renderer now picks `layout_auth.html` vs `layout_app.html` per-page via an explicit `pageLayouts` map (option (b)); pages absent from the map fail at `NewRenderer`. App-shell renders wrap data once into `AppPage{Page, Shell}` (handler-supplied wrappers pass through unchanged), so `layout_app.html` reads `.Shell.*` for partials while page `content`/`scripts` blocks keep receiving the original page view as dot. HX-fragment renders unwrap any `AppPage` so partials look the same regardless of retrofit state. Removed `web/templates/layout.html`. New test in `internal/server/render_test.go` (`TestRendererPicksLayoutPerPage`) asserts each shell on a representative page and exercises the `AppPage` wrapper path. |
| T3   | DONE   |        | Implemented the seven Phase-2 primitives in `web/static/app.css`: `.huck-shell` is a two-column CSS grid (sidebar | main) collapsing to single-column at 768px — breakpoint documented inline per T7; `.huck-sidebar`, `.huck-topbar`, `.huck-breadcrumbs` carry minimal column/strip chrome layered on Pico; `.huck-content` controls width via `--huck-content-max: 80ch` (pages opt out by overriding the variable); `.huck-page-header` and `.huck-form-stack` are flex containers for header/form rhythm. Removed the `form.inline` rule in favour of `.huck-topbar form` per the plan. Legacy `class="inline"` markup still present on pre-shell page headers in `home_authed.html`/`account.html`/`admin*.html` becomes inert until T4.x retrofits those pages. |
| T4.1 | DONE   |        | Audited the four auth-shell page templates (`home_public.html`, `login.html`, `signup.html`, `error.html`): each defines only `title`/`content` blocks and carries no nav/header/wrapper assumptions, so they render correctly through `layout_auth.html` without template edits. Added `TestAuthShellPagesUseAuthLayout` in `internal/server/render_test.go` to pin all four pages: each render must contain `<!doctype html>` + `<main class="container">` (auth-shell markers) and must not contain any `huck-shell`/`huck-sidebar`/`huck-topbar`/`huck-breadcrumbs` markers, plus a page-specific content fragment per page so a stripped content block fails the test. Iterating per-page rather than a single representative prevents a future `pageLayouts` rewire from silently moving one page into the app shell. |
| T4.2 | DONE   |        | Retrofitted `pages/home_authed.html` onto the app shell: dropped the in-page `<header><nav>` (sidebar/topbar own that chrome now) and wrapped the H1 in `.huck-page-header`. `handleHome` now hands the renderer an `AppPage{Page: homeAuthedView, Shell: ShellView}` with `SectionHome`, a `Welcome` topbar title, and `Crumbs: [{Label: "Home"}]`. Dropped the now-unused `Admin` field from `homeAuthedView` (sidebar reads `claims.Admin` via the shell). Also trimmed the stale "Sprint 2 will add invites and an admin console" copy — that work has shipped. New `TestAuthedHomeRendersAppShell` in `render_test.go` pins the shell wrappers, `.huck-page-header`, sidebar/breadcrumb current-page marks, the topbar title+handle, and the absence of the legacy `<li><strong>huck</strong></li>` / `class="inline"` markers. |
| T4.3 | DONE   |        | Retrofitted `pages/account.html` onto the app shell: dropped the in-page `<header><nav>` (sidebar/topbar own that chrome now) and wrapped the H1 in `.huck-page-header`. `handleAccount` now hands the renderer an `AppPage{Page: newAdminUserView(claims, u), Shell: ShellView}` with `SectionAccount`, an `Account` topbar title, and `Crumbs: [{Label: "Home", URL: "/"}, {Label: "Account"}]`. Page view shape (`adminUserView`) is unchanged so the existing `account_test.go` assertions on handle/email/admin-link still apply; the admin link in the body's `<dl>` row went away with the header strip, but the admin sidebar entry still surfaces `href="/admin"` for admin users (and stays hidden for non-admins). New `TestAccountRendersAppShell` in `render_test.go` pins the shell wrappers, `.huck-page-header`, sidebar/breadcrumb current-page marks, the topbar title+handle, and the absence of the legacy `<li><strong>huck</strong></li>` / `class="inline"` markers. |
| T4.4 | DONE   |        | Retrofitted `pages/admin.html` and `pages/admin_invites.html` onto the app shell: dropped the in-page `<header><nav>` (sidebar/topbar own that chrome now) and wrapped each H1 in `.huck-page-header`; gave the invite create form `.huck-form-stack` for vertical rhythm. `handleAdminIndex` now hands the renderer an `AppPage{Page: adminIndexView{}, Shell: …}` with `SectionAdminDashboard`, an `Admin` topbar title, and `Crumbs: [{Label:"Home", URL:"/"}, {Label:"Admin"}]`. All three /admin/invites render sites (list, create success, create error) route through a new `invitesShell` helper so they stay in lockstep on `SectionAdminInvites`, an `Invites` topbar title, and `Crumbs: [Home, Admin, Invites]`. Dropped the now-unused `Handle` field from both `adminIndexView` and `adminInvitesView` (sidebar/topbar read `claims.Handle` via the shell); `adminIndexView` becomes an empty struct, kept as a named type so the renderer's `pageLayouts` map and `AppPage.Page` slot retain a typed seam. `partials/invite_row.html` is unchanged and remains a bare `<tr>` suitable for HX swap inside the table inside `.huck-content`. New `TestAdminIndexRendersAppShell` and `TestAdminInvitesRendersAppShell` in `render_test.go` pin the shell wrappers, `.huck-page-header`, `.huck-form-stack` (invites only), sidebar/breadcrumb current-page marks, the topbar title+handle, and the absence of the legacy `<li><strong>huck</strong></li>` / `class="inline"` markers. Updated `breadcrumbs_test.go::TestAppPageWrapping` to wrap `homeAuthedView` (still has a Handle field) since `adminIndexView` no longer carries inspectable state. |
| T4.5 | TODO   |        |       |
| T5   | TODO   |        |       |
| T6   | TODO   |        |       |
| T7   | TODO   |        |       |

### T1.1 — Add app-shell view contracts

Create the typed data contract that the shell partials and app layout
will consume before any template starts depending on it.

- Add `internal/server/breadcrumbs.go` defining
  `type Crumb struct { Label, URL string }`.
- Add small typed shell/sidebar/topbar view structs in
  `internal/server` for:
  - current signed-in handle,
  - admin/non-admin state,
  - current path or current section for nav highlighting,
  - page title,
  - breadcrumbs.
- Decide and document in code how app-page handlers provide shell data.
  Prefer an explicit typed field or method over magic map keys. The
  renderer may wrap app pages as `{ Page: <page view>, Shell: <shell view> }`,
  but individual page templates should still receive their existing page
  view as dot.
- Keep this task to Go contracts and small tests only; do not retrofit
  every handler yet.

### T1.2 — Implement the breadcrumbs partial

Per the plan (§5).

- Add `web/templates/partials/breadcrumbs.html` that renders a
  `[]Crumb` as a `<nav aria-label="Breadcrumb">` containing an
  `<ol>` with separators.
- Last crumb (`URL == ""`) renders as
  `<span aria-current="page">`, not as a link.
- If the slice is empty, the partial renders nothing (no empty `<nav>`).
- Add a unit test for the partial: empty slice → empty output; three
  crumbs with the last lacking a URL → expected HTML shape (link, link,
  current-page span).

### T1.3 — Implement the sidebar partial

Per the plan (§2), the sidebar reflects facts that are true today. No
game-scoped links yet (no game model).

- Add `web/templates/partials/sidebar.html`.
- Always-visible items: **Home** (`/`), **Account** (`/account`, but
  Sprint 3 T11 makes this a real page before Sprint 4 starts).
- If `is_admin`: an **Admin** section with **Dashboard** (`/admin`),
  **Invites** (`/admin/invites`), and **Users** (`/admin/users`).
- The partial receives the typed sidebar view from T1.1 so it can hide
  admin items for non-admins and mark the current section with
  `aria-current="page"`.
- Add focused render tests for admin and non-admin sidebar states.

### T1.4 — Implement the topbar partial

The topbar is the strip across the top of the app shell's main column.

- Add `web/templates/partials/topbar.html`.
- Left: the current page title (matching the page's `{{ block "title" }}`).
- Right: the signed-in handle and a logout form.
- The partial receives the typed topbar view from T1.1.
- Move the existing `form.inline` styling decision into T3: prefer
  `.huck-topbar form` over a general utility rule.

### T2.1 — Split the layout templates

The current `web/templates/layout.html` is a single
`<main class="container">` shared by every page. Per the plan (§3),
pre-auth and post-auth pages need different shells.

- Create `web/templates/layout_auth.html`: centered, narrow, no
  sidebar, no breadcrumbs. Keeps the current `<main class="container">`
  feel. Used by public home, login, signup, error.
- Create `web/templates/layout_app.html`: the three-region grid
  (sidebar | topbar / breadcrumbs / content). It includes the T1.2,
  T1.3, and T1.4 partials.
- Both layouts keep the existing `{{ block "title" }}`,
  `{{ block "content" }}`, `{{ block "scripts" }}` contract. For app
  pages, the layout may call `content` with the wrapped `.Page` value so
  existing page templates do not need to use `.Page.Handle`, `.Page.Rows`,
  etc.
- Drop the hard-coded `data-theme="light"` from `<html>` (plan §7).
  Follow `prefers-color-scheme` instead.
- Keep `layout.html` in place until T2.2 switches the renderer.

### T2.2 — Teach the renderer which layout each page uses

The `Renderer` in `internal/server` currently picks page-vs-partial
based on template name and the `HX-Request` header. It now also
needs to pick *which* page layout to wrap a page in.

- Pick the simplest workable mechanism. Two candidates:
  - **(a) Naming convention**: pages whose name starts with
    `auth_` use `layout_auth.html`; everything else uses
    `layout_app.html`. Or:
  - **(b) Explicit registration**: a small `map[string]string`
    in the renderer mapping page name → layout name.
  - Pick **(b)**. Naming conventions invite renames and silent
    breakage; an explicit map is grep-able and fails loudly if a
    page is added without registration.
- The renderer is the only place that knows about layout names.
  Handlers and templates do not branch on layout.
- Ensure app-shell full-page renders wrap data once for the shell while
  preserving the existing page view as dot for `content` and `scripts`.
- Remove `web/templates/layout.html` once nothing references it.
- Add a unit test that renders one auth-shell page and one
  app-shell page and asserts the wrapping layout was used.

### T3 — Add the Phase-2 CSS primitives to `app.css`

Per the plan (§4), the entire Sprint-4 CSS vocabulary is:
`.huck-shell`, `.huck-sidebar`, `.huck-topbar`, `.huck-breadcrumbs`,
`.huck-content`, `.huck-page-header`, `.huck-form-stack`.

- Implement each as a semantic, low-specificity rule layered on
  top of Pico. No utility classes. No modifiers (no
  `.huck-sidebar--wide`, etc.).
- `.huck-shell` is a CSS grid: sidebar column + main column on
  desktop; sidebar stacks above main on narrow viewports
  (single-column collapse, no hamburger — see plan §8).
- `.huck-content` controls width: target ~80ch for prose-heavy
  pages; tables and dashboards may opt out by being placed
  directly in the grid cell. Use a CSS variable like
  `--huck-content-max` so a single page can override if it has
  to (but we expect almost none to).
- Resist the urge to introduce a token / variable system. Use
  Pico's existing variables where possible.
- Move the old `form.inline` rule to `.huck-topbar form` unless the
  implementation proves a broader semantic selector is needed.

### T4.1 — Retrofit auth-shell pages

Touch only the auth-shell pages and their immediate render tests:

- `home_public.html`
- `login.html`
- `signup.html`
- `error.html`

Confirm these pages render through `layout_auth.html`, remain centered
and narrow, and do not assume the old `layout.html` wrapper still exists.

### T4.2 — Retrofit the authed home page

Touch the signed-in home path only:

- `home_authed.html`
- `handleHome` and any home view structs/tests

Add shell data with `[Home]` breadcrumbs, correct sidebar/topbar state,
and a `.huck-page-header` around the H1/header area.

### T4.3 — Retrofit the account page

Touch the signed-in account surface only:

- `account.html`
- the `/account` handler from Sprint 3 T11 and related tests

For now, `/account` shows the same content as the admin
`/admin/users/:id` detail page, scoped to the current signed-in user.
Add shell data with `[Home, Account]` breadcrumbs, correct sidebar/topbar
state, and a `.huck-page-header` around the H1/header area.

### T4.4 — Retrofit the admin dashboard and invites pages

Touch the admin dashboard and invites surfaces only:

- `admin.html`
- `admin_invites.html`
- `handleAdminIndex`
- `handleAdminInvitesList`, invite create error/success re-renders, and
  related tests

Add shell data with `[Home, Admin]` breadcrumbs for `/admin` and
`[Home, Admin, Invites]` breadcrumbs for `/admin/invites`, correct
current sidebar state, `.huck-page-header`, and `.huck-form-stack`
where it improves form rhythm. Confirm `partials/invite_row.html`
remains suitable for injection inside `.huck-content`.

### T4.5 — Retrofit the admin users pages

Touch the users admin surface only:

- `admin_users.html`
- `admin_user_view.html`
- `admin_user_edit.html`
- `handleAdminUsersList`, `handleAdminUsersView`,
  `handleAdminUsersEditForm`, edit error re-renders, and related tests

Add shell data and breadcrumbs:

- `/admin/users` → `[Home, Admin, Users]`
- `/admin/users/:id` → `[Home, Admin, Users, <handle>]`
- `/admin/users/:id/edit` → `[Home, Admin, Users, <handle>, Edit]`

Wrap H1/header actions in `.huck-page-header`, use `.huck-form-stack`
where it helps forms, and keep URL parameters numeric even when labels
use handles.

### T5 — Document and verify the HTMX `.huck-content` rule

The plan (§6) says HTMX swaps live inside `.huck-content`. Today
the renderer already returns partial HTML for `HX-Request`
calls; the new constraint is just naming the target.

- Confirm the existing partials produce HTML suitable for
  injection into `.huck-content` (no top-level `<main>` or
  shell elements in any partial).
- If any future HTMX call would replace something *outside*
  `.huck-content` (e.g. updating the sidebar after a state
  change), it should trigger a full-page reload via
  `HX-Refresh: true` instead. Document this rule next to
  `hxRedirect` (or wherever the HTMX response helpers landed
  in Sprint 3).
- No behaviour change expected from this task; it is
  primarily a code-comment + reviewer-checkpoint task. If a
  divergent partial is found, fix it.

### T6 — Document the deferred items in the right places

So the next sprint reviewer doesn't accidentally re-litigate
them.

- `docs/front-end-plan.md` already has the §8 "Not yet, on
  purpose" list. Cross-link it from `AGENTS.md` under a new
  short section ("Front-end conventions") that points at the
  plan and lists the no-go's by name (no Tailwind, no utility
  classes, no design tokens yet, no mobile hamburger, no
  forced theme).
- Add a single sentence to `docs/DESIGN.md` (front-end /
  templates section) noting that the layout split and the
  primitive vocabulary are defined in
  `docs/front-end-plan.md` and not duplicated in DESIGN.md.

### T7 — Polish notes (small, batch into one PR)

- Confirm `web/static/app.css` has no rules that contradict the
  Phase-2 primitives (e.g. global `body` margin overrides that
  fight the grid).
- Verify `<title>` still renders correctly through both shells
  (regression risk during the layout split).
- Verify `/login` still works for users with no JS (HTMX-less
  fallback path).
- Quick visual pass at 1920px, 1366px, and 768px viewports.
  Sidebar should remain visible at 1366; collapse to stacked
  at 768. Document the breakpoint in `app.css` as a comment.

---

## Out of scope

- Status cards, upload wizards, validation panels, GM-density
  tables, design tokens, light/dark toggle, mobile hamburger.
  All deferred per `docs/front-end-plan.md` §8.
- Account editing. Sprint 3 adds `/account` as a read-only detail page;
  profile editing can land in a later sprint.
- Any change to `internal/auth`, `internal/users`,
  `internal/invites`, or the schema. Sprint 4 is a templates +
  CSS sprint with a tiny renderer change.
- Component-level visual identity (colors, type scale tuning).
  Phase 4 of Miko's roadmap.

---

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new T2.2 renderer test
  and the T1.2 breadcrumbs test).
- `go vet ./...` is clean.
- `huck db create --db /tmp/huck-sprint4.db` succeeds on a
  fresh path, and `huck db migrate --db /tmp/huck-sprint4.db`
  is a no-op the second time.
- Manual smoke-test of `huck serve`:
  - Public home, login, signup, error all render in the auth
    shell (centered, no sidebar).
  - Authed home, `/account`, `/admin`, `/admin/invites`,
    `/admin/users`, `/admin/users/:id`, `/admin/users/:id/edit` all
    render in the app shell with the correct sidebar items
    and breadcrumb trail.
  - Logging out from the topbar works and lands on the
    public home.
  - Browser dev tools confirm no HTMX swap replaces anything
    outside `.huck-content`.
  - `prefers-color-scheme: dark` produces a dark page; no
    forced theme.

---

## Change log

- **2026-05-10** — Drafted from `docs/front-end-plan.md` and
  the Sprint 4 planning thread.
- **2026-05-10** — Reordered for implementation dependencies and split
  broad tasks into agent-sized slices.
- **2026-05-10** — Updated for Sprint 3's real `/admin` dashboard and
  read-only `/account` page.
