# Step 07 — Backend Gin Integration

## Goal

Provide the server-side runtime built on Gin. The server must serve the
compiled frontend bundle, expose JSON API routes, and correctly support SPA
client-side routing (unknown paths fall back to `index.html`).

## Deliverables

1. `pkg/server/server.go` — `New(cfg Config) *Server` with methods:
   - `Use(middleware ...gin.HandlerFunc)`
   - `API(prefix string, fn func(r *gin.RouterGroup))`
   - `Static(fs fs.FS)` — serves the frontend bundle.
   - `Run(addr string) error`
   - `Handler() http.Handler` — useful for tests.
2. SPA fallback: requests whose `Accept` header includes `text/html` and whose
   path is not an API route and does not match a static asset serve
   `index.html` (so deep links like `/todos/5` work on refresh).
3. Structured logging via `pkg/log`.
4. Graceful shutdown (SIGINT/SIGTERM → 10s drain).
5. CORS configuration in `Config` (default: same-origin only).
6. A `HealthHandler` exposed at `/healthz` returning `{"status":"ok","version":...}`.
7. A `pkg/httperr` helper for JSON error envelopes.

## Implementation notes

### Config

```go
type Config struct {
    Env          string // "dev" | "prod"
    StaticFS     fs.FS  // embedded bundle in prod; disk in dev
    IndexPath    string // default "index.html"
    APIPrefix    string // default "/api"
    CORSOrigins  []string
    TrustedProxies []string
    Logger       *slog.Logger
}
```

### SPA fallback algorithm

1. If the path starts with `APIPrefix`, let the API router handle it (404 if
   no match).
2. Else, try to serve from `StaticFS`:
   - If the file exists, serve it with appropriate `Content-Type` and
     long-cache headers for hashed assets.
3. Else, if the `Accept` header contains `text/html`, serve `IndexPath`
   (no-cache).
4. Else return 404.

### Error envelope

```go
type Error struct {
    Code    string            `json:"code"`    // e.g. "validation_failed"
    Message string            `json:"message"`
    Fields  map[string]string `json:"fields,omitempty"`
}
```

`httperr.Write(c, status, err)` writes the envelope and logs the error with
a request ID.

### Request ID middleware

Every request gets an `X-Request-Id` (generated if missing). It's attached
to the logger, the error envelope, and the response header.

## Testing scenarios

### T07.1 — Health endpoint

- `GET /healthz` returns 200 and body contains `"status":"ok"`.

### T07.2 — API routing

- A registered handler at `/api/todos` returns 200 with expected body.
- An unknown `/api/...` path returns 404 with the error envelope.

### T07.3 — Static asset serving

- Given a `StaticFS` containing `app.js` and `index.html`, `GET /app.js`
  returns the JS content with `Content-Type: application/javascript`.

### T07.4 — SPA fallback for deep links

- `GET /todos/5` with `Accept: text/html` returns the contents of
  `index.html` and status 200.
- `GET /todos/5` with `Accept: application/json` returns 404.

### T07.5 — No fallback for asset-like paths

- `GET /does-not-exist.js` returns 404 (we do not send HTML for requests
  that look like an asset by extension: `.js`, `.css`, `.map`, `.png`, etc.).

### T07.6 — CORS

- With `CORSOrigins=["https://example.com"]`:
  - A preflight from that origin returns the right `Access-Control-Allow-*`
    headers.
  - A preflight from another origin does not return `Access-Control-Allow-Origin`.

### T07.7 — Graceful shutdown

- Start the server, send a long-running request (handler sleeps 2s), then
  send SIGINT. Assert:
  - The in-flight request completes successfully.
  - New requests after shutdown begins receive 503 or connection refused.
  - The server exits within 10s.

### T07.8 — Request ID propagation

- A request with no `X-Request-Id` header gets a generated ID in the response.
- A request with a header value keeps it.
- The ID is visible in log output (capture via a test handler on `slog`).

### T07.9 — Error envelope

- A handler that calls `httperr.Write(c, 422, httperr.New("validation_failed", "bad input", map[string]string{"title":"required"}))`
  produces a JSON body matching a golden file.

### T07.10 — Load balance readiness

- Hitting `/healthz` while the server is draining returns 503 (so an LB can
  remove it from rotation).

### T07.11 — Integration with frontend bundle

- Run `goflex build` on `examples/hello`, embed the resulting directory into
  a test binary via `embed.FS`, serve it through `pkg/server`, and assert a
  real `GET /` returns the embedded `index.html`.

## Acceptance criteria

- All T07.1–T07.11 pass in CI.
- `pkg/server` coverage >= 80%.
- Documentation `docs/server.md` covers Config, SPA fallback, CORS,
  graceful shutdown, and error envelopes.
- A hello-world app can be served end-to-end: `goflex build && go run
  ./cmd/goflex-example-hello` → browser loads the React render.
