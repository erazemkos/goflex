package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/goflex/goflex/pkg/httperr"
)

//go:embed testdata/bundle/*
var embeddedBundle embed.FS

func TestHealthEndpoint(t *testing.T) {
	s := New(Config{})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Fatalf("health body missing status: %s", rr.Body.String())
	}
}

func TestAPIRoutingAndUnknownEnvelope(t *testing.T) {
	s := New(Config{})
	s.API("", func(r *gin.RouterGroup) {
		r.GET("/todos", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"items": []string{"write tests"}}) })
	})

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "write tests") {
		t.Fatalf("api response = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown api status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body httperr.Error
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if body.Code != "not_found" || body.RequestID == "" {
		t.Fatalf("bad envelope: %+v", body)
	}
}

func TestStaticAssetServing(t *testing.T) {
	s := New(Config{StaticFS: fstest.MapFS{
		"index.html":         {Data: []byte("<h1>app</h1>")},
		"app.js":             {Data: []byte("console.log(1)")},
		"app.12345678.js":    {Data: []byte("console.log(2)")},
		"app.12345678.js.gz": {Data: []byte("gzip-js")},
		"app.12345678.js.br": {Data: []byte("brotli-js")},
	}})

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != "console.log(1)" {
		t.Fatalf("static response = %d %q", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/javascript") {
		t.Fatalf("content type = %q", ct)
	}
	if cache := rr.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Fatalf("non-hashed cache = %q", cache)
	}

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/app.12345678.js", nil))
	if cache := rr.Header().Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Fatalf("hashed cache = %q", cache)
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/app.12345678.js", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	s.Handler().ServeHTTP(rr, req)
	if rr.Header().Get("Content-Encoding") != "br" || rr.Body.String() != "brotli-js" {
		t.Fatalf("br response enc=%q body=%q", rr.Header().Get("Content-Encoding"), rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/app.12345678.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	s.Handler().ServeHTTP(rr, req)
	if rr.Header().Get("Content-Encoding") != "gzip" || rr.Body.String() != "gzip-js" {
		t.Fatalf("gzip response enc=%q body=%q", rr.Header().Get("Content-Encoding"), rr.Body.String())
	}
}

func TestSPAFallback(t *testing.T) {
	s := New(Config{StaticFS: fstest.MapFS{"index.html": {Data: []byte("<h1>app</h1>")}}})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/todos/5", nil)
	req.Header.Set("Accept", "text/html")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "app") {
		t.Fatalf("html fallback = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/todos/5", nil)
	req.Header.Set("Accept", "application/json")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("json fallback status = %d", rr.Code)
	}
}

func TestNoFallbackForAssetLikePaths(t *testing.T) {
	s := New(Config{StaticFS: fstest.MapFS{"index.html": {Data: []byte("<h1>app</h1>")}}})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist.js", nil)
	req.Header.Set("Accept", "text/html")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound || strings.Contains(rr.Body.String(), "app") {
		t.Fatalf("asset-like fallback = %d %s", rr.Code, rr.Body.String())
	}
}

func TestCORS(t *testing.T) {
	s := New(Config{CORSOrigins: []string{"https://example.com"}})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/todos", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Headers", "X-Request-Id, Content-Type")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "X-Request-Id, Content-Type" {
		t.Fatalf("allow headers = %q", got)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/api/todos", nil)
	req.Header.Set("Origin", "https://evil.example")
	s.Handler().ServeHTTP(rr, req)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin got allow header %q", got)
	}
}

func TestGracefulShutdownDrainsInFlightRequests(t *testing.T) {
	s := New(Config{})
	s.API("", func(r *gin.RouterGroup) {
		r.GET("/slow", func(c *gin.Context) {
			time.Sleep(150 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: time.Second}
	signals := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	go func() { errCh <- s.serveHTTP(srv, func() error { return srv.Serve(ln) }, signals) }()

	client := &http.Client{Timeout: 2 * time.Second}
	resCh := make(chan *http.Response, 1)
	errReqCh := make(chan error, 1)
	go func() {
		res, err := client.Get("http://" + ln.Addr().String() + "/api/slow")
		if err != nil {
			errReqCh <- err
			return
		}
		resCh <- res
	}()

	time.Sleep(25 * time.Millisecond)
	signals <- os.Interrupt

	select {
	case err := <-errReqCh:
		t.Fatalf("in-flight request failed: %v", err)
	case res := <-resCh:
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("in-flight status = %d body = %s", res.StatusCode, string(b))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("in-flight request did not complete")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestRequestIDPropagationAndLogging(t *testing.T) {
	var logs bytes.Buffer
	s := New(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	generated := rr.Header().Get("X-Request-Id")
	if generated == "" {
		t.Fatal("missing generated request id")
	}
	if !strings.Contains(logs.String(), generated) {
		t.Fatalf("logs do not contain generated request id %q: %s", generated, logs.String())
	}

	logs.Reset()
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "req-custom")
	s.Handler().ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Request-Id"); got != "req-custom" {
		t.Fatalf("request id = %q", got)
	}
	if !strings.Contains(logs.String(), "req-custom") {
		t.Fatalf("logs do not contain supplied request id: %s", logs.String())
	}
}

func TestErrorEnvelopeGolden(t *testing.T) {
	s := New(Config{})
	s.API("", func(r *gin.RouterGroup) {
		r.POST("/todos", func(c *gin.Context) {
			httperr.Write(c, http.StatusUnprocessableEntity, httperr.New("validation_failed", "bad input", map[string]string{"title": "required"}))
		})
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/todos", nil)
	req.Header.Set("X-Request-Id", "req-123")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	golden, err := os.ReadFile("../httperr/testdata/error_envelope.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(rr.Body.String()) != strings.TrimSpace(string(golden)) {
		t.Fatalf("body mismatch\nwant: %s\n got: %s", strings.TrimSpace(string(golden)), strings.TrimSpace(rr.Body.String()))
	}
}

func TestHealthWhileDraining(t *testing.T) {
	s := New(Config{})
	s.draining.Store(true)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "draining") {
		t.Fatalf("health while draining = %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/todos", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("request while draining = %d", rr.Code)
	}
}

func TestEmbeddedFrontendBundle(t *testing.T) {
	bundle, err := fsSub(embeddedBundle, "testdata/bundle")
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{StaticFS: bundle})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "embedded app") {
		t.Fatalf("embedded index = %d %s", rr.Code, rr.Body.String())
	}
}

func TestCustomAPIPrefixBoundary(t *testing.T) {
	s := New(Config{APIPrefix: "/rpc", StaticFS: fstest.MapFS{"index.html": {Data: []byte("app")}}})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/rpc/missing", nil)
	req.Header.Set("Accept", "text/html")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound || strings.Contains(rr.Body.String(), "app") {
		t.Fatalf("api prefix should not SPA fallback: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/rpcish", nil)
	req.Header.Set("Accept", "text/html")
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("prefix boundary fallback = %d %s", rr.Code, rr.Body.String())
	}
}

func fsSub(fsys fs.FS, dir string) (fs.FS, error) {
	return fs.Sub(fsys, dir)
}
