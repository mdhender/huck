package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/mdhender/huck/internal/invites"
)

// errorView is the data shape consumed by pages/error.html and
// partials/error.html.
type errorView struct {
	Status     int
	StatusText string
	Message    string
}

// installErrorHandler wires Echo's HTTPErrorHandler so every error returned
// by a handler is mapped to either pages/error.html or partials/error.html
// (for HTMX). Sentinel errors are mapped to friendly status codes.
func (s *Server) installErrorHandler() {
	s.echo.HTTPErrorHandler = func(c *echo.Context, err error) {
		if resp, _ := echo.UnwrapResponse(c.Response()); resp != nil && resp.Committed {
			return
		}

		status := http.StatusInternalServerError
		message := "Something went wrong."

		// Echo v5 returns its unexported *httpError (not *echo.HTTPError) for
		// router misses like ErrNotFound and ErrMethodNotAllowed, so the
		// HTTPStatusCoder interface is the portable extraction path. Check
		// *echo.HTTPError first so its Message field still surfaces to the user.
		var he *echo.HTTPError
		var sc echo.HTTPStatusCoder
		switch {
		case errors.Is(err, invites.ErrNotFound):
			status = http.StatusNotFound
			message = "This invite link does not exist."
		case errors.Is(err, invites.ErrExpired):
			status = http.StatusGone
			message = "This invite has expired. Ask your admin to resend it."
		case errors.Is(err, invites.ErrConsumed):
			status = http.StatusGone
			message = "This invite has already been used."
		case errors.As(err, &he):
			status = he.Code
			if he.Message != "" {
				message = he.Message
			} else {
				message = http.StatusText(status)
			}
		case errors.As(err, &sc):
			status = sc.StatusCode()
			message = http.StatusText(status)
		default:
			message = http.StatusText(status)
			s.logger.Error("unhandled handler error", "err", err, "path", c.Request().URL.Path)
		}

		view := errorView{
			Status:     status,
			StatusText: http.StatusText(status),
			Message:    message,
		}
		name := "pages/error.html"
		if isHXFragmentRequest(c) {
			name = "partials/error.html"
		}
		if rerr := c.Render(status, name, view); rerr != nil {
			// Last-ditch fallback so we never leak a blank response.
			http.Error(c.Response(), message, status)
			s.logger.Error("error renderer failed", "err", rerr)
		}
	}
}
