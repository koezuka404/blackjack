package main

import (
	"log"
	"os"

	"blackjack/backend/db"
)

var (
	openDB    = db.Open
	migrateDB = db.Migrate
)

func run(dsn string) error {
	if dsn == "" {
		return os.ErrInvalid
	}
	gdb, err := openDB(dsn)
	if err != nil {
		return err
	}
	return migrateDB(gdb)
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if err := run(dsn); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	log.Println("migration completed")
}
