package middleware

import (
	"net/http"

	"blackjack/backend/dto"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

func AuthMiddleware(secret []byte) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		Skipper: skipJWTAuth,
		SigningKey: secret,
		NewClaimsFunc: func(c echo.Context) jwt.Claims {
			return &jwt.RegisteredClaims{}
		},
		ErrorHandler: func(c echo.Context, err error) error {
			if c.Request().Header.Get("Authorization") == "" {
				return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
			}
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "invalid or expired token"))
		},
		SuccessHandler: func(c echo.Context) {
			setAuthContextFromToken(c)
		},
	})
}

func setAuthContextFromToken(c echo.Context) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil {
		return
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return
	}
	c.Set("user_id", claims.Subject)
	if claims.ID != "" {
		c.Set("session_id", claims.ID)
	}
}


func skipJWTAuth(c echo.Context) bool {
	p := c.Path()
	switch p {
	case "/api/auth/login", "/api/auth/signup":
		return true
	case "/ws/rooms/:id", "/api/ws/rooms/:id":
		return true
	default:
		return false
	}
}
