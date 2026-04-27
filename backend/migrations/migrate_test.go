package main

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestRun_EmptyDSN(t *testing.T) {
	err := run("")
	if err == nil {
		t.Fatal("expected error for empty dsn")
	}
}

func TestRun_OpenFailure(t *testing.T) {
	prevOpen := openDB
	prevMigrate := migrateDB
	t.Cleanup(func() {
		openDB = prevOpen
		migrateDB = prevMigrate
	})

	openDB = func(string) (*gorm.DB, error) {
		return nil, errors.New("open failed")
	}
	migrateDB = func(*gorm.DB) error {
		return nil
	}

	err := run("postgres://example")
	if err == nil || err.Error() != "open failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_MigrateFailure(t *testing.T) {
	prevOpen := openDB
	prevMigrate := migrateDB
	t.Cleanup(func() {
		openDB = prevOpen
		migrateDB = prevMigrate
	})

	openDB = func(string) (*gorm.DB, error) {
		return &gorm.DB{}, nil
	}
	migrateDB = func(*gorm.DB) error {
		return errors.New("migrate failed")
	}

	err := run("postgres://example")
	if err == nil || err.Error() != "migrate failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_Success(t *testing.T) {
	prevOpen := openDB
	prevMigrate := migrateDB
	t.Cleanup(func() {
		openDB = prevOpen
		migrateDB = prevMigrate
	})

	openCalled := false
	migrateCalled := false
	openDB = func(string) (*gorm.DB, error) {
		openCalled = true
		return &gorm.DB{}, nil
	}
	migrateDB = func(*gorm.DB) error {
		migrateCalled = true
		return nil
	}

	if err := run("postgres://example"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !openCalled || !migrateCalled {
		t.Fatalf("expected both open and migrate to be called (open=%v migrate=%v)", openCalled, migrateCalled)
	}
}

func TestMain_SuccessAndFailure(t *testing.T) {
	prevGetenv := getenv
	prevFatalf := logFatalf
	prevPrintln := logPrintln
	prevOpen := openDB
	prevMigrate := migrateDB
	t.Cleanup(func() {
		getenv = prevGetenv
		logFatalf = prevFatalf
		logPrintln = prevPrintln
		openDB = prevOpen
		migrateDB = prevMigrate
	})

	openDB = func(string) (*gorm.DB, error) { return &gorm.DB{}, nil }

	t.Run("success", func(t *testing.T) {
		getenv = func(string) string { return "postgres://ok" }
		fatalCalled := false
		printCalled := false
		logFatalf = func(string, ...any) { fatalCalled = true }
		logPrintln = func(...any) { printCalled = true }
		migrateDB = func(*gorm.DB) error { return nil }

		main()
		if fatalCalled {
			t.Fatal("fatal should not be called")
		}
		if !printCalled {
			t.Fatal("print should be called on success")
		}
	})

	t.Run("failure", func(t *testing.T) {
		getenv = func(string) string { return "" }
		fatalCalled := false
		logFatalf = func(string, ...any) { fatalCalled = true; panic("fatal-exit") }
		logPrintln = func(...any) {}
		migrateDB = func(*gorm.DB) error { return nil }

		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic to stop execution")
			}
		}()
		main()
		if !fatalCalled {
			t.Fatal("fatal should be called on run error")
		}
	})
}

