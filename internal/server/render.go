package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/web"
)

// Renderer is huck's html/template renderer. It implements
// echo.Renderer and dispatches based on the template name and the
// HX-Request header.
//
//   - "partials/foo.html"      → render the partial alone (no layout).
//   - "pages/foo.html" + HX    → render only the page's "content" block.
//   - "pages/foo.html"         → render the layout registered for that
//     page (auth or app shell) with the page's blocks (title, content,
//     scripts).
//
// Layout selection is an explicit map (pageLayouts) rather than a naming
// convention so adding a page without registering it fails loudly at
// startup, not silently at request time.
//
// Email templates live alongside but are reached via [Renderer.RenderEmail]
// rather than echo.Renderer.Render — they never go through any layout and
// are produced as a string for the mailer.
type Renderer struct {
	pages    map[string]*template.Template // page name → cloned layout + page
	layouts  map[string]string             // page name → layout file name
	partials *template.Template            // partials/* parsed alone
	emails   *template.Template            // email/*   parsed alone, no layout
}

// Layout file names. Centralised so the registration map and the render
// dispatch agree on the spelling.
const (
	layoutAuth = "layout_auth.html"
	layoutApp  = "layout_app.html"
)

// pageLayouts maps each page template to the layout that wraps a full
// page render. Pages absent from this map fail at NewRenderer; this is
// intentional so a new page cannot ship without an explicit shell choice.
var pageLayouts = map[string]string{
	"pages/home_public.html":     layoutAuth,
	"pages/login.html":           layoutAuth,
	"pages/signup.html":          layoutAuth,
	"pages/error.html":           layoutAuth,
	"pages/home_authed.html":     layoutApp,
	"pages/account.html":         layoutApp,
	"pages/admin.html":           layoutApp,
	"pages/admin_invites.html":   layoutApp,
	"pages/admin_users.html":     layoutApp,
	"pages/admin_user_view.html": layoutApp,
	"pages/admin_user_edit.html": layoutApp,
}

// NewRenderer builds the renderer from the embedded template FS.
func NewRenderer() (*Renderer, error) {
	r := &Renderer{
		pages:   map[string]*template.Template{},
		layouts: map[string]string{},
	}

	// Parse each layout once into its own root template. Each page later
	// gets its own clone so per-page block redefinitions don't leak.
	layoutRoots := map[string]*template.Template{}
	for _, name := range []string{layoutAuth, layoutApp} {
		t, err := template.New(name).ParseFS(web.Templates, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("server: parse %s: %w", name, err)
		}
		layoutRoots[name] = t
	}

	// Parse all partials together; each defines its own template name.
	partialPaths, err := fs.Glob(web.Templates, "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("server: glob partials: %w", err)
	}
	r.partials = template.New("partials")
	if len(partialPaths) > 0 {
		if _, err := r.partials.ParseFS(web.Templates, partialPaths...); err != nil {
			return nil, fmt.Errorf("server: parse partials: %w", err)
		}
	}

	pages, err := fs.Glob(web.Templates, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("server: glob pages: %w", err)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("server: no pages found")
	}
	for _, p := range pages {
		// Key by "pages/<base>" so handlers can use the same name as
		// the file path under templates/.
		key := strings.TrimPrefix(p, "templates/")
		layoutName, ok := pageLayouts[key]
		if !ok {
			return nil, fmt.Errorf("server: page %q has no layout registered in pageLayouts", key)
		}
		root, ok := layoutRoots[layoutName]
		if !ok {
			return nil, fmt.Errorf("server: layout %q for page %q not parsed", layoutName, key)
		}
		clone, err := root.Clone()
		if err != nil {
			return nil, fmt.Errorf("server: clone %s: %w", layoutName, err)
		}
		// Make partials available to pages and to the layout via
		// {{ template "partials/foo.html" . }}.
		if len(partialPaths) > 0 {
			if _, err := clone.ParseFS(web.Templates, partialPaths...); err != nil {
				return nil, fmt.Errorf("server: parse partials into %s: %w", p, err)
			}
		}
		if _, err := clone.ParseFS(web.Templates, p); err != nil {
			return nil, fmt.Errorf("server: parse %s: %w", p, err)
		}
		r.pages[key] = clone
		r.layouts[key] = layoutName
	}

	// Email templates live in their own tree: no layout, executed as a
	// string by RenderEmail (the mailer takes a body, not a writer).
	emailPaths, err := fs.Glob(web.Templates, "templates/email/*.html")
	if err != nil {
		return nil, fmt.Errorf("server: glob emails: %w", err)
	}
	r.emails = template.New("emails")
	if len(emailPaths) > 0 {
		if _, err := r.emails.ParseFS(web.Templates, emailPaths...); err != nil {
			return nil, fmt.Errorf("server: parse emails: %w", err)
		}
	}

	return r, nil
}

