package main

import (
	"log"
	"os"

	"blackjack/backend/db"
)

var (
	openDB    = db.Open
	migrateDB = db.Migrate
	getenv    = os.Getenv
	logFatalf = log.Fatalf
	logPrintln = log.Println
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
	dsn := getenv("DATABASE_URL")
	if err := run(dsn); err != nil {
		logFatalf("migrate: %v", err)
	}

	logPrintln("migration completed")
}
