package db

import (
	"database/sql"
	"context"
	"fmt"
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
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(time.Hour)
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
