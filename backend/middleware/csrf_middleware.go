package middleware

import (
	"net/http"

	"blackjack/backend/dto"

	"github.com/labstack/echo/v4"
)

func CSRFMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !needsCSRF(c) {
				return next(c)
			}
			ck, err := c.Cookie("csrf_token")
			if err != nil || ck.Value == "" {
				return c.JSON(http.StatusForbidden, dto.Fail("csrf_invalid", "csrf token is required"))
			}
			header := c.Request().Header.Get("X-CSRF-Token")
			if header == "" || header != ck.Value {
				return c.JSON(http.StatusForbidden, dto.Fail("csrf_invalid", "csrf token mismatch"))
			}
			return next(c)
		}
	}
}

func needsCSRF(c echo.Context) bool {
	switch c.Request().Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	path := c.Path()
	if path == "/api/auth/login" || path == "/api/auth/signup" {
		return false
	}
	return true
}
