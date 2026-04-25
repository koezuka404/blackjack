package db

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestOpen_EmptyDSN(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Fatal("expected error for empty dsn")
	}
}

func TestOpen_InvalidDSN(t *testing.T) {
	_, err := Open("://invalid-dsn")
	if err == nil {
		t.Fatal("expected error for invalid dsn")
	}
}

func TestPing_InvalidDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid gorm db")
		}
	}()
	_ = Ping(context.Background(), &gorm.DB{})
}

