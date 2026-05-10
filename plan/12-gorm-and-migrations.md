# Step 12 — GORM Integration and Migrations

## Goal

Provide a thin, opinionated GORM integration plus a first-class migration
story. Apps should be able to declare models, run migrations as a normal
CLI step, and get a ready-to-use DB handle in their handlers.

## Deliverables

1. `pkg/db/db.go`:
   - `Open(cfg Config) (*gorm.DB, error)` — pools, logger, defaults.
   - `MustOpen(cfg Config) *gorm.DB` for bootstrapping.
   - Helpers: `WithTx(ctx, fn)`.
2. Migration tooling (`pkg/db/migrate/`):
   - Use `golang-migrate` under the hood (not GORM AutoMigrate for
     production).
   - CLI integration:
     - `goflex db create <name>` creates a new migration file pair
       (`NNN_name.up.sql`, `NNN_name.down.sql`).
     - `goflex db migrate` applies all pending migrations.
     - `goflex db rollback [--step N]` rolls back N (default 1) migrations.
     - `goflex db status` prints applied/pending.
3. Drivers: SQLite (default for dev), Postgres, MySQL.
4. `pkg/db/seed/` — a small seeding helper (`Seed(ctx, fn)`) that runs idempotent
   seed functions.
5. Repository-style helpers are **not** part of the framework; GORM is used
   directly. We only provide conventions in `docs/db.md`.

## Implementation notes

### Config

```go
type Config struct {
    Driver       string // "sqlite" | "postgres" | "mysql"
    DSN          string
    MaxOpenConns int
    MaxIdleConns int
    ConnMaxLife  time.Duration
    LogLevel     string // "silent"|"error"|"warn"|"info"
    Migrations   fs.FS  // embedded .sql files
}
```

### Why not rely on AutoMigrate?

- Not safe in production (schema drift, destructive changes).
- No reversible migrations.
- No version history.

AutoMigrate is allowed for local dev (`goflex db migrate --auto`), but is
disabled when `Env=prod`.

### Migration file layout

```text
db/migrations/
├── 001_create_users.up.sql
├── 001_create_users.down.sql
├── 002_add_todos.up.sql
└── 002_add_todos.down.sql
```

Migrations are embedded into the binary via `embed.FS` so production
deploys don't need the source tree.

### Model location conventions

- `internal/models/` — GORM models (server-only).
- `shared/` — DTOs only; these are what the API exposes.
- A function pattern `model.ToDTO() shared.XDTO` is recommended, never
  exposing models directly in API responses.

## Testing scenarios

### T12.1 — Open succeeds for SQLite

- `Open({Driver:"sqlite", DSN:":memory:"})` returns a non-nil `*gorm.DB`
  and a working ping.

### T12.2 — Connection pool settings respected

- A custom `MaxOpenConns=5` is reflected in `db.DB().Stats()`.

### T12.3 — Migration create

- `goflex db create add_todos` creates two files with the next sequential
  number, matching the expected naming pattern.
- Running it twice creates two different numeric prefixes.

### T12.4 — Migration apply and rollback

- Starting from an empty SQLite DB:
  - `goflex db migrate` applies all; tables exist.
  - `goflex db rollback` drops the last migration's tables.
  - `goflex db status` reports the correct state at each step.

### T12.5 — Idempotent migrate

- Running `migrate` twice produces the same state on the second run
  (no errors, no extra changes).

### T12.6 — Embedded migrations in binary

- Build a small test binary with `embed.FS` over a `migrations/` folder.
- Running migrations uses only the embedded files (no disk access except
  the DB).
- Confirmed by running the binary from a temp dir that doesn't contain the
  migrations source.

### T12.7 — Postgres integration (optional CI job)

- With a `postgres:16` test container (e.g. via `ory/dockertest`):
  - Migrations apply cleanly.
  - Rollback works.
- Gated with `//go:build integration`.

### T12.8 — WithTx semantics

- `WithTx(ctx, fn)` runs `fn` inside a transaction.
- Panic inside `fn` rolls back; caller observes the original panic.
- Returning an error rolls back.
- Returning nil commits.

### T12.9 — AutoMigrate disabled in prod

- With `Env="prod"`, calling the auto-migrate entry point returns an error
  `"AutoMigrate disabled in production"`.

### T12.10 — Model vs DTO separation enforced by lint

- A lint test walks `internal/api/*.go` and fails if any handler returns a
  `internal/models.*` type directly.

### T12.11 — Seed helper is idempotent

- Running a seed function twice produces the same row count.

## Acceptance criteria

- All T12.1–T12.6, T12.8, T12.9, T12.10, T12.11 pass in CI.
- T12.7 passes in the integration CI job (optional lane).
- `pkg/db` coverage >= 75%.
- `docs/db.md` shows: driver setup, writing a migration, applying in
  production, using `WithTx`, structuring models vs DTOs.
