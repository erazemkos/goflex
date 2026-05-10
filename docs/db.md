# Database

GoFlex uses GORM directly and provides only thin setup, migrations, transaction helpers, and seed conventions.

## Opening a database

```go
g, err := db.Open(db.Config{
    Driver:       "sqlite", // sqlite | postgres | mysql
    DSN:          "app.db",
    MaxOpenConns: 10,
    MaxIdleConns: 5,
    ConnMaxLife:  time.Hour,
    LogLevel:     "warn", // silent | error | warn | info
})
```

Defaults are SQLite with `goflex.db`, silent logging, and GORM's default pool settings. `db.MustOpen` panics on failure and is convenient in `main`.

Driver DSN examples:

```go
// SQLite
db.Open(db.Config{Driver: "sqlite", DSN: "app.db"})

// Postgres
db.Open(db.Config{Driver: "postgres", DSN: "postgres://user:pass@localhost:5432/app?sslmode=disable"})

// MySQL
db.Open(db.Config{Driver: "mysql", DSN: "user:pass@tcp(localhost:3306)/app?parseTime=true"})
```

## Migrations

Production schema changes should be SQL migrations, not GORM `AutoMigrate`.

Create a migration:

```sh
goflex db create create_users --dir db/migrations
```

This creates:

```text
db/migrations/001_create_users.up.sql
db/migrations/001_create_users.down.sql
```

Apply and inspect migrations:

```sh
goflex db migrate --driver sqlite --dsn app.db --dir db/migrations
goflex db status  --driver sqlite --dsn app.db --dir db/migrations
goflex db rollback --step 1 --driver sqlite --dsn app.db --dir db/migrations
```

Migrations are idempotent: already-applied versions are skipped. For production binaries, embed the migrations directory and run `migrate.UpWith(migrate.Config{FS: embeddedMigrations, Driver: "postgres", DSN: dsn})`.

## AutoMigrate

`db.AutoMigrate` is allowed for local development but intentionally blocked in production:

```go
if err := db.AutoMigrate(db.Config{Env: env}, g, &models.User{}); err != nil {
    return err // "AutoMigrate disabled in production" when Env == "prod"
}
```

## Transactions

```go
err := db.WithTx(ctx, g, func(tx *gorm.DB) error {
    if err := tx.Create(&models.User{Email: email}).Error; err != nil {
        return err // rollback
    }
    return nil // commit
})
```

Returning an error rolls back. Panics roll back and re-panic.

## Seeds

Seeds should be idempotent, usually with `FirstOrCreate` or upserts:

```go
err := seed.Seed(ctx, func(ctx context.Context) error {
    return g.WithContext(ctx).
        Where(models.User{Email: "admin@example.com"}).
        FirstOrCreate(&models.User{Email: "admin@example.com"}).Error
})
```

Use `seed.All(ctx, seedA, seedB)` to run multiple seed functions in order.

## Models vs DTOs

Keep GORM models server-only, typically under `internal/models`. Shared API contracts belong in `shared` packages as DTOs. Convert explicitly:

```go
// internal/models/user.go
type User struct { ID uint; Email string }

func (u User) ToDTO() shared.UserDTO {
    return shared.UserDTO{ID: u.ID, Email: u.Email}
}
```

Handlers should return DTOs, never `internal/models.*` types directly.
