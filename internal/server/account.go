package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/users"
)

// handleAccount renders the signed-in user's account detail page. The
// page view shares the data shape of pages/admin_user_view.html scoped
// to the current user; the renderer wraps it in an [AppPage] so the
// app-shell sidebar/topbar/breadcrumbs see the typed shell view while
// the template's content block still receives the original page view.
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
	return c.Render(http.StatusOK, "pages/account.html", AppPage{
		Page: newAdminUserView(claims, u),
		Shell: ShellView{
			Sidebar: SidebarView{
				Handle:  claims.Handle,
				IsAdmin: claims.Admin,
				Section: SectionAccount,
			},
			Topbar: TopbarView{
				Handle: claims.Handle,
				Title:  "Account",
			},
			Crumbs: []Crumb{
				{Label: "Home", URL: "/"},
				{Label: "Account"},
			},
		},
	})
}
