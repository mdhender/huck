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
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/mail"
	"github.com/mdhender/huck/internal/users"
	"github.com/mdhender/huck/web"
)

// Server bundles the dependencies a handler needs. It is intentionally
// a struct, not a context lookup, so handlers stay typed.
type Server struct {
	cfg      *config.Config
	echo     *echo.Echo
	renderer *Renderer
	pool     *sqlitex.Pool
	users    *users.Store
	invites  *invites.Store
	mailer   mail.Mailer
	logger   *slog.Logger

	jwtKey []byte
}

// New returns an Echo instance fully configured with middleware, routes,
// and the renderer. The caller drives the lifecycle (Start/Shutdown).
//
// pool is held by the Server so multi-store transactions (notably the
// signup pipeline in handleSignupSubmit) can acquire a connection and
// run users.CreateOnConn + invites.Consume inside one boundary.
//
// mailer is taken as an interface so tests can inject mail.FakeMailer.
func New(cfg *config.Config, pool *sqlitex.Pool, usersStore *users.Store, invitesStore *invites.Store, mailer mail.Mailer, logger *slog.Logger) (*Server, error) {
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
		cfg:      cfg,
		echo:     e,
		renderer: r,
		pool:     pool,
		users:    usersStore,
		invites:  invitesStore,
		mailer:   mailer,
		logger:   logger,
		jwtKey:   []byte(cfg.JWTSecret),
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
	// Mounted as Echo middleware (rather than wrapping srv.Echo() in
	// cmd/huck/runServe) so all request-shaping middleware lives in one
	// place and httptest in this package exercises the same chain.
	s.echo.Use(crossOriginProtection())
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
	s.echo.GET("/signup/:token", s.handleSignupForm)
	s.echo.POST("/signup/:token", s.handleSignupSubmit)

	admin := s.echo.Group("/admin", s.requireAdmin())
	admin.GET("", s.handleAdminIndex)
	admin.GET("/invites", s.handleAdminInvitesList)
	admin.POST("/invites", s.handleAdminInvitesCreate)
	admin.POST("/invites/:token/resend", s.handleAdminInvitesResend)
	admin.POST("/invites/:token/revoke", s.handleAdminInvitesRevoke)
	admin.GET("/users", s.handleAdminUsersList)
	admin.GET("/users/:id", s.handleAdminUsersView)
	admin.GET("/users/:id/edit", s.handleAdminUsersEditForm)
	admin.POST("/users/:id/edit", s.handleAdminUsersEditSubmit)
	admin.POST("/users/:id/delete", s.handleAdminUsersDelete)
}

// homeView is the data shape consumed by both home_public.html and
// home_authed.html.
type homeView struct {
	Authed bool
	Handle string
	Admin  bool
}

// handleAdminIndex is the GET /admin landing. The home page's "Admin"
// link points here; rather than a dedicated index page, redirect to
// the invite list (the operator's most common entry point).
func (s *Server) handleAdminIndex(c *echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/admin/invites")
}

