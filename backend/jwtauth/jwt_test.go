package jwtauth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignAccessToken_RejectsShortSecret(t *testing.T) {
	_, _, _, err := SignAccessToken([]byte("short"), "user-1", time.Hour)
	if err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestSignAndParseAccessToken_Success(t *testing.T) {
	secret := []byte("this-is-a-very-long-secret")
	token, exp, jti, err := SignAccessToken(secret, "user-123", time.Hour)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}
	if token == "" {
		t.Fatal("token must not be empty")
	}
	if jti == "" {
		t.Fatal("jti must not be empty")
	}
	if time.Until(exp) <= 0 {
		t.Fatal("expiration must be in the future")
	}

	userID, parsedJTI, err := ParseAndValidate(secret, token)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("unexpected user id: %s", userID)
	}
	if parsedJTI != jti {
		t.Fatalf("unexpected jti: got %s want %s", parsedJTI, jti)
	}
}

func TestParseAndValidate_InvalidToken(t *testing.T) {
	_, _, err := ParseAndValidate([]byte("this-is-a-very-long-secret"), "not-a-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestParseAndValidate_RejectsShortSecret(t *testing.T) {
	_, _, err := ParseAndValidate([]byte("short"), "token")
	if err == nil {
		t.Fatal("expected error for short secret")
	}
	if !strings.Contains(err.Error(), "at least 16 bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

