# Huck Front-End Plan

Status: **Draft 2026-05-10.**

This document is the working plan for evolving Huck's front end past
"pages glued to Pico defaults." It records the decisions we have made,
the vocabulary we have agreed on, and — just as importantly — the
things we have explicitly chosen *not* to do yet.

It complements two existing documents:

- [`docs/front-end-design.md`](front-end-design.md) — Miko's vision
  and emotional goals for the interface. Stable. Aspirational.
- [`docs/pico-css.md`](pico-css.md) — Miko's framing of the four-layer
  front-end model and how Huck should grow through it.

This plan is the bridge between those two documents and the actual
templates and CSS in `web/`. When something in this plan stabilises,
it should graduate into `front-end-design.md` (see the closing note).

---

## 1. The four-layer model, applied to Huck

Per `docs/pico-css.md`, every long-lived front end develops four
layers. Today Huck only has Layer 1.

| Layer | Name | Status in Huck | Owned by |
| ----- | ---- | -------------- | -------- |
| 1 | Foundation (typography, forms, buttons, tables) | **Done.** | Pico.css |
| 2 | Layout system (shell, sidebar, content widths, breadcrumbs) | **This sprint.** | `web/static/app.css` + `web/templates/layout_*.html` |
| 3 | Component system (status cards, wizards, validation panels, GM tables) | **Deferred.** Driven by real workflows. | TBD |
| 4 | Application identity (density, color, rhythm, polish) | **Deferred.** Emerges last. | TBD |

The point of naming the layers is to keep the next several months of
work honest. We are doing Layer 2 now. We are not doing Layer 3 or 4
disguised as Layer 2.

---

## 2. Roles and what they mean for navigation

Huck has **two scopes** of role.

### Application roles

These are stored on the user record and govern what someone can do at
the platform level.

- `admin` — manages Huck itself: invites, users, server settings.
- `user` — the default. Can be invited into games as a player or
  gamemaster.

### Game roles

These are scoped to a single game instance. A user becomes one of
these by being assigned to a game, not by an account flag.

- `gamemaster` — owns a game instance. Configures it, processes
  turns, manages the players within it.
- `player` — participates in a game instance. Submits orders,
  downloads artifacts.

A worked example: Penny is an `admin`. Jane and Joe are `user`s.
Penny creates a new game and assigns Joe as its `gamemaster`. Joe
adds Jane to the game as a `player`. Joe is still a `user` at the
platform level; his `gamemaster` role only exists inside that one
game.

### Implications for the sidebar

The sidebar is built from facts that are true *now*, not from
placeholders. Per "no mystery," dead links are forbidden.

- **Always (any authed user):** Home, Account.
- **If `is_admin`:** an Admin section (Dashboard, Invites, Users, …).
- **If the user is currently inside a game** (i.e. the URL is scoped
  to a specific game): a game-scoped section appears, populated
  according to the user's role *in that game*. Player-scoped links
  for a player; gamemaster-scoped links for a GM.

We are **not** stubbing player or gamemaster sidebars before the game
model exists. They will arrive when the game model arrives.

---

## 3. Two shells

Pre-auth pages and post-auth pages have different needs and should
not share a layout.

### Auth shell — `layout_auth.html`

Centered, narrow, no sidebar, no breadcrumbs. Used for:

- public home (`home_public.html`),
- login (`login.html`),
- signup (`signup.html`),
- error pages (`error.html`).

These pages have one job each. A sidebar would imply "you're inside
the app," which is wrong.

### App shell — `layout_app.html`

Three regions on desktop:

```diagram
╭─────────────┬───────────────────────────────────────────╮
│             │ topbar (page title, user menu, logout)    │
│  sidebar    ├───────────────────────────────────────────┤
│             │ breadcrumbs                               │
│  (always    ├───────────────────────────────────────────┤
│   visible)  │                                           │
│             │ content (page header + body)              │
│             │                                           │
╰─────────────┴───────────────────────────────────────────╯
```

Used for everything post-login: `home_authed.html`, `account.html`,
all `admin_*` pages, future game pages, future account-edit pages.

For now, `/account` reuses the same read-only detail shape as the admin
`/admin/users/:id` page, scoped to the signed-in user. It can grow into
account editing later, but the navigation destination is real in Sprint 3.

### How handlers choose

The renderer dispatches based on the page template's declared layout,
not on a request flag. A handler does not know or care which shell
its page lives in.

---

## 4. Phase-2 layout primitives (the entire vocabulary)

These are the only named CSS classes we commit to in this sprint. No
utilities, no variants, no modifiers. If something needs a variant,
we add it in Sprint 5+ when a real second use case exists.

