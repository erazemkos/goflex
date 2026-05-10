# Step 13 — Styling / Tailwind Integration

## Goal

Make styling ergonomic and consistent. The default path is Tailwind CSS —
integrated into the build system with no manual Node tooling required from
the user once the framework CLI is installed.

## Deliverables

1. `internal/build/tailwind.go` — wrapper around the Tailwind CLI (standalone
   binary download, no `node` dependency).
2. `goflex build` and `goflex dev` invoke the Tailwind pipeline:
   - Scan `**/*.go` for class literals inside `ui.Class(...)` calls.
   - Merge with `tailwind.config.js` (or `.css` using `@source`).
   - Produce `dist/app.css`.
3. `pkg/ui/classes.go`:
   - `Class(classes ...string)` already exists; extend with helpers.
   - `ClassIf(cond bool, classes ...string)`.
   - `ClassMap(m map[string]bool)`.
   - Class merging handles duplicate/conflicting utilities via an opt-in
     helper `Tw(...)` using the `tailwind-merge`-equivalent algorithm.
4. A default `tailwind.config.css` in `templates/new-app/`.
5. `docs/styling.md` describing Tailwind integration and how to use plain
   CSS/CSS modules as an alternative.

## Implementation notes

### Avoiding a Node dependency

Use the official Tailwind standalone binaries:
- Download once on first `goflex dev`/`goflex build`.
- Cache in `~/.cache/goflex/tailwindcss-<version>-<os>-<arch>`.
- Verify SHA against a pinned version.

### Class extraction

Tailwind needs to know which class names appear so it can include them.
Options:

1. **Content globs** — Tailwind already scans files by path + regex. Configure
   it to scan `**/*.go` and let its default extractor find string literals.
2. **Safelist** — The framework can also emit a `classes.txt` at build time
   listing all strings passed to `ui.Class(...)`, for cases where literals
   are built at runtime.

Prefer (1) for simplicity; add (2) as a fallback.

### `Tw(...)` helper

Similar to the JS `tailwind-merge` library:

```go
ui.Class(ui.Tw("px-2 py-1 bg-red-500", userClass)) // userClass can override
```

Handles conflicts like `px-2` vs `px-4`. Implementation can be a simple
Go port using the known Tailwind utility groups (can be generated, not
hand-written).

### Non-Tailwind path

If the user deletes `tailwind.config.css`, the framework does not run the
Tailwind pipeline. They can include any stylesheet by putting it in
`assets/` — files there are fingerprinted and served by the static handler.

## Testing scenarios

### T13.1 — Binary download and cache

- First invocation downloads the Tailwind standalone binary to the cache.
- Second invocation uses the cached binary (no network).
- If the cached binary's SHA does not match the pinned value, re-download.

### T13.2 — CSS is generated

- Given a test Go file containing `ui.Class("text-red-500 p-4")`, running
  `internal/build.BuildCSS` produces a `dist/app.css` containing rules for
  `.text-red-500` and `.p-4`.

### T13.3 — Unused classes are purged

- A project using only `.p-4` does not include rules for random classes
  like `.mt-96`.

### T13.4 — ClassIf behavior

- `ui.ClassIf(true, "bg-red-500")` yields className `"bg-red-500"`.
- `ui.ClassIf(false, "bg-red-500")` yields empty className.

### T13.5 — ClassMap behavior

- `ui.ClassMap(map[string]bool{"a": true, "b": false})` yields `"a"`.
- Ordering is deterministic (sorted by key) so output is stable in tests.

### T13.6 — Tw merge behavior

- `ui.Tw("px-2", "px-4")` → `"px-4"` (later wins).
- `ui.Tw("p-2", "px-4")` → `"p-2 px-4"` (different scopes, both kept).
- `ui.Tw("text-red-500", "text-blue-500")` → `"text-blue-500"`.
- Table-driven test with 20+ cases covering every major utility group.

### T13.7 — No Tailwind path compiles too

- With Tailwind config removed, `goflex build` still produces a working
  bundle (no CSS generation step runs, no error).

### T13.8 — Custom CSS files served

- Placing `assets/app.css` in a project causes it to be served at
  `/assets/app-<hash>.css` with a `Cache-Control` max-age of 1 year.

### T13.9 — Browser E2E

- A page with `ui.Div(ui.Class("text-red-500"), ui.Text("Hi"))`:
  - chromedp confirms the rendered `<div>`'s computed `color` is
    `rgb(239, 68, 68)` (Tailwind's red-500).
- Gated `//go:build e2e`.

### T13.10 — Reproducible builds

- Two consecutive `goflex build` invocations on unchanged source produce
  byte-identical `dist/app.css`.

## Acceptance criteria

- All T13.1–T13.8, T13.10 pass in CI.
- T13.9 passes in the `e2e` job.
- Developers never need to run `npm install` for the default Tailwind flow.
- `docs/styling.md` covers: default Tailwind flow, `Tw()`/`ClassIf`/`ClassMap`,
  disabling Tailwind, adding custom CSS.
