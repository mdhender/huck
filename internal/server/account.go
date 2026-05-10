package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/users"
)

// handleAccount renders the signed-in user's account detail page. The
// view shares the data shape of pages/admin_user_view.html scoped to
// the current user, so Sprint 4's layout sprint can swap the template
// without revisiting the data contract.
//
// The route is mounted under requireAuth, so anonymous requests have
// already been redirected to /login. A missing user row implies a
// deleted account whose JWT has not yet expired; clear the cookie and
// bounce to /login so the session does not silently outlive the user.
func (s *Server) handleAccount(c *echo.Context) error {
	claims := currentClaims(c)
	u, err := s.users.GetByID(c.Request().Context(), claims.UserID())
	if err != nil {
		if errors.Is(err, users.ErrNotFound) {
			s.clearAuthCookie(c)
			return hxRedirect(c, "/login")
		}
		return err
	}
	return c.Render(http.StatusOK, "pages/account.html",
		newAdminUserView(claims, u))
}
