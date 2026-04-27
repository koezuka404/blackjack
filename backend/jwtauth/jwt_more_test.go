package jwtauth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseAndValidate_UnexpectedMethod(t *testing.T) {
	secret := []byte("this-is-a-very-long-secret")
	tok := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.RegisteredClaims{Subject: "u1"})
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, _, err = ParseAndValidate(secret, signed)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestParseAndValidate_EmptySubject(t *testing.T) {
	secret := []byte("this-is-a-very-long-secret")
	claims := jwt.RegisteredClaims{
		Subject:   "",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	_, _, err = ParseAndValidate(secret, signed)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestSignAccessToken_SignError(t *testing.T) {
	prev := signTokenFn
	t.Cleanup(func() { signTokenFn = prev })
	signTokenFn = func(*jwt.Token, []byte) (string, error) {
		return "", errors.New("sign failed")
	}

	_, _, _, err := SignAccessToken([]byte("this-is-a-very-long-secret"), "u1", time.Hour)
	if err == nil || err.Error() != "sign failed" {
		t.Fatalf("expected sign failure, got: %v", err)
	}
}
