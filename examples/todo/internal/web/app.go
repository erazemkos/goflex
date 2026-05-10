package web

import (
	"context"
	"fmt"
	"strconv"

	"github.com/erazemkos/goflex/examples/todo/shared"
	"github.com/erazemkos/goflex/pkg/apiclient"
	"github.com/erazemkos/goflex/pkg/auth"
	"github.com/erazemkos/goflex/pkg/form"
	"github.com/erazemkos/goflex/pkg/hooks"
	"github.com/erazemkos/goflex/pkg/query"
	"github.com/erazemkos/goflex/pkg/router"
	"github.com/erazemkos/goflex/pkg/ui"
)

func App() ui.Element {
	r := router.New()
	r.Route("/", homePage)
	r.Route("/login", loginPage)
	r.Route("/signup", signupPage)
	r.Route("/todos", todosPage)
	r.Route("/todos/:id", todoDetailPage)
	r.Route("/settings", settingsPage)
	r.NotFound(func() ui.Element { return layout(ui.H1("Not found")) })
	return r.Root()
}

func homePage() ui.Element {
	if auth.UseUser() != nil {
		router.UseNavigate()("/todos", router.Replace())
	} else {
		router.UseNavigate()("/login", router.Replace())
	}
	return ui.Fragment()
}

func layout(children ...ui.Element) ui.Element {
	u := auth.UseUser()
	nav := ui.Nav(ui.Class("flex items-center gap-4 p-4 bg-gray-100"),
		router.Link("/todos", ui.Text("Todos")),
		router.Link("/settings", ui.Text("Settings")),
	)
	if u != nil {
		nav = ui.Nav(ui.Class("flex items-center gap-4 p-4 bg-gray-100"),
			router.Link("/todos", ui.Text("Todos")),
			router.Link("/settings", ui.Text("Settings")),
			ui.Span(ui.Class("ml-auto text-sm text-gray-500"), ui.Text(u.Email)),
			ui.Button(ui.Class("px-3 py-1 rounded bg-blue-500 text-white"), ui.OnClick(func() {
				_, _ = apiclient.Call[struct{}, struct{}](context.Background(), shared.Logout, struct{}{})
				auth.LogoutUser()
				router.UseNavigate()("/login", router.Replace())
			}), ui.Text("Log out")),
		)
	}
	return ui.Div(ui.Class("min-h-screen bg-white text-gray-900"), nav, ui.Div(ui.Class("mx-auto max-w-3xl p-4"), children))
}

func loginPage() ui.Element {
	type loginValues = shared.LoginRequest
	f := form.UseForm(loginValues{})
	submit := f.Submit(func(v loginValues) error {
		u, err := apiclient.Call[shared.LoginRequest, shared.User](context.Background(), shared.Login, v)
		if err != nil {
			return err
		}
		auth.SetCurrentUser(&auth.User{ID: strconv.FormatUint(uint64(u.ID), 10), Email: u.Email, Name: u.Name})
		router.UseNavigate()("/todos", router.Replace())
		return nil
	})
	return layout(card("Log in",
		ui.Form(ui.OnSubmit(submit),
			label("Email", ui.EmailInput(f.Field("email"), ui.Class("border rounded p-2 w-full")), ui.FieldError(f.Field("email"))),
			label("Password", ui.PasswordInput(f.Field("password"), ui.Class("border rounded p-2 w-full")), ui.FieldError(f.Field("password"))),
			ui.Button(ui.Type("submit"), ui.Class("mt-4 px-4 py-2 rounded bg-blue-500 text-white"), ui.Text("Log in")),
		),
		router.Link("/signup", ui.Text("Create an account")),
	))
}

func signupPage() ui.Element {
	f := form.UseForm(shared.SignUpRequest{})
	submit := f.Submit(func(v shared.SignUpRequest) error {
		u, err := apiclient.Call[shared.SignUpRequest, shared.User](context.Background(), shared.SignUp, v)
		if err != nil {
			return err
		}
		auth.SetCurrentUser(&auth.User{ID: strconv.FormatUint(uint64(u.ID), 10), Email: u.Email, Name: u.Name})
		router.UseNavigate()("/todos", router.Replace())
		return nil
	})
	return layout(card("Sign up",
		ui.Form(ui.OnSubmit(submit),
			label("Email", ui.EmailInput(f.Field("email"), ui.Class("border rounded p-2 w-full")), ui.FieldError(f.Field("email"))),
			label("Password", ui.PasswordInput(f.Field("password"), ui.Class("border rounded p-2 w-full")), ui.FieldError(f.Field("password"))),
			ui.Button(ui.Type("submit"), ui.Class("mt-4 px-4 py-2 rounded bg-green-500 text-white"), ui.Text("Sign up")),
		),
		router.Link("/login", ui.Text("Already have an account?")),
	))
}

func todosPage() ui.Element {
	return auth.RequireLogin(todosContent())
}

