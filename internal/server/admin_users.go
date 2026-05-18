package server

import (
	"errors"
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
	Role         string
	Status       string
	LastLogin    string
	LastLoginISO string
	CreatedAt    string
	CreatedAtISO string
}

// adminUsersView is the data shape consumed by pages/admin_users.html.
// Sidebar/topbar chrome lives on the surrounding AppPage.Shell now, so
// the page-only view no longer carries the signed-in handle.
type adminUsersView struct {
	Notice string
	Rows   []userRowView
}

// adminUserView is the data shape consumed by pages/admin_user_view.html,
// pages/admin_user_edit.html, and pages/account.html (scoped to self).
// Sidebar/topbar chrome lives on the surrounding AppPage.Shell now, so
// the page-only view no longer carries the signed-in handle.
type adminUserView struct {
	User           users.User
	IsSelf         bool
	CreatedAt      string
	CreatedAtISO   string
	UpdatedAt      string
	UpdatedAtISO   string
	Status         string
	SuspendedAt    string
	SuspendedAtISO string
	Error          string
	Notice         string
}

// usersShell builds the [Home, Admin, Users] app-shell context shared by
// the list page. The per-user view/edit pages extend this trail and live
// in userDetailShell / userEditShell.
func usersShell(claims *auth.Claims) ShellView {
	return ShellView{
		Sidebar: SidebarView{
			Handle:  claims.Handle,
			IsAdmin: claims.Admin,
			Section: SectionAdminUsers,
		},
		Topbar: TopbarView{
			Handle: claims.Handle,
			Title:  "Users",
		},
		Crumbs: []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Administration", URL: "/admin"},
			{Label: "Users"},
		},
	}
}

// userDetailShell extends usersShell with the per-user current crumb and
// switches the topbar title to the user's handle. The Users entry is
// promoted to a link so the trail reads [Home, Admin, Users, <handle>].
func userDetailShell(claims *auth.Claims, u users.User) ShellView {
	return ShellView{
		Sidebar: SidebarView{
			Handle:  claims.Handle,
			IsAdmin: claims.Admin,
			Section: SectionAdminUsers,
		},
		Topbar: TopbarView{
			Handle: claims.Handle,
			Title:  u.Handle,
		},
		Crumbs: []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Administration", URL: "/admin"},
			{Label: "Users", URL: "/admin/users"},
			{Label: u.Handle},
		},
	}
}

// userEditShell extends userDetailShell with the trailing Edit crumb and
// promotes the user-detail entry to a link so the trail reads
// [Home, Admin, Users, <handle>, Edit].
func userEditShell(claims *auth.Claims, u users.User) ShellView {
	return ShellView{
		Sidebar: SidebarView{
			Handle:  claims.Handle,
			IsAdmin: claims.Admin,
			Section: SectionAdminUsers,
		},
		Topbar: TopbarView{
			Handle: claims.Handle,
			Title:  "Edit " + u.Handle,
		},
		Crumbs: []Crumb{
			{Label: "Home", URL: "/"},
			{Label: "Administration", URL: "/admin"},
			{Label: "Users", URL: "/admin/users"},
			{Label: u.Handle, URL: "/admin/users/" + strconv.FormatInt(u.ID, 10)},
			{Label: "Edit"},
		},
	}
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
		role := "User"
		if u.IsAdmin {
			role = "Admin"
		}
		status := users.StatusActive
		if u.IsSuspended() {
			status = users.StatusSuspended
		}
		var lastLogin, lastLoginISO string
		if !u.LastLoginAt.IsZero() {
			lastLogin, lastLoginISO = fmtUTC(u.LastLoginAt)
		}
		rows = append(rows, userRowView{
			ID:           u.ID,
			Handle:       u.Handle,
			Email:        u.Email,
			IsAdmin:      u.IsAdmin,
			IsSelf:       u.ID == claims.UserID(),
			Role:         role,
			Status:       status,
			LastLogin:    lastLogin,
			LastLoginISO: lastLoginISO,
			CreatedAt:    createdAt,
			CreatedAtISO: createdAtISO,
		})
	}
	return c.Render(http.StatusOK, "pages/admin_users.html", AppPage{
		Page:  adminUsersView{Rows: rows},
		Shell: usersShell(claims),
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
	return c.Render(http.StatusOK, "pages/admin_user_view.html", AppPage{
		Page:  newAdminUserView(claims, u),
		Shell: userDetailShell(claims, u),
	})
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
	return c.Render(http.StatusOK, "pages/admin_user_edit.html", AppPage{
		Page:  newAdminUserView(claims, u),
		Shell: userEditShell(claims, u),
	})
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

	if u.IsAdmin != wantAdmin {
		if err := s.users.SetAdmin(ctx, id, wantAdmin); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusSeeOther, "/admin/users/"+strconv.FormatInt(id, 10))
}

// handleAdminUsersSuspend soft-suspends a user. Refuses to suspend self
// (mirror of the existing self-demote 403 guard). Existing JWTs continue
// to work until expiry; project policy is to rotate --jwt-secret to mass-
// invalidate. See docs/sprint-5.md T3.1 and DESIGN.md §8.
func (s *Server) handleAdminUsersSuspend(c *echo.Context) error {
	claims := currentClaims(c)
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	if id == claims.UserID() {
		return echo.NewHTTPError(http.StatusForbidden,
			"You cannot suspend yourself. Ask another admin to make this change.")
	}
	if err := s.users.Suspend(c.Request().Context(), id); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}
	return hxRedirect(c, "/admin/users/"+strconv.FormatInt(id, 10))
}

// handleAdminUsersReactivate clears suspended_at on a user. No self-guard:
// a user cannot suspend themselves, so cannot reactivate themselves either.
func (s *Server) handleAdminUsersReactivate(c *echo.Context) error {
	id, err := parseUserID(c)
	if err != nil {
		return err
	}
	if err := s.users.Reactivate(c.Request().Context(), id); err != nil {
		if errors.Is(err, users.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "User not found.")
		}
		return err
	}
	return hxRedirect(c, "/admin/users/"+strconv.FormatInt(id, 10))
}

// renderAdminUserEditError re-renders the edit form with an error banner.
func (s *Server) renderAdminUserEditError(c *echo.Context, claims *auth.Claims, u users.User, msg string, status int) error {
	view := newAdminUserView(claims, u)
	view.Error = msg
	return c.Render(status, "pages/admin_user_edit.html", AppPage{
		Page:  view,
		Shell: userEditShell(claims, u),
	})
}

// newAdminUserView decorates a User with the format/self-aware fields the
// view + edit templates need.
func newAdminUserView(claims *auth.Claims, u users.User) adminUserView {
	createdAt, createdAtISO := fmtUTC(u.CreatedAt)
	updatedAt, updatedAtISO := fmtUTC(u.UpdatedAt)
	status := users.StatusActive
	var suspendedAt, suspendedAtISO string
	if u.IsSuspended() {
		status = users.StatusSuspended
		suspendedAt, suspendedAtISO = fmtUTC(u.SuspendedAt)
	}
	return adminUserView{
		User:           u,
		IsSelf:         u.ID == claims.UserID(),
		CreatedAt:      createdAt,
		CreatedAtISO:   createdAtISO,
		UpdatedAt:      updatedAt,
		UpdatedAtISO:   updatedAtISO,
		Status:         status,
		SuspendedAt:    suspendedAt,
		SuspendedAtISO: suspendedAtISO,
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
