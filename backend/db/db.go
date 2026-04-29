package db

import (
	"database/sql"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openPostgres(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

var openGormFn = openPostgres

func sqlDBFromGorm(gdb *gorm.DB) (sqlDB *sql.DB, err error) {
	defer func() {
		if r := recover(); r != nil {
			sqlDB = nil
			err = fmt.Errorf("gorm db handle unavailable")
		}
	}()
	return gdb.DB()
}

func configureSQLDB(sqlDB *sql.DB) {
	maxIdle := intFromEnv("DB_MAX_IDLE_CONNS", 20)
	maxOpen := intFromEnv("DB_MAX_OPEN_CONNS", 80)
	connMaxLifetime := durationFromEnv("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	connMaxIdleTime := durationFromEnv("DB_CONN_MAX_IDLE_TIME", 5*time.Minute)

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)
}

func intFromEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func Open(dsn string) (*gorm.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is empty")
	}
	gdb, err := openGormFn(dsn)
	if err != nil {
		return nil, err
	}
	sqlDB, err := sqlDBFromGorm(gdb)
	if err != nil {
		return nil, err
	}
	configureSQLDB(sqlDB)
	return gdb, nil
}

func Ping(ctx context.Context, gdb *gorm.DB) error {
	sqlDB, err := sqlDBFromGorm(gdb)
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
