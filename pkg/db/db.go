package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	Driver       string
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
	ConnMaxLife  time.Duration
	LogLevel     string
	Migrations   fs.FS
	Env          string
}

func Open(cfg Config) (*gorm.DB, error) {
	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}
	if cfg.DSN == "" {
		cfg.DSN = defaultDSN(cfg.Driver)
	}
	dial, err := dialector(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, err
	}
	g, err := gorm.Open(dial, &gorm.Config{Logger: logger.Default.LogMode(logLevel(cfg.LogLevel))})
	if err != nil {
		return nil, err
	}
	sqldb, err := g.DB()
	if err == nil {
		applyPool(sqldb, cfg)
	}
	return g, nil
}

func MustOpen(cfg Config) *gorm.DB {
	g, err := Open(cfg)
	if err != nil {
		panic(err)
	}
	return g
}

func WithTx(ctx context.Context, g *gorm.DB, fn func(*gorm.DB) error) error {
	return g.WithContext(ctx).Transaction(fn)
}

func AutoMigrate(cfg Config, g *gorm.DB, values ...any) error {
	if cfg.Env == "prod" {
		return fmt.Errorf("AutoMigrate disabled in production")
	}
	return g.AutoMigrate(values...)
}

func dialector(driver, dsn string) (gorm.Dialector, error) {
	switch driver {
	case "sqlite", "sqlite3":
		return sqlite.Open(dsn), nil
	case "postgres", "postgresql":
		return postgres.Open(dsn), nil
	case "mysql":
		return mysql.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported driver %s", driver)
	}
}

func defaultDSN(driver string) string {
	if driver == "sqlite" || driver == "sqlite3" || driver == "" {
		return "goflex.db"
	}
	return ""
}

func logLevel(level string) logger.LogLevel {
	switch level {
	case "error":
		return logger.Error
	case "warn":
		return logger.Warn
	case "info":
		return logger.Info
	case "silent", "":
		return logger.Silent
	default:
		return logger.Silent
	}
}

func applyPool(db *sql.DB, cfg Config) {
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLife)
	}
}
