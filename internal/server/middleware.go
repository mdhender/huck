package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/mdhender/huck/internal/auth"
)

// claimsContextKey is the c.Get/c.Set key under which requireAuth stashes
// the parsed claims so downstream handlers can read them without
// re-parsing the cookie.
const claimsContextKey = "auth.claims"

// requireAuth is the gate for routes that need an authenticated user. A
// missing/invalid/expired cookie sends the user to /login (HX-Redirect
// for HTMX, 303 otherwise). On success the typed claims are stashed in
// the context under claimsContextKey.
func (s *Server) requireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			claims, ok := s.bestEffortClaims(c)
			if !ok {
				if c.Request().Header.Get("HX-Request") == "true" {
					c.Response().Header().Set("HX-Redirect", "/login")
					return c.NoContent(http.StatusNoContent)
				}
				return c.Redirect(http.StatusSeeOther, "/login")
			}
			c.Set(claimsContextKey, claims)
			return next(c)
		}
	}
}

// requireAdmin runs requireAuth and then refuses non-admin authed users
// with 403. The central error handler renders the friendly error page.
func (s *Server) requireAdmin() echo.MiddlewareFunc {
	authMW := s.requireAuth()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return authMW(func(c *echo.Context) error {
			claims := currentClaims(c)
			if claims == nil || !claims.Admin {
				return echo.NewHTTPError(http.StatusForbidden, "Admin access required.")
			}
			return next(c)
		})
	}
}

// currentClaims returns the claims stashed by requireAuth, or nil if
// the middleware was not in the chain.
func currentClaims(c *echo.Context) *auth.Claims {
	if v := c.Get(claimsContextKey); v != nil {
		if claims, ok := v.(*auth.Claims); ok {
			return claims
		}
	}
	return nil
}

// crossOriginProtection delegates to net/http.CrossOriginProtection
// (Go 1.25+), the stdlib replacement for the older double-submit
// _csrf cookie scheme. The default ruleset rejects state-changing
// browser requests whose Sec-Fetch-Site or Origin headers mark them
// as cross-origin; safe methods (GET/HEAD/OPTIONS) and non-browser
// requests pass through. Huck is single-origin, so no
// AddTrustedOrigin call is needed.
func crossOriginProtection() echo.MiddlewareFunc {
	cop := http.NewCrossOriginProtection()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := cop.Check(c.Request()); err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "cross-origin request blocked")
			}
			return next(c)
		}
	}
}

// securityHeaders sets the headers from docs/DESIGN.md §12 on every
// response. HSTS is only set when --cookie-secure is on; that flag means
// "we expect to be served over HTTPS".
func securityHeaders(secure bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					"script-src 'self'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data:; "+
					"base-uri 'self'; "+
					"frame-ancestors 'none'")
			if secure {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			return next(c)
		}
	}
}

// requestLogger emits a single slog line per request via Echo's
// RequestLoggerWithConfig. We only log the things that are safe to log
// (no JWT, no cookies, no form bodies).
func requestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogUserAgent: false,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			if v.Status >= 500 {
				level = slog.LevelError
			} else if v.Status >= 400 {
				level = slog.LevelWarn
			}
			logger.LogAttrs(c.Request().Context(), level, "http",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency.Round(time.Microsecond)),
				slog.String("ip", v.RemoteIP),
			)
			return nil
		},
	})
}
