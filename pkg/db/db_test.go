package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

type txRow struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func TestOpenSQLiteAndPoolSettings(t *testing.T) {
	g, err := Open(Config{Driver: "sqlite", DSN: ":memory:", MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLife: time.Minute, LogLevel: "info"})
	if err != nil {
		t.Fatal(err)
	}
	sqldb, err := g.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqldb.Ping(); err != nil {
		t.Fatal(err)
	}
	if got := sqldb.Stats().MaxOpenConnections; got != 5 {
		t.Fatalf("MaxOpenConnections=%d", got)
	}
}

func TestUnsupportedDriver(t *testing.T) {
	if _, err := Open(Config{Driver: "oracle"}); err == nil {
		t.Fatal("want unsupported driver error")
	}
}

func TestMustOpenPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	_ = MustOpen(Config{Driver: "oracle"})
}

func TestWithTxCommitErrorRollbackAndPanic(t *testing.T) {
	g, err := Open(Config{Driver: "sqlite", DSN: "file:txtest?mode=memory&cache=shared"})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(Config{}, g, &txRow{}); err != nil {
		t.Fatal(err)
	}
	if err := WithTx(context.Background(), g, func(tx *gorm.DB) error {
		return tx.Create(&txRow{Name: "commit"}).Error
	}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := g.Model(&txRow{}).Where("name = ?", "commit").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("commit count=%d err=%v", count, err)
	}
	errBoom := errors.New("boom")
	if err := WithTx(context.Background(), g, func(tx *gorm.DB) error {
		if err := tx.Create(&txRow{Name: "rollback"}).Error; err != nil {
			return err
		}
		return errBoom
	}); !errors.Is(err, errBoom) {
		t.Fatalf("rollback err=%v", err)
	}
	if err := g.Model(&txRow{}).Where("name = ?", "rollback").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rollback count=%d err=%v", count, err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "panic rollback" {
				t.Fatalf("recovered=%v", recovered)
			}
		}()
		_ = WithTx(context.Background(), g, func(tx *gorm.DB) error {
			if err := tx.Create(&txRow{Name: "panic"}).Error; err != nil {
				return err
			}
			panic("panic rollback")
		})
	}()
	if err := g.Model(&txRow{}).Where("name = ?", "panic").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("panic count=%d err=%v", count, err)
	}
}

func TestAutoMigrateProdGuard(t *testing.T) {
	g, err := Open(Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(Config{Env: "prod"}, g, &txRow{}); err == nil || err.Error() != "AutoMigrate disabled in production" {
		t.Fatalf("err=%v", err)
	}
}
