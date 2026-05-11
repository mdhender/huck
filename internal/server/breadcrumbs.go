package server

// App-shell view contracts.
//
// These types are the typed seam between app-page handlers and the
// layout_app.html shell (added in Sprint 4 T2.1) and its sidebar / topbar
// / breadcrumbs partials (T1.2–T1.4). Sprint 4 T1.1 lands the contracts
// only; handler retrofits are T4.1–T4.5 and renderer wrapping is T2.2.
//
// How an app-page handler provides shell data:
//
//  1. Build the page view as today (e.g. adminIndexView, homeAuthedView).
//  2. Build a [ShellView] with sidebar state, topbar state, and the
//     breadcrumb trail — explicitly, not by parsing URLs.
//  3. Hand both to the renderer. The renderer (T2.2) wraps them into an
//     [AppPage] whose dot is passed to layout_app.html. The layout pulls
//     .Shell.* for the shell partials and invokes the page's content
//     and scripts blocks with .Page, so existing page templates keep
//     receiving their original page view as dot.
//
// We prefer typed structs (not map[string]any) so callers fail to
// compile when a field renames or moves, and so grep finds usages.

// Crumb is one entry in an app-shell breadcrumb trail. Handlers build
// the trail explicitly; the renderer never parses URLs to infer it.
type Crumb struct {
	Label string // human-readable, already-escaped plain text
	URL   string // empty for the current page (last crumb)
}

// Current reports whether this crumb represents the current page. The
// last crumb in a trail has URL == "" and renders as
// <span aria-current="page"> rather than as a link.
func (c Crumb) Current() bool { return c.URL == "" }

// SidebarView is the data shape consumed by partials/sidebar.html
// (Sprint 4 T1.3). Section identifies the currently active nav entry so
// the partial can apply aria-current="page" to exactly one link; valid
// values are the Section* constants below.
type SidebarView struct {
	Handle  string
	IsAdmin bool
	Section string
}

// TopbarView is the data shape consumed by partials/topbar.html
// (Sprint 4 T1.4). Title is the page title rendered on the left of the
// topbar; it usually matches the page's {{ block "title" }} output.
// Handle is the signed-in user's handle shown on the right next to the
// logout form.
type TopbarView struct {
	Handle string
	Title  string
}

// ShellView is the app-shell context that surrounds a page render.
// Built by app-page handlers and consumed by the layout's shell
// partials. App pages always render through this; auth-shell pages
// (login, signup, public home, error) do not use it.
type ShellView struct {
	Sidebar SidebarView
	Topbar  TopbarView
	Crumbs  []Crumb
}

// AppPage is the dot value passed to layout_app.html. The layout pulls
// .Shell.Sidebar, .Shell.Topbar, and .Shell.Crumbs for the shell
// partials and invokes the page's content/scripts blocks with .Page so
// existing page templates keep receiving their original page view as
// dot. The renderer (T2.2) constructs this wrapper; handlers do not.
type AppPage struct {
	Page  any
	Shell ShellView
}

// Sidebar section identifiers. Handlers set ShellView.Sidebar.Section
// to one of these so the sidebar partial can apply aria-current="page"
// to exactly one entry. New sections must be added here so the set of
// valid values stays grep-able.
const (
	SectionHome           = "home"
	SectionAccount        = "account"
	SectionAdminDashboard = "admin-dashboard"
	SectionAdminInvites   = "admin-invites"
	SectionAdminUsers     = "admin-users"
)
