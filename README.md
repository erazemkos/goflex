# GoFlex

GoFlex is a Reflex-like full-stack web framework for Go: write your UI, shared API contracts, backend handlers, database code, and production server in one Go workspace — with a more scalable package-first architecture built around typed boundaries, generated clients, and ordinary Go tooling.

> Status: experimental v0 scaffold. The framework and todo example are intended as a working reference for the roadmap in [`plan/`](plan/).

## Installation

### Prerequisites

- Go 1.23+ for the framework and backend tooling.
- GopherJS for frontend compilation. Current GopherJS releases require a compatible Go 1.20 toolchain for the frontend build lane; see [`docs/gopherjs.md`](docs/gopherjs.md).
- `golangci-lint` for the full lint target.

### Install the CLI from this checkout

```sh
git clone git@github.com:erazemkos/goflex.git
cd goflex
go install ./cmd/goflex
```

Verify:

```sh
goflex version
goflex --help
```

### Validate the repository

```sh
go test ./...
go test ./... -race -cover
go test -tags=e2e ./...
go vet ./...
go build ./...
golangci-lint run
make e2e
```

## Quick start

Run the reference todo server:

```sh
go run ./examples/todo/cmd/server
```

Then open <http://localhost:8080>.

Build a production binary:

```sh
goflex build --out ./bin/app
PORT=8080 DATABASE_URL=todo.db GOFLEX_ENV=prod ./bin/app
```

## What GoFlex provides

### One language across the stack

GoFlex is designed so teams can build full-stack applications with Go-first primitives:

- frontend component trees in Go via `pkg/ui`
- hooks-style state via `pkg/hooks`
- routing via `pkg/router`
- typed API contracts via `pkg/api`
- generated API clients via `pkg/apiclient`
- query caching and optimistic mutations via `pkg/query`
- shared form validation via `pkg/form`
- auth, sessions, CSRF, and protected routes via `pkg/auth`
- database setup, migrations, and seeds via `pkg/db`
- production HTTP serving via `pkg/server`

### Typed shared contracts

Shared endpoint declarations live beside DTOs:

```go
var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
    Method: "POST",
    Path:   "/todos",
}
```

`goflex generate --only api` discovers these endpoints and emits deterministic typed client/server helpers under `generated/`.

### Backend runtime

`pkg/server` wraps Gin with framework defaults:

- `/healthz` and `/api/healthz`
- request IDs
- structured request logging
- CORS allow-lists
- graceful shutdown
- SPA fallback
- embedded/static asset serving
- immutable cache headers for fingerprinted files
- precompressed Brotli/gzip asset negotiation

### Frontend runtime

The frontend layer includes:

- a small UI element DSL
- component composition helpers
- hooks-like state/effect/memo/ref primitives
- client-side router with params and links
- query cache with stale-while-revalidate behavior
- mutations with optimistic updates and rollback
- form binding helpers with accessible error rendering

### Styling without Node project setup

GoFlex integrates Tailwind using the standalone Tailwind binary:

- downloads and verifies the binary once
- scans Go files for `ui.Class(...)`, `ui.ClassIf(...)`, `ui.ClassMap(...)`, and `ui.Tw(...)`
- emits `dist/app.css`
- supports plain `assets/` CSS/static files as an alternative

See [`docs/styling.md`](docs/styling.md).

### Dev mode and hot reload

`goflex dev` provides a development server with:

- recursive file watching via `fsnotify`
- smart ignore rules
- frontend rebuilds
- Tailwind rebuilds
- backend rebuild/restart hooks
- Server-Sent Events live reload
- browser error overlay
- state-preserving full reload fallback

See [`docs/dev-mode.md`](docs/dev-mode.md).

### Production builds

`goflex build` produces a single deployable binary:

- GopherJS frontend build
- minified Tailwind CSS
- content-hashed `app.<hash>.js` and `app.<hash>.css`
- copied/fingerprinted `assets/*`
- `manifest.json`
- embedded `dist/` tree
- static binary build with `-trimpath`
- optional cross-compilation with `--target`

See [`docs/deploy.md`](docs/deploy.md).

## Reference app

[`examples/todo`](examples/todo) is the end-to-end reference application. It covers:

- signup, login, logout, and session refresh
- CSRF-protected mutations
- todo list/create/edit/toggle/delete
- optimistic toggle updates with rollback
- validation on client-shaped and server paths
- auth-guarded routes
- GORM models and SQL migrations
- Tailwind-styled UI
- e2e tests against fresh SQLite databases
- production-mode smoke tests

Run it with:

```sh
go test -tags=e2e ./examples/todo/...
make e2e
```

## Repository layout

```text
cmd/goflex                 CLI entrypoint
internal/build             frontend, CSS, assets, production binary pipeline
internal/cli               Cobra commands
internal/devserver         dev server, watcher, SSE hot reload
internal/gen               deterministic API code generator
pkg/api                    shared endpoint contracts
pkg/apiclient              typed HTTP client
pkg/auth                   sessions, login/logout, CSRF, frontend auth helpers
pkg/db                     DB opening, migrations, seeds
pkg/form                   forms and validation
pkg/hooks                  frontend hooks runtime
pkg/query                  query cache and mutations
pkg/router                 client router
pkg/server                 Gin production runtime
pkg/ui                     UI element DSL and Tailwind class helpers
examples/todo              reference full-stack app
plan                       roadmap and acceptance criteria
docs                       feature documentation
```

## How it works

1. Define shared DTOs and endpoints in a `shared` package.
2. Implement backend handlers that return shared DTOs rather than internal models.
3. Generate typed API helpers with `goflex generate`.
4. Build frontend UI in Go with `ui`, `hooks`, `router`, `query`, and `form`.
5. Use `goflex dev` for hot reload during development.
6. Use `goflex build` to compile a single production binary with embedded assets.
7. Deploy the binary behind your process manager or container runtime.

## Future additions

Potential next improvements:

- Real browser-backed React Fast Refresh instead of the current state-preserving reload fallback.
- First-class project scaffolding in `goflex new` based on the todo app structure.
- More complete Tailwind merge coverage generated from Tailwind metadata.
- Component library primitives for common layouts, dialogs, tables, and navigation.
- Built-in OpenAPI export from shared endpoint declarations.
- More database adapters and migration ergonomics.
- Role/permission helpers on top of `pkg/auth`.
- Deployment templates for Fly.io, Render, Railway, Kubernetes, and systemd.
- Devtools UI for query cache, routes, logs, and build errors.
- Full chromedp browser tests for visual/state-persistence scenarios.
- Pluggable frontend compilers once alternatives to GopherJS are mature.
