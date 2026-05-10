# Step 04 — UI Component DSL

## Goal

Build the `pkg/ui` package: a Go-native DSL over React that lets developers
describe components as a tree of Go values. This layer translates Go values
to `React.createElement` calls at runtime via GopherJS.

This step makes things like this possible:

```go
ui.Div(
    ui.Class("p-4"),
    ui.H1(ui.Text("Hello")),
    ui.Button(
        ui.Class("btn"),
        ui.OnClick(func() { /* ... */ }),
        ui.Text("Click me"),
    ),
)
```

## Deliverables

1. `pkg/ui/element.go` — `Element` type and core render function
   (`Render(root Element, container js.Value)`).
2. `pkg/ui/primitives.go` — HTML element constructors: `Div`, `Span`, `H1`…`H6`,
   `P`, `A`, `Button`, `Input`, `Form`, `Label`, `Ul`, `Li`, `Img`, `Nav`,
   `Section`, `Article`, `Header`, `Footer`.
3. `pkg/ui/props.go` — prop helpers: `Class`, `ID`, `Style`, `Attr`, `Href`,
   `Src`, `Alt`, `Type`, `Value`, `Placeholder`, `Disabled`, `Name`.
4. `pkg/ui/events.go` — event helpers: `OnClick`, `OnChange`, `OnInput`,
   `OnSubmit`, `OnKeyDown`, `OnFocus`, `OnBlur`.
5. `pkg/ui/text.go` — `Text(string)`, `Textf(format, args...)`.
6. `pkg/ui/control.go` — `If(cond bool, a, b Element)`, `When(cond bool, a Element)`,
   `For[T any](xs []T, fn func(T) Element) Element`, `Fragment(children...)`.
7. `pkg/ui/custom.go` — `Component(name string, render func(Props) Element)` for
   reusable user components.
8. A zero-dep test harness `internal/uitest/` that mocks `js.Value` so tests
   can run under standard `go test` without GopherJS.

## Implementation notes

### Element representation

`Element` is an internal struct, not a `js.Value`, until render time. This is
critical: it makes the tree testable under normal `go test`, and it decouples
construction from the React runtime.

```go
type Element struct {
    kind     elementKind // tag | component | text | fragment
    tag      string
    props    map[string]any
    events   map[string]func(Event)
    children []Element
    text     string
    comp     func(Props) Element
}
```

Render walks the tree and emits `React.createElement(...)` calls. Events are
wrapped in a closure that receives a `ui.Event` (a thin wrapper around the
underlying `js.Value`).

### Props

`Class("foo")` returns a `Prop` whose `Apply(props)` method mutates the props
map. This allows composition: `Class("a", "b")` merges into `className="a b"`.

### Keys for lists

`For` requires stable keys. Signature:

```go
func For[T any](xs []T, key func(T) string, fn func(T) Element) Element
```

The key function is mandatory to prevent accidental React key warnings.

### Escape hatch

Expose `ui.Raw(value js.Value) Element` that lets advanced users drop to raw
React elements when needed (e.g. third-party JS components).

## Testing scenarios

All tests run under `go test` (no browser required) by using the mock runtime
in `internal/uitest`.

### T04.1 — Basic element construction

- `ui.Div(ui.Class("x"))` produces an `Element` with `tag="div"` and
  `props["className"]=="x"`.
- `ui.H1(ui.Text("hi"))` has one text child with value `"hi"`.

### T04.2 — Nested tree

- A three-level nested tree (`Div > Ul > Li*3`) has correct depth, child
  counts, and tag names at each level.

### T04.3 — Multiple classes merge

- `ui.Div(ui.Class("a"), ui.Class("b"))` → `className="a b"`.

### T04.4 — Events register as callable closures

- `ui.Button(ui.OnClick(func() { called++ }))`:
  - The element's `events["onClick"]` is non-nil.
  - Invoking it increments `called`.

### T04.5 — Conditional rendering

- `ui.If(true, a, b)` returns `a`.
- `ui.If(false, a, b)` returns `b`.
- `ui.When(true, a)` returns `a`; `ui.When(false, a)` returns an empty
  fragment.

### T04.6 — `For` renders each item

- `ui.For([]int{1,2,3}, strconv.Itoa, func(i int) Element { return ui.Text(strconv.Itoa(i)) })`
  returns a fragment with 3 text children: "1", "2", "3".
- Missing key function fails compile (compile-time property test).
- Duplicate keys are detected and cause a `panic` in dev mode
  (controlled by `ui.DevMode` global).

### T04.7 — Render to mock runtime

- `uitest.Render(elem)` returns a `MockNode` mirroring the tree structure.
- Asserted via JSON snapshot:
  ```json
  { "tag": "div", "props": {"className": "x"},
    "children": [{ "tag": "h1", "children": [{"text": "Hi"}]}] }
  ```
- Snapshot files live in `pkg/ui/testdata/` and are diffed on CI.

### T04.8 — Custom components

- `ui.Component("Greeting", func(p Props) Element { return ui.Text("Hello " + p.String("name")) })`
  called with `{"name": "World"}` renders text `"Hello World"`.
- Component receives a typed `Props` wrapper (not a raw map) with helpers
  `String`, `Int`, `Bool`, `Any`.

### T04.9 — Escape hatch

- `ui.Raw(jsValue)` roundtrips: when rendered, the original `js.Value` is
  passed to `React.createElement` unchanged.
- Tested via mock that records the argument identity.

### T04.10 — Browser smoke test

- Build a small page: `ui.Div(ui.H1(ui.Text("Hi")), ui.Button(ui.OnClick(inc), ui.Textf("count=%d", count)))`.
- Click the button in chromedp; assert the text updates.
- Gated with `//go:build e2e`.

## Acceptance criteria

- All T04.1–T04.9 pass under `go test ./pkg/ui/...`.
- T04.10 passes in the `e2e` job.
- Unit test coverage for `pkg/ui` is >= 80%.
- No test requires running GopherJS, thanks to `internal/uitest`.
- Code review confirms APIs feel idiomatic (no leaking `js.Value` in public
  signatures except the explicit `Raw` escape hatch).
