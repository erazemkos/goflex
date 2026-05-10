# Step 08 — Shared DTOs and Typed API Client Codegen

## Goal

Make shared Go types the single source of truth for HTTP API contracts. The
framework should:

- Let developers declare endpoints with typed request/response Go structs.
- Generate a typed Go client the frontend uses to call them.
- Generate the Gin handler registration boilerplate on the backend.

This is the feature that most dramatically reduces boilerplate compared to a
classic REST setup.

## Deliverables

1. `pkg/api/spec.go` — endpoint declaration types:
   ```go
   type Endpoint[Req, Res any] struct {
       Method  string
       Path    string
       Handler func(ctx Context, req Req) (Res, error)
   }
   ```
2. `pkg/api/registry.go` — central registry where both client and server
   generators look up endpoints.
3. `internal/gen/apigen.go` — code generator invoked by `goflex generate`:
   - Input: all `Endpoint` declarations in the user's module.
   - Output:
     - `gen_server.go` (server-side registration helpers).
     - `gen_client.go` (frontend-side typed client).
4. CLI: `goflex generate --only api` runs only the API generator.
5. Runtime client in `pkg/apiclient/` handling: JSON encoding, path param
   substitution, query string building, error envelope parsing.
6. A shared `errors` mapping so a server-side `httperr.Error` roundtrips to
   a typed Go error on the client.

## Implementation notes

### Endpoint declaration style

```go
// shared/endpoints.go
package shared

import "github.com/goflex/goflex/pkg/api"

var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
    Method: "POST",
    Path:   "/todos",
}

var ListTodos = api.Endpoint[struct{}, []Todo]{
    Method: "GET",
    Path:   "/todos",
}

var GetTodo = api.Endpoint[struct{ ID uint `path:"id"` }, Todo]{
    Method: "GET",
    Path:   "/todos/:id",
}
```

### Path and query tags

Request struct fields can have tags:
- `path:"id"` — substituted into the URL path.
- `query:"q"` — URL-encoded into query string.
- Default / untagged — goes into the JSON body (only for bodyful methods).

### Server registration

The handler is attached separately so `shared` can be imported by both sides
without pulling server-only code:

```go
// internal/api/todos.go (server only)
func init() {
    shared.CreateTodo.Register(func(ctx api.Context, req shared.CreateTodoRequest) (shared.Todo, error) {
        // validate, save, return
    })
}
```

`goflex generate` walks the `init()` registrations and emits:

```go
// gen_server.go
func RegisterRoutes(r *gin.Engine) {
    r.POST("/api/todos", wrap(shared.CreateTodo))
    r.GET ("/api/todos", wrap(shared.ListTodos))
    r.GET ("/api/todos/:id", wrap(shared.GetTodo))
}
```

### Generated client

```go
// gen_client.go (frontend)
func CreateTodo(ctx context.Context, req shared.CreateTodoRequest) (shared.Todo, error) {
    return apiclient.Call[shared.CreateTodoRequest, shared.Todo](ctx, shared.CreateTodo, req)
}
```

Users call it ergonomically:

```go
todo, err := api.CreateTodo(ctx, shared.CreateTodoRequest{Title: "Buy milk"})
```

### Error mapping

`httperr.Error{Code: "validation_failed", ...}` becomes a typed Go error on
the client:

```go
var ErrValidation = apiclient.Code("validation_failed")

if errors.Is(err, ErrValidation) {
    // show field-level errors from err.(apiclient.FieldError).Fields
}
```

## Testing scenarios

### T08.1 — Endpoint registration

- An `Endpoint` with method and path is stored in the registry and can be
  retrieved by `api.Registry.All()`.
- Duplicate `(method, path)` registration panics at startup with a clear
  message.

### T08.2 — Path param substitution

- Given `GetTodo` with path `/todos/:id` and request `{ID: 42}`, the client
  builds URL `/api/todos/42`.
- Unencoded special chars (`"/"`, `" "`) in a path param are URL-escaped.

### T08.3 — Query param encoding

- Request with `Q string `query:"q"`; Limit int `query:"limit"`` and
  `{Q: "hello world", Limit: 10}` produces `?q=hello+world&limit=10`.

### T08.4 — JSON body for POST

- `CreateTodo` with body `{Title: "x"}` sends `Content-Type: application/json`
  and body `{"title":"x"}`.

### T08.5 — Response decoding

- A server returning `{"id":1,"title":"x","done":false}` decodes into
  `shared.Todo`.
- Missing/extra fields are handled per standard `encoding/json` rules.

### T08.6 — Error decoding

- A 422 response with `{"code":"validation_failed","fields":{"title":"required"}}`
  decodes into a `FieldError` with `Fields["title"]=="required"`.
- `errors.Is(err, apiclient.Code("validation_failed"))` is true.

### T08.7 — Server wrapper validates method

- Sending a `GET` to a `POST`-registered endpoint returns 405.

### T08.8 — Codegen output compiles

- The generator runs on the `examples/todo` module and produces
  `gen_server.go` and `gen_client.go`.
- `go build ./...` on the resulting tree succeeds.
- Test script `scripts/test-codegen.sh` performs this in a temp dir.

### T08.9 — Codegen is deterministic

- Running the generator twice on unchanged input produces byte-identical
  output. Asserted via SHA comparison in a test.

### T08.10 — Round-trip integration test

- Start an in-process server with `CreateTodo` and `ListTodos` registered.
- From Go (not the browser), call the generated client functions against
  the server.
- Assert the full roundtrip: creating two todos then listing them returns
  both, in order.

### T08.11 — Endpoints imported by both sides

- Under `go vet` and `go build`, the `shared` package can be imported by
  both `cmd/web` (GopherJS target) and `cmd/server` (regular Go).
- The shared package must not import Gin, GORM, `net/http.Server`, or other
  server-only packages.
- A lint test walks imports and fails if a server-only package is pulled
  into `shared`.

### T08.12 — Generator change detection

- If the user adds a new `Endpoint`, running `goflex generate` updates the
  generated files.
- Running it a second time reports `"no changes"` and exits 0.

## Acceptance criteria

- All T08.1–T08.12 pass in CI.
- Generated client has a docstring on each function matching the endpoint
  description.
- Developer can declare a new endpoint in one place and have both server
  registration and client wrapper appear after `goflex generate`.
