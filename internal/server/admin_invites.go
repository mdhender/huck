package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/users"
)

// inviteRowView is the per-row data shape consumed by both
// pages/admin_invites.html (in a range) and partials/invite_row.html
// directly (HTMX swaps after resend/revoke). Status uses Invite.Status's
// Pending/Accepted/Expired/Revoked vocabulary (sprint-5 T2.2); the
// CanResend/CanRevoke/CanCopy gates branch the row's action cell by
// that status (sprint-5 T5.1, T5.3). Link is the absolute signup URL
// the Copy-link button writes to the clipboard.
type inviteRowView struct {
	Token        string
	Email        string
	Role         string
	Status       string
	CreatedAt    string
	CreatedAtISO string
	ExpiresAt    string
	ExpiresAtISO string
	Link         string
	CanResend    bool
	CanRevoke    bool
	CanCopy      bool
}

// adminInvitesView is the data shape consumed by pages/admin_invites.html.
// Sidebar/topbar chrome lives on the surrounding AppPage.Shell now, so
// the page-only view no longer carries the signed-in handle.
type adminInvitesView struct {
	FormEmail string
	FormRole  string
	Error     string
	Notice    string
	Rows      []inviteRowView
}

// adminInviteConfirmView is the data shape consumed by
// pages/admin_invite_confirm.html — the interstitial step that gates
// admin-invite creation behind a confirm POST (sprint-5 T5.2). The
// values are already normalised; the template re-posts them as hidden
// fields with confirm=true.
type adminInviteConfirmView struct {
	Email string
	Role  string
}

// invitesShell builds the app-shell context for any /admin/invites
// render (list, create success, create error). Centralised so the three
// callsites stay in lockstep on sidebar section, topbar title, and the
// [Home, Administration, Invitations] breadcrumb trail.
func invitesShell(claims *auth.Claims) ShellView {
	return ShellView{
		Sidebar: SidebarView{
			Handle:  claims.Handle,
			IsAdmin: claims.Admin,
			Section: SectionAdminInvites,
		},
		Topbar: TopbarView{
			Handle: claims.Handle,
			Title:  "Invitations",
		},
		Crumbs: []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Administration", URL: "/admin"},
			{Label: "Invitations"},
		},
	}
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
	return c.Render(http.StatusOK, "pages/admin_invites.html", AppPage{
		Page:  adminInvitesView{Rows: rows},
		Shell: invitesShell(claims),
	})
}

// handleAdminInvitesCreate creates an invite and sends the welcome
// email. The mail send happens OUTSIDE any SQLite write transaction so
// Mailgun latency does not block other writers (SQLite is single-writer
// and our prior in-transaction send serialised every concurrent write
// behind the Mailgun round-trip — sprint-5-review.md H1).
//
// Mail-failure semantics: if the Mailgun call fails after the row was
// inserted, we Revoke the new invite (soft-delete) and surface 5xx so
// the admin sees the failure. The revoked row stays for audit; the
// partial unique index excludes revoked rows, so a retry with the same
// email succeeds immediately (sprint-5 T2.1 partial-index predicate).
//
// Admin invites take a two-step path (sprint-5 T5.2 / DESIGN.md §9):
// a first POST without confirm=true renders an interstitial that
// re-posts the normalised values with confirm=true; only the second
// POST actually creates the row and sends mail. The signup form
// reads invites.is_admin server-side, so a tampered POST that sets
// role=admin without the prior interstitial cannot promote — the
// invite was created with IsAdmin: false.
func (s *Server) handleAdminInvitesCreate(c *echo.Context) error {
	claims := currentClaims(c)
	email := users.Normalise(c.FormValue("email"))
	role := c.FormValue("role")
	if role != "admin" {
		role = "user"
	}
	confirm := c.FormValue("confirm") == "true"

	if email == "" {
		return s.renderAdminInvitesError(c, claims, email, role,
			"Email is required.", http.StatusUnprocessableEntity)
	}

	if role == "admin" && !confirm {
		return c.Render(http.StatusOK, "pages/admin_invite_confirm.html", AppPage{
			Page:  adminInviteConfirmView{Email: email, Role: role},
			Shell: invitesShell(claims),
		})
	}

	ctx := c.Request().Context()
	created, err := s.invites.Create(ctx, invites.NewInvite{
		Email:     email,
		InvitedBy: claims.UserID(),
		IsAdmin:   role == "admin",
	})
	if err != nil {
		if errors.Is(err, invites.ErrEmailAlreadyInvited) {
			return s.renderAdminInvitesError(c, claims, email, role,
				"That email already has an active invite. Revoke it first or wait for it to be consumed.",
				http.StatusConflict)
		}
		s.logger.Error("admin invite create failed", "err", err, "email", email)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"Could not create the invite. Please try again.")
	}

	body, err := s.renderer.RenderEmail("email/invite.html", inviteEmailView{
		Email: created.Email,
		URL:   s.signupURL(created),
	})
	if err != nil {
		s.revokeAfterMailFailure(ctx, created.Token, "render invite mail", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"Could not render the invite mail. The pending invite has been revoked; please try again.")
	}
	if err := s.mailer.Send(ctx, created.Email, inviteEmailSubject, body); err != nil {
		s.revokeAfterMailFailure(ctx, created.Token, "send invite mail", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			"Could not send the invite. The pending invite has been revoked; please try again.")
	}

	rows, err := s.loadInviteRows(c)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "pages/admin_invites.html", AppPage{
		Page: adminInvitesView{
			Notice: fmt.Sprintf("Invite sent to %s.", created.Email),
			Rows:   rows,
		},
		Shell: invitesShell(claims),
	})
}

