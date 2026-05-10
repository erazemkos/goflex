# Step 16 — Reference Example: Todo App (End-to-End)

## Goal

Build a full `examples/todo` application that exercises every framework
feature end-to-end. It acts as:

1. A living test target for the whole framework.
2. Documentation (copy/paste starting point for users).
3. The primary integration and E2E test suite.

If this example works, GoFlex v0 works.

## Deliverables

1. `examples/todo/` with complete project structure:
   ```text
   examples/todo/
   ├── cmd/
   │   ├── server/main.go        # backend entrypoint
   │   └── web/main.go           # frontend entrypoint (GopherJS target)
   ├── internal/
   │   ├── api/                  # handlers wired to shared endpoints
   │   ├── models/               # GORM models
   │   └── web/                  # frontend pages/components
   ├── shared/
   │   ├── types.go              # DTOs
   │   └── endpoints.go          # api.Endpoint declarations
   ├── db/migrations/
   │   ├── 001_init.up.sql
   │   └── 001_init.down.sql
   ├── assets/                   # tailwind config + any static files
   └── README.md
   ```
2. Features covered:
   - Sign up / log in / log out (step 11).
   - List todos, create, edit, toggle complete, delete.
   - Optimistic updates for toggle (step 9).
   - Form validation client + server (step 10).
   - Deep links with router (step 06).
   - Protected routes via `RequireLogin`.
   - Tailwind-styled UI (step 13).
3. End-to-end test suite under `examples/todo/e2e/` using chromedp.
4. Makefile target `make e2e` that runs it.

## Implementation notes

### Shared endpoints

```go
// examples/todo/shared/endpoints.go
var (
    SignUp    = api.Endpoint[SignUpRequest, User]{Method: "POST", Path: "/auth/signup"}
    Login     = api.Endpoint[LoginRequest, User]{Method: "POST", Path: "/auth/login"}
    Logout    = api.Endpoint[struct{}, struct{}]{Method: "POST", Path: "/auth/logout"}
    Me        = api.Endpoint[struct{}, User]{Method: "GET", Path: "/auth/me"}

    ListTodos = api.Endpoint[ListTodosRequest, []Todo]{Method: "GET",    Path: "/todos"}
    CreateTodo= api.Endpoint[CreateTodoRequest, Todo]{Method: "POST",   Path: "/todos"}
    UpdateTodo= api.Endpoint[UpdateTodoRequest, Todo]{Method: "PATCH",  Path: "/todos/:id"}
    DeleteTodo= api.Endpoint[DeleteTodoRequest, struct{}]{Method: "DELETE", Path: "/todos/:id"}
)
```

### Frontend pages

- `/` — redirect to `/todos` if logged in, else `/login`.
- `/login` — form with email + password.
- `/signup` — form with email + password.
- `/todos` — list with inline add form, filters (all/open/done), optimistic
  toggle.
- `/todos/:id` — detail and edit page.
- `/settings` — basic account settings (change password).

### State strategy

- Server owns todos.
- UseQuery for lists.
- UseMutation + optimistic updates for toggle.
- UseState (client-only) for UI state: filter selection, modal open/close,
  draft form values.
- No server-side session state beyond the auth session itself.

## Testing scenarios (E2E)

Run in chromedp against a fresh SQLite database created per test run.

### T16.1 — Signup and login flow

- Navigate to `/signup`.
- Fill email + password, submit.
- Assert redirect to `/todos` and visible email in header.
- Log out; assert redirect to `/login`.
- Log in with the same credentials; assert back on `/todos`.

### T16.2 — Create todo

- Type "Buy milk", press Enter (or click Add).
- Todo appears in list immediately (optimistic).
- Page refresh still shows the todo (server persisted).

### T16.3 — Toggle complete (optimistic + rollback)

- Click toggle — UI updates immediately.
- With server error injected (test-only endpoint), UI rolls back within
  1 second.

### T16.4 — Filter by status

- Create three todos; complete one.
- Click "Open" filter → 2 items; "Done" filter → 1 item; "All" → 3.
- Filter state persists via URL query param `?filter=open`.

### T16.5 — Edit todo

- Navigate to `/todos/:id`.
- Edit title, save.
- Navigate back; assert the updated title appears in the list.

### T16.6 — Delete todo

- Delete a todo from the list.
- It disappears immediately.
- Page refresh confirms deletion persisted.

### T16.7 — Validation errors

- Try to create a todo with empty title → form shows a required error
  client-side; no network request made.
- Try with title > 120 chars → error appears; no network request.
- With a taken title (server-enforced uniqueness), server returns
  `Fields{title: "already exists"}` → form shows the error.

### T16.8 — Auth-guarded routes

- Visit `/todos` without session → redirect to `/login`.
- Log in → redirect back to original URL.

### T16.9 — CSRF

- Manually strip CSRF header from a request (test-only helper) → server
  returns 403.

### T16.10 — Browser refresh preserves auth

- Log in, refresh the page, still on `/todos` with user data loaded.

### T16.11 — Production binary E2E

- Build the app with `goflex build`.
- Run the produced binary.
- Run T16.1–T16.10 against it (not just the dev server).

### T16.12 — Coverage of framework features

- A meta-test asserts that the example project transitively uses at least
  one public API from each `pkg/*` package:
  - `ui`, `hooks`, `router`, `api`, `apiclient`, `query`, `form`, `auth`,
    `db`, `server`.
- Implemented via `go/packages` inspection.

### T16.13 — Performance smoke

- Build prod binary; serve via localhost.
- Measure:
  - Transferred JS (gzip) for `/`: < 400 KB.
  - First meaningful paint under 2 seconds on a clean load (approximate,
    via chromedp performance metrics).
- Soft assertions that log warnings only.

## Acceptance criteria

- All T16.1–T16.12 pass in CI (E2E lane).
- T16.13 emits warnings only.
- `examples/todo/README.md` walks through the app in < 5 minutes.
- A fresh developer can run:
  ```sh
  goflex new myapp
  cd myapp
  goflex dev
  ```
  and get an app that resembles the todo example, minus todo-specific
  screens.
- The todo app is the de facto "this framework is ready" gate — GoFlex v0
  is released when this app works reliably end-to-end.
