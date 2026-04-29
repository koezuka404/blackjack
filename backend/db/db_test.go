package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOpen_EmptyDSN(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Fatal("expected error for empty dsn")
	}
}

func TestOpen_InvalidDSN(t *testing.T) {
	prev := openGormFn
	t.Cleanup(func() { openGormFn = prev })
	openGormFn = func(string) (*gorm.DB, error) { return nil, errors.New("open failed") }
	_, err := Open("postgres://example")
	if err == nil || err.Error() != "open failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPing_InvalidDB(t *testing.T) {
	err := Ping(context.Background(), &gorm.DB{})
	if err == nil {
		t.Fatal("expected error for invalid gorm db")
	}
}

func TestOpen_DBHandleUnavailable(t *testing.T) {
	prev := openGormFn
	t.Cleanup(func() { openGormFn = prev })
	openGormFn = func(string) (*gorm.DB, error) { return &gorm.DB{}, nil }
	_, err := Open("postgres://example")
	if err == nil {
		t.Fatal("expected db handle unavailable error")
	}
}

func TestOpen_AndPing_Success(t *testing.T) {
	prev := openGormFn
	t.Cleanup(func() { openGormFn = prev })
	openGormFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:dbtest?mode=memory&cache=shared"), &gorm.Config{})
	}
	gdb, err := Open("dummy-dsn")
	if err != nil {
		t.Fatalf("open should succeed: %v", err)
	}
	if err := Ping(context.Background(), gdb); err != nil {
		t.Fatalf("ping should succeed: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	if stats := sqlDB.Stats(); stats.MaxOpenConnections != 80 {
		t.Fatalf("expected max open conns=80, got=%d", stats.MaxOpenConnections)
	}
}

func TestOpenPostgres_UnreachableHost(t *testing.T) {
	_, err := openPostgres("postgres://user:pass@127.0.0.1:1/postgres?sslmode=disable")
	if err == nil {
		t.Fatal("expected dial error from openPostgres")
	}
}

func TestConfigureSQLDB(t *testing.T) {
	gdb, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:cfg_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	configureSQLDB(sqlDB)
	if stats := sqlDB.Stats(); stats.MaxOpenConnections != 80 {
		t.Fatalf("expected max open conns=80, got=%d", stats.MaxOpenConnections)
	}
}

func TestIntFromEnv(t *testing.T) {
	key := "DB_TEST_INT_FROM_ENV"
	t.Setenv(key, "")
	if got := intFromEnv(key, 12); got != 12 {
		t.Fatalf("empty env should fallback, got=%d", got)
	}

	t.Setenv(key, "64")
	if got := intFromEnv(key, 12); got != 64 {
		t.Fatalf("valid env should parse, got=%d", got)
	}

	t.Setenv(key, "oops")
	if got := intFromEnv(key, 12); got != 12 {
		t.Fatalf("invalid number should fallback, got=%d", got)
	}

	t.Setenv(key, "0")
	if got := intFromEnv(key, 12); got != 12 {
		t.Fatalf("non-positive number should fallback, got=%d", got)
	}
}

func TestDurationFromEnv(t *testing.T) {
	key := "DB_TEST_DURATION_FROM_ENV"
	t.Setenv(key, "")
	if got := durationFromEnv(key, 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("empty env should fallback, got=%s", got)
	}

	t.Setenv(key, "45s")
	if got := durationFromEnv(key, 2*time.Minute); got != 45*time.Second {
		t.Fatalf("valid duration should parse, got=%s", got)
	}

	t.Setenv(key, "not-a-duration")
	if got := durationFromEnv(key, 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("invalid duration should fallback, got=%s", got)
	}

	t.Setenv(key, "-10s")
	if got := durationFromEnv(key, 2*time.Minute); got != 2*time.Minute {
		t.Fatalf("non-positive duration should fallback, got=%s", got)
	}
}

