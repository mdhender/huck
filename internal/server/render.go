package server

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
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
type Renderer struct {
	pages    map[string]*template.Template // page name → cloned layout + page
	partials *template.Template            // partials/* parsed alone
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
		if _, err := clone.ParseFS(web.Templates, p); err != nil {
			return nil, fmt.Errorf("server: parse %s: %w", p, err)
		}
		// Key by "pages/<base>" so handlers can use the same name as
		// the file path under templates/.
		key := strings.TrimPrefix(p, "templates/")
		r.pages[key] = clone
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

	return r, nil
}

// Render satisfies echo.Renderer.
func (r *Renderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	switch {
	case strings.HasPrefix(name, "partials/"):
		return r.partials.ExecuteTemplate(w, path.Base(name), data)
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
