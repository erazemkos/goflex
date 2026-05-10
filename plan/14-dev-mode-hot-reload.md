# Step 14 — Dev Mode and Hot Reload

## Goal

`goflex dev` should feel instant: edit a Go file, see the change in the
browser in ~1 second without losing most component state, with readable
error overlays when the build fails.

## Deliverables

1. `internal/devserver/devserver.go` orchestrating:
   - Backend Go build + restart (on server-side file changes).
   - GopherJS rebuild (on frontend file changes).
   - Tailwind watch.
   - Live-reload signaling to the browser.
2. File watcher using `github.com/fsnotify/fsnotify` with smart ignore rules
   (`.git`, `node_modules`, `dist/`, `*.generated.go` unless input changed).
3. Browser reload transport — Server-Sent Events (simpler than WebSockets
   for one-way signals).
4. Error overlay injected into the page when a build fails — renders the
   error in the browser instead of leaving stale JS.
5. React Fast Refresh integration (via `react-refresh`), or — if too
   complex — a reliable "full reload with state persistence" fallback that
   snapshots `UseState` values to `sessionStorage`.

## Implementation notes

### Architecture

```text
fsnotify → classify change
            ├── server file? → rebuild Go binary, graceful restart
            ├── frontend file? → invoke GopherJS, send SSE "reload"
            ├── CSS file / Go class change? → rebuild Tailwind, send SSE "css"
            └── shared DTO → run codegen, then both of the above
```

### Dev endpoints

The dev server exposes:

- `/_goflex/events` (SSE): emits `reload`, `css`, `error`, `ok`.
- `/_goflex/error.json`: last build error details.
- `/_goflex/runtime.js`: tiny JS client subscribing to SSE and performing
  reloads / overlay rendering.

`goflex dev` injects a `<script src="/_goflex/runtime.js">` tag into the
served `index.html`.

### Error overlay

When a build fails:

1. Server captures GopherJS stderr and parses lines like
   `file.go:12:4: undefined: Foo`.
2. Sends an `error` event with the structured error.
3. The runtime client renders a fixed overlay `<div>` showing the file,
   line, column, and message; clicking the location opens it in the
   editor via `window.open("vscode://file/...")` (configurable).

### State persistence fallback

If we can't ship React Fast Refresh cleanly through GopherJS:

- Before reload, the runtime serializes `UseState` values (keyed by
  component path) into `sessionStorage`.
- On fresh load, `UseState` first checks `sessionStorage` for a matching
  key and seeds from it.
- Stale entries expire after 30s or on explicit Cmd+R.

### Performance targets

- Go server rebuild + restart: < 500ms for small changes.
- GopherJS incremental rebuild: < 1500ms for small changes.
- Browser reload time from save to visible update: < 2000ms typical.

## Testing scenarios

Some tests here are integration-level and run under `//go:build integration`.

### T14.1 — Watcher ignores noise

- Changing a file under `.git/` or `node_modules/` does not trigger a
  rebuild.

### T14.2 — Backend restart on server file change

- Start dev server; write a file in `internal/api/`.
- Within 2s, the server reports a restart and serves updated behavior.
- Measured by hitting an endpoint that returns a string that was edited.

### T14.3 — Frontend rebuild on .go change

- Start dev server; edit a component file.
- Within 3s the `app.js` served at `/app.js` contains the new text.

### T14.4 — SSE reload event

- A connected SSE client receives a `reload` event within 3s of a frontend
  file change.

### T14.5 — Error overlay

- Introduce a syntax error in a component file.
- `/_goflex/error.json` returns a structured error.
- SSE emits `error` event with the same payload.
- Fixing the error emits `ok`.

### T14.6 — State persistence across reload

- Load a counter page, click increment 3 times (count=3).
- Edit the component's markup only (no state shape change).
- After auto-reload, counter shows 3 (state restored from sessionStorage).
- Gated `//go:build e2e`.

### T14.7 — Port handling

- `goflex dev --addr :0` chooses a free port and prints it.
- The printed URL is reachable.

### T14.8 — Graceful restart

- When the server restarts, any open SSE connections are re-established
  automatically by the runtime client.

### T14.9 — Tailwind watch

- Add a new class in a component file → CSS update arrives as a `css` SSE
  event within 3s.
- Verified by fetching `/dist/app.css` before and after.

### T14.10 — Multiple simultaneous changes

- Rapidly edit three different files within 200ms:
  - Only one rebuild cycle runs (debounce).
  - The final state reflects all three edits.

### T14.11 — Clean shutdown

- SIGINT on `goflex dev` exits within 2s and terminates all child
  processes (GopherJS, Tailwind, dev server).

## Acceptance criteria

- All T14.1–T14.5, T14.7–T14.11 pass in CI (integration lane).
- T14.6 passes in the `e2e` job.
- Measured median end-to-end reload time is < 2s on CI hardware.
- `docs/dev-mode.md` documents architecture, perf expectations, and
  troubleshooting tips (stale cache, port collisions, GopherJS crashes).
