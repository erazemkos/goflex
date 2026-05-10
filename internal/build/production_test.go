package build

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var productionHookMu sync.Mutex

func fakeProductionPipeline(t *testing.T) func() {
	t.Helper()
	productionHookMu.Lock()
	oldFrontend := productionBuildFrontend
	oldCSS := productionBuildCSS
	oldAssets := productionCopyAssets
	oldSHA := productionGitSHA
	productionBuildFrontend = func(_ context.Context, opts Options) (Artifacts, error) {
		b, err := os.ReadFile(filepath.Join(opts.Entry, "frontend.txt"))
		if err != nil {
			b = []byte("console.log('app')\n")
		}
		if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
			return Artifacts{}, err
		}
		out := filepath.Join(opts.OutDir, "app.js")
		if opts.Minify {
			b = []byte(strings.TrimSpace(string(b)))
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return Artifacts{}, err
		}
		return Artifacts{JSPath: out, SizeBytes: int64(len(b))}, nil
	}
	productionBuildCSS = func(opts CSSOptions) error {
		b, err := os.ReadFile(filepath.Join(opts.Dir, "styles.txt"))
		if err != nil {
			b = []byte(".p-4{padding:1rem}\n")
		}
		if opts.Minify {
			b = []byte(strings.TrimSpace(string(b)))
		}
		if err := os.MkdirAll(filepath.Dir(opts.Out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(opts.Out, b, 0o644)
	}
	productionCopyAssets = func(opts AssetOptions) (AssetManifest, error) {
		return CopyAssets(opts)
	}
	productionGitSHA = func(context.Context, string) string { return "testsha" }
	return func() {
		productionBuildFrontend = oldFrontend
		productionBuildCSS = oldCSS
		productionCopyAssets = oldAssets
		productionGitSHA = oldSHA
		productionHookMu.Unlock()
	}
}

func TestBuildDistFingerprintsManifestCompressionAndStability(t *testing.T) {
	restore := fakeProductionPipeline(t)
	defer restore()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), `<!doctype html><html><head></head><body><div id="root"></div></body></html>`)
	writeFile(t, filepath.Join(dir, "frontend.txt"), "console.log('v1')\n")
	writeFile(t, filepath.Join(dir, "styles.txt"), ".p-4{padding:1rem}\n")
	writeFile(t, filepath.Join(dir, "assets", "logo.svg"), "<svg></svg>")
	res1, err := BuildDist(context.Background(), ProductionOptions{Dir: dir, Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	assertDist(t, res1)
	manifest1 := readManifest(t, filepath.Join(res1.Dir, "manifest.json"))
	if got := manifest1["assets/logo.svg"]; !strings.HasPrefix(got, "assets/logo-") || !strings.HasSuffix(got, ".svg") {
		t.Fatalf("asset manifest=%v", manifest1)
	}
	index, err := os.ReadFile(filepath.Join(res1.Dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "/dist/"+manifest1["app.js"]) || !strings.Contains(string(index), "/dist/"+manifest1["app.css"]) {
		t.Fatalf("index does not reference hashed assets: %s manifest=%v", index, manifest1)
	}
	for _, name := range []string{manifest1["app.js"], manifest1["app.css"]} {
		for _, ext := range []string{".gz", ".br"} {
			if st, err := os.Stat(filepath.Join(res1.Dir, name+ext)); err != nil || st.Size() == 0 {
				t.Fatalf("missing compressed %s%s: %v", name, ext, err)
			}
		}
	}

	res2, err := BuildDist(context.Background(), ProductionOptions{Dir: dir, Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	manifest2 := readManifest(t, filepath.Join(res2.Dir, "manifest.json"))
	if manifest1["app.js"] != manifest2["app.js"] || manifest1["app.css"] != manifest2["app.css"] {
		t.Fatalf("fingerprints not stable: %v vs %v", manifest1, manifest2)
	}

	writeFile(t, filepath.Join(dir, "styles.txt"), ".p-4{padding:1rem}.text-red-500{color:red}\n")
	res3, err := BuildDist(context.Background(), ProductionOptions{Dir: dir, Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	manifest3 := readManifest(t, filepath.Join(res3.Dir, "manifest.json"))
	if manifest3["app.css"] == manifest1["app.css"] || manifest3["app.js"] != manifest1["app.js"] {
		t.Fatalf("css change should only change css hash: before=%v after=%v", manifest1, manifest3)
	}

	writeFile(t, filepath.Join(dir, "frontend.txt"), "console.log('v2')\n")
	res4, err := BuildDist(context.Background(), ProductionOptions{Dir: dir, Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	manifest4 := readManifest(t, filepath.Join(res4.Dir, "manifest.json"))
	if manifest4["app.js"] == manifest3["app.js"] || manifest4["app.css"] != manifest3["app.css"] {
		t.Fatalf("js change should only change js hash: before=%v after=%v", manifest3, manifest4)
	}
}

func TestGenerateDistEmbedContainsExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	writeFile(t, filepath.Join(dist, "index.html"), "<h1>x</h1>")
	writeFile(t, filepath.Join(dist, "manifest.json"), `{}`)
	writeFile(t, filepath.Join(dist, "app.12345678.js"), "console.log(1)")
	writeFile(t, filepath.Join(dist, "app.12345678.css"), ".x{}")
	work := filepath.Join(dir, "work")
	if err := GenerateDistEmbed(work, dist); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(work, "internal", "web", "dist_embed.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "func DistFS() fs.FS") || !strings.Contains(string(b), "//go:embed dist") {
		t.Fatalf("bad dist_embed.go:\n%s", b)
	}
	names, err := walkFSNames(os.DirFS(filepath.Join(work, "internal", "web", "dist")))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"index.html", "manifest.json", "app.12345678.js", "app.12345678.css"} {
		if !contains(names, want) {
			t.Fatalf("generated dist missing %s: %v", want, names)
		}
	}
}

func TestProductionBinarySmokeAndCacheHeaders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process signal/exec smoke is unix-oriented")
	}
	restore := fakeProductionPipeline(t)
	defer restore()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), `<!doctype html><html><head></head><body><div id="root"></div></body></html>`)
	writeFile(t, filepath.Join(dir, "frontend.txt"), "console.log('prod smoke')\n")
	writeFile(t, filepath.Join(dir, "styles.txt"), ".p-4{padding:1rem}\n")
	out := filepath.Join(dir, "bin", "app")
	if err := Production(context.Background(), ProductionOptions{Dir: dir, Out: out, Minify: true}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("binary missing: %v", err)
	} else if st.Size() > 25*1024*1024 {
		t.Logf("warning: production binary is larger than 25 MB: %d bytes", st.Size())
	}
	port := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, out)
	cmd.Env = append(os.Environ(), "PORT="+port)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	base := "http://127.0.0.1:" + port
	waitForHTTP(t, base+"/api/healthz")
	index := httpGetString(t, base+"/")
	manifest := readManifest(t, filepath.Join(dir, "dist", "manifest.json"))
	jsURL := "/dist/" + manifest["app.js"]
	if !strings.Contains(index, jsURL) {
		t.Fatalf("index missing js url %s: %s", jsURL, index)
	}
	resp, body := httpGetResp(t, base+jsURL, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "prod smoke") {
		t.Fatalf("js status=%d body=%s", resp.StatusCode, body)
	}
	if cache := resp.Header.Get("Cache-Control"); !strings.Contains(cache, "max-age=31536000") || !strings.Contains(cache, "immutable") {
		t.Fatalf("js cache=%q", cache)
	}
	resp, body = httpGetResp(t, base+jsURL, "br")
	if resp.Header.Get("Content-Encoding") != "br" || body == "" {
		t.Fatalf("br response enc=%q body len=%d", resp.Header.Get("Content-Encoding"), len(body))
	}
	resp, body = httpGetResp(t, base+jsURL, "gzip")
	if resp.Header.Get("Content-Encoding") != "gzip" || body == "" {
		t.Fatalf("gzip response enc=%q body len=%d", resp.Header.Get("Content-Encoding"), len(body))
	}
	resp, body = httpGetRespWithAccept(t, base+"/something/random", "", "text/html")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, manifest["app.js"]) {
		t.Fatalf("SPA fallback status=%d body=%s", resp.StatusCode, body)
	}
	resp, _ = httpGetResp(t, base+"/_goflex/events", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("debug endpoint should not be registered: %d", resp.StatusCode)
	}
}

