# Step 09 — Query / Cache Layer

## Goal

Provide a React-Query-like layer in Go for data fetching. This avoids
boilerplate loading/error handling and enables caching, refetching, and
optimistic updates — the features that make a stateless-backend SPA feel
fast.

## Deliverables

1. `pkg/query/query.go`:
   - `UseQuery[T any](key Key, fetch func(ctx) (T, error), opts ...Opt) *QueryResult[T]`
   - `UseMutation[Req, Res any](fn func(ctx, Req) (Res, error), opts ...MOpt) *Mutation[Req, Res]`
2. `QueryResult[T]` fields: `Data() T`, `Err() error`, `Loading() bool`,
   `Fetching() bool`, `Refetch()`, `Invalidate()`.
3. `Mutation[Req,Res]`: `Mutate(req)`, `MutateAsync(req)`, `Pending()`,
   `Err()`, `Data()`.
4. `pkg/query/cache.go` — in-memory cache keyed by normalized `Key`.
5. `pkg/query/invalidate.go` — `Invalidate(keyPrefix Key)` and
   `SetData(key Key, value any)` for optimistic writes.
6. `pkg/query/client.go` — `Provider(children...)` to mount the cache at the
   root of the React tree.
7. Stale-while-revalidate semantics with configurable `StaleTime` and
   `CacheTime`.

## Implementation notes

### Keys

Keys are slices of serializable primitives: `query.Key{"todos"}`, or
`query.Key{"todo", id}`. They are normalized to a stable string for cache
lookups.

### Lifecycle

On mount of a `UseQuery`:
1. Check cache.
2. If present and not stale → return cached, no fetch.
3. If present and stale → return cached, trigger background refetch.
4. If missing → return loading, trigger fetch.

On focus/reconnect events (configurable), trigger refetches for active
queries.

### Mutations with cache updates

```go
createTodo := query.UseMutation(api.CreateTodo,
    query.OnSuccess(func(res shared.Todo) {
        query.Invalidate(query.Key{"todos"})
    }),
)
```

Optimistic updates:

```go
createTodo := query.UseMutation(api.CreateTodo,
    query.Optimistic(func(req shared.CreateTodoRequest) {
        query.SetData(query.Key{"todos"}, func(old []shared.Todo) []shared.Todo {
            return append(old, shared.Todo{Title: req.Title, ID: 0})
        })
    }),
    query.OnError(func(req shared.CreateTodoRequest, err error) {
        query.Invalidate(query.Key{"todos"}) // rollback
    }),
)
```

### Dev tools

Expose a simple in-browser inspector (`window.__GOFLEX_QUERY__`) listing
active queries and their states. Enable only when `ui.DevMode`.

## Testing scenarios

All tests use an in-memory fake time and a mocked React runtime.

### T09.1 — UseQuery fetches once

- First render triggers `fetch`.
- Fetch resolves with a value; second render reads from cache without
  another fetch.
- Asserted by counting calls to a spy `fetch`.

### T09.2 — Loading and data transitions

- Before fetch completes: `Loading()==true`, `Data()` zero-value.
- After fetch: `Loading()==false`, `Data()==expected`, `Err()==nil`.

### T09.3 — Error path

- `fetch` returns an error; `Err()` holds it, `Loading()==false`.
- `Refetch()` re-invokes `fetch` and succeeds the second time → `Err()==nil`.

### T09.4 — Stale-while-revalidate

- With `StaleTime=100ms`: after 150ms the next render returns cached data
  and triggers a background refetch. `Fetching()==true` during the refetch,
  `Loading()==false` throughout.

### T09.5 — Cache eviction

- With `CacheTime=200ms` and no subscribers: after 200ms past the last
  unmount, the cache entry is removed.

### T09.6 — Invalidate by prefix

- `Invalidate(Key{"todos"})` marks all keys starting with `"todos"` stale
  and triggers refetch on mounted queries.

### T09.7 — Mutation + invalidation

- Running a mutation with `OnSuccess(Invalidate({"todos"}))` causes the
  todos query to refetch, with observable updated data on the next render.

### T09.8 — Optimistic update with rollback

- With optimistic setter that appends a todo and an error return from the
  server:
  1. Before server response, data includes optimistic todo.
  2. After server error, rollback reverts the cache.
- Verified by snapshot of cache at each step.

### T09.9 — Two components, one query

- Two components mount `UseQuery` with the same key.
- Only one fetch happens; both components receive the same data.

### T09.10 — Concurrent mutations

- Firing two mutations quickly invokes each exactly once and invalidates
  the query once per success (last-write-wins semantics documented).

### T09.11 — Focus refetch (configurable)

- With `RefetchOnFocus=true` enabled, dispatching a fake `focus` event
  triggers a refetch for active queries with stale data.

### T09.12 — Browser E2E

- Todo list app with query + optimistic mutation.
- In chromedp: add a todo, assert it appears immediately; simulate server
  failure, assert rollback.
- Gated `//go:build e2e`.

## Acceptance criteria

- All T09.1–T09.11 pass under `go test`.
- T09.12 passes in the `e2e` job.
- `pkg/query` coverage >= 80%.
- `docs/query.md` has recipes for: basic query, paginated query,
  dependent query, optimistic mutation, invalidation after mutation.
