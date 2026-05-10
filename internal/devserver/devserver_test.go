package devserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	frontendbuild "github.com/erazemkos/goflex/internal/build"
)

func TestWatcherIgnoresNoiseAndClassifies(t *testing.T) {
	ignored := ignoredForTest([]string{
		filepath.Join("app", ".git", "HEAD"),
		filepath.Join("app", "node_modules", "x.js"),
		filepath.Join("app", "dist", "app.js"),
		filepath.Join("app", "pkg", "api.generated.go"),
		filepath.Join("app", "main.go"),
	})
	if len(ignored) != 4 {
		t.Fatalf("ignored=%v", ignored)
	}
	root := filepath.Join("tmp", "app")
	if got := classifyChange(root, []string{filepath.Join(root, "internal", "api", "todos.go")}); got&changeServer == 0 || got&changeFrontend != 0 {
		t.Fatalf("server classify=%b", got)
	}
	if got := classifyChange(root, []string{filepath.Join(root, "shared", "endpoints.go")}); got&(changeShared|changeServer|changeFrontend) != changeShared|changeServer|changeFrontend {
		t.Fatalf("shared classify=%b", got)
	}
}

func TestDevServerFrontendReloadErrorAndOK(t *testing.T) {
	restore := fakeBuilders(t)
	defer restore()
	dir := newDevApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := startTestServer(t, ctx, dir)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/_goflex/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	events := make(chan string, 16)
	go readEvents(resp.Body, events)
	waitEvent(t, events, "ok")
	b, err := httpGet(url + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "initial") {
		t.Fatalf("initial app.js=%s", b)
	}

	writeFile(t, filepath.Join(dir, "main.go"), `package main
const Message = "updated"
`)
	waitEvent(t, events, "reload")
	waitFor(t, func() bool {
		b, err := httpGet(url + "/app.js")
		return err == nil && strings.Contains(string(b), "updated")
	})

	writeFile(t, filepath.Join(dir, "main.go"), `package main
const SyntaxError =
`)
	waitEvent(t, events, "error")
	errResp, err := http.Get(url + "/_goflex/error.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = errResp.Body.Close() }()
	b, err = io.ReadAll(errResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var be BuildError
	if err := json.Unmarshal(b, &be); err != nil {
		t.Fatal(err)
	}
	if be.Line == 0 || !strings.Contains(be.Message, "expected expression") {
		t.Fatalf("bad build error: %+v body=%s", be, b)
	}

	writeFile(t, filepath.Join(dir, "main.go"), `package main
const Message = "fixed"
`)
	waitEvent(t, events, "ok")
}

func TestDevServerTailwindCSSAndDebounce(t *testing.T) {
	restore := fakeBuilders(t)
	defer restore()
	dir := newDevApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	url := startTestServer(t, ctx, dir)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/_goflex/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	events := make(chan string, 16)
	go readEvents(resp.Body, events)
	waitEvent(t, events, "ok")

	for i := 0; i < 3; i++ {
		writeFile(t, filepath.Join(dir, "style.css"), strings.Repeat("a", i+1))
	}
	waitEvent(t, events, "css")
	b, err := httpGet(url + "/dist/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "css-build-2") {
		t.Fatalf("css=%s", b)
	}
	status := readStatus(t, url)
	if status.CSS != 1 {
		t.Fatalf("debounce failed status=%+v", status)
	}
}

func TestDevServerPortRuntimeAndShutdown(t *testing.T) {
	restore := fakeBuilders(t)
	defer restore()
	dir := newDevApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	url := startTestServer(t, ctx, dir)
	b, err := httpGet(url + "/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `/_goflex/runtime.js`) {
		t.Fatalf("runtime not injected: %s", b)
	}
	b, err = httpGet(url + "/_goflex/runtime.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "sessionStorage") || !strings.Contains(string(b), "EventSource") {
		t.Fatalf("runtime missing reload/state code: %s", b)
	}
	cancel()
	waitFor(t, func() bool {
		_, err := httpGet(url + "/")
		return err != nil
	})
}

func TestParseBuildError(t *testing.T) {
	be := parseBuildError(errors.New("main.go:12:4: undefined: Foo"))
	if be.File != "main.go" || be.Line != 12 || be.Column != 4 || be.Message != "undefined: Foo" {
		t.Fatalf("be=%+v", be)
	}
}

type chanWriter struct{ ch chan string }

func (w *chanWriter) Write(p []byte) (int, error) {
	w.ch <- string(p)
	return len(p), nil
}

var fakeMu sync.Mutex

func fakeBuilders(t *testing.T) func() {
	t.Helper()
	fakeMu.Lock()
	oldFrontend, oldCSS, oldGen := buildFrontend, buildCSS, generateAPI
	cssBuilds := 0
	buildFrontend = func(_ context.Context, opts frontendbuild.Options) (frontendbuild.Artifacts, error) {
		b, err := os.ReadFile(filepath.Join(opts.Entry, "main.go"))
		if err != nil {
			return frontendbuild.Artifacts{}, err
		}
		if strings.Contains(string(b), "SyntaxError") {
			return frontendbuild.Artifacts{Stderr: "main.go:2:19: expected expression"}, errors.New("main.go:2:19: expected expression")
		}
		if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
			return frontendbuild.Artifacts{}, err
		}
		out := filepath.Join(opts.OutDir, "app.js")
		if err := os.WriteFile(out, []byte("// "+string(b)), 0o644); err != nil {
			return frontendbuild.Artifacts{}, err
		}
		return frontendbuild.Artifacts{JSPath: out, SizeBytes: int64(len(b))}, nil
	}
	buildCSS = func(opts frontendbuild.CSSOptions) error {
		cssBuilds++
		if err := os.MkdirAll(filepath.Dir(opts.Out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(opts.Out, []byte("/* css-build-"+strconv.Itoa(cssBuilds)+" */"), 0o644)
	}
	generateAPI = func(string, string) (bool, error) { return false, nil }
	return func() {
		buildFrontend, buildCSS, generateAPI = oldFrontend, oldCSS, oldGen
		fakeMu.Unlock()
	}
}

func startTestServer(t *testing.T, ctx context.Context, dir string) string {
	t.Helper()
	ch := make(chan string, 1)
	go func() {
		if err := Run(ctx, Options{Addr: "127.0.0.1:0", Dir: dir, Out: &chanWriter{ch: ch}}); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("dev server: %v", err)
		}
	}()
	select {
	case line := <-ch:
		fields := strings.Fields(line)
		return fields[len(fields)-1]
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start")
		return ""
	}
}

func newDevApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), `<!doctype html><html><body><div id="root"></div></body></html>`)
	writeFile(t, filepath.Join(dir, "tailwind.config.css"), `@import "tailwindcss";`)
	writeFile(t, filepath.Join(dir, "main.go"), `package main
const Message = "initial"
`)
	return dir
}

func readEvents(r io.Reader, out chan<- string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			out <- strings.TrimPrefix(line, "event: ")
		}
	}
}

func waitEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case got := <-events:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition timed out")
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func readStatus(t *testing.T, url string) Status {
	t.Helper()
	b, err := httpGet(url + "/_goflex/status.json")
	if err != nil {
		t.Fatal(err)
	}
	var st Status
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st
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
