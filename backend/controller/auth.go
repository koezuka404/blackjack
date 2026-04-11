package controller

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type AuthController struct {
	auth usecase.AuthUsecase
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewAuthController(auth usecase.AuthUsecase) *AuthController {
	return &AuthController{auth: auth}
}

func (a *AuthController) Register(g *echo.Group) {
	g.POST("/auth/signup", a.Signup)
	g.POST("/auth/login", a.Login)
	g.POST("/auth/logout", a.Logout)
	g.GET("/me", a.Me)
}

func (a *AuthController) Signup(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "username and password are required"))
	}
	res, err := a.auth.Signup(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidInput):
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "username must be 3-100 chars and password must be at least 8 chars"))
		case errors.Is(err, usecase.ErrUsernameTaken):
			return c.JSON(http.StatusConflict, dto.Fail("username_taken", "username already exists"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	setSessionCookie(c, res.SessionToken(), int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()))
	csrfToken, err := generateCSRFToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	setCSRFCookie(c, csrfToken, int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()))
	return c.JSON(http.StatusCreated, dto.OK(map[string]any{
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
		},
		"csrf_token": csrfToken,
	}))
}

func (a *AuthController) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "username and password are required"))
	}
	res, err := a.auth.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, usecase.ErrUnauthorized) {
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "invalid credentials"))
		}
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	setSessionCookie(c, res.SessionToken(), int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()))
	csrfToken, err := generateCSRFToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	setCSRFCookie(c, csrfToken, int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()))
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
		},
		"csrf_token": csrfToken,
	}))
}

func (a *AuthController) Logout(c echo.Context) error {
	sessionID, _ := readSessionCookie(c)
	if err := a.auth.Logout(c.Request().Context(), sessionID); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	clearSessionCookie(c)
	clearCSRFCookie(c)
	return c.JSON(http.StatusOK, dto.OK(map[string]any{}))
}

func (a *AuthController) Me(c echo.Context) error {
	sessionID, ok := readSessionCookie(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "not logged in"))
	}
	user, err := a.auth.Me(c.Request().Context(), sessionID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "not logged in"))
	}
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"id":       user.ID,
		"username": user.Username,
	}))
}

func readSessionCookie(c echo.Context) (string, bool) {
	ck, err := c.Cookie("session_id")
	if err != nil || ck.Value == "" {
		return "", false
	}
	return ck.Value, true
}

func setSessionCookie(c echo.Context, token string, maxAge int) {
	c.SetCookie(&http.Cookie{
		Name:     "session_id",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
	})
}

func clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
	})
}

func cookieSecure() bool {
	return true
}

func setCSRFCookie(c echo.Context, token string, maxAge int) {
	c.SetCookie(&http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
	})
}

func clearCSRFCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     "csrf_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   cookieSecure(),
	})
}

func generateCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
