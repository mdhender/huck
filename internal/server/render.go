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
//   - "pages/foo.html"         → render layout.html with the page's blocks
//     (title, content, scripts).
//
// Email templates live alongside but are reached via [Renderer.RenderEmail]
// rather than echo.Renderer.Render — they never go through layout.html and
// are produced as a string for the mailer.
type Renderer struct {
	pages    map[string]*template.Template // page name → cloned layout + page
	partials *template.Template            // partials/* parsed alone
	emails   *template.Template            // email/*   parsed alone, no layout
}

// NewRenderer builds the renderer from the embedded template FS.
func NewRenderer() (*Renderer, error) {
	r := &Renderer{pages: map[string]*template.Template{}}

	// Parse the layout once. Each page gets its own clone so its block
	// definitions override layout's defaults without leaking across pages.
	layout, err := template.New("layout.html").ParseFS(web.Templates, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("server: parse layout: %w", err)
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
		clone, err := layout.Clone()
		if err != nil {
			return nil, fmt.Errorf("server: clone layout: %w", err)
		}
		// Make partials available to pages via {{ template "partials/foo.html" . }}.
		if len(partialPaths) > 0 {
			if _, err := clone.ParseFS(web.Templates, partialPaths...); err != nil {
				return nil, fmt.Errorf("server: parse partials into %s: %w", p, err)
			}
		}
		if _, err := clone.ParseFS(web.Templates, p); err != nil {
			return nil, fmt.Errorf("server: parse %s: %w", p, err)
		}
		// Key by "pages/<base>" so handlers can use the same name as
		// the file path under templates/.
		key := strings.TrimPrefix(p, "templates/")
		r.pages[key] = clone
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
		if isHXFragmentRequest(c) {
			return page.ExecuteTemplate(w, "content", data)
		}
		return page.ExecuteTemplate(w, "layout.html", data)
	default:
		return fmt.Errorf("server: template name must start with pages/ or partials/, got %q", name)
	}
}

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
