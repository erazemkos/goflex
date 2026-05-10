# Step 05 — Hooks Layer

## Goal

Wrap the React hooks API in ergonomic, generic Go helpers so that component
authors can manage state, side effects, memoization, and refs without
touching `js.Value` directly.

## Deliverables

1. `pkg/hooks/state.go` — `UseState[T any](initial T) *State[T]`
2. `pkg/hooks/effect.go` — `UseEffect(fn func() (cleanup func()), deps ...any)`
3. `pkg/hooks/memo.go` — `UseMemo[T any](fn func() T, deps ...any) T`
4. `pkg/hooks/ref.go` — `UseRef[T any](initial T) *Ref[T]`
5. `pkg/hooks/callback.go` — `UseCallback(fn any, deps ...any) any`
6. `pkg/hooks/context.go` — `CreateContext[T any](default T) *Context[T]` with
   `Provider(value T, children ...Element)` and `UseContext[T](ctx)`.
7. `pkg/hooks/reducer.go` — `UseReducer[S, A any](reducer func(S, A) S, init S) (S, func(A))`.
8. Integration with `pkg/ui` so that hook calls inside a component's render
   function are bound to that component's React fiber.

## Implementation notes

### `State[T]`

```go
type State[T any] struct { /* unexported */ }
func (s *State[T]) Get() T
func (s *State[T]) Set(v T)
func (s *State[T]) Update(fn func(T) T)
```

Under the hood this calls `React.useState`. The returned setter function from
React is stored and called by `Set`. Generics ensure type safety at the Go
layer.

### Dependency comparison

React compares dependency arrays by reference equality (`Object.is`). For Go
values passed through GopherJS this maps to pointer/primitive equality. Our
wrapper should:

- Accept `...any` for deps.
- Convert each dep to a JS-friendly value.
- Warn (in dev mode) if a dep is a non-comparable Go value (e.g. slice, map,
  func) and suggest using `UseMemo` or a pointer.

### Effect cleanup

`UseEffect` accepts a function returning a cleanup. If the returned cleanup
is nil, we pass `undefined` to React.

### Hook-call ordering enforcement

In dev mode (`ui.DevMode == true`), maintain a per-render hook counter and
panic with a clear message if hook call counts differ between renders —
mirroring React's own "Rules of Hooks" enforcement but in Go.

## Testing scenarios

All tests run under `go test` using the mock runtime. Each hook has a matching
mock that records calls and returns deterministic values.

### T05.1 — UseState basic

- `s := UseState(0)` inside a mocked render returns a state whose `Get()`
  equals 0.
- After `s.Set(5)`, the mock records a state-change call and the next render
  returns 5.

### T05.2 — UseState with Update

- `s.Update(func(v int) int { return v+1 })` calls the React setter with a
  function (not a value).
- Mock verifies the function was applied to the previous state.

### T05.3 — UseEffect runs with deps

- `UseEffect(fn, a, b)` on first render calls `fn` once.
- On re-render with same deps, `fn` is not called again.
- On re-render with changed deps, cleanup from previous effect runs, then
  `fn` runs.
- Verified via the mock runtime recording calls in order.

### T05.4 — UseEffect cleanup

- An effect returning a cleanup has that cleanup invoked when:
  - The component unmounts.
  - The effect re-runs due to dep change.

### T05.5 — UseMemo

- `UseMemo(fn, dep)` calls `fn` once per dep change.
- Verified by a counter inside `fn` and multiple render cycles.

### T05.6 — UseRef

- `r := UseRef(42)` has `r.Current == 42`.
- Setting `r.Current = 100` does not trigger a re-render (mock asserts no
  render was requested).

### T05.7 — UseReducer

- Given `reducer := func(s int, a string) int { if a=="inc" { return s+1 }; return s }`
  and `init = 0`:
  - First render returns `(0, dispatch)`.
  - After `dispatch("inc")`, next render returns `(1, dispatch)`.

### T05.8 — Context

- `ctx := CreateContext("default")`.
- Rendering a tree with `ctx.Provider("hello", child)` makes
  `UseContext(ctx)` inside `child` return `"hello"`.
- Outside a provider, `UseContext(ctx)` returns `"default"`.

### T05.9 — Rules-of-Hooks enforcement (dev mode)

- A component that conditionally calls `UseState` panics in dev mode with a
  message mentioning `"hooks must be called in the same order"`.
- In production mode (`ui.DevMode=false`), no panic occurs (matching React).

### T05.10 — Browser integration

- Build a small counter component using `UseState` + `OnClick`.
- In chromedp:
  - Click increment button 5 times.
  - Assert displayed count is 5.
- Gated with `//go:build e2e`.

### T05.11 — Typed hook helpers compile for common types

- Table-driven compile test ensuring the following type parameters work:
  `int`, `string`, `bool`, `struct{Name string}`, `[]int`, `map[string]int`,
  `*User`, `any`.
- Implemented as an `errcheck`-style test using `go/packages` to verify the
  code compiles without errors.

## Acceptance criteria

- All T05.1–T05.9, T05.11 pass under `go test`.
- T05.10 passes in the `e2e` job.
- Hook API docs in `docs/hooks.md` cover every hook with a working example.
- `pkg/hooks` coverage >= 80%.
