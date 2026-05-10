package build

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erazemkos/goflex/pkg/server"
)

func TestCopyAssetsFingerprintsAndServerCaches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "assets", "app.css"), "body{color:red}\n")
	dist := filepath.Join(dir, "dist")
	manifest, err := CopyAssets(AssetOptions{Dir: dir, Out: dist})
	if err != nil {
		t.Fatal(err)
	}
	hashed := manifest["assets/app.css"]
	if !strings.HasPrefix(hashed, "assets/app-") || !strings.HasSuffix(hashed, ".css") {
		t.Fatalf("bad hashed name %q manifest=%v", hashed, manifest)
	}
	if _, err := os.Stat(filepath.Join(dist, filepath.FromSlash(hashed))); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dist, "index.html"), "<!doctype html><title>x</title>")
	srv := server.New(server.Config{StaticFS: os.DirFS(dist)})
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	resp, err := http.Get(httpSrv.URL + "/" + hashed)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "color:red") {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "max-age=31536000") {
		t.Fatalf("cache-control=%q", got)
	}
}
