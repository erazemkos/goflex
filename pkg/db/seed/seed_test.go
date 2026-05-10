package seed

import (
	"context"
	"testing"

	frameworkdb "github.com/erazemkos/goflex/pkg/db"
)

type seedUser struct {
	ID    uint   `gorm:"primaryKey"`
	Email string `gorm:"uniqueIndex"`
}

func TestSeedIdempotent(t *testing.T) {
	g, err := frameworkdb.Open(frameworkdb.Config{Driver: "sqlite", DSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.AutoMigrate(&seedUser{}); err != nil {
		t.Fatal(err)
	}
	fn := func(ctx context.Context) error {
		return g.WithContext(ctx).Where(seedUser{Email: "admin@example.com"}).FirstOrCreate(&seedUser{Email: "admin@example.com"}).Error
	}
	if err := Seed(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
	if err := All(context.Background(), fn); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := g.Model(&seedUser{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
}
