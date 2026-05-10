# GoFlex Todo Example

This is the reference GoFlex v0 app. It demonstrates the full stack in one small project:

- Gin-backed API via `pkg/server`
- cookie sessions, login/logout, CSRF via `pkg/auth`
- GORM + SQLite models and migrations via `pkg/db`
- shared DTOs and endpoint declarations via `pkg/api`
- typed client calls via `pkg/apiclient`
- frontend UI with `pkg/ui`, `pkg/hooks`, `pkg/router`, `pkg/query`, and `pkg/form`
- Tailwind class extraction and production asset builds

## Project layout

```text
cmd/server        backend entrypoint
cmd/web           frontend entrypoint
internal/api      HTTP handlers using shared DTOs
internal/models   GORM models and persistence
internal/web      frontend pages/components
shared            DTOs and api.Endpoint declarations
db/migrations     SQL migrations
assets            custom static assets
```

## Run locally

From the repository root:

```sh
go run ./examples/todo/cmd/server
```

Then open <http://localhost:8080>. The server stores data in `todo.db` by default. Override with:

```sh
DATABASE_URL=/tmp/todo.db PORT=3000 go run ./examples/todo/cmd/server
```

## Try the app

1. Visit `/signup` and create an account.
2. Add todos from `/todos`.
3. Toggle completion, filter by all/open/done, edit via `/todos/:id`, and delete.
4. Visit `/settings` for a protected account page.
5. Log out and log back in; the session is preserved with a secure cookie flow in production.

## Database

Development uses GORM auto-migration for convenience. The equivalent SQL is in `db/migrations/` for production-style deployments:

```sh
goflex db migrate --dir examples/todo/db/migrations --dsn todo.db --driver sqlite
```

## Tests

```sh
go test ./examples/todo/...
go test -tags=e2e ./examples/todo/...
```

The e2e lane starts a fresh SQLite database per test and exercises signup/login, CRUD, validation, CSRF, optimistic rollback behavior, protected routes, production-mode server behavior, and framework package coverage.

## Production build

The framework production builder can compile a single deployable binary:

```sh
goflex build --out ./bin/todo
PORT=8080 DATABASE_URL=todo.db GOFLEX_ENV=prod ./bin/todo
```

The binary embeds fingerprinted frontend assets and serves `/api/healthz` for health checks.
