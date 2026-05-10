# Server

`pkg/server` provides the Gin-backed GoFlex runtime.

```go
s := server.New(server.Config{
    Env:         "prod",
    StaticFS:    os.DirFS("dist"),
    APIPrefix:   "/api",          // default
    IndexPath:   "index.html",    // default
    CORSOrigins: []string{"https://app.example.com"},
})
s.API("", func(r *gin.RouterGroup) {
    r.GET("/todos", listTodos)
})
log.Fatal(s.Run(":8080"))
```

## Config

- `Env`: `"dev"` or `"prod"`; controls Gin mode and default log level.
- `StaticFS`: frontend bundle filesystem, usually `os.DirFS("dist")` in dev or an embedded `fs.FS` in prod.
- `IndexPath`: SPA entry point, default `index.html`.
- `APIPrefix`: JSON API prefix, default `/api`.
- `CORSOrigins`: explicit cross-origin allow-list. Empty means same-origin only.
- `TrustedProxies`: forwarded-header proxy allow-list passed to Gin.
- `Logger`: optional `*slog.Logger`; defaults to `pkg/log`.

## Routing and SPA fallback

`API(prefix, fn)` registers Gin routes. Unknown paths under `APIPrefix` return a JSON `not_found` envelope.

For non-API `GET`/`HEAD` requests, the server first tries `StaticFS`. If no asset exists and the `Accept` header includes `text/html`, it serves `IndexPath` with `Cache-Control: no-cache`, enabling client-side router deep links such as `/todos/5`. Asset-like missing paths (`.js`, `.css`, `.map`, images, fonts, etc.) return 404 instead of HTML.

Hash-named assets receive `Cache-Control: public, max-age=31536000, immutable`; other files are served with `no-cache`.

## CORS

Only origins in `CORSOrigins` receive `Access-Control-Allow-Origin`. Preflight `OPTIONS` requests from other origins complete without CORS allow headers.

## Health and shutdown

`GET /healthz` returns:

```json
{"status":"ok","version":"..."}
```

`Run` listens for `SIGINT`/`SIGTERM`, marks the server as draining, and allows up to 10 seconds for in-flight requests to complete. While draining, `/healthz` returns 503 so a load balancer can remove the instance.

## Errors and request IDs

Every request has an `X-Request-Id` response header. Incoming IDs are preserved; missing IDs are generated. Logs include the request ID.

Use `pkg/httperr` for JSON errors:

```go
httperr.Write(c, 422, httperr.New("validation_failed", "bad input", map[string]string{
    "title": "required",
}))
```

Response:

```json
{"code":"validation_failed","message":"bad input","fields":{"title":"required"},"request_id":"..."}
```
