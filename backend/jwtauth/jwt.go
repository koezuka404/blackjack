package jwtauth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid jwt")
var signTokenFn = func(t *jwt.Token, secret []byte) (string, error) {
	return t.SignedString(secret)
}


func SignAccessToken(secret []byte, userID string, ttl time.Duration) (token string, expiresAt time.Time, jti string, err error) {
	if len(secret) < 16 {
		return "", time.Time{}, "", fmt.Errorf("jwt secret must be at least 16 bytes")
	}
	jti = uuid.NewString()
	now := time.Now().UTC()
	expiresAt = now.Add(ttl)
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ID:        jti,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := signTokenFn(t, secret)
	if err != nil {
		return "", time.Time{}, "", err
	}
	return signed, expiresAt, jti, nil
}


func ParseAndValidate(secret []byte, tokenString string) (userID, jti string, err error) {
	if len(secret) < 16 {
		return "", "", fmt.Errorf("jwt secret must be at least 16 bytes")
	}
	tok, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret, nil
	}, jwt.WithLeeway(30*time.Second))
	if err != nil || !tok.Valid {
		return "", "", ErrInvalidToken
	}
	claims, ok := tok.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return "", "", ErrInvalidToken
	}
	return claims.Subject, claims.ID, nil
}
