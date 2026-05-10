# Step 06 — Frontend Router

## Goal

Provide client-side routing in Go. Users should register routes that map paths
(including path parameters) to components, navigate programmatically, and
render a `<Link>`-style element.

## Deliverables

1. `pkg/router/router.go` — route table, matcher, navigation API.
2. `pkg/router/link.go` — `Link(href, children...)` component that prevents a
   full page reload.
3. `pkg/router/outlet.go` — `Outlet()` renders the currently-matched component.
4. Hooks:
   - `UseLocation() Location`
   - `UseParams() map[string]string`
   - `UseNavigate() func(path string, opts ...NavOpt)`
5. Nested route support (at least one level).
6. History modes: `HistoryBrowser` (default, pushState) and `HistoryHash`.
7. A not-found fallback route.

## Implementation notes

### Route definition

```go
r := router.New()
r.Route("/", HomePage)
r.Route("/todos", TodoListPage)
r.Route("/todos/:id", TodoDetailPage)
r.Route("/settings/*", SettingsPage) // catch-all
r.NotFound(NotFoundPage)

app.Mount(r.Root())
```

### Path matching

Implement a small trie-based matcher:

- Literal segments match verbatim.
- `:name` captures into `params`.
- `*` matches the rest of the path.

Parameter encoding/decoding should be URI-safe. Document behavior for
trailing slashes (we normalize: `"/foo/"` == `"/foo"`).

### Navigation

- `UseNavigate()("/todos/5")` calls `history.pushState` and triggers a
  re-render of the root `Outlet`.
- `NavOpt` flags: `Replace()`, `State(any)`, `Scroll(Top|Preserve)`.

### `Link`

```go
router.Link("/todos/5", ui.Text("View todo"))
```

Renders as `<a href="/todos/5" onclick=...>`. Click handler:
- Ignores clicks with modifiers (ctrl, shift, meta, middle click).
- Calls `preventDefault()` and invokes `navigate(...)` otherwise.

### Pluggability

The router exposes a pure-Go API for tests and a thin adapter for browser
`window.history`. The adapter is injected, so tests can replace it with a
fake.

## Testing scenarios

### T06.1 — Exact route matches

- Table-driven test over `(pattern, path, expectedParams, expectedMatch)`:
  - `/` vs `/` → match, no params.
  - `/todos` vs `/todos` → match.
  - `/todos/:id` vs `/todos/5` → match, `{id: "5"}`.
  - `/todos/:id` vs `/todos/` → no match.
  - `/settings/*` vs `/settings/account/email` → match, `{*: "account/email"}`.

### T06.2 — Route precedence

- Given routes `/users/me` and `/users/:id`, path `/users/me` matches the
  literal route, not the parametric one.

### T06.3 — Not-found fallback

- Unknown path renders the component registered via `NotFound`.

### T06.4 — Navigate pushes history

- With the fake history adapter:
  - `navigate("/todos")` records a `pushState` call.
  - `navigate("/todos", Replace())` records `replaceState`.

### T06.5 — Back/forward works

- Sequence: navigate `/a`, `/b`, `/c`, then `popstate` → current location is
  `/b`. Asserted via the fake adapter.

### T06.6 — UseParams returns captured params

- Mount `Route("/todos/:id", C)`; with URL `/todos/42`, `UseParams()` inside
  `C` returns `{"id": "42"}`.

### T06.7 — Link click behavior

- Clicking a `Link("/x")` with no modifier:
  - Calls `preventDefault`.
  - Triggers navigation.
- Clicking with ctrl/meta/shift:
  - Does not preventDefault.
  - Does not trigger navigation (lets the browser do a new-tab open).
- Middle-click (`button=1`):
  - Does not navigate.

### T06.8 — Hash history mode

- With `HistoryHash`, `navigate("/todos")` updates `location.hash` to
  `#/todos` and the matcher still finds the right route.

### T06.9 — Nested routes

- Definition:
  ```go
  r.Route("/app", AppLayout,
      router.Child("/dashboard", Dashboard),
      router.Child("/profile", Profile),
  )
  ```
- Path `/app/dashboard` renders `AppLayout` with `Dashboard` mounted where
  `Outlet()` appears.

### T06.10 — Browser E2E

- chromedp test:
  1. Load app.
  2. Click three `Link`s in sequence.
  3. Assert URL and visible page title match each step.
  4. Hit browser back — assert previous page restores.
- Gated `//go:build e2e`.

### T06.11 — Trailing slash normalization

- Routes `/foo` and `/foo/` resolve to the same handler.
- Asserted via table-driven test.

## Acceptance criteria

- All T06.1–T06.9 and T06.11 pass under `go test`.
- T06.10 passes in the `e2e` job.
- `docs/router.md` has code samples covering every feature above.
- `pkg/router` coverage >= 80%.
