package middleware

import (
	"net/http"
	"time"

	"blackjack/backend/observability"

	"github.com/labstack/echo/v4"
)


func HTTPTelemetryMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}
			observability.ObserveHTTPRequest(
				c.Request().Method,
				c.Path(),
				status,
				time.Since(start).Seconds(),
			)
			return err
		}
	}
}
