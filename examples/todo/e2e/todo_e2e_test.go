//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"

	appapi "github.com/goflex/goflex/examples/todo/internal/api"
	"github.com/goflex/goflex/examples/todo/internal/models"
	"github.com/goflex/goflex/examples/todo/shared"
	"github.com/goflex/goflex/pkg/auth"
	"github.com/goflex/goflex/pkg/db"
	"github.com/goflex/goflex/pkg/httperr"
	"github.com/goflex/goflex/pkg/query"
	"github.com/goflex/goflex/pkg/server"
)

func TestSignupLoginLogoutAndRefreshFlow(t *testing.T) {
	fx := newFixture(t, "dev")
	c := fx.client(t)
	email := "user-1@example.com"
	u := postJSON[shared.SignUpRequest, shared.User](t, c, fx.URL, "/api/auth/signup", shared.SignUpRequest{Email: email, Password: "password123"}, true, http.StatusOK)
	if u.Email != email || u.ID == 0 {
		t.Fatalf("signup user=%+v", u)
	}
	items := getJSON[[]shared.Todo](t, c, fx.URL, "/api/todos", http.StatusOK)
	if len(items) != 0 {
		t.Fatalf("new account todos=%v", items)
	}
	postJSON[struct{}, map[string]any](t, c, fx.URL, "/api/auth/logout", struct{}{}, true, http.StatusOK)
	getJSON[httperr.Error](t, c, fx.URL, "/api/auth/me", http.StatusUnauthorized)
	postJSON[shared.LoginRequest, shared.User](t, c, fx.URL, "/api/auth/login", shared.LoginRequest{Email: email, Password: "password123"}, true, http.StatusOK)
	me := getJSON[shared.User](t, c, fx.URL, "/api/auth/me", http.StatusOK)
	if me.Email != email {
		t.Fatalf("me=%+v", me)
	}
	// Browser refresh equivalent: a new request with the same cookie still loads user data.
	me = getJSON[shared.User](t, c, fx.URL, "/api/auth/me", http.StatusOK)
	if me.Email != email {
		t.Fatalf("me after refresh=%+v", me)
	}
}

func TestTodoCRUDValidationFiltersAndCSRF(t *testing.T) {
	fx := newFixture(t, "dev")
	c := fx.client(t)
	signup(t, c, fx.URL, "crud@example.com")
	postJSON[shared.CreateTodoRequest, httperr.Error](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{}, true, http.StatusUnprocessableEntity)
	postJSON[shared.CreateTodoRequest, httperr.Error](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: strings.Repeat("x", 121)}, true, http.StatusUnprocessableEntity)
	milk := postJSON[shared.CreateTodoRequest, shared.Todo](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "Buy milk"}, true, http.StatusOK)
	postJSON[shared.CreateTodoRequest, httperr.Error](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "Buy milk"}, true, http.StatusUnprocessableEntity)
	postJSON[shared.CreateTodoRequest, shared.Todo](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "Walk dog"}, true, http.StatusOK)
	postJSON[shared.CreateTodoRequest, shared.Todo](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "Write docs"}, true, http.StatusOK)
	done := true
	milk = patchJSON[shared.UpdateTodoRequest, shared.Todo](t, c, fx.URL, fmt.Sprintf("/api/todos/%d", milk.ID), shared.UpdateTodoRequest{ID: milk.ID, Done: &done}, true, http.StatusOK)
	if !milk.Done {
		t.Fatalf("toggle failed: %+v", milk)
	}
	open := getJSON[[]shared.Todo](t, c, fx.URL, "/api/todos?filter=open", http.StatusOK)
	doneItems := getJSON[[]shared.Todo](t, c, fx.URL, "/api/todos?filter=done", http.StatusOK)
	all := getJSON[[]shared.Todo](t, c, fx.URL, "/api/todos?filter=all", http.StatusOK)
	if len(open) != 2 || len(doneItems) != 1 || len(all) != 3 {
		t.Fatalf("filter counts open=%d done=%d all=%d", len(open), len(doneItems), len(all))
	}
	updated := patchJSON[shared.UpdateTodoRequest, shared.Todo](t, c, fx.URL, fmt.Sprintf("/api/todos/%d", milk.ID), shared.UpdateTodoRequest{ID: milk.ID, Title: "Buy oat milk"}, true, http.StatusOK)
	if updated.Title != "Buy oat milk" {
		t.Fatalf("updated=%+v", updated)
	}
	deleteJSON[shared.DeleteTodoRequest, map[string]any](t, c, fx.URL, fmt.Sprintf("/api/todos/%d", milk.ID), shared.DeleteTodoRequest{ID: milk.ID}, true, http.StatusOK)
	all = getJSON[[]shared.Todo](t, c, fx.URL, "/api/todos", http.StatusOK)
	if len(all) != 2 {
		t.Fatalf("delete not persisted: %v", all)
	}
	postJSON[shared.CreateTodoRequest, httperr.Error](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "No CSRF"}, false, http.StatusForbidden)
}

