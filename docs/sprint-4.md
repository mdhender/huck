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

Before starting T1, confirm the Sprint 3 front-end readiness work is
complete:

- CSRF tokens, hidden `_csrf` fields, and CSRF view fields are gone.
- The shared `hxRedirect` helper exists and covers HTMX vs. non-HTMX
  redirects.
- The `/admin` index redirect has been dropped or deliberately handled
  per Sprint 3 T11.
- `homeView` has been split into auth-shell and app-shell view structs
  per Sprint 3 T15.
- A baseline renderer smoke test exists for the current page-vs-partial
  dispatch before T2 changes the layout selection logic.
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
| T1   | TODO   |        |
| T2   | TODO   |        |
| T3   | TODO   |        |
| T4   | TODO   |        |
| T5   | TODO   |        |
| T6   | TODO   |        |
| T7   | TODO   |        |
| T8   | TODO   |        |
| T9   | TODO   |        |
| T10  | TODO   |        |

### T1 — Split `layout.html` into `layout_auth.html` and `layout_app.html`

The current `web/templates/layout.html` is a single
`<main class="container">` shared by every page. Per the plan
(§3), pre-auth and post-auth pages need different shells.

- Create `web/templates/layout_auth.html`: centered, narrow, no
  sidebar, no breadcrumbs. Keeps the current `<main class="container">`
  feel. Used by public home, login, signup, error.
- Create `web/templates/layout_app.html`: the three-region grid
  (sidebar | topbar / breadcrumbs / content). Used by every
  post-login page.
- Both layouts keep the existing `{{ block "title" }}`,
  `{{ block "content" }}`, `{{ block "scripts" }}` contract so
  pages don't have to learn a new vocabulary.
- Remove the original `layout.html` once nothing references it.
- Drop the hard-coded `data-theme="light"` from `<html>` (plan
  §7). Follow `prefers-color-scheme` instead.

### T2 — Teach the renderer which layout each page uses

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

### T4 — Implement the breadcrumbs partial and Go contract

Per the plan (§5).

- Add `internal/server/breadcrumbs.go` defining
  `type Crumb struct { Label, URL string }`.
- Add `web/templates/partials/breadcrumbs.html` that renders a
  `[]Crumb` as a `<nav aria-label="Breadcrumb">` containing an
  `<ol>` with separators. Last crumb (URL == "") renders as
  `<span aria-current="page">`, not as a link.
- The app shell calls the partial unconditionally; if the slice
  is empty, the partial renders nothing (no empty `<nav>`).
- Add a unit test for the partial: empty slice → empty output;
  three crumbs with the last lacking a URL → expected HTML
  shape (link, link, current-page span).

### T5 — Define the app-shell sidebar (today's nav only)

Per the plan (§2), the sidebar reflects facts that are true
today. No game-scoped links yet (no game model).

- Always-visible items: **Home** (`/`), **Account**
  (`/account`, but only if a real page or intentional stub exists;
  otherwise omit the link rather than adding a dead one).
- If `is_admin`: an **Admin** section with **Invites**
  (`/admin/invites`) and **Users** (`/admin/users`).
- The sidebar lives in `web/templates/partials/sidebar.html`
  and is included by `layout_app.html`. It receives the
  current-user view data (or a small dedicated `SidebarView`
  struct) so it can:
  - hide admin items for non-admins,
  - mark the current section as `aria-current="page"` so CSS
    can highlight it.
- Decide and document: does the sidebar partial receive its
  own typed view, or does it inspect a field on the page
  view? Pick the typed view; it keeps each page's view struct
  honest and avoids a magic field every page must remember to
  populate. The renderer composes
  `{ Page: <page view>, Shell: <shell view> }` once.

### T6 — Define the app-shell topbar

The topbar is the strip across the top of the main column.

- Left: the current page's title (mirrors `{{ block "title" }}`).
- Right: the signed-in handle and a logout form.
- Lives in `web/templates/partials/topbar.html`, included by
  `layout_app.html`.
- The existing `form.inline` rule in `app.css` either moves to
  `.huck-topbar form` (more specific, semantic) or stays as a
  general utility — pick the former.

### T7 — Retrofit existing pages onto the new shells

Touch every page in `web/templates/pages/` and confirm:

- **Auth shell** (`layout_auth.html`):
  - `home_public.html`
  - `login.html`
  - `signup.html`
  - `error.html`
- **App shell** (`layout_app.html`):
  - `home_authed.html`
  - `admin_invites.html`
  - `admin_users.html`
  - `admin_user_view.html`
  - `admin_user_edit.html`

For each app-shell page:

- Add the breadcrumbs in the handler that renders it. Examples:
  - `/admin/invites` → `[Home, Admin, Invites]`
  - `/admin/users` → `[Home, Admin, Users]`
  - `/admin/users/:id` → `[Home, Admin, Users, <handle>]`
  - `/admin/users/:id/edit` → `[Home, Admin, Users, <handle>, Edit]`
  - `/` (authed) → `[Home]` (single crumb is fine; the partial
    will render it as the current page)
- Wrap the H1 and any header-level actions in a
  `.huck-page-header`.
- Wrap forms in `.huck-form-stack` where it improves rhythm.
- Confirm no page assumes the old `<main class="container">`
  wrapper is still present.

### T8 — Update the renderer's HTMX path to target `.huck-content`

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

### T9 — Document the deferred items in the right places

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

### T10 — Polish notes (small, batch into one PR)

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
- A full Account page (only the *link* is in scope; the page
  itself can land in Sprint 5).
- Any change to `internal/auth`, `internal/users`,
  `internal/invites`, or the schema. Sprint 4 is a templates +
  CSS sprint with a tiny renderer change.
- Component-level visual identity (colors, type scale tuning).
  Phase 4 of Miko's roadmap.

---

## Verification before closing the sprint

Per AGENTS.md "Verification before saying 'done'":

- `go build ./...` succeeds.
- `go test ./...` passes (including the new T2 renderer test
  and the T4 breadcrumbs test).
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