func TestProductionBinaryReproducible(t *testing.T) {
	restore := fakeProductionPipeline(t)
	defer restore()
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "frontend.txt"), "console.log('same')")
	writeFile(t, filepath.Join(dir, "styles.txt"), ".same{}")
	out1 := filepath.Join(dir, "bin", "app1")
	out2 := filepath.Join(dir, "bin", "app2")
	if err := Production(context.Background(), ProductionOptions{Dir: dir, Out: out1, Minify: true}); err != nil {
		t.Fatal(err)
	}
	if err := Production(context.Background(), ProductionOptions{Dir: dir, Out: out2, Minify: true}); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatal("production binaries are not byte-identical")
	}
}

func TestProductionCrossCompileTargetProducesNamedBinary(t *testing.T) {
	restore := fakeProductionPipeline(t)
	defer restore()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "frontend.txt"), "console.log('x')")
	writeFile(t, filepath.Join(dir, "styles.txt"), ".x{}")
	out := filepath.Join(dir, "bin", "app")
	if err := Production(context.Background(), ProductionOptions{Dir: dir, Out: out, Target: runtime.GOOS + "/" + runtime.GOARCH}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("single target should use requested output path: %v", err)
	}
}

func assertDist(t *testing.T, res DistResult) {
	t.Helper()
	if !strings.HasPrefix(res.JSName, "app.") || !strings.HasSuffix(res.JSName, ".js") || !strings.HasPrefix(res.CSSName, "app.") || !strings.HasSuffix(res.CSSName, ".css") {
		t.Fatalf("bad names: %+v", res)
	}
}

func readManifest(t *testing.T, file string) Manifest {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("manifest decode: %v body=%s", err, b)
	}
	return m
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
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

func waitForHTTP(t *testing.T, url string) {
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

func httpGetString(t *testing.T, url string) string {
	t.Helper()
	_, body := httpGetResp(t, url, "")
	return body
}

func httpGetResp(t *testing.T, url, acceptEncoding string) (*http.Response, string) {
	t.Helper()
	return httpGetRespWithAccept(t, url, acceptEncoding, "")
}

func httpGetRespWithAccept(t *testing.T, url, acceptEncoding, accept string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}
