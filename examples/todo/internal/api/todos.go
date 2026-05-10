package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/goflex/goflex/examples/todo/shared"
	flexapi "github.com/goflex/goflex/pkg/api"
	"github.com/goflex/goflex/pkg/auth"
	"github.com/goflex/goflex/pkg/form"
	"github.com/goflex/goflex/pkg/httperr"
)

type Store interface {
	CreateUser(ctx context.Context, email, passwordHash string) (shared.User, error)
	UserByEmail(ctx context.Context, email string) (shared.User, string, error)
	UserByID(ctx context.Context, id uint) (shared.User, error)
	UpdatePassword(ctx context.Context, userID uint, passwordHash string) (shared.User, error)
	ListTodos(ctx context.Context, userID uint, filter, q string, limit int) ([]shared.Todo, error)
	CreateTodo(ctx context.Context, userID uint, title string) (shared.Todo, error)
	TodoByID(ctx context.Context, userID, id uint) (shared.Todo, error)
	UpdateTodo(ctx context.Context, userID, id uint, title string, done *bool) (shared.Todo, error)
	DeleteTodo(ctx context.Context, userID, id uint) error
	SetToggleFailure(enabled bool)
}

func RegisterRoutes(r *gin.RouterGroup, a *auth.Auth, store Store) {
	registerEndpointHandlers(a, store)
	r.POST(shared.SignUp.Path, signup(a, store))
	r.POST(shared.Login.Path, login(a, store))
	r.POST(shared.Logout.Path, logout(a))
	r.GET(shared.Me.Path, me())
	r.POST(shared.ChangePassword.Path, a.RequireUser(), changePassword(store))
	r.GET(shared.ListTodos.Path, a.RequireUser(), listTodos(store))
	r.POST(shared.CreateTodo.Path, a.RequireUser(), createTodo(store))
	r.GET(shared.GetTodo.Path, a.RequireUser(), getTodo(store))
	r.PATCH(shared.UpdateTodo.Path, a.RequireUser(), updateTodo(store))
	r.DELETE(shared.DeleteTodo.Path, a.RequireUser(), deleteTodo(store))
	r.POST(shared.SetToggleFailure.Path, setToggleFailure(store))
}

func registerEndpointHandlers(a *auth.Auth, store Store) {
	shared.SignUp.Handler = func(ctx flexapi.Context, req shared.SignUpRequest) (shared.User, error) {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			return shared.User{}, err
		}
		return store.CreateUser(ctx, req.Email, hash)
	}
	shared.Login.Handler = func(ctx flexapi.Context, req shared.LoginRequest) (shared.User, error) {
		u, hash, err := store.UserByEmail(ctx, req.Email)
		if err != nil || !auth.ComparePassword(hash, req.Password) {
			return shared.User{}, shared.ErrUnauthorized
		}
		return u, nil
	}
	shared.Me.Handler = func(ctx flexapi.Context, req struct{}) (shared.User, error) { return shared.User{}, nil }
	shared.ListTodos.Handler = func(ctx flexapi.Context, req shared.ListTodosRequest) ([]shared.Todo, error) {
		return store.ListTodos(ctx, 0, req.Filter, req.Q, req.Limit)
	}
	_ = a
}

func signup(a *auth.Auth, store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req shared.SignUpRequest
		if !decode(c, &req) || !validate(c, req) {
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			writeErr(c, err)
			return
		}
		u, err := store.CreateUser(c.Request.Context(), req.Email, hash)
		if err != nil {
			writeErr(c, fieldConflict(err, "email", "already exists"))
			return
		}
		a.Login(c, strconv.FormatUint(uint64(u.ID), 10))
		c.JSON(http.StatusOK, u)
	}
}

func login(a *auth.Auth, store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req shared.LoginRequest
		if !decode(c, &req) || !validate(c, req) {
			return
		}
		u, hash, err := store.UserByEmail(c.Request.Context(), req.Email)
		if err != nil || !auth.ComparePassword(hash, req.Password) {
			httperr.Write(c, http.StatusUnauthorized, httperr.New("invalid_credentials", "invalid email or password", nil))
			return
		}
		a.Login(c, strconv.FormatUint(uint64(u.ID), 10))
		c.JSON(http.StatusOK, u)
	}
}

func logout(a *auth.Auth) gin.HandlerFunc {
	return func(c *gin.Context) {
		a.Logout(c)
		c.JSON(http.StatusOK, gin.H{})
	}
}

func me() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if u == nil {
			httperr.Write(c, http.StatusUnauthorized, httperr.New("unauthorized", "login required", nil))
			return
		}
		id, _ := strconv.ParseUint(u.ID, 10, 64)
		c.JSON(http.StatusOK, shared.User{ID: uint(id), Email: u.Email, Name: u.Name})
	}
}

