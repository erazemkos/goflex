# Forms

`pkg/form` stores a typed struct, exposes field bindings for `pkg/ui`, and reuses `validate` tags on both frontend and backend.

## Basic form

```go
type CreateTodoRequest struct {
    Title       string `json:"title" validate:"required,min=2,max=120"`
    Description string `json:"description" validate:"max=1000"`
    Priority    int    `json:"priority" validate:"oneof=1 2 3"`
}

func NewTodoForm() ui.Element {
    f := form.UseForm(CreateTodoRequest{Priority: 2})
    create := query.UseMutation(api.CreateTodo,
        query.OnSuccess[CreateTodoRequest, Todo](func(Todo) { f.Reset() }),
    )

    return ui.Form(
        ui.OnSubmit(f.Submit(func(req CreateTodoRequest) error {
            _, err := create.MutateAsync(req)
            return err
        })),
        ui.TextInput(f.Field("Title")),
        ui.FieldError(f.Field("Title")),
        ui.Textarea(f.Field("Description")),
        ui.NumberInput(f.Field("Priority")),
        ui.Button(ui.Disabled(!f.IsValid() || f.IsSubmitting()), ui.Text("Create")),
    )
}
```

## API

- `UseForm(initial, opts...)` creates a typed form.
- `Value()` returns the typed value.
- `Field(name)` returns a UI binding by struct field name (`"Title"`) or JSON name (`"title"`).
- `Set(name, value)` updates a field and validates on change by default.
- `Submit(fn)` validates the whole struct, toggles `IsSubmitting`, and calls `fn` only when valid.
- `Reset()` restores the initial value and clears errors.
- `IsValid`, `IsDirty`, `IsSubmitting`, `FieldError(name)` expose form state.

## Validation modes

Default mode validates on change and submit:

```go
f := form.UseForm(CreateTodoRequest{})
```

Submit-only validation:

```go
f := form.UseForm(CreateTodoRequest{}, form.WithMode(form.OnSubmitOnly))
```

## Custom field validators

```go
f := form.UseForm(CreateTodoRequest{}, form.WithValidator("Title", func(value any, whole any) string {
    if value == "admin" { return "reserved" }
    return ""
}))
```

## Server-side parity

Use the same struct tags in handlers:

```go
if fields := form.Validate(req); fields != nil {
    return Todo{}, httperr.New("validation_failed", "bad input", fields)
}
```

If a submit returns `httperr.Error` or an API-client field error, the form merges its `Fields` map so `FieldError("Title")` displays backend errors such as uniqueness failures.

## UI bindings

Built-in helpers include `ui.TextInput`, `ui.NumberInput`, `ui.EmailInput`, `ui.PasswordInput`, `ui.Textarea`, `ui.Checkbox`, `ui.Radio`, `ui.Select[T]`, and `ui.DateInput`. Invalid bindings receive `aria-invalid="true"` and `aria-describedby="<name>-error"`; render `ui.FieldError(binding)` for accessible error text.