func TestAuthGuardedRoutesAndOptimisticRollback(t *testing.T) {
	fx := newFixture(t, "dev")
	anon := fx.client(t)
	getJSON[httperr.Error](t, anon, fx.URL, "/api/todos", http.StatusUnauthorized)
	c := fx.client(t)
	signup(t, c, fx.URL, "guard@example.com")
	todo := postJSON[shared.CreateTodoRequest, shared.Todo](t, c, fx.URL, "/api/todos", shared.CreateTodoRequest{Title: "Rollback"}, true, http.StatusOK)
	postJSON[shared.ToggleFailureRequest, map[string]any](t, c, fx.URL, "/api/test/toggle-failure", shared.ToggleFailureRequest{Enabled: true}, true, http.StatusOK, e2eHeader())

	query.Clear()
	key := query.Key{"todos", "all"}
	query.SetData(key, []shared.Todo{todo})
	failure := query.UseMutation[shared.UpdateTodoRequest, shared.Todo](func(context.Context, shared.UpdateTodoRequest) (shared.Todo, error) {
		return shared.Todo{}, fmt.Errorf("server failed")
	}, query.Optimistic[shared.UpdateTodoRequest, shared.Todo](func(req shared.UpdateTodoRequest) {
		query.SetData(key, func(items []shared.Todo) []shared.Todo {
			items = append([]shared.Todo(nil), items...)
			for i := range items {
				if items[i].ID == req.ID && req.Done != nil {
					items[i].Done = *req.Done
				}
			}
			return items
		})
	}))
	next := true
	_, err := failure.MutateAsync(shared.UpdateTodoRequest{ID: todo.ID, Done: &next})
	if err == nil {
		t.Fatal("expected mutation failure")
	}
	snap := query.Snapshot()
	entry := snap[`"todos"/"all"`].(map[string]any)
	items := entry["data"].([]shared.Todo)
	if items[0].Done {
		t.Fatalf("optimistic rollback did not restore cache: %+v", items)
	}

	patchJSON[shared.UpdateTodoRequest, httperr.Error](t, c, fx.URL, fmt.Sprintf("/api/todos/%d", todo.ID), shared.UpdateTodoRequest{ID: todo.ID, Done: &next}, true, http.StatusInternalServerError)
	fresh := getJSON[shared.Todo](t, c, fx.URL, fmt.Sprintf("/api/todos/%d", todo.ID), http.StatusOK)
	if fresh.Done {
		t.Fatalf("failed toggle persisted unexpectedly: %+v", fresh)
	}
}

func TestProductionServerBinarySmoke(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "todo-server")
	cmd := exec.Command("go", "build", "-trimpath", "-o", bin, "./cmd/server")
	cmd.Dir = filepath.Join("..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build todo server: %v: %s", err, out)
	}
	dbFile := filepath.Join(t.TempDir(), "prod.db")
	gdb := db.MustOpen(db.Config{Driver: "sqlite", DSN: dbFile, Env: "test"})
	if err := models.NewStore(gdb).AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := exec.CommandContext(ctx, bin)
	run.Dir = filepath.Join("..")
	run.Env = append(os.Environ(), "GOFLEX_ENV=prod", "DATABASE_URL="+dbFile, "PORT="+port)
	var stderr bytes.Buffer
	run.Stderr = &stderr
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = run.Process.Kill(); _ = run.Wait() }()
	base := "http://127.0.0.1:" + port
	waitHTTP(t, base+"/api/healthz")
	c := &http.Client{Jar: mustJar(t), Timeout: 5 * time.Second}
	httpGet(t, c, base, "/", http.StatusOK)
	user := signup(t, c, base, "binary@example.com")
	if user.Email != "binary@example.com" {
		t.Fatalf("binary signup user=%+v stderr=%s", user, stderr.String())
	}
	postJSON[shared.CreateTodoRequest, shared.Todo](t, c, base, "/api/todos", shared.CreateTodoRequest{Title: "binary todo"}, true, http.StatusOK)
	items := getJSON[[]shared.Todo](t, c, base, "/api/todos", http.StatusOK)
	if len(items) != 1 || items[0].Title != "binary todo" {
		t.Fatalf("binary todos=%+v", items)
	}
}

func TestProductionModeSmoke(t *testing.T) {
	fx := newFixture(t, "prod")
	c := fx.client(t)
	body := httpGet(t, c, fx.URL, "/", http.StatusOK)
	if !strings.Contains(string(body), "Todo") && !strings.Contains(string(body), "root") {
		t.Fatalf("index body=%s", body)
	}
	getJSON[map[string]string](t, c, fx.URL, "/api/healthz", http.StatusOK)
	resp, _ := raw(t, c, fx.URL+"/_goflex/events", http.MethodGet, nil, false, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("debug endpoint status=%d", resp.StatusCode)
	}
	t.Log("performance smoke: API and HTML loaded under httptest; browser FMP is measured in the dedicated chromedp lane")
}

