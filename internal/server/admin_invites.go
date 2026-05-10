package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/users"
)

// inviteRowView is the per-row data shape consumed by both
// pages/admin_invites.html (in a range) and partials/invite_row.html
// directly (HTMX swaps after resend).
type inviteRowView struct {
	Token        string
	Email        string
	Status       string
	CreatedAt    string
	CreatedAtISO string
	ExpiresAt    string
	ExpiresAtISO string
	CanResend    bool
}

// adminInvitesView is the data shape consumed by pages/admin_invites.html.
type adminInvitesView struct {
	Handle    string
	FormEmail string
	Error     string
	Notice    string
	Rows      []inviteRowView
}

// inviteEmailView is the data shape consumed by templates/email/invite.html.
type inviteEmailView struct {
	Email string
	URL   string
}

// inviteEmailSubject is fixed by sprint-2.md "In scope".
const inviteEmailSubject = "Welcome to Huck!"

// handleAdminInvitesList renders the admin invites page: a create form
// plus the list of every invite, most recent first.
func (s *Server) handleAdminInvitesList(c *echo.Context) error {
	claims := currentClaims(c)
	rows, err := s.loadInviteRows(c)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "pages/admin_invites.html", adminInvitesView{
		Handle: claims.Handle,
		Rows:   rows,
	})
}

// handleAdminInvitesCreate creates an invite + sends the welcome email
// inside one sqlitex.Transaction. A Mailgun failure rolls the row back
// and the response is 5xx so the admin sees the failure (sprint-2.md T6).
func (s *Server) handleAdminInvitesCreate(c *echo.Context) error {
	claims := currentClaims(c)
	email := users.Normalise(c.FormValue("email"))
	if email == "" {
		return s.renderAdminInvitesError(c, claims, email, "Email is required.", http.StatusUnprocessableEntity)
	}

	conn, err := s.pool.Take(c.Request().Context())
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	var created invites.Invite
	txErr := func() (txErr error) {
		end := sqlitex.Transaction(conn)
		defer end(&txErr)

		inv, err := s.invites.CreateOnConn(conn, email, claims.UserID())
		if err != nil {
			return err
		}
		body, err := s.renderer.RenderEmail("email/invite.html", inviteEmailView{
			Email: inv.Email,
			URL:   s.signupURL(inv),
		})
		if err != nil {
			return err
		}
		if err := s.mailer.Send(c.Request().Context(), inv.Email, inviteEmailSubject, body); err != nil {
			return fmt.Errorf("send invite mail: %w", err)
		}
		created = inv
		return nil
	}()

	if txErr != nil {
		switch {
		case errors.Is(txErr, invites.ErrEmailAlreadyInvited):
			return s.renderAdminInvitesError(c, claims, email,
				"That email already has an active invite. Revoke it first or wait for it to be consumed.",
				http.StatusConflict)
		default:
			s.logger.Error("admin invite create failed", "err", txErr, "email", email)
			return echo.NewHTTPError(http.StatusInternalServerError,
				"Could not send the invite. The row was rolled back; please try again.")
		}
	}

	rows, err := s.loadInviteRows(c)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "pages/admin_invites.html", adminInvitesView{
		Handle: claims.Handle,
		Notice: fmt.Sprintf("Invite sent to %s.", created.Email),
		Rows:   rows,
	})
}

// handleAdminInvitesResend refreshes the invite's expires_at and re-sends
// the welcome email. Returns the freshly-rendered row partial for HTMX
// to swap; non-HTMX requests get redirected back to the list page.
func (s *Server) handleAdminInvitesResend(c *echo.Context) error {
	tok := invites.Token(c.Param("token"))
	inv, err := s.invites.Resend(c.Request().Context(), tok)
	if err != nil {
		return err
	}
	body, err := s.renderer.RenderEmail("email/invite.html", inviteEmailView{
		Email: inv.Email,
		URL:   s.signupURL(inv),
	})
	if err != nil {
		return err
	}
	if err := s.mailer.Send(c.Request().Context(), inv.Email, inviteEmailSubject, body); err != nil {
		s.logger.Error("admin invite resend mail failed", "err", err, "email", inv.Email)
		return echo.NewHTTPError(http.StatusBadGateway, "Resend failed at the mail provider.")
	}
	if c.Request().Header.Get("HX-Request") == "true" {
		return c.Render(http.StatusOK, "partials/invite_row.html", rowView(inv))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/invites")
}

// handleAdminInvitesRevoke deletes the invite. For HTMX swaps the row
// disappears (empty body, hx-swap=outerHTML); other requests redirect.
func (s *Server) handleAdminInvitesRevoke(c *echo.Context) error {
	tok := invites.Token(c.Param("token"))
	if err := s.invites.Revoke(c.Request().Context(), tok); err != nil {
		return err
	}
	if c.Request().Header.Get("HX-Request") == "true" {
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, "/admin/invites")
}

// renderAdminInvitesError re-renders the page with a form-level error
// banner. Used for validation + duplicate-active-invite (409).
func (s *Server) renderAdminInvitesError(c *echo.Context, claims *auth.Claims, email, msg string, status int) error {
	rows, err := s.loadInviteRows(c)
	if err != nil {
		return err
	}
	return c.Render(status, "pages/admin_invites.html", adminInvitesView{
		Handle:    claims.Handle,
		FormEmail: email,
		Error:     msg,
		Rows:      rows,
	})
}

// loadInviteRows fetches every invite via the store and decorates each
// one with the display fields the templates need (Status, formatted
// times, can-resend gate).
func (s *Server) loadInviteRows(c *echo.Context) ([]inviteRowView, error) {
	all, err := s.invites.ListAll(c.Request().Context())
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]inviteRowView, 0, len(all))
	for _, inv := range all {
		out = append(out, rowViewAt(inv, now))
	}
	return out, nil
}

// signupURL builds the absolute signup link embedded in invite mail.
// The email query param mirrors what the recipient will see prefilled
// in the form (DESIGN.md §9, sprint-2.md "In scope").
func (s *Server) signupURL(inv invites.Invite) string {
	base := strings.TrimRight(s.cfg.BaseURL, "/")
	return fmt.Sprintf("%s/signup/%s?email=%s",
		base, inv.Token.String(), url.QueryEscape(inv.Email))
}

// rowView is rowViewAt with time.Now() supplied; convenient for callers
// that only render a single row (resend swaps).
func rowView(inv invites.Invite) inviteRowView {
	return rowViewAt(inv, time.Now().UTC())
}

func rowViewAt(inv invites.Invite, now time.Time) inviteRowView {
	status := "pending"
	canResend := true
	switch {
	case inv.Consumed():
		status = "consumed"
		canResend = false
	case inv.Expired(now):
		status = "expired"
	}
	createdAt, createdAtISO := fmtUTC(inv.CreatedAt)
	expiresAt, expiresAtISO := fmtUTC(inv.ExpiresAt)
	return inviteRowView{
		Token:        inv.Token.String(),
		Email:        inv.Email,
		Status:       status,
		CreatedAt:    createdAt,
		CreatedAtISO: createdAtISO,
		ExpiresAt:    expiresAt,
		ExpiresAtISO: expiresAtISO,
		CanResend:    canResend,
	}
}
