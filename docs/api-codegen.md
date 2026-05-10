# API Codegen

Declare shared endpoints with `pkg/api`:

```go
var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
    Method: "POST",
    Path: "/todos",
    Description: "creates a todo",
}
```

Attach handlers from server-only packages with `Endpoint.Register`, then run:

```sh
goflex generate --only api
```

The generator writes `generated/gen_server.go` with `RegisterRoutes(*gin.Engine)` and `generated/gen_client.go` with typed client functions that call `pkg/apiclient`.

Request field tags:

- `path:"id"` substitutes `:id` in the path.
- `query:"q"` adds URL query parameters.
- Other exported fields are encoded as JSON for POST/PUT/PATCH requests.

Server `httperr.Error` responses decode into `apiclient.FieldError`; use `errors.Is(err, apiclient.Code("validation_failed"))` for code checks.
