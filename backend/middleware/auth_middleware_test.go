package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackjack/backend/jwtauth"

	"github.com/labstack/echo/v4"
)

func TestSkipJWTAuth(t *testing.T) {
	e := echo.New()
	tests := []struct {
		path string
		want bool
	}{
		{path: "/api/auth/login", want: true},
		{path: "/api/auth/signup", want: true},
		{path: "/ws/rooms/:id", want: true},
		{path: "/api/ws/rooms/:id", want: true},
		{path: "/api/me", want: false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(tt.path)
		if got := skipJWTAuth(c); got != tt.want {
			t.Fatalf("path=%s got=%v want=%v", tt.path, got, tt.want)
		}
	}
}

func TestAuthMiddleware_ProtectedRouteWithoutAuthReturns401(t *testing.T) {
	e := echo.New()
	secret := []byte("this-is-a-very-long-secret")
	e.GET("/api/me", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, AuthMiddleware(secret))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestAuthMiddleware_ProtectedRouteWithInvalidTokenReturns401(t *testing.T) {
	e := echo.New()
	secret := []byte("this-is-a-very-long-secret")
	e.GET("/api/me", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, AuthMiddleware(secret))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidTokenSetsUserContext(t *testing.T) {
	e := echo.New()
	secret := []byte("this-is-a-very-long-secret")
	token, _, jti, err := jwtauth.SignAccessToken(secret, "user-123", time.Hour)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	e.GET("/api/me", func(c echo.Context) error {
		uid, _ := c.Get("user_id").(string)
		sid, _ := c.Get("session_id").(string)
		if uid != "user-123" {
			t.Fatalf("unexpected user id: %q", uid)
		}
		if sid != jti {
			t.Fatalf("unexpected session id: %q", sid)
		}
		return c.NoContent(http.StatusNoContent)
	}, AuthMiddleware(secret))

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
}