// RenderEmail renders the named email template (e.g. "email/invite.html")
// to a string suitable for handing to a Mailer.
func (r *Renderer) RenderEmail(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := r.emails.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("server: render email %q: %w", name, err)
	}
	return buf.String(), nil
}

// Render satisfies echo.Renderer.
func (r *Renderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	switch {
	case strings.HasPrefix(name, "partials/"):
		// Partials are registered under their full "partials/foo.html"
		// name via {{ define "partials/foo.html" }}. Execute by the same
		// name so we render the body, not the empty file template.
		return r.partials.ExecuteTemplate(w, name, data)
	case strings.HasPrefix(name, "pages/"):
		page, ok := r.pages[name]
		if !ok {
			return fmt.Errorf("server: unknown page template %q", name)
		}
		layout := r.layouts[name]
		if isHXFragmentRequest(c) {
			// Page templates' content blocks always receive the raw page
			// view as dot. Unwrap any AppPage so HX swaps look the same
			// regardless of whether the handler has been retrofitted.
			return page.ExecuteTemplate(w, "content", unwrapAppPage(data))
		}
		if layout == layoutApp {
			data = wrapAppPage(data)
		}
		return page.ExecuteTemplate(w, layout, data)
	default:
		return fmt.Errorf("server: template name must start with pages/ or partials/, got %q", name)
	}
}

// wrapAppPage normalises the data passed to layout_app.html into an
// AppPage so the layout can reference .Page and .Shell uniformly.
// Handlers may pass an AppPage directly (the post-T4.x form); if they
// pass a raw page view (the pre-retrofit form), wrap it with an empty
// shell so the page still renders while shell data is filled in
// incrementally per Sprint 4 T4.1–T4.5.
func wrapAppPage(data any) AppPage {
	if ap, ok := data.(AppPage); ok {
		return ap
	}
	return AppPage{Page: data}
}

// unwrapAppPage returns the inner page view if data is an AppPage, or
// the value unchanged otherwise. Used on the HX-fragment path so page
// content blocks never see the wrapper.
func unwrapAppPage(data any) any {
	if ap, ok := data.(AppPage); ok {
		return ap.Page
	}
	return data
}

// HTMX swap target: .huck-content
//
// Per docs/front-end-plan.md §6, the app shell renders once per full-page
// load; HTMX swaps target fragments inside .huck-content and never replace
// the sidebar, topbar, or breadcrumb regions. The renderer enforces the
// response side of this rule: pages/* + HX-Request returns the page's
// "content" block alone, and the partials handlers actually render through
// c.Render (partials/error.html, partials/invite_row.html) are written as
// content-region HTML — no top-level <main> and no .huck-shell /
// .huck-sidebar / .huck-topbar / .huck-breadcrumbs markers. The shell
// partials (partials/sidebar.html, partials/topbar.html,
// partials/breadcrumbs.html) do carry shell classes but are only invoked
// from layout_app.html via {{ template … }}, never as HX responses.
//
// If a future interaction needs to update something *outside* .huck-content
// (e.g. flipping a user's admin flag should refresh the sidebar), the
// handler should set HX-Refresh: true on the response instead of returning
// a partial. That tells HTMX to do a full-page reload, which re-renders
// the shell from scratch — much cheaper than teaching every state-changing
// handler to swap out shell regions piecemeal.
//
// hxRedirect issues a redirect that works for both HTMX and non-HTMX
// requests. HTMX would swallow a 303 inside its swap pipeline, so for
// HX-Request it sets the HX-Redirect header on a 204 No Content; other
// requests get a plain 303 See Other. Centralised here so handlers do
// not branch on HX-Request, matching the renderer's contract.
func hxRedirect(c *echo.Context, path string) error {
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", path)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, path)
}

// isHXFragmentRequest reports whether the request is a "real" HTMX swap
// (HX-Request: true) and not a hx-boost full-page navigation.
func isHXFragmentRequest(c *echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	h := c.Request().Header
	if h.Get("HX-Request") != "true" {
		return false
	}
	if h.Get("HX-Boosted") != "" {
		return false
	}
	return true
}