| Class | Purpose |
| ----- | ------- |
| `.huck-shell` | Top-level CSS grid for the app shell: sidebar column + main column. |
| `.huck-sidebar` | Persistent left navigation. Labeled links, current-section highlight. |
| `.huck-topbar` | Strip across the main column: page title on the left, user menu / logout on the right. |
| `.huck-breadcrumbs` | `<nav aria-label="Breadcrumb">` rendered as a horizontal list with separators. |
| `.huck-content` | Width-controlled main region. Holds page header + body. HTMX swaps target this region. |
| `.huck-page-header` | The H1 + optional action-button row that opens a content region. |
| `.huck-form-stack` | Vertical form rhythm for player-facing forms (account edits, login, etc). |

Tables stay on Pico's `<table>` for now. A `.huck-table` (and its GM
density variant) will land in Phase 3 when a real GM screen demands
it.

---

## 5. Breadcrumb data contract

Breadcrumbs are explicit, handler-supplied data. No path parsing.

### Go side

```go
// internal/server/breadcrumbs.go
type Crumb struct {
    Label string // human-readable, already-escaped plain text
    URL   string // empty for the current page (last crumb)
}
```

Every app-shell render provides a typed shell view containing
`Crumbs []Crumb`. Handlers build the breadcrumbs explicitly; the
renderer may compose the page view and shell view once so page
templates keep receiving their own page view as dot:

```go
crumbs := []server.Crumb{
    {Label: "Home", URL: "/"},
    {Label: "Admin", URL: "/admin"},
    {Label: "Invites", URL: ""}, // current page, no link
}
```

### Template side

A single partial, `web/templates/partials/breadcrumbs.html`, renders
the slice. The app shell calls it; pages do not.

The last crumb (URL == "") renders as `<span aria-current="page">`,
not as a link.

### Why explicit

- Searchable: `git grep "Label: \"Invites\""` finds the page.
- Cheap: three lines per handler.
- No coupling between URL structure and human labels.
- A handler that forgets crumbs renders no crumb bar at all, which
  is loud during review.

---

## 6. HTMX rule: swaps live inside `.huck-content`

The app shell renders once per full-page load. HTMX requests target
fragments inside `.huck-content` and never replace the sidebar,
topbar, or breadcrumb regions.

Concretely:

- Partials in `web/templates/partials/` render content-region HTML.
- Page templates render through one of the two layouts.
- The renderer's existing page-vs-partial decision (based on template
  name and `HX-Request`) already enforces this; we are just naming
  the rule out loud.

This keeps the sidebar from flickering on every interaction and
matches the "stable navigation" principle in
`front-end-design.md`.

---

## 7. Color scheme

Drop the hard-coded `data-theme="light"` on `<html>`. Let Pico follow
the browser's `prefers-color-scheme`.

Rationale: Miko's doc forbids *forcing* dark mode but is not a
mandate to force light mode either. Following the OS preference is
the most "calm and unsurprising" default.

A user-facing light/dark toggle is **deferred**. See §8.

---

## 8. Not yet, on purpose

Each of the following is real work we have decided to defer. They
are listed here so reviewers don't try to drag them into Sprint 4 by
accident.

- **Design tokens / CSS custom properties for spacing, color,
  density.** We don't have enough repeating components yet to know
  which values deserve a name. Pico's defaults are fine until a real
  pattern forces the issue.
- **Density variants** (player-spacious vs. GM-dense). Per Miko,
  this is Phase 4. We need actual GM tables in front of users
  before tuning.
- **Status cards.** Phase 3. Will be designed against the real game
  status data shape when it exists.
- **Upload wizard / multi-step workflow component.** Phase 3. Will
  be designed against the real orders-upload flow.
- **Validation panels / error summary component.** Phase 3. Will be
  designed when we have a real validator producing real errors.
- **GM table standards.** Phase 3. Wait for the first real GM CRUD
  screen.
- **Light/dark mode toggle.** Phase 4. Needs a discoverable place in
  the UI (not a hidden gear icon — see "no mystery"), and a
  persistence mechanism (cookie? user setting in DB?). Document at
  the time, don't pre-design.
- **Mobile sidebar collapse / hamburger.** Phase 4 at the earliest.
  Desktop-first per Miko. Mobile gets a stacked layout with the
  sidebar moved to the top of the page (or omitted on the most
  read-only screens) when we get there.

---

## 9. Sprint 4 work plan

The ordered tasks for actually building this — file-by-file — live
in [`docs/sprint-4.md`](sprint-4.md). This document defines *what*
we are building and why. Sprint 4 defines the *how* and *in what
order*.

---

## A note on graduating this content

`front-end-design.md` is Miko's vision document and should stay
clean. This plan deliberately holds the messier, more concrete
decisions (role taxonomy, primitive names, breadcrumb contract,
deferral list).

When a section here has survived a sprint or two without being
revised, it has earned the right to migrate into
`front-end-design.md` as established design — not as a working
plan. Likely first candidates, after Sprint 4 ships:

- §2 Roles and what they mean for navigation.
- §3 Two shells.
- §6 HTMX rule.

Until then, treat `front-end-design.md` as the why, this document as
the what, and sprint docs as the how.
