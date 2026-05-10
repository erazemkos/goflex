# Hooks

GoFlex hooks are typed Go wrappers for React-style component state. In standard `go test` they run against an in-memory mock runtime; in browser builds the same API can be adapted to React.

## State

```go
func Counter() ui.Element {
  count := hooks.UseState(0)
  return ui.Button(
    ui.OnClick(func() { count.Update(func(v int) int { return v + 1 }) }),
    ui.Textf("count=%d", count.Get()),
  )
}
```

`UseState[T]` returns `*State[T]` with:

- `Get() T`
- `Set(v T)`
- `Update(func(T) T)`

## Effects

```go
hooks.UseEffect(func() func() {
  startSubscription()
  return func() { stopSubscription() }
}, userID)
```

The cleanup runs before the effect reruns and when the component is unmounted. In dev mode GoFlex warns when dependencies are non-comparable values such as maps or slices.

## Memo and callbacks

```go
value := hooks.UseMemo(func() Expensive { return compute(input) }, input)
onClick := hooks.UseCallback(func() { save(value) }, value)
```

The cached value/function is reused until dependencies change.

## Refs

```go
ref := hooks.UseRef(0)
ref.Current++ // does not request a re-render
```

Refs are stable mutable holders for non-render state.

## Reducers

```go
state, dispatch := hooks.UseReducer(func(s int, action string) int {
  if action == "inc" { return s + 1 }
  return s
}, 0)
```

## Context

```go
Theme := hooks.CreateContext("light")

func Page() ui.Element {
  return Theme.Provider("dark", ui.Text("children"))
}

func Child() ui.Element {
  return ui.Text(hooks.UseContext(Theme))
}
```

Use `hooks.UseContext(Theme)` to read the current value. `Context.WithProvider` is available for scoped mock-render tests.

## Rules of hooks

When `ui.DevMode` is true, GoFlex records hook order per component fiber and panics if a later render calls a different number or kind of hooks. This mirrors React's Rules of Hooks diagnostics.

`ui.Component` is integrated with `pkg/hooks`: hook state is scoped by component name during the component render.
