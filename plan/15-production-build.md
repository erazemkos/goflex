# Step 15 — Production Build and Single Binary

## Goal

`goflex build` produces a single statically-linked Go binary that serves both
the API and the compiled frontend assets, with cache-friendly fingerprints,
minified JS/CSS, and reproducible output.

## Deliverables

1. `internal/build/production.go` orchestrating:
   - GopherJS build with `--minify`.
   - Tailwind build in production mode (purged + minified).
   - Fingerprinting of `app.js`, `app.css`, and any `assets/*` files.
   - `index.html` template rendering with the right hashed asset URLs.
   - Embedding the dist tree into the Go binary via generated
     `dist_embed.go`.
2. `pkg/server.Static(fs.FS)` reads the embedded FS and serves:
   - Fingerprinted files with `Cache-Control: public, max-age=31536000, immutable`.
   - `index.html` with `Cache-Control: no-cache`.
3. CLI: `goflex build --out ./bin/app`:
   - Compiles frontend → `dist/`.
   - Generates `dist_embed.go`.
   - Runs `go build -ldflags "-X pkg/version.buildVersion=$GIT_SHA" -o ./bin/app`.
4. Reproducibility:
   - `SOURCE_DATE_EPOCH` respected for timestamps.
   - Go `-trimpath` set by default.
   - Asset fingerprints are content-hash based (SHA-256 truncated).
5. Optional release helpers:
   - `goflex build --target linux/amd64,linux/arm64,darwin/arm64` cross-compiles
     multiple binaries.

## Implementation notes

### Fingerprinting

- Compute the SHA-256 of each asset.
- Rename `app.js` → `app.<8-hex>.js`.
- Write a manifest `dist/manifest.json` mapping logical → hashed names.
- `index.html` template uses manifest lookups so URLs are correct.

### `index.html`

```html
<!doctype html>
<html>
  <head>
    <link rel="stylesheet" href="/dist/app.{{.CSSHash}}.css">
  </head>
  <body>
    <div id="root"></div>
    <script src="/dist/app.{{.JSHash}}.js"></script>
  </body>
</html>
```

### Embedding

`dist_embed.go` is generated:

```go
//go:embed dist
var distFS embed.FS

func DistFS() fs.FS { return distFS }
```

It's placed in the app's `internal/web/` and imported by the server code.

### Compression

- Emit `app.<hash>.js.gz` and `app.<hash>.js.br` alongside uncompressed
  versions.
- `pkg/server.Static` serves the pre-compressed file when the client's
  `Accept-Encoding` allows.
- This is a big win for time-to-interactive.

## Testing scenarios

### T15.1 — Single binary runs

- After `goflex build`, running the produced binary:
  - Listens on the configured port.
  - Serves `GET /` and the JS/CSS referenced from it returns 200.

### T15.2 — Assets are fingerprinted

- `dist/` contains `app.<hash>.js` and `app.<hash>.css` with matching
  entries in `manifest.json`.
- `index.html` references the hashed filenames.

### T15.3 — Hash stability

- Two builds of identical source produce identical fingerprints.
- Modifying one class triggers a change in the CSS hash but not the JS hash
  (and vice versa) — asserted by diffing two build manifests.

### T15.4 — Cache headers

- `GET /dist/app.<hash>.js` returns `Cache-Control: public, max-age=31536000, immutable`.
- `GET /` returns `Cache-Control: no-cache`.

### T15.5 — Precompressed serving

- With `Accept-Encoding: br`, response includes `Content-Encoding: br` and
  the body is the precompressed Brotli file.
- With `Accept-Encoding: gzip`, same for `gzip`.
- Without compression support, uncompressed file is served.

### T15.6 — Embedded FS contains expected files

- A unit test walks `DistFS()` and asserts presence of `index.html`,
  `manifest.json`, hashed JS and CSS.

### T15.7 — Binary size budget (soft)

- Built binary size is logged; CI warns if > 25 MB for the hello example.
- Soft assertion only — regressions are visible, not blocking.

### T15.8 — Reproducible builds

- Two builds with the same `SOURCE_DATE_EPOCH`, `-trimpath`, and source
  produce byte-identical binaries on the same OS/arch.
- Verified via SHA-256 of the output in CI.

### T15.9 — Cross-compile target

- `goflex build --target linux/arm64` produces a binary reporting
  `GOOS=linux GOARCH=arm64` when inspected via `file` or `go version -m`.

### T15.10 — Prod mode defaults

- The binary runs with `Env="prod"` by default.
- Cookies are `Secure`, AutoMigrate refuses to run, debug endpoints
  (`/_goflex/*`) are not registered.

### T15.11 — Prod smoke via HTTP

- Start the built binary in a test; run a small HTTP scenario:
  - `GET /` → 200 with HTML referencing hashed JS.
  - `GET /dist/app.<hash>.js` → 200 with cache headers.
  - `GET /api/healthz` → 200.
  - `GET /something/random` with `Accept: text/html` → 200 (SPA fallback).

## Acceptance criteria

- All T15.1–T15.6, T15.8, T15.10, T15.11 pass in CI.
- T15.7 is a warning check.
- T15.9 passes when the target toolchain is available.
- `docs/deploy.md` explains how to deploy the built binary (systemd
  example, Docker example, env var list).
- A hello app built by `goflex build` is ~ready to deploy by copying the
  binary and setting `PORT` and `DATABASE_URL`.
