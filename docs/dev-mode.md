# Dev Mode

`goflex dev` starts a development server with file watching, rebuilds, and browser live reload.

## Architecture

```text
fsnotify -> debounce changes -> classify
                         ├─ server Go files  -> go build + backend restart
                         ├─ frontend Go/html -> GopherJS rebuild + SSE reload
                         ├─ CSS / Go classes -> Tailwind rebuild + SSE css
                         └─ shared DTOs      -> API codegen, then server/frontend work
```

The watcher ignores noisy paths such as `.git/`, `node_modules/`, `dist/`, `vendor/`, `.goflex/`, temporary files, and `*.generated.go` outputs.

## Dev endpoints

The dev server exposes:

- `/_goflex/events` — Server-Sent Events stream emitting `reload`, `css`, `error`, and `ok`.
- `/_goflex/error.json` — current structured build error, if any.
- `/_goflex/runtime.js` — browser runtime injected into `index.html`.
- `/_goflex/status.json` — debug counters for reloads, CSS rebuilds, and backend restarts.

`goflex dev --addr :0` picks a free port and prints the reachable URL.

## Error overlay

Build failures are parsed from compiler-style lines like:

```text
file.go:12:4: undefined: Foo
```

The browser receives an `error` SSE event and renders a fixed overlay with the file, line, column, and raw compiler output. When the build succeeds, an `ok` event clears the overlay.

## Reload and state persistence

The runtime uses SSE for one-way reload messages. For the Fast Refresh fallback, it snapshots form values and scroll position to `sessionStorage` immediately before a full reload, then restores them on page load. Entries expire after 30 seconds.

CSS-only changes emit `css` and the runtime cache-busts stylesheet links without a full page reload.

## Troubleshooting

- **Stale CSS:** delete `dist/app.css` and restart `goflex dev`; the Tailwind pipeline rebuilds it on the next CSS/Go change.
- **Port collisions:** use `goflex dev --addr :0` to select a free port.
- **GopherJS crashes or version mismatch:** run `gopherjs version` and ensure it matches the Go toolchain expected by the project.
- **No reload:** check that changed files are not under ignored directories and that browser devtools shows an open `/_goflex/events` connection.