func todosContent() ui.Element {
	filter := hooks.UseState("all")
	q := query.UseQuery[[]shared.Todo](query.Key{"todos", filter.Get()}, func(ctx context.Context) ([]shared.Todo, error) {
		return apiclient.Call[shared.ListTodosRequest, []shared.Todo](ctx, shared.ListTodos, shared.ListTodosRequest{Filter: filter.Get()})
	})
	createForm := form.UseForm(shared.CreateTodoRequest{})
	create := query.UseMutation[shared.CreateTodoRequest, shared.Todo](func(ctx context.Context, req shared.CreateTodoRequest) (shared.Todo, error) {
		return apiclient.Call[shared.CreateTodoRequest, shared.Todo](ctx, shared.CreateTodo, req)
	}, query.OnSuccess[shared.CreateTodoRequest](func(shared.Todo) { q.Invalidate() }))
	toggle := query.UseMutation[shared.UpdateTodoRequest, shared.Todo](func(ctx context.Context, req shared.UpdateTodoRequest) (shared.Todo, error) {
		return apiclient.Call[shared.UpdateTodoRequest, shared.Todo](ctx, shared.UpdateTodo, req)
	}, query.Optimistic[shared.UpdateTodoRequest, shared.Todo](func(req shared.UpdateTodoRequest) {
		query.SetData(query.Key{"todos", filter.Get()}, func(items []shared.Todo) []shared.Todo {
			items = append([]shared.Todo(nil), items...)
			for i := range items {
				if items[i].ID == req.ID && req.Done != nil {
					items[i].Done = *req.Done
				}
			}
			return items
		})
	}))
	remove := query.UseMutation[shared.DeleteTodoRequest, struct{}](func(ctx context.Context, req shared.DeleteTodoRequest) (struct{}, error) {
		return apiclient.Call[shared.DeleteTodoRequest, struct{}](ctx, shared.DeleteTodo, req)
	}, query.OnSuccess[shared.DeleteTodoRequest](func(struct{}) { q.Invalidate() }))
	items := q.Data()
	return layout(ui.H1(ui.Class("text-2xl font-bold mb-4"), ui.Text("Todos")),
		ui.Form(ui.OnSubmit(createForm.Submit(func(v shared.CreateTodoRequest) error { _, err := create.MutateAsync(v); return err })),
			ui.Div(ui.Class("flex gap-2"),
				ui.TextInput(createForm.Field("title"), ui.Placeholder("Buy milk"), ui.Class("border rounded p-2 flex-1")),
				ui.Button(ui.Type("submit"), ui.Class("px-4 py-2 rounded bg-blue-500 text-white"), ui.Text("Add")),
			),
			ui.FieldError(createForm.Field("title")),
		),
		ui.Div(ui.Class("my-4 flex gap-2"), filterButton("All", "all", filter), filterButton("Open", "open", filter), filterButton("Done", "done", filter)),
		ui.When(q.Loading(), ui.P("Loading...")),
		ui.Ul(ui.Class("divide-y"), ui.For(items, func(t shared.Todo) string { return strconv.FormatUint(uint64(t.ID), 10) }, func(t shared.Todo) ui.Element {
			done := t.Done
			return ui.Li(ui.Class("flex items-center gap-3 py-3"),
				ui.Checkbox(ui.SimpleField{N: "done", V: done, Setter: func(any) { next := !done; toggle.Mutate(shared.UpdateTodoRequest{ID: t.ID, Done: &next}) }}),
				router.Link(fmt.Sprintf("/todos/%d", t.ID), ui.Span(ui.ClassIf(t.Done, "line-through text-gray-500"), ui.Text(t.Title))),
				ui.Button(ui.Class("ml-auto text-red-500"), ui.OnClick(func() { remove.Mutate(shared.DeleteTodoRequest{ID: t.ID}) }), ui.Text("Delete")),
			)
		})),
	)
}

func todoDetailPage() ui.Element {
	return auth.RequireLogin(todoDetailContent())
}

func todoDetailContent() ui.Element {
	params := router.UseParams()
	id64, _ := strconv.ParseUint(params["id"], 10, 64)
	id := uint(id64)
	q := query.UseQuery[shared.Todo](query.Key{"todo", id}, func(ctx context.Context) (shared.Todo, error) {
		return apiclient.Call[shared.DeleteTodoRequest, shared.Todo](ctx, shared.GetTodo, shared.DeleteTodoRequest{ID: id})
	})
	item := q.Data()
	f := form.UseForm(shared.UpdateTodoRequest{ID: id, Title: item.Title})
	save := f.Submit(func(v shared.UpdateTodoRequest) error {
		updated, err := apiclient.Call[shared.UpdateTodoRequest, shared.Todo](context.Background(), shared.UpdateTodo, v)
		if err != nil {
			return err
		}
		query.SetData(query.Key{"todo", id}, func(shared.Todo) shared.Todo { return updated })
		router.UseNavigate()("/todos")
		return nil
	})
	return layout(card("Edit todo", ui.Form(ui.OnSubmit(save), label("Title", ui.TextInput(f.Field("title"), ui.Class("border rounded p-2 w-full")), ui.FieldError(f.Field("title"))), ui.Button(ui.Type("submit"), ui.Class("mt-4 px-4 py-2 rounded bg-blue-500 text-white"), ui.Text("Save"))), router.Link("/todos", ui.Text("Back"))))
}

func settingsPage() ui.Element {
	return auth.RequireLogin(layout(card("Settings", ui.P("Change password"))))
}

func filterButton(labelText, value string, state *hooks.State[string]) ui.Element {
	classes := "px-3 py-1 rounded border"
	if state.Get() == value {
		classes = ui.Tw(classes, "bg-blue-500 text-white")
	}
	return ui.Button(ui.Class(classes), ui.OnClick(func() { state.Set(value); router.UseNavigate()("/todos?filter="+value, router.Replace()) }), ui.Text(labelText))
}

func card(title string, children ...ui.Element) ui.Element {
	args := []any{ui.Class("rounded border p-6 shadow bg-white"), ui.H1(ui.Class("text-2xl font-bold mb-4"), ui.Text(title))}
	for _, child := range children {
		args = append(args, child)
	}
	return ui.Section(args...)
}

func label(text string, children ...ui.Element) ui.Element {
	args := []any{ui.Class("block my-3"), ui.Span(ui.Class("block text-sm font-medium mb-1"), ui.Text(text))}
	for _, child := range children {
		args = append(args, child)
	}
	return ui.Label(args...)
}
