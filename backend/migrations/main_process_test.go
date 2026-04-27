package main

import (
	"os"
	"os/exec"
	"testing"

	"gorm.io/gorm"
)

func TestMain_ProcessSuccessAndFailure(t *testing.T) {
	if os.Getenv("TEST_MIGRATIONS_MAIN_HELPER") == "1" {
		if os.Getenv("TEST_MIGRATIONS_MAIN_MODE") == "success" {
			openDB = func(string) (*gorm.DB, error) { return &gorm.DB{}, nil }
			migrateDB = func(*gorm.DB) error { return nil }
		} else {
			openDB = func(string) (*gorm.DB, error) { return &gorm.DB{}, nil }
			migrateDB = func(*gorm.DB) error { return os.ErrInvalid }
		}
		main()
		return
	}

	t.Run("success", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=TestMain_ProcessSuccessAndFailure/success")
		cmd.Env = append(os.Environ(), "TEST_MIGRATIONS_MAIN_HELPER=1", "TEST_MIGRATIONS_MAIN_MODE=success", "DATABASE_URL=dummy")
		if err := cmd.Run(); err != nil {
			t.Fatalf("main should succeed: %v", err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=TestMain_ProcessSuccessAndFailure/failure")
		cmd.Env = append(os.Environ(), "TEST_MIGRATIONS_MAIN_HELPER=1", "TEST_MIGRATIONS_MAIN_MODE=failure", "DATABASE_URL=dummy")
		err := cmd.Run()
		if err == nil {
			t.Fatal("main should fail with non-zero exit")
		}
	})
}
