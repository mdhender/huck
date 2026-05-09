// Package server wires the Echo instance, mounts middleware, and routes
// HTTP requests to handlers. It owns the Renderer and the global error
// handler.
package server

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/users"
	"github.com/mdhender/huck/web"
)

// Server bundles the dependencies a handler needs. It is intentionally
// a struct, not a context lookup, so handlers stay typed.
type Server struct {
	cfg    *config.Config
	echo   *echo.Echo
	users  *users.Store
	logger *slog.Logger

	jwtKey []byte
}

// New returns an Echo instance fully configured with middleware, routes,
// and the renderer. The caller drives the lifecycle (Start/Shutdown).
func New(cfg *config.Config, store *users.Store, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	r, err := NewRenderer()
	if err != nil {
		return nil, err
	}

	e := echo.New()
	e.Renderer = r

	s := &Server{
		cfg:    cfg,
		echo:   e,
		users:  store,
		logger: logger,
		jwtKey: []byte(cfg.JWTSecret),
	}

	s.installErrorHandler()
	s.installMiddleware()
	if err := s.installStatic(); err != nil {
		return nil, err
	}
	s.installRoutes()
	return s, nil
}

// Echo exposes the underlying Echo instance for callers that need it
// (the cmd package uses it to drive StartConfig + graceful shutdown, and
// the integration test uses it for httptest).
func (s *Server) Echo() *echo.Echo { return s.echo }

func (s *Server) installMiddleware() {
	s.echo.Use(requestLogger(s.logger))
	s.echo.Use(middleware.Recover())
	s.echo.Use(securityHeaders(s.cfg.CookieSecure))

	s.echo.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "header:X-CSRF-Token,form:_csrf",
		CookieName:     "_csrf",
		CookiePath:     "/",
		CookieSecure:   s.cfg.CookieSecure,
		CookieHTTPOnly: false, // JS must read it to mirror into the header.
		CookieSameSite: http.SameSiteLaxMode,
		CookieMaxAge:   86400,
	}))
}

func (s *Server) installStatic() error {
	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return fmt.Errorf("server: sub static: %w", err)
	}
	s.echo.StaticFS("/static", staticFS)
	return nil
}

func (s *Server) installRoutes() {
	s.echo.GET("/", s.handleHome)
	s.echo.GET("/login", s.handleLoginForm)
	s.echo.POST("/login", s.handleLoginSubmit)
	s.echo.POST("/logout", s.handleLogout)
}

// homeView is the data shape consumed by both home_public.html and
// home_authed.html.
type homeView struct {
	Authed bool
	Handle string
	Admin  bool
	CSRF   string
}

// handleHome renders home_public.html for anonymous visitors and
// home_authed.html for authenticated ones. Best-effort auth: an
// invalid/missing/expired cookie just means anonymous, never 401.
func (s *Server) handleHome(c *echo.Context) error {
	view := homeView{CSRF: csrfToken(c)}

	if claims, ok := s.bestEffortClaims(c); ok {
		view.Authed = true
		view.Handle = claims.Handle
		view.Admin = claims.Admin
		return c.Render(http.StatusOK, "pages/home_authed.html", view)
	}
	return c.Render(http.StatusOK, "pages/home_public.html", view)
}

// bestEffortClaims attempts to parse and validate the auth cookie. A
// missing or invalid cookie is not an error; it just means anonymous.
func (s *Server) bestEffortClaims(c *echo.Context) (*auth.Claims, bool) {
	cookie, err := c.Cookie(auth.CookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	claims, err := auth.Parse(cookie.Value, s.jwtKey)
	if err != nil {
		return nil, false
	}
	return claims, true
}

// csrfToken extracts the token Echo's CSRF middleware just put in the
// context, falling back to "" if the middleware was somehow skipped.
func csrfToken(c *echo.Context) string {
	if v := c.Get("csrf"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// loginView is the data shape consumed by pages/login.html.
type loginView struct {
	CSRF   string
	Handle string
	Error  string
}

func (s *Server) handleLoginForm(c *echo.Context) error {
	if _, ok := s.bestEffortClaims(c); ok {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	return c.Render(http.StatusOK, "pages/login.html", loginView{CSRF: csrfToken(c)})
}

func (s *Server) handleLoginSubmit(c *echo.Context) error {
	handle := c.FormValue("handle")
	password := c.FormValue("password")

	user, err := s.users.GetByHandle(c.Request().Context(), handle)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return s.renderLoginFailure(c, handle)
		}
		return err
	}
	if err := auth.Verify(user.PasswordHash, password); err != nil {
		if errors.Is(err, auth.ErrBadPassword) {
			return s.renderLoginFailure(c, handle)
		}
		return err
	}

	token, err := auth.Issue(user, s.jwtKey, auth.DefaultTokenTTL)
	if err != nil {
		return err
	}
	s.setAuthCookie(c, token)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/")
}

func (s *Server) renderLoginFailure(c *echo.Context, handle string) error {
	return c.Render(http.StatusUnauthorized, "pages/login.html", loginView{
		CSRF:   csrfToken(c),
		Handle: users.Normalise(handle),
		Error:  "Unknown handle or wrong password.",
	})
}

func (s *Server) handleLogout(c *echo.Context) error {
	s.clearAuthCookie(c)
	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/")
}

// setAuthCookie applies the attributes from docs/DESIGN.md §8.1.
func (s *Server) setAuthCookie(c *echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Domain:   s.cfg.CookieDomain,
		MaxAge:   int(auth.DefaultTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearAuthCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		Domain:   s.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
