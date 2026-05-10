# Sprint 4 — Implementation Plan

Status: **Draft 2026-05-10.**

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

## Entry checklist

Before starting T1.1, confirm the Sprint 3 front-end readiness work is
complete:

- CSRF tokens, hidden `_csrf` fields, and CSRF view fields are gone.
- The shared `hxRedirect` helper exists and covers HTMX vs. non-HTMX
  redirects.
- The `/admin` index redirect has been dropped or deliberately handled
  per Sprint 3 T11.
- `homeView` has been split into auth-shell and app-shell view structs
  per Sprint 3 T15.
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

| Task | Status | Commit |
|------|--------|--------|
| T1.1 | TODO   |        |
| T1.2 | TODO   |        |
| T1.3 | TODO   |        |
| T1.4 | TODO   |        |
| T2.1 | TODO   |        |
| T2.2 | TODO   |        |
| T3   | TODO   |        |
| T4.1 | TODO   |        |
| T4.2 | TODO   |        |
| T4.3 | TODO   |        |
| T4.4 | TODO   |        |
| T5   | TODO   |        |
| T6   | TODO   |        |
| T7   | TODO   |        |

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
  only if a real page or intentional stub exists; otherwise omit the
  link rather than adding a dead one).
- If `is_admin`: an **Admin** section with **Invites**
  (`/admin/invites`) and **Users** (`/admin/users`).
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
  existing page templates do not need to use `.Page.CSRF`, `.Page.Rows`,
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

### T4.3 — Retrofit the admin invites page

Touch the invites admin surface only:

- `admin_invites.html`
- `handleAdminInvitesList`, invite create error/success re-renders, and
  related tests

Add shell data with `[Home, Admin, Invites]` breadcrumbs, correct current
sidebar state, `.huck-page-header`, and `.huck-form-stack` where it
improves form rhythm. Confirm `partials/invite_row.html` remains suitable
for injection inside `.huck-content`.

### T4.4 — Retrofit the admin users pages

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
- A full Account page. The sidebar may link to `/account` only if a
  real page or intentional stub exists; otherwise omit the link until
  Sprint 5.
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
  - Authed home, `/admin/invites`, `/admin/users`,
    `/admin/users/:id`, `/admin/users/:id/edit` all
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
