# GoFlex — Overview and Roadmap

## What is GoFlex?

GoFlex is a fullstack Go web framework inspired by Reflex (Python), but with a
stateless backend and a Go-compiled-to-JavaScript frontend.

Instead of Reflex's WebSocket-driven, server-owned state model, GoFlex uses:

```text
Frontend: Go → GopherJS → JavaScript → React runtime
Backend : Go + Gin (stateless HTTP JSON APIs)
DB      : GORM + migrations
Shared  : Go DTOs/types used by both sides
Tooling : `goflex` CLI (new/dev/build/generate/db)
```

The developer writes a single Go codebase and gets:

- A React single-page application in the browser.
- A stateless Go HTTP API served by Gin.
- A typed API client generated from shared Go types.
- A single deployable Go binary serving the API and the frontend bundle.

## Guiding principles

1. **One language end-to-end.** All app code is Go. TypeScript/JS usage is
   minimal and generated.
2. **Stateless backend.** No server-owned UI state. No mandatory WebSockets.
   Horizontal scaling should "just work".
3. **Typed contracts.** Shared Go structs are the source of truth for the API.
4. **Developer velocity first.** The CLI and generators must make common tasks
   trivial; boilerplate must be generated, not written by hand.
5. **Escape hatches.** Raw React interop, raw Gin handlers, and raw GORM access
   must always be possible.
6. **Security boundaries.** GORM models are never exposed directly. DTOs are
   always explicit.

## High-level architecture

```text
┌──────────────────────────────────────────────────────────┐
│                        Browser                           │
│  ┌────────────────────────────────────────────────────┐  │
│  │ React runtime (loaded as a JS lib)                 │  │
│  │ GopherJS-compiled Go bundle                        │  │
│  │   - UI component DSL                               │  │
│  │   - Hooks (UseState/UseEffect/UseQuery/...)        │  │
│  │   - Router                                         │  │
│  │   - Generated typed API client                     │  │
│  └────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘
                     │  HTTP/JSON
                     ▼
┌──────────────────────────────────────────────────────────┐
│                   Go binary (Gin)                        │
│  - /api/* JSON handlers (generated + custom)             │
│  - /* static frontend assets (embed.FS in production)    │
│  - Auth middleware, CSRF, sessions                       │
│  - GORM + migrations                                     │
└──────────────────────────────────────────────────────────┘
```

## Repository layout (target)

```text
goflex/
├── cmd/
│   └── goflex/                 # CLI entry point
├── pkg/
│   ├── ui/                     # Go component DSL over React
│   ├── hooks/                  # React hook wrappers
│   ├── router/                 # Frontend router
│   ├── api/                    # Typed API client runtime
│   ├── query/                  # Query/cache layer
│   ├── form/                   # Forms and validation
│   ├── auth/                   # Auth primitives
│   └── server/                 # Gin integration, static serving
├── internal/
│   ├── build/                  # GopherJS build pipeline
│   ├── gen/                    # Code generators
│   └── devserver/              # Dev mode + live reload
├── templates/
│   └── new-app/                # `goflex new` template
├── examples/
│   └── todo/                   # Reference end-to-end app
├── plan/                       # This directory
└── go.mod
```

## Phased plan (one file per step)

Each step lives in its own `NN-*.md` file with goals, deliverables,
implementation notes, and explicit **testing scenarios**. The test scenarios
are written so they can be translated directly into Go tests (table-driven
where possible) or integration scripts.

| Step | File | Focus |
|-----:|------|-------|
|  01 | `01-project-bootstrap.md` | Repo, go.mod, baseline CI, linting |
|  02 | `02-cli-skeleton.md` | `goflex` CLI: new/dev/build stubs |
|  03 | `03-gopherjs-pipeline.md` | Compile Go → JS, bundle, serve |
|  04 | `04-ui-component-dsl.md` | Element tree, React interop |
|  05 | `05-hooks-layer.md` | UseState/UseEffect/UseMemo/UseRef |
|  06 | `06-frontend-router.md` | Client-side routes, params, links |
|  07 | `07-backend-gin-integration.md` | Gin server, static assets, 404/SPA |
|  08 | `08-shared-dtos-and-codegen.md` | Shared types + typed client gen |
|  09 | `09-query-cache-layer.md` | UseQuery/UseMutation, cache |
|  10 | `10-forms-and-validation.md` | UseForm, field errors, submit |
|  11 | `11-auth-and-sessions.md` | Cookie sessions, CSRF, middleware |
|  12 | `12-gorm-and-migrations.md` | DB wiring, migration tool |
|  13 | `13-styling-tailwind.md` | Tailwind integration, class helpers |
|  14 | `14-dev-mode-hot-reload.md` | Watchers, live reload, error overlay |
|  15 | `15-production-build.md` | Embedded assets, single binary |
|  16 | `16-example-todo-app.md` | End-to-end reference app + E2E tests |

## Definition of done for the framework v0

- `goflex new myapp && cd myapp && goflex dev` runs a working app.
- Editing a Go component in the app updates the browser within ~1s.
- `goflex build` produces a single static binary.
- `examples/todo` passes a full E2E test (Playwright or chromedp).
- All pkg/* packages have >= 70% unit test coverage.
- CI is green on Linux and macOS.

## Non-goals for v0

- Server-side rendering (SSR).
- WebSocket-based live state (may come later as an optional module).
- Native mobile.
- Plugin system.
- Multi-tenant hosting platform.
