package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/users"
)

// userRowView is the per-row data shape consumed by pages/admin_users.html.
type userRowView struct {
	ID           int64
	Handle       string
	Email        string
	IsAdmin      bool
	IsSelf       bool
	CreatedAt    string
	CreatedAtISO string
}

// adminUsersView is the data shape consumed by pages/admin_users.html.
type adminUsersView struct {
	Handle string
	Notice string
	Rows   []userRowView
}

// adminUserView is the data shape consumed by pages/admin_user_view.html
// and pages/admin_user_edit.html.
type adminUserView struct {
	Handle       string
	User         users.User
	IsSelf       bool
	CreatedAt    string
	CreatedAtISO string
	UpdatedAt    string
	UpdatedAtISO string
	Error        string
	Notice       string
}

// handleAdminUsersList renders the admin users page.
func (s *Server) handleAdminUsersList(c *echo.Context) error {
	claims := currentClaims(c)
	all, err := s.users.ListAll(c.Request().Context())
	if err != nil {
		return err
	}
	rows := make([]userRowView, 0, len(all))
	for _, u := range all {
		createdAt, createdAtISO := fmtUTC(u.CreatedAt)
		rows = append(rows, userRowView{
			ID:           u.ID,
			Handle:       u.Handle,
			Email:        u.Email,
			IsAdmin:      u.IsAdmin,
			IsSelf:       u.ID == claims.UserID(),
			CreatedAt:    createdAt,
			CreatedAtISO: createdAtISO,
		})
	}
	return c.Render(http.StatusOK, "pages/admin_users.html", adminUsersView{
		Handle: claims.Handle,
		Rows:   rows,
	})
}

// handleAdminUsersView renders a read-only summary of one user.
func (s *Server) handleAdminUsersView(c *echo.Context) error {
	claims := currentClaims(c)
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}
	return c.Render(http.StatusOK, "pages/admin_user_view.html",
		newAdminUserView(claims, u))
}

// handleAdminUsersEditForm renders the edit form for one user.
func (s *Server) handleAdminUsersEditForm(c *echo.Context) error {
	claims := currentClaims(c)
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}
	return c.Render(http.StatusOK, "pages/admin_user_edit.html",
		newAdminUserView(claims, u))
}

// handleAdminUsersEditSubmit applies an is_admin toggle and/or password
// reset. Refuses to demote self (sprint-2.md self-lockout guard).
func (s *Server) handleAdminUsersEditSubmit(c *echo.Context) error {
	claims := currentClaims(c)
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	u, err := s.users.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}

	wantAdmin := c.FormValue("is_admin") == "1"
	if id == claims.UserID() && !wantAdmin {
		return s.renderAdminUserEditError(c, claims, u,
			"You cannot demote yourself. Ask another admin to make this change.",
			http.StatusForbidden)
	}

	password := c.FormValue("password")

	if u.IsAdmin != wantAdmin {
		if err := s.users.SetAdmin(ctx, id, wantAdmin); err != nil {
			return err
		}
	}
	if password != "" {
		if err := auth.ValidatePassword(password); err != nil {
			return s.renderAdminUserEditError(c, claims, u,
				adminUserPasswordErrorMessage(err), http.StatusUnprocessableEntity)
		}
		hash, err := auth.Hash(password)
		if err != nil {
			return err
		}
		if err := s.users.SetPassword(ctx, id, hash); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users/"+strconv.FormatInt(id, 10))
}

// handleAdminUsersDelete hard-deletes a user. Refuses to delete self
// (sprint-2.md self-lockout guard).
func (s *Server) handleAdminUsersDelete(c *echo.Context) error {
	claims := currentClaims(c)
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	if id == claims.UserID() {
		return echo.NewHTTPError(http.StatusForbidden,
			"You cannot delete yourself. Ask another admin to remove this account.")
	}
	if err := s.users.Delete(c.Request().Context(), id); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users")
}

// renderAdminUserEditError re-renders the edit form with an error banner.
func (s *Server) renderAdminUserEditError(c *echo.Context, claims *auth.Claims, u users.User, msg string, status int) error {
	view := newAdminUserView(claims, u)
	view.Error = msg
	return c.Render(status, "pages/admin_user_edit.html", view)
}

// newAdminUserView decorates a User with the format/self-aware fields the
// view + edit templates need.
func newAdminUserView(claims *auth.Claims, u users.User) adminUserView {
	createdAt, createdAtISO := fmtUTC(u.CreatedAt)
	updatedAt, updatedAtISO := fmtUTC(u.UpdatedAt)
	return adminUserView{
		Handle:       claims.Handle,
		User:         u,
		IsSelf:       u.ID == claims.UserID(),
		CreatedAt:    createdAt,
		CreatedAtISO: createdAtISO,
		UpdatedAt:    updatedAt,
		UpdatedAtISO: updatedAtISO,
	}
}

// parseUserID reads :id from the URL and rejects non-numeric or
// non-positive values with 400.
func parseUserID(c *echo.Context) (int64, error) {
	raw := c.Param("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "Invalid user id.")
	}
	return id, nil
}

// adminUserPasswordErrorMessage maps a password-validator error to a
// short message naming the rule that failed. Falls back to a generic
// message for unexpected errors so we never leak internals.
func adminUserPasswordErrorMessage(err error) string {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		return fmt.Sprintf("Password must be at least %d characters.", auth.MinPasswordLen)
	case errors.Is(err, auth.ErrPasswordTooLong):
		return fmt.Sprintf("Password must be at most %d characters.", auth.MaxPasswordLen)
	case errors.Is(err, auth.ErrPasswordNotPrintable):
		return "Password contains a non-printable character."
	}
	return "Password is invalid."
}