// handleHome renders home_public.html for anonymous visitors and
// home_authed.html for authenticated ones. Best-effort auth: an
// invalid/missing/expired cookie just means anonymous, never 401.
func (s *Server) handleHome(c *echo.Context) error {
	var view homeView

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

// loginView is the data shape consumed by pages/login.html.
type loginView struct {
	Handle string
	Error  string
}

func (s *Server) handleLoginForm(c *echo.Context) error {
	if _, ok := s.bestEffortClaims(c); ok {
		return c.Redirect(http.StatusSeeOther, "/")
	}
	return c.Render(http.StatusOK, "pages/login.html", loginView{})
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

// signupView is the data shape consumed by pages/signup.html.
type signupView struct {
	Token  string
	Email  string
	Handle string
	Error  string
}

// handleSignupForm renders the invite-landing form for a valid token.
// A missing/expired/consumed token produces an error page; the central
// error handler maps the invites sentinels to friendly status codes.
func (s *Server) handleSignupForm(c *echo.Context) error {
	tok := invites.Token(c.Param("token"))
	inv, err := s.invites.GetByToken(c.Request().Context(), tok)
	if err != nil {
		return err
	}
	if inv.Consumed() {
		return invites.ErrConsumed
	}
	if inv.Expired(time.Now().UTC()) {
		return invites.ErrExpired
	}
	return c.Render(http.StatusOK, "pages/signup.html", signupView{
		Token: tok.String(),
		Email: inv.Email,
	})
}

// handleSignupSubmit runs the entire pipeline (token re-validation,
// email re-check, validators, user insert, invite consumption) inside a
// single zombiezen sqlitex.Transaction so two parallel form submits
// cannot both succeed (DESIGN.md §9 step 5).
func (s *Server) handleSignupSubmit(c *echo.Context) error {
	tok := invites.Token(c.Param("token"))
	submittedEmail := users.Normalise(c.FormValue("email"))
	handle := c.FormValue("handle")
	password := c.FormValue("password")

	conn, err := s.pool.Take(c.Request().Context())
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	var newUser users.User

	txErr := func() (txErr error) {
		end := sqlitex.Transaction(conn)
		defer end(&txErr)

		inv, err := s.invites.GetByTokenOnConn(conn, tok)
		if err != nil {
			return err
		}
		if inv.Consumed() {
			return invites.ErrConsumed
		}
		if inv.Expired(time.Now().UTC()) {
			return invites.ErrExpired
		}
		if submittedEmail != inv.Email {
			return errEmailMismatch
		}
		if err := auth.ValidateHandle(handle); err != nil {
			return err
		}
		if err := auth.ValidatePassword(password); err != nil {
			return err
		}
		hash, err := auth.Hash(password)
		if err != nil {
			return err
		}
		u, err := s.users.CreateOnConn(conn, users.NewUser{
			Handle:       handle,
			Email:        inv.Email,
			PasswordHash: hash,
			IsAdmin:      false,
		})
		if err != nil {
			return err
		}
		if err := s.invites.Consume(c.Request().Context(), conn, tok); err != nil {
			return err
		}
		newUser = u
		return nil
	}()

	if txErr != nil {
		return s.renderSignupFailure(c, tok.String(), submittedEmail, handle, txErr)
	}

	jwtToken, err := auth.Issue(newUser, s.jwtKey, auth.DefaultTokenTTL)
	if err != nil {
		return err
	}
	s.setAuthCookie(c, jwtToken)

	if c.Request().Header.Get("HX-Request") == "true" {
		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, "/")
}

// errEmailMismatch is returned when the form-submitted email differs
// from the email bound to the invite (defence-in-depth against
// tampering with the readonly field). Mapped to a form-level error,
// not an HTTP error.
var errEmailMismatch = errors.New("server: signup email does not match invite")

// renderSignupFailure re-renders the signup form with a user-facing
// message for validation/uniqueness errors, and propagates expired or
// consumed invites to the central error handler.
func (s *Server) renderSignupFailure(c *echo.Context, token, email, handle string, err error) error {
	switch {
	case errors.Is(err, invites.ErrNotFound),
		errors.Is(err, invites.ErrExpired),
		errors.Is(err, invites.ErrConsumed):
		return err
	}

	view := signupView{
		Token:  token,
		Email:  email,
		Handle: handle,
		Error:  signupErrorMessage(err),
	}
	return c.Render(http.StatusUnprocessableEntity, "pages/signup.html", view)
}

// signupErrorMessage maps a validator/store error to a short message
// the user can act on. Falls back to a generic message for unexpected
// errors so we never leak internal details.
func signupErrorMessage(err error) string {
	switch {
	case errors.Is(err, errEmailMismatch):
		return "Submitted email does not match the invite."
	case errors.Is(err, auth.ErrPasswordTooShort):
		return fmt.Sprintf("Password must be at least %d characters.", auth.MinPasswordLen)
	case errors.Is(err, auth.ErrPasswordTooLong):
		return fmt.Sprintf("Password must be at most %d characters.", auth.MaxPasswordLen)
	case errors.Is(err, auth.ErrPasswordNotPrintable):
		return "Password contains a non-printable character."
	case errors.Is(err, auth.ErrHandleTooShort),
		errors.Is(err, auth.ErrHandleTooLong),
		errors.Is(err, auth.ErrHandleBadFirstChar),
		errors.Is(err, auth.ErrHandleBadChar):
		return "Handle must be 3–32 characters, start with a lowercase letter, and use only lowercase letters, digits, and _ , . ' -"
	case errors.Is(err, users.ErrHandleTaken):
		return "That handle is already taken."
	case errors.Is(err, users.ErrEmailTaken):
		return "That email is already in use."
	}
	return "Could not create account. Please try again."
}
