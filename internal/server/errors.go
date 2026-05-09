package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
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

		var he *echo.HTTPError
		if errors.As(err, &he) {
			status = he.Code
			if he.Message != "" {
				message = he.Message
			} else {
				message = http.StatusText(status)
			}
		} else {
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
