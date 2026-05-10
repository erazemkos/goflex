# Query Cache

`pkg/query` is a small React-Query-like cache for GoFlex frontends. It keeps data in memory by stable keys, returns cached data immediately, and refetches stale active queries in the background.

## Basic query

```go
result := query.UseQuery(query.Key{"todos"}, func(ctx context.Context) ([]shared.Todo, error) {
    return api.ListTodos(ctx, shared.ListTodosRequest{})
})

if result.Loading() { /* render spinner */ }
if err := result.Err(); err != nil { /* render error */ }
todos := result.Data()
```

`Loading` is true only before the first value arrives. `Fetching` is true for any in-flight fetch, including background refetches with cached data available.

## Paginated query

Put all inputs that affect the result in the key and in the request:

```go
page := 2
result := query.UseQuery(query.Key{"todos", "page", page}, func(ctx context.Context) (shared.Page[shared.Todo], error) {
    return api.ListTodos(ctx, shared.ListTodosRequest{Page: page})
})
```

## Dependent query

Start dependent queries only after the parent value is available:

```go
user := query.UseQuery(query.Key{"me"}, api.Me)
if !user.Loading() && user.Err() == nil {
    projects := query.UseQuery(query.Key{"projects", user.Data().ID}, func(ctx context.Context) ([]shared.Project, error) {
        return api.ListProjects(ctx, shared.ListProjectsRequest{UserID: user.Data().ID})
    })
    _ = projects
}
```

## Stale and cache times

```go
result := query.UseQuery(query.Key{"todos"}, fetchTodos,
    query.StaleTime(30*time.Second),
    query.CacheTime(5*time.Minute),
    query.RefetchOnFocus(true),
)
```

Stale cached data is returned immediately while a background refetch runs. After the last mounted query releases, the entry is evicted after `CacheTime`.

## Invalidation after mutation

```go
createTodo := query.UseMutation(api.CreateTodo,
    query.OnSuccess[shared.CreateTodoRequest, shared.Todo](func(shared.Todo) {
        query.Invalidate(query.Key{"todos"})
    }),
)
createTodo.Mutate(shared.CreateTodoRequest{Title: "Buy milk"})
```

`Invalidate(prefix)` marks every matching key stale and refetches mounted queries.

## Optimistic mutation with rollback

```go
createTodo := query.UseMutation(api.CreateTodo,
    query.Optimistic[shared.CreateTodoRequest, shared.Todo](func(req shared.CreateTodoRequest) {
        query.SetData(query.Key{"todos"}, func(old []shared.Todo) []shared.Todo {
            return append(old, shared.Todo{ID: 0, Title: req.Title})
        })
    }),
    query.OnError[shared.CreateTodoRequest, shared.Todo](func(req shared.CreateTodoRequest, err error) {
        // The cache snapshot is automatically restored; invalidate if you want
        // to confirm server state.
        query.Invalidate(query.Key{"todos"})
    }),
)
```

## Dev tools

`query.Inspector()` returns query states suitable for exposing as `window.__GOFLEX_QUERY__` in dev builds.
