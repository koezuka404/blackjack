package middleware

import (
	"net/http"

	"blackjack/backend/dto"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

func RateLimitMiddleware(limiter usecase.RateLimitUsecase) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if limiter == nil {
				return next(c)
			}
			userID, _ := c.Get("user_id").(string)
			if userID == "" {
				return next(c)
			}
			ok, err := limiter.Allow(c.Request().Context(), "http:"+userID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
			}
			if !ok {
				return c.JSON(http.StatusTooManyRequests, dto.Fail("rate_limited", "too many requests"))
			}
			return next(c)
		}
	}
}