func changePassword(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		var req shared.ChangePasswordRequest
		if !decode(c, &req) || !validate(c, req) {
			return
		}
		u := auth.CurrentUser(c)
		_, hash, err := store.UserByEmail(c.Request.Context(), u.Email)
		if err != nil || !auth.ComparePassword(hash, req.CurrentPassword) {
			httperr.Write(c, http.StatusUnprocessableEntity, httperr.New("validation_failed", "validation failed", map[string]string{"currentPassword": "invalid"}))
			return
		}
		newHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			writeErr(c, err)
			return
		}
		updated, err := store.UpdatePassword(c.Request.Context(), uid, newHash)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func listTodos(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		var req shared.ListTodosRequest
		if err := flexapi.DecodeRequest(c.Request, c.Param, &req); err != nil {
			httperr.Write(c, http.StatusBadRequest, httperr.New("bad_request", err.Error(), nil))
			return
		}
		if req.Filter == "" {
			req.Filter = "all"
		}
		if !validate(c, req) {
			return
		}
		items, err := store.ListTodos(c.Request.Context(), uid, req.Filter, req.Q, req.Limit)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(http.StatusOK, items)
	}
}

func createTodo(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		var req shared.CreateTodoRequest
		if !decode(c, &req) || !validate(c, req) {
			return
		}
		item, err := store.CreateTodo(c.Request.Context(), uid, req.Title)
		if err != nil {
			writeErr(c, fieldConflict(err, "title", "already exists"))
			return
		}
		c.JSON(http.StatusOK, item)
	}
}

func getTodo(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		id, ok := pathID(c)
		if !ok {
			return
		}
		item, err := store.TodoByID(c.Request.Context(), uid, id)
		if err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(http.StatusOK, item)
	}
}

func updateTodo(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		var req shared.UpdateTodoRequest
		if err := flexapi.DecodeRequest(c.Request, c.Param, &req); err != nil {
			httperr.Write(c, http.StatusBadRequest, httperr.New("bad_request", err.Error(), nil))
			return
		}
		if !validate(c, req) {
			return
		}
		item, err := store.UpdateTodo(c.Request.Context(), uid, req.ID, req.Title, req.Done)
		if err != nil {
			writeErr(c, fieldConflict(err, "title", "already exists"))
			return
		}
		c.JSON(http.StatusOK, item)
	}
}

func deleteTodo(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := currentUserID(c)
		if !ok {
			return
		}
		id, ok := pathID(c)
		if !ok {
			return
		}
		if err := store.DeleteTodo(c.Request.Context(), uid, id); err != nil {
			writeErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{})
	}
}

func setToggleFailure(store Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-GoFlex-E2E") != "1" {
			httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
			return
		}
		var req shared.ToggleFailureRequest
		if !decode(c, &req) {
			return
		}
		store.SetToggleFailure(req.Enabled)
		c.JSON(http.StatusOK, gin.H{})
	}
}

func decode(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		httperr.Write(c, http.StatusBadRequest, httperr.New("bad_request", "invalid JSON payload", nil))
		return false
	}
	return true
}

func validate(c *gin.Context, v any) bool {
	if fields := form.Validate(v); len(fields) > 0 {
		httperr.Write(c, http.StatusUnprocessableEntity, httperr.New("validation_failed", "validation failed", fields))
		return false
	}
	return true
}

func currentUserID(c *gin.Context) (uint, bool) {
	u := auth.CurrentUser(c)
	if u == nil {
		httperr.Write(c, http.StatusUnauthorized, httperr.New("unauthorized", "login required", nil))
		return 0, false
	}
	id, err := strconv.ParseUint(u.ID, 10, 64)
	if err != nil || id == 0 {
		httperr.Write(c, http.StatusUnauthorized, httperr.New("unauthorized", "invalid session", nil))
		return 0, false
	}
	return uint(id), true
}

func pathID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		httperr.Write(c, http.StatusBadRequest, httperr.New("bad_request", "invalid id", nil))
		return 0, false
	}
	return uint(id), true
}

func fieldConflict(err error, field, message string) error {
	if errors.Is(err, shared.ErrConflict) {
		return httperr.New("validation_failed", "validation failed", map[string]string{field: message})
	}
	return err
}

func writeErr(c *gin.Context, err error) {
	var he *httperr.Error
	if errors.As(err, &he) && he != nil {
		status := http.StatusInternalServerError
		if he.Code == "validation_failed" {
			status = http.StatusUnprocessableEntity
		}
		httperr.Write(c, status, he)
		return
	}
	switch {
	case errors.Is(err, shared.ErrNotFound):
		httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
	case errors.Is(err, shared.ErrConflict):
		httperr.Write(c, http.StatusConflict, httperr.New("conflict", "conflict", nil))
	case errors.Is(err, shared.ErrUnauthorized):
		httperr.Write(c, http.StatusUnauthorized, httperr.New("unauthorized", "login required", nil))
	default:
		httperr.Write(c, http.StatusInternalServerError, err)
	}
}
