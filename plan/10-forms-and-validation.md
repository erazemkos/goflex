# Step 10 — Forms and Validation

## Goal

A typed form layer built on top of hooks and the query layer. Developers
declare a Go struct; the framework wires field state, change handlers,
validation, submit state, and error display automatically.

## Deliverables

1. `pkg/form/form.go`:
   - `UseForm[T any](initial T, opts ...Opt) *Form[T]`
   - `*Form[T]` methods: `Value()`, `Field(name)`, `Set(name, value)`,
     `Submit(onSubmit func(T) error)`, `Reset()`, `IsValid()`, `IsDirty()`,
     `IsSubmitting()`, `FieldError(name)`.
2. `pkg/form/validate.go` — tag-driven validator built on
   `github.com/go-playground/validator/v10`, but with hooks for custom
   validators per field.
3. Shared validation: the same tags produce the same errors on the
   backend (via `pkg/form/validate.go` reused from server handlers).
4. `pkg/ui/form_bindings.go` — helpers like `ui.Input(form.Field("Title"))`
   that wire value + onChange + aria-invalid + error text.
5. Built-in field types: text, number, email, password, textarea, checkbox,
   radio, select, date.

## Implementation notes

### Field binding

```go
type CreateTodoRequest struct {
    Title       string `json:"title"       validate:"required,min=2,max=120"`
    Description string `json:"description" validate:"max=1000"`
    Priority    int    `json:"priority"    validate:"oneof=1 2 3"`
}

func NewTodoForm() ui.Element {
    form := form.UseForm(shared.CreateTodoRequest{Priority: 2})
    createTodo := query.UseMutation(api.CreateTodo,
        query.OnSuccess(func(t shared.Todo) { form.Reset() }),
    )

    return ui.Form(
        ui.OnSubmit(form.Submit(func(req shared.CreateTodoRequest) error {
            return createTodo.MutateAsync(req)
        })),
        ui.Input(form.Field("Title")),
        ui.Textarea(form.Field("Description")),
        ui.Select(form.Field("Priority"), 1, 2, 3),
        ui.Button(
            ui.Disabled(!form.IsValid() || form.IsSubmitting()),
            ui.Text("Create"),
        ),
    )
}
```

### Validation timing

- On change (per field).
- On blur (per field).
- On submit (entire struct, always).

Developers can override with `form.WithMode(OnSubmitOnly)`.

### Server-side validation parity

The server handler can call `form.Validate(req)` with the same struct. If
the backend reports additional errors (e.g. uniqueness), it returns them
via the `httperr.Error.Fields` map, which the form layer merges into its
field errors automatically.

## Testing scenarios

### T10.1 — Form holds typed value

- `UseForm(shared.CreateTodoRequest{Priority: 2})` starts with the passed
  initial value.
- `form.Value().Priority == 2`.

### T10.2 — Field setter updates state

- `form.Set("Title", "Buy milk")` results in `form.Value().Title == "Buy milk"`.
- Setting a non-existent field panics in dev mode with a helpful message.

### T10.3 — Validation on change

- Title must be min=2. Setting `Title="a"` yields `form.FieldError("Title") != ""`.
- Setting `Title="ab"` clears the field error.

### T10.4 — Validation on submit

- A submit with invalid data blocks the `onSubmit` callback and exposes
  errors for each invalid field.
- `form.IsValid()` returns false during invalid state.

### T10.5 — Submit happy path

- Valid data triggers `onSubmit` exactly once.
- While the callback runs (async), `form.IsSubmitting()==true`; after, it
  returns to false.

### T10.6 — Server-side errors merge

- `onSubmit` returns an error with `Fields{"Title": "already taken"}`.
- `form.FieldError("Title") == "already taken"` after submit resolves.

### T10.7 — Reset

- After `form.Reset()`, `form.Value()` equals the initial value, errors
  are cleared, `IsDirty()==false`.

### T10.8 — Dirty tracking

- Initial: `IsDirty()==false`.
- After setting a field: `IsDirty()==true`.
- After resetting: `IsDirty()==false`.

### T10.9 — Typed fields

- `form.Field("Priority")` returns a binding whose `Value()` type is `int`.
- Attempting to bind it to a text input (that only accepts string) produces
  a compile-time error (enforced via separate typed helpers
  `ui.NumberInput`, `ui.Select[int](...)`).

### T10.10 — Tag-based validation parity

- Same struct validated on client and server produces identical field-error
  maps for the same input (golden test with ~10 cases).

### T10.11 — Accessibility wiring

- `ui.Input(form.Field("Title"))` when invalid renders with
  `aria-invalid="true"` and an `aria-describedby` referencing an error
  text node.
- Snapshot-tested in `pkg/ui`.

### T10.12 — Browser E2E

- A page with the form above:
  1. Type an invalid value → error message appears.
  2. Fix it → error disappears.
  3. Submit → mutation fires, form resets.
- Gated `//go:build e2e`.

## Acceptance criteria

- All T10.1–T10.11 pass under `go test`.
- T10.12 passes in the `e2e` job.
- `pkg/form` coverage >= 80%.
- Developers can build a working validated form with < 20 lines of code in
  the reference example.
