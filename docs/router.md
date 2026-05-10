# Router

GoFlex provides a pure-Go client-side router with path parameters, nested routes, links, and injectable history adapters.

## Basic routes

```go
r := router.New()
r.Route("/", HomePage)
r.Route("/todos", TodoListPage)
r.Route("/todos/:id", TodoDetailPage)
r.Route("/settings/*", SettingsPage)
r.NotFound(NotFoundPage)

app := r.Root()
```

Routes are normalized so `/foo` and `/foo/` refer to the same route.

## Params

```go
func TodoDetailPage() ui.Element {
  params := router.UseParams()
  return ui.Text("todo id=" + params["id"])
}
```

Path parameters are URI-decoded. Catch-all routes store the remainder under `"*"`.

## Navigation

```go
navigate := router.UseNavigate()
navigate("/todos/5")
navigate("/login", router.Replace(), router.State("from-guard"), router.Scroll(router.Top))
```

`Replace` uses replaceState instead of pushState. `State` attaches arbitrary history state. `Scroll` accepts `Top` or `Preserve`.

## Links

```go
router.Link("/todos/5", ui.Text("View todo"))
```

`Link` renders an `<a href>` and intercepts ordinary left-clicks. Ctrl/meta/shift/alt clicks and middle clicks are left to the browser.

## Nested routes

```go
r.Route("/app", AppLayout,
  router.Child("/dashboard", Dashboard),
  router.Child("/profile", Profile),
)

func AppLayout() ui.Element {
  return ui.Div(
    ui.Nav(ui.Text("App")),
    router.Outlet(), // renders Dashboard or Profile
  )
}
```

## History modes

```go
router.New(router.WithHistoryMode(router.HistoryBrowser)) // /todos
router.New(router.WithHistoryMode(router.HistoryHash))    // #/todos
```

Tests can inject `router.NewMemoryHistory()` with `router.WithHistory(history)` to assert push, replace, back, and forward behavior.