// revokeAfterMailFailure soft-deletes a freshly-created invite whose
// follow-up mail step failed. Errors during the revoke are logged but
// not surfaced — the caller is already returning a 5xx for the original
// mail failure. A failed revoke leaves a Pending row that an admin can
// revoke manually; the partial unique index keeps blocking retries for
// the same email until that happens, which is a loud signal something
// went wrong.
func (s *Server) revokeAfterMailFailure(ctx context.Context, tok invites.Token, stage string, cause error) {
	s.logger.Error("admin invite "+stage+" failed",
		"err", cause, "token", tok.String())
	if rerr := s.invites.Revoke(ctx, tok); rerr != nil {
		s.logger.Error("admin invite revoke after mail failure failed",
			"err", rerr, "token", tok.String())
	}
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
	if isHXFragmentRequest(c) {
		return c.Render(http.StatusOK, "partials/invite_row.html", rowView(inv, s.signupURL(inv)))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/invites")
}

// handleAdminInvitesRevoke soft-deletes the invite by stamping
// revoked_at and re-renders the row so it stays visible in the table
// with Status=Revoked and no row actions (sprint-5 T5.3). Non-HTMX
// callers get a redirect; on the next render the same row shows up via
// loadInviteRows in the Revoked state.
func (s *Server) handleAdminInvitesRevoke(c *echo.Context) error {
	tok := invites.Token(c.Param("token"))
	ctx := c.Request().Context()
	if err := s.invites.Revoke(ctx, tok); err != nil {
		return err
	}
	if !isHXFragmentRequest(c) {
		return c.Redirect(http.StatusSeeOther, "/admin/invites")
	}
	inv, err := s.invites.GetByToken(ctx, tok)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "partials/invite_row.html", rowView(inv, s.signupURL(inv)))
}

// renderAdminInvitesError re-renders the page with a form-level error
// banner. Used for validation + duplicate-active-invite (409). FormRole
// is echoed back so the admin's role selection survives the error.
func (s *Server) renderAdminInvitesError(c *echo.Context, claims *auth.Claims, email, role, msg string, status int) error {
	rows, err := s.loadInviteRows(c)
	if err != nil {
		return err
	}
	return c.Render(status, "pages/admin_invites.html", AppPage{
		Page: adminInvitesView{
			FormEmail: email,
			FormRole:  role,
			Error:     msg,
			Rows:      rows,
		},
		Shell: invitesShell(claims),
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
		out = append(out, rowViewAt(inv, now, s.signupURL(inv)))
	}
	return out, nil
}

// signupURL builds the absolute signup link embedded in invite mail.
// The email query param mirrors what the recipient will see prefilled
// in the form (DESIGN.md §9, sprint-2.md "In scope"). s.baseURL is
// pre-trimmed at server construction, so this is allocation-light
// enough for the per-row loadInviteRows call site.
func (s *Server) signupURL(inv invites.Invite) string {
	return fmt.Sprintf("%s/signup/%s?email=%s",
		s.baseURL, inv.Token.String(), url.QueryEscape(inv.Email))
}

// rowView is rowViewAt with time.Now() supplied; convenient for callers
// that only render a single row (resend / revoke HTMX swaps).
func rowView(inv invites.Invite, link string) inviteRowView {
	return rowViewAt(inv, time.Now().UTC(), link)
}

func rowViewAt(inv invites.Invite, now time.Time, link string) inviteRowView {
	status := inv.Status(now)
	// Pending and Expired invites are still actionable (Resend extends
	// expiry); Accepted and Revoked invites stay in the list for audit
	// with no row actions — including Copy link, which T5.1 lists only
	// for actionable rows (sprint-5 T5.1, T5.3).
	actionable := status == invites.StatusPending || status == invites.StatusExpired
	role := "User"
	if inv.IsAdmin {
		role = "Admin"
	}
	createdAt, createdAtISO := fmtUTC(inv.CreatedAt)
	expiresAt, expiresAtISO := fmtUTC(inv.ExpiresAt)
	return inviteRowView{
		Token:        inv.Token.String(),
		Email:        inv.Email,
		Role:         role,
		Status:       status,
		CreatedAt:    createdAt,
		CreatedAtISO: createdAtISO,
		ExpiresAt:    expiresAt,
		ExpiresAtISO: expiresAtISO,
		Link:         link,
		CanResend:    actionable,
		CanRevoke:    actionable,
		CanCopy:      actionable,
	}
}
