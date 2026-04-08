package middleware

import (
	"net/http"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/repository"

	"github.com/labstack/echo/v4"
)

func AuthMiddleware(store repository.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipAuth(c.Path()) {
				return next(c)
			}
			ck, err := c.Cookie("session_id")
			if err != nil || ck.Value == "" {
				return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
			}
			sess, err := store.GetAuthSession(c.Request().Context(), ck.Value)
			if err != nil || sess.ExpiresAt.Before(time.Now().UTC()) {
				return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
			}
			c.Set("user_id", sess.UserID)
			c.Set("session_id", sess.ID)
			return next(c)
		}
	}
}

func skipAuth(path string) bool {
	return path == "/api/auth/login" || path == "/api/auth/signup"
}
