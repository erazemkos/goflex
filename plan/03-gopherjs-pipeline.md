# Step 03 — GopherJS Build Pipeline

## Goal

Wire GopherJS into the framework so that a Go package can be compiled into a
JavaScript bundle that loads in the browser, interops with a globally-loaded
React, and runs a trivial "hello world" render. This step does **not** yet
build the component DSL — it only proves the compile/serve/load path works
end-to-end.

## Deliverables

1. `internal/build/` package that wraps the `gopherjs` binary.
   - `func Build(ctx context.Context, opts Options) (Artifacts, error)`
   - Options include: entry package path, output directory, minify flag,
     source map flag.
2. CLI integration: `goflex build` invokes `internal/build.Build` (even if
   only partially, at this stage).
3. A minimal demo under `examples/hello/` containing:
   - `main.go` (entrypoint compiled by GopherJS).
   - `index.html` loading React, ReactDOM, and `app.js`.
4. A static file server (reuse `net/http` for now, Gin comes in step 07) that
   serves the `index.html` and `app.js` for manual verification.
5. Automatic detection of the GopherJS binary with a clear error message if
   it isn't installed, including the exact `go install` command to run.

## Implementation notes

- GopherJS cannot always target the latest Go version. Document the supported
  Go version in `docs/gopherjs.md` and check it at runtime; fail fast with a
  helpful error if the user's Go version is incompatible.
- Invocation model: spawn `gopherjs build -o <out>/app.js <entry>` via
  `os/exec`. Capture stdout/stderr. Return both as part of the `Artifacts`
  struct for test assertions.
- React is loaded as a global from a CDN (or from `node_modules` later). At
  this step, the demo `index.html` uses `unpkg.com` UMD builds:
  ```html
  <script src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  ```
- The Go entry package uses `github.com/gopherjs/gopherjs/js` directly to
  call `React.createElement` and `ReactDOM.createRoot().render(...)`.
- Provide a `BuildResult` with fields: `JSPath`, `MapPath`, `SizeBytes`,
  `DurationMS`, `Stderr`.

## Example `examples/hello/main.go` (target)

```go
package main

import "github.com/gopherjs/gopherjs/js"

func main() {
	react := js.Global.Get("React")
	reactDOM := js.Global.Get("ReactDOM")

	el := react.Call("createElement", "h1", nil, "Hello from Go")

	root := reactDOM.Call("createRoot", js.Global.Get("document").
		Call("getElementById", "root"))
	root.Call("render", el)
}
```

## Testing scenarios

### T03.1 — GopherJS detection

- When GopherJS is missing from `$PATH`, `internal/build.Build` returns an
  error whose message contains `go install github.com/gopherjs/gopherjs@...`.
- Tested by running the function with a `PATH` that excludes `gopherjs`.

### T03.2 — Hello world compiles

- Running `internal/build.Build` against `examples/hello` with a temp
  output dir produces a non-empty `app.js` file.
- File size is > 1 KB (sanity bound).
- Exit status is 0 and no lines in stderr match the regex `(?i)error`.

### T03.3 — CLI build command works

- `goflex build --out <tmp>` from `examples/hello` produces the same
  artifacts as T03.2.
- Asserted by an integration test under `internal/build/build_integration_test.go`
  gated with a `//go:build integration` tag so CI can run it separately.

### T03.4 — Bundle loads and renders in a headless browser

- Using `chromedp` (preferred) or `rod`:
  1. Start a test HTTP server serving `examples/hello/index.html` + built
     `app.js`.
  2. Navigate to the page.
  3. Wait for the `<h1>` with text `"Hello from Go"` to appear.
  4. Assert it renders within 5 seconds.
- Gated with `//go:build e2e`.

### T03.5 — Source map generation

- `Build(..., Options{SourceMap: true})` produces a `.map` file alongside
  `app.js` and the map file's JSON has a `"sources"` array referencing the
  Go source files.

### T03.6 — Incompatible Go version warning

- With a mocked `go version` returning an unsupported version, `Build`
  returns an error containing `"unsupported Go version"` and does not
  attempt to invoke GopherJS.

### T03.7 — Build performance budget (soft)

- Warm rebuild of the `hello` example is < 5 seconds on CI.
- This is a soft assertion — failing it logs a warning but does not fail CI.

## Acceptance criteria

- All T03.1–T03.6 pass in CI; T03.7 emits a warning only.
- Running `goflex build` from `examples/hello` succeeds.
- Running the headless browser test confirms the Go-compiled React render
  actually shows in a real browser.
- Documentation in `docs/gopherjs.md` explains version constraints and how
  to install GopherJS.
