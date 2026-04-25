package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestNeedsCSRF(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/me")
	if needsCSRF(c) {
		t.Fatal("GET must not require csrf")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	c = e.NewContext(req, rec)
	c.SetPath("/api/auth/login")
	if needsCSRF(c) {
		t.Fatal("login must not require csrf")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/rooms", nil)
	c = e.NewContext(req, rec)
	c.SetPath("/api/rooms")
	if !needsCSRF(c) {
		t.Fatal("POST /api/rooms must require csrf")
	}
}

func TestHasBearerAuth(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if hasBearerAuth(c) {
		t.Fatal("expected false for missing header")
	}
	req.Header.Set("Authorization", "Bearer token")
	if !hasBearerAuth(c) {
		t.Fatal("expected true for bearer header")
	}
}

func TestCSRFMiddleware(t *testing.T) {
	e := echo.New()
	mw := CSRFMiddleware()

	t.Run("allows bearer auth without csrf header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", nil)
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/rooms")
		handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
		if err := handler(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("rejects without csrf cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/rooms")
		handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
		if err := handler(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("rejects mismatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", nil)
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc"})
		req.Header.Set("X-CSRF-Token", "def")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/rooms")
		handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
		if err := handler(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("allows valid csrf pair", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/rooms", nil)
		req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "abc"})
		req.Header.Set("X-CSRF-Token", "abc")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/api/rooms")
		handler := mw(func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
		if err := handler(c); err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

