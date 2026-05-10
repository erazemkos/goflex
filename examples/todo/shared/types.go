package shared

import "time"

type StoreError string

func (e StoreError) Error() string { return string(e) }

const (
	ErrNotFound     StoreError = "not found"
	ErrConflict     StoreError = "conflict"
	ErrUnauthorized StoreError = "unauthorized"
)

type User struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type SignUpRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" validate:"required"`
	NewPassword     string `json:"newPassword" validate:"required,min=8"`
}

type Todo struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ListTodosRequest struct {
	Filter string `query:"filter" json:"-" validate:"omitempty,oneof=all open done"`
	Q      string `query:"q" json:"-"`
	Limit  int    `query:"limit" json:"-"`
}

type CreateTodoRequest struct {
	Title string `json:"title" validate:"required,max=120"`
}

type UpdateTodoRequest struct {
	ID    uint   `path:"id" json:"-"`
	Title string `json:"title,omitempty" validate:"omitempty,max=120"`
	Done  *bool  `json:"done,omitempty"`
}

type DeleteTodoRequest struct {
	ID uint `path:"id" json:"-"`
}

type ToggleFailureRequest struct {
	Enabled bool `json:"enabled"`
}
