package shared

import "github.com/erazemkos/goflex/pkg/api"

var SignUp = api.Endpoint[SignUpRequest, User]{
	Method:      "POST",
	Path:        "/auth/signup",
	Description: "creates an account and starts a session",
}

var Login = api.Endpoint[LoginRequest, User]{
	Method:      "POST",
	Path:        "/auth/login",
	Description: "starts a session",
}

var Logout = api.Endpoint[struct{}, struct{}]{
	Method:      "POST",
	Path:        "/auth/logout",
	Description: "ends the current session",
}

var Me = api.Endpoint[struct{}, User]{
	Method:      "GET",
	Path:        "/auth/me",
	Description: "returns the current user",
}

var ChangePassword = api.Endpoint[ChangePasswordRequest, User]{
	Method:      "POST",
	Path:        "/auth/password",
	Description: "changes the current user's password",
}

var ListTodos = api.Endpoint[ListTodosRequest, []Todo]{
	Method:      "GET",
	Path:        "/todos",
	Description: "lists todos for the current user",
}

var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
	Method:      "POST",
	Path:        "/todos",
	Description: "creates a todo and returns it",
}

var GetTodo = api.Endpoint[DeleteTodoRequest, Todo]{
	Method:      "GET",
	Path:        "/todos/:id",
	Description: "gets a todo by ID",
}

var UpdateTodo = api.Endpoint[UpdateTodoRequest, Todo]{
	Method:      "PATCH",
	Path:        "/todos/:id",
	Description: "updates a todo title or completion flag",
}

var DeleteTodo = api.Endpoint[DeleteTodoRequest, struct{}]{
	Method:      "DELETE",
	Path:        "/todos/:id",
	Description: "deletes a todo",
}

var SetToggleFailure = api.Endpoint[ToggleFailureRequest, struct{}]{
	Method:      "POST",
	Path:        "/test/toggle-failure",
	Description: "test-only endpoint that injects optimistic toggle failures",
}
