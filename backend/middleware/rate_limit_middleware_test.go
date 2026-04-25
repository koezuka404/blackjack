package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type fakeLimiter struct {
	result usecase.RateLimitResult
	err    error
}

func (f *fakeLimiter) Allow(ctx context.Context, key string) (usecase.RateLimitResult, error) {
	_ = ctx
	_ = key
	return f.result, f.err
}

func (f *fakeLimiter) AllowSignup(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}

func (f *fakeLimiter) AllowLogin(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}

func (f *fakeLimiter) AllowTasks(context.Context, uint) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}

func runMiddlewareRequest(t *testing.T, mw echo.MiddlewareFunc, userID string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if userID != "" {
		c.Set("user_id", userID)
	}

	handler := mw(func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})
	if err := handler(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return rec
}

func TestRateLimitMiddleware_AllowsWhenLimiterNil(t *testing.T) {
	rec := runMiddlewareRequest(t, RateLimitMiddleware(nil), "user-1")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestRateLimitMiddleware_SkipsWhenNoUserID(t *testing.T) {
	rec := runMiddlewareRequest(t, RateLimitMiddleware(&fakeLimiter{}), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestRateLimitMiddleware_ReturnsInternalErrorOnLimiterFailure(t *testing.T) {
	rec := runMiddlewareRequest(t, RateLimitMiddleware(&fakeLimiter{err: errors.New("redis down")}), "user-1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestRateLimitMiddleware_Returns429WithRetryHeader(t *testing.T) {
	rec := runMiddlewareRequest(t, RateLimitMiddleware(&fakeLimiter{
		result: usecase.RateLimitResult{Allowed: false, RetryAfterMS: 1234},
	}), "user-1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("X-RateLimit-Retry-After-Ms"); got != "1234" {
		t.Fatalf("unexpected retry-after header: %q", got)
	}
}

func TestRateLimitMiddleware_AllowsWhenLimiterAllows(t *testing.T) {
	rec := runMiddlewareRequest(t, RateLimitMiddleware(&fakeLimiter{
		result: usecase.RateLimitResult{Allowed: true},
	}), "user-1")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