type fixture struct {
	URL string
	srv *httptest.Server
}

func newFixture(t *testing.T, env string) *fixture {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	gdb := db.MustOpen(db.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "todo.db"), Env: "test"})
	store := models.NewStore(gdb)
	if err := store.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	authn := auth.NewAuth(auth.Config{Env: env, UserLoader: func(ctx context.Context, id string) (auth.User, error) {
		uid, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return auth.User{}, err
		}
		u, err := store.UserByID(ctx, uint(uid))
		return auth.User{ID: strconv.FormatUint(uint64(u.ID), 10), Email: u.Email, Name: u.Name}, err
	}})
	app := server.New(server.Config{Env: env, StaticFS: fstest.MapFS{"index.html": {Data: []byte(`<!doctype html><html><body><div id="root">Todo</div></body></html>`)}}})
	app.Use(authn.Middleware(), authn.CSRFMiddleware())
	app.API("", func(r *gin.RouterGroup) { appapi.RegisterRoutes(r, authn, store) })
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	return &fixture{URL: srv.URL, srv: srv}
}

func (f *fixture) client(t *testing.T) *http.Client {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 5 * time.Second}
	httpGet(t, c, f.URL, "/", http.StatusOK)
	return c
}

func mustJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func signup(t *testing.T, c *http.Client, base, email string) shared.User {
	t.Helper()
	return postJSON[shared.SignUpRequest, shared.User](t, c, base, "/api/auth/signup", shared.SignUpRequest{Email: email, Password: "password123"}, true, http.StatusOK)
}

func postJSON[Req, Res any](t *testing.T, c *http.Client, base, path string, req Req, csrf bool, status int, headers ...http.Header) Res {
	t.Helper()
	return jsonReq[Req, Res](t, c, base, http.MethodPost, path, req, csrf, status, headers...)
}

func patchJSON[Req, Res any](t *testing.T, c *http.Client, base, path string, req Req, csrf bool, status int, headers ...http.Header) Res {
	t.Helper()
	return jsonReq[Req, Res](t, c, base, http.MethodPatch, path, req, csrf, status, headers...)
}

func deleteJSON[Req, Res any](t *testing.T, c *http.Client, base, path string, req Req, csrf bool, status int, headers ...http.Header) Res {
	t.Helper()
	return jsonReq[Req, Res](t, c, base, http.MethodDelete, path, req, csrf, status, headers...)
}

func getJSON[Res any](t *testing.T, c *http.Client, base, path string, status int) Res {
	t.Helper()
	body := httpGet(t, c, base, path, status)
	var out Res
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %s: %v body=%s", path, err, body)
	}
	return out
}

func jsonReq[Req, Res any](t *testing.T, c *http.Client, base, method, path string, req Req, csrf bool, status int, headers ...http.Header) Res {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := raw(t, c, base+path, method, b, csrf, mergeHeaders(headers...))
	if resp.StatusCode != status {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, resp.StatusCode, status, body)
	}
	var out Res
	if len(bytes.TrimSpace(body)) != 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("decode %s %s: %v body=%s", method, path, err, body)
		}
	}
	return out
}

func httpGet(t *testing.T, c *http.Client, base, path string, status int) []byte {
	t.Helper()
	resp, body := raw(t, c, base+path, http.MethodGet, nil, false, nil)
	if resp.StatusCode != status {
		t.Fatalf("GET %s status=%d want=%d body=%s", path, resp.StatusCode, status, body)
	}
	return body
}

func raw(t *testing.T, c *http.Client, url, method string, body []byte, csrf bool, headers http.Header) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, values := range headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	if csrf {
		for _, cookie := range c.Jar.Cookies(req.URL) {
			if cookie.Name == "csrf_token" {
				req.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, b
}

func e2eHeader() http.Header {
	h := http.Header{}
	h.Set("X-GoFlex-E2E", "1")
	return h
}

func mergeHeaders(headers ...http.Header) http.Header {
	out := http.Header{}
	for _, h := range headers {
		for k, values := range h {
			for _, v := range values {
				out.Add(k, v)
			}
		}
	}
	return out
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func TestFrameworkPackageCoverageMeta(t *testing.T) {
	root := filepath.Clean(filepath.Join(".."))
	wanted := map[string]bool{"ui": false, "hooks": false, "router": false, "api": false, "apiclient": false, "query": false, "form": false, "auth": false, "db": false, "server": false}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for pkg := range wanted {
			if strings.Contains(string(b), "github.com/goflex/goflex/pkg/"+pkg) {
				wanted[pkg] = true
			}
		}
		return nil
	})
	for pkg, ok := range wanted {
		if !ok {
			t.Fatalf("todo example does not use pkg/%s", pkg)
		}
	}
}
