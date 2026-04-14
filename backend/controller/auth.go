package controller

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// NewAuthController は認証 API ハンドラを生成する。
func NewAuthController(auth usecase.AuthUsecase) *AuthController {
	return &AuthController{auth: auth}
}

// Register は /api 配下に認証ルートを登録する。
func (a *AuthController) Register(g *echo.Group) {
	g.POST("/auth/signup", a.Signup)
	g.POST("/auth/login", a.Login)
	g.POST("/auth/logout", a.Logout)
	g.GET("/me", a.Me)
}

// Signup は新規登録し、セッション・CSRF Cookie を返す。
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

// Login はログインし、セッション・CSRF Cookie を返す。
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

// Logout はサーバー側セッション削除と Cookie クリアを行う。
func (a *AuthController) Logout(c echo.Context) error {
	sessionID, _ := readSessionCookie(c)
	if err := a.auth.Logout(c.Request().Context(), sessionID); err != nil {
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	clearSessionCookie(c)
	clearCSRFCookie(c)
	return c.JSON(http.StatusOK, dto.OK(map[string]any{}))
}

// Me は Cookie セッションから現在ユーザーを返す。
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

// readSessionCookie は session_id Cookie を読む。
func readSessionCookie(c echo.Context) (string, bool) {
	ck, err := c.Cookie("session_id")
	if err != nil || ck.Value == "" {
		return "", false
	}
	return ck.Value, true
}

// setSessionCookie は HttpOnly セッション Cookie をセットする。
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

// clearSessionCookie はセッション Cookie を消す。
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

// cookieSecure は Secure 属性を環境変数 COOKIE_SECURE で切り替える。
// 未設定または不正値は安全側（true）を使う。
func cookieSecure() bool {
	raw := strings.TrimSpace(os.Getenv("COOKIE_SECURE"))
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return v
}

// setCSRFCookie は Double Submit 用 csrf_token をセットする。
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

// clearCSRFCookie は CSRF Cookie を消す。
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

// generateCSRFToken は CSRF トークン文字列を生成する。
func generateCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
