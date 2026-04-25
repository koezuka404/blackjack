package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type authResponseStub struct {
	token string
	exp   time.Time
	user  *model.User
}

func (a authResponseStub) SessionToken() string { return a.token }
func (a authResponseStub) ExpiresAt() time.Time { return a.exp }
func (a authResponseStub) User() *model.User    { return a.user }

type authUsecaseStub struct {
	signupFn func(context.Context, string, string) (usecase.AuthResponse, error)
	loginFn  func(context.Context, string, string) (usecase.AuthResponse, error)
	logoutFn func(context.Context) error
	meFn     func(context.Context, string) (*model.User, error)
}

func (a authUsecaseStub) Signup(ctx context.Context, username, password string) (usecase.AuthResponse, error) {
	if a.signupFn != nil {
		return a.signupFn(ctx, username, password)
	}
	return nil, nil
}
func (a authUsecaseStub) Login(ctx context.Context, username, password string) (usecase.AuthResponse, error) {
	if a.loginFn != nil {
		return a.loginFn(ctx, username, password)
	}
	return nil, nil
}
func (a authUsecaseStub) Logout(ctx context.Context) error {
	if a.logoutFn != nil {
		return a.logoutFn(ctx)
	}
	return nil
}
func (a authUsecaseStub) Me(ctx context.Context, userID string) (*model.User, error) {
	if a.meFn != nil {
		return a.meFn(ctx, userID)
	}
	return nil, errors.New("not found")
}

func newJSONContext(t *testing.T, method, path string, body any) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	var buf []byte
	if body != nil {
		var err error
		buf, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body failed: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestAuthController_Signup_StatusBranches(t *testing.T) {
	t.Run("201 success", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/signup", map[string]any{
			"username": "alice",
			"password": "password12",
		})
		ctrl := NewAuthController(authUsecaseStub{
			signupFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
				return authResponseStub{
					token: "tok",
					exp:   time.Now().Add(time.Hour),
					user:  &model.User{ID: "u1", Username: "alice"},
				}, nil
			},
		})
		if err := ctrl.Signup(c); err != nil {
			t.Fatalf("signup failed: %v", err)
		}
		if rec.Code != http.StatusCreated {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("400 bad request bind/empty", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/signup", map[string]any{
			"username": "",
			"password": "",
		})
		ctrl := NewAuthController(authUsecaseStub{})
		_ = ctrl.Signup(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("400 invalid input from usecase", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/signup", map[string]any{
			"username": "ab",
			"password": "short",
		})
		ctrl := NewAuthController(authUsecaseStub{
			signupFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
				return nil, usecase.ErrInvalidInput
			},
		})
		_ = ctrl.Signup(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("409 username taken", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/signup", map[string]any{
			"username": "alice",
			"password": "password12",
		})
		ctrl := NewAuthController(authUsecaseStub{
			signupFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
				return nil, usecase.ErrUsernameTaken
			},
		})
		_ = ctrl.Signup(c)
		if rec.Code != http.StatusConflict {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

func TestAuthController_LoginAndMe_StatusBranches(t *testing.T) {
	t.Run("login 400 invalid payload", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/login", map[string]any{
			"username": "",
			"password": "",
		})
		ctrl := NewAuthController(authUsecaseStub{})
		_ = ctrl.Login(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("login 401 unauthorized", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/login", map[string]any{
			"username": "alice",
			"password": "wrong",
		})
		ctrl := NewAuthController(authUsecaseStub{
			loginFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
				return nil, usecase.ErrUnauthorized
			},
		})
		_ = ctrl.Login(c)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("login 200 success", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodPost, "/api/auth/login", map[string]any{
			"username": "alice",
			"password": "password12",
		})
		ctrl := NewAuthController(authUsecaseStub{
			loginFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
				return authResponseStub{
					token: "tok",
					exp:   time.Now().Add(time.Hour),
					user:  &model.User{ID: "u1", Username: "alice"},
				}, nil
			},
		})
		_ = ctrl.Login(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("me 401 without user_id", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodGet, "/api/me", nil)
		ctrl := NewAuthController(authUsecaseStub{})
		_ = ctrl.Me(c)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("me 200 success", func(t *testing.T) {
		c, rec := newJSONContext(t, http.MethodGet, "/api/me", nil)
		c.Set("user_id", "u1")
		ctrl := NewAuthController(authUsecaseStub{
			meFn: func(context.Context, string) (*model.User, error) {
				return &model.User{ID: "u1", Username: "alice"}, nil
			},
		})
		_ = ctrl.Me(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

