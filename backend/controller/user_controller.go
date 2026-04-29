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
	Email    string `json:"email"`
	Password string `json:"password"`
}

type signupRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
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
	var req signupRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "ユーザー名・メールアドレス・パスワードを入力してください"))
	}
	res, err := a.auth.Signup(c.Request().Context(), req.Username, req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidInput):
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "ユーザー名は3〜100文字、メール形式、パスワードは8文字以上の英字+数字で入力してください"))
		case errors.Is(err, usecase.ErrUsernameTaken), errors.Is(err, usecase.ErrEmailTaken):
			return c.JSON(http.StatusConflict, dto.Fail("email_taken", "ユーザー名またはメールアドレスは既に使われています"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", "サーバーエラーが発生しました"))
		}
	}
	return c.JSON(http.StatusCreated, dto.OK(map[string]any{
		"access_token": res.SessionToken(),
		"token_type":   "Bearer",
		"expires_in":   int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()),
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
			"email":    res.User().Email,
		},
	}))
}


func (a *AuthController) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "メールアドレスとパスワードを入力してください"))
	}
	res, err := a.auth.Login(c.Request().Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, usecase.ErrUnauthorized) {
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "メールアドレスまたはパスワードが違います"))
		}
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", "サーバーエラーが発生しました"))
	}
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"access_token": res.SessionToken(),
		"token_type":   "Bearer",
		"expires_in":   int(res.ExpiresAt().Sub(time.Now().UTC()).Seconds()),
		"user": map[string]any{
			"id":       res.User().ID,
			"username": res.User().Username,
			"email":    res.User().Email,
		},
	}))
}


func (a *AuthController) Logout(c echo.Context) error {
	_ = a.auth.Logout(c.Request().Context())
	return c.JSON(http.StatusOK, dto.OK(map[string]any{}))
}


func (a *AuthController) Me(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "ログインしてください"))
	}
	user, err := a.auth.Me(c.Request().Context(), userID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "ログインしてください"))
	}
	return c.JSON(http.StatusOK, dto.OK(map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	}))
}
