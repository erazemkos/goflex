package apiclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erazemkos/goflex/pkg/api"
)

type req struct {
	ID    string `path:"id" json:"-"`
	Q     string `query:"q" json:"-"`
	Limit int    `query:"limit" json:"-"`
	Title string `json:"title"`
}

type todo struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

func TestBuildRequestPathQueryAndBody(t *testing.T) {
	u, b, err := BuildRequest(http.MethodPost, "/todos/:id", req{ID: "a/b c", Q: "hello world", Limit: 10, Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/todos/a%2Fb%20c") || !strings.Contains(u, "limit=10") || !strings.Contains(u, "q=hello+world") {
		t.Fatal(u)
	}
	body, err := io.ReadAll(b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"title":"x"}` {
		t.Fatal(string(body))
	}
}

func TestBuildRequestGETHasNoBody(t *testing.T) {
	u, b, err := BuildRequest(http.MethodGet, "/todos", req{Q: "hello world", Limit: 10, Title: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		t.Fatal("GET request should not have a body")
	}
	if u != "/todos?limit=10&q=hello+world" {
		t.Fatal(u)
	}
}

func TestCallDecodesResponse(t *testing.T) {
	oldBase, oldClient := BaseURL, Client
	defer func() { BaseURL, Client = oldBase, oldClient }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/todos/1" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"id":1,"title":"x","done":false,"extra":"ignored"}`))
	}))
	defer srv.Close()
	BaseURL = srv.URL
	Client = srv.Client()
	ep := api.Endpoint[struct {
		ID uint `path:"id"`
	}, todo]{Method: http.MethodGet, Path: "/todos/:id"}
	got, err := Call(context.Background(), ep, struct {
		ID uint `path:"id"`
	}{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != 1 || got.Title != "x" || got.Done {
		t.Fatalf("got=%+v", got)
	}
}

func TestCallDecodesFieldError(t *testing.T) {
	oldBase, oldClient := BaseURL, Client
	defer func() { BaseURL, Client = oldBase, oldClient }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"code":"validation_failed","message":"bad input","fields":{"title":"required"}}`))
	}))
	defer srv.Close()
	BaseURL = srv.URL
	Client = srv.Client()
	ep := api.Endpoint[req, todo]{Method: http.MethodPost, Path: "/todos"}
	_, err := Call(context.Background(), ep, req{Title: ""})
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, Code("validation_failed")) {
		t.Fatalf("errors.Is failed for %v", err)
	}
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Fields["title"] != "required" {
		t.Fatalf("bad field error: %#v", err)
	}
}
