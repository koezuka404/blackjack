package db

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"blackjack/backend/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrate_ErrorsAndSuccess(t *testing.T) {
	prevAuto := autoMigrateFn
	prevFK := ensureForeignKeysFn
	prevIdx := ensurePlayerStatesSessionSeatUniqueIndexFn
	prevLegacy := ensureLegacyUsersEmailFn
	t.Cleanup(func() {
		autoMigrateFn = prevAuto
		ensureForeignKeysFn = prevFK
		ensurePlayerStatesSessionSeatUniqueIndexFn = prevIdx
		ensureLegacyUsersEmailFn = prevLegacy
	})

	gdb := &gorm.DB{}

	ensureLegacyUsersEmailFn = func(*gorm.DB) error { return nil }
	autoMigrateFn = func(*gorm.DB, ...any) error { return errors.New("auto") }
	if err := Migrate(gdb); err == nil || err.Error() != "auto" {
		t.Fatalf("unexpected error: %v", err)
	}

	autoMigrateFn = func(*gorm.DB, ...any) error { return nil }
	ensureForeignKeysFn = func(*gorm.DB) error { return errors.New("fk") }
	if err := Migrate(gdb); err == nil || err.Error() != "fk" {
		t.Fatalf("unexpected error: %v", err)
	}

	ensureForeignKeysFn = func(*gorm.DB) error { return nil }
	ensurePlayerStatesSessionSeatUniqueIndexFn = func(*gorm.DB) error { return errors.New("idx") }
	if err := Migrate(gdb); err == nil || err.Error() != "idx" {
		t.Fatalf("unexpected error: %v", err)
	}

	ensurePlayerStatesSessionSeatUniqueIndexFn = func(*gorm.DB) error { return nil }
	if err := Migrate(gdb); err != nil {
		t.Fatalf("unexpected success error: %v", err)
	}
}

func TestMigrate_LegacyEmailFailurePropagates(t *testing.T) {
	prevLegacy := ensureLegacyUsersEmailFn
	t.Cleanup(func() { ensureLegacyUsersEmailFn = prevLegacy })
	gdb := &gorm.DB{}
	ensureLegacyUsersEmailFn = func(*gorm.DB) error { return errors.New("legacy") }
	if err := Migrate(gdb); err == nil || err.Error() != "legacy" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureFunctions_UseExecHook(t *testing.T) {
	prevExec := execSQLFn
	t.Cleanup(func() { execSQLFn = prevExec })
	gdb := &gorm.DB{}

	calls := 0
	execSQLFn = func(*gorm.DB, string) error {
		calls++
		return nil
	}
	if err := ensurePlayerStatesSessionSeatUniqueIndex(gdb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 exec calls, got %d", calls)
	}

	execSQLFn = func(*gorm.DB, string) error { return errors.New("exec") }
	if err := ensureForeignKeys(gdb); err == nil || err.Error() != "exec" {
		t.Fatalf("unexpected error: %v", err)
	}

	execSQLFn = func(*gorm.DB, string) error { return nil }
	if err := ensureForeignKeys(gdb); err != nil {
		t.Fatalf("unexpected success error: %v", err)
	}
}

func TestAutoMigrateModels_ExecSQLGorm_DefaultImplementations(t *testing.T) {
	dsn := fmt.Sprintf("file:migrate_impl_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := autoMigrateModels(gdb, &repository.UserRecord{}); err != nil {
		t.Fatalf("autoMigrateModels: %v", err)
	}
	if err := execSQLGorm(gdb, "SELECT 1"); err != nil {
		t.Fatalf("execSQLGorm: %v", err)
	}
}
