package gen

import (
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAPIOutputCompilesAndIsDeterministic(t *testing.T) {
	root := writeTempModule(t)
	changed, err := Generate(root, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first generate should report changes")
	}
	server := mustRead(t, filepath.Join(root, "generated", "gen_server.go"))
	client := mustRead(t, filepath.Join(root, "generated", "gen_client.go"))
	if !bytes.Contains(client, []byte("// CreateTodo creates a todo.")) {
		t.Fatalf("client missing endpoint docstring:\n%s", client)
	}
	if !bytes.Contains(server, []byte("apiGroup.Handle(\"POST\", \"/todos\"")) {
		t.Fatalf("server missing route:\n%s", server)
	}
	sum1 := sha256.Sum256(append(server, client...))
	changed, err = Generate(root, "api")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second generate should report no changes")
	}
	sum2 := sha256.Sum256(append(mustRead(t, filepath.Join(root, "generated", "gen_server.go")), mustRead(t, filepath.Join(root, "generated", "gen_client.go"))...))
	if sum1 != sum2 {
		t.Fatal("generator output changed between runs")
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, out)
	}
	writeRoundTripTest(t, root)
	cmd = exec.Command("go", "test", "./...")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test failed: %v\n%s", err, out)
	}
	cmd = exec.Command("go", "build", "./...")
	cmd.Dir = root
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
}

func TestGeneratorChangeDetection(t *testing.T) {
	root := writeTempModule(t)
	if changed, err := Generate(root, "api"); err != nil || !changed {
		t.Fatalf("first generate changed=%v err=%v", changed, err)
	}
	appendFile(t, filepath.Join(root, "shared", "endpoints.go"), `
var GetTodo = api.Endpoint[struct { ID uint `+"`path:\"id\"`"+` }, Todo]{
	Method: "GET",
	Path: "/todos/:id",
	Description: "gets a todo",
}
`)
	changed, err := Generate(root, "api")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("new endpoint should update generated files")
	}
	if !bytes.Contains(mustRead(t, filepath.Join(root, "generated", "gen_client.go")), []byte("func GetTodo")) {
		t.Fatal("generated client missing new endpoint")
	}
}

func TestOnlyOtherGeneratorIsNoop(t *testing.T) {
	root := writeTempModule(t)
	changed, err := Generate(root, "routes")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("non-api generator should be no-op in this step")
	}
}

func TestExampleSharedPackageAvoidsServerOnlyImports(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "examples", "todo", "shared", "endpoints.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"github.com/gin-gonic/gin", "gorm.io/gorm", "net/http"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("shared package imports server-only package %s", forbidden)
		}
	}
}

func writeTempModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.22\n\nrequire github.com/goflex/goflex v0.0.0\n\nreplace github.com/goflex/goflex => "+filepath.ToSlash(repoRoot)+"\n")
	writeFile(t, filepath.Join(root, "shared", "endpoints.go"), `package shared

import "github.com/goflex/goflex/pkg/api"

type Todo struct {
	ID uint `+"`json:\"id\"`"+`
	Title string `+"`json:\"title\"`"+`
}

type CreateTodoRequest struct {
	Title string `+"`json:\"title\"`"+`
}

var CreateTodo = api.Endpoint[CreateTodoRequest, Todo]{
	Method: "POST",
	Path: "/todos",
	Description: "creates a todo",
}

var ListTodos = api.Endpoint[struct{}, []Todo]{
	Method: "GET",
	Path: "/todos",
	Description: "lists todos",
}
`)
	writeFile(t, filepath.Join(root, "internal", "handlers", "todos.go"), `package handlers

import (
	"github.com/goflex/goflex/pkg/api"
	"example.com/app/shared"
)

var todos []shared.Todo

func init() {
	shared.CreateTodo.Register(func(ctx api.Context, req shared.CreateTodoRequest) (shared.Todo, error) {
		todo := shared.Todo{ID: uint(len(todos) + 1), Title: req.Title}
		todos = append(todos, todo)
		return todo, nil
	})
	shared.ListTodos.Register(func(ctx api.Context, req struct{}) ([]shared.Todo, error) {
		return append([]shared.Todo(nil), todos...), nil
	})
}
`)
	return root
}

func writeRoundTripTest(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "roundtrip_test.go"), `package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goflex/goflex/pkg/apiclient"
	"example.com/app/generated"
	_ "example.com/app/internal/handlers"
	"example.com/app/shared"
)

func TestGeneratedRoundTripAndMethodValidation(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	generated.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	defer srv.Close()
	oldBase, oldClient := apiclient.BaseURL, apiclient.Client
	defer func() { apiclient.BaseURL, apiclient.Client = oldBase, oldClient }()
	apiclient.BaseURL = srv.URL
	apiclient.Client = srv.Client()

	one, err := generated.CreateTodo(context.Background(), shared.CreateTodoRequest{Title: "one"})
	if err != nil { t.Fatal(err) }
	if one.ID != 1 || one.Title != "one" { t.Fatalf("one=%+v", one) }
	two, err := generated.CreateTodo(context.Background(), shared.CreateTodoRequest{Title: "two"})
	if err != nil { t.Fatal(err) }
	if two.ID != 2 || two.Title != "two" { t.Fatalf("two=%+v", two) }
	list, err := generated.ListTodos(context.Background(), struct{}{})
	if err != nil { t.Fatal(err) }
	if len(list) != 2 || list[0].Title != "one" || list[1].Title != "two" { t.Fatalf("list=%+v", list) }

	putReq, err := http.NewRequest(http.MethodPut, srv.URL+"/api/todos", nil)
	if err != nil { t.Fatal(err) }
	res, err := srv.Client().Do(putReq)
	if err != nil { t.Fatal(err) }
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusMethodNotAllowed { t.Fatalf("PUT /api/todos = %d", res.StatusCode) }
}
`)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
