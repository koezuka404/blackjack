package controller

import (
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

// Signup は新規登録し JWT を返す。
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
	return c.JSON(http.StatusCreated, dto.OK(map[string]any{
		"access_token": res.SessionToken(),
		"token_type":   "Bearer",
		"expires_in":   int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()),
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
		},
	}))
}

// Login はログインし JWT を返す。
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
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"access_token": res.SessionToken(),
		"token_type":   "Bearer",
		"expires_in":   int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()),
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
		},
	}))
}

// Logout はクライアント側トークン破棄用の成功応答（サーバーはステートレスで無効化しない）。
func (a *AuthController) Logout(c echo.Context) error {
	_ = a.auth.Logout(c.Request().Context())
	return c.JSON(http.StatusOK, dto.OK(map[string]any{}))
}

// Me は JWT から解決した現在ユーザーを返す。
func (a *AuthController) Me(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "not logged in"))
	}
	user, err := a.auth.Me(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "not logged in"))
	}
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"id":       user.ID,
		"username": user.Username,
	}))
}
