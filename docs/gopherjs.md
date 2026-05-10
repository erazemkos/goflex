# GopherJS build pipeline

GoFlex step 03 compiles a frontend Go package into `dist/app.js` by invoking the `gopherjs` binary:

```sh
gopherjs build -o dist/app.js <entry-package>
```

Install GopherJS with:

```sh
go install github.com/gopherjs/gopherjs@latest
```

## Version constraints

The current released GopherJS line is `1.20.x` and requires a Go `1.20.x` toolchain/standard library for frontend compilation. The GoFlex framework module itself targets newer Go, but `internal/build.Build` fails fast with `unsupported Go version` before invoking GopherJS if the active `go version` is incompatible.

If you see this error, install/use a Go 1.20 toolchain for the frontend build lane, then rerun `goflex build`.

## Hello example

`examples/hello/index.html` loads React and ReactDOM UMD globals from unpkg. `examples/hello/main.go` imports `github.com/gopherjs/gopherjs/js` and calls `React.createElement`/`ReactDOM.createRoot(...).render(...)` directly. Later roadmap steps replace this direct interop with the GoFlex UI DSL.
