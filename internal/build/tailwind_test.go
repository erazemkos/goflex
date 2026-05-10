package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailwindBinaryDownloadAndCache(t *testing.T) {
	restore := fakeTailwindDownload(t, []byte("tailwind-bin-v1"))
	defer restore()
	cache := t.TempDir()
	sha := sha256.Sum256([]byte("tailwind-bin-v1"))
	tailwindSHA256[TailwindVersion+"/linux/amd64"] = hex.EncodeToString(sha[:])
	bin, err := EnsureTailwind(context.Background(), TailwindOptions{CacheDir: cache, GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if downloads != 1 {
		t.Fatalf("downloads=%d", downloads)
	}
	bin2, err := EnsureTailwind(context.Background(), TailwindOptions{CacheDir: cache, GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if bin2 != bin || downloads != 1 {
		t.Fatalf("cache not reused bin=%q bin2=%q downloads=%d", bin, bin2, downloads)
	}
	if err := os.WriteFile(bin, []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = EnsureTailwind(context.Background(), TailwindOptions{CacheDir: cache, GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if downloads != 2 {
		t.Fatalf("corrupt cache did not redownload downloads=%d", downloads)
	}
}

var downloads int

func fakeTailwindDownload(t *testing.T, content []byte) func() {
	t.Helper()
	oldDownload := tailwindDownload
	oldSHA := tailwindSHA256
	downloads = 0
	tailwindSHA256 = map[string]string{}
	tailwindDownload = func(_ context.Context, _ string, dest string) error {
		downloads++
		return os.WriteFile(dest, content, 0o644)
	}
	return func() {
		tailwindDownload = oldDownload
		tailwindSHA256 = oldSHA
		downloads = 0
	}
}

func TestBuildCSSFallbackGeneratesPurgedDeterministicCSS(t *testing.T) {
	restore := disableTailwindStandalone()
	defer restore()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tailwind.config.css"), "@import \"tailwindcss\";\n@source \"./**/*.go\";\n")
	writeFile(t, filepath.Join(dir, "main.go"), `package main

import "github.com/goflex/goflex/pkg/ui"

func view() any {
	return ui.Div(ui.Class("text-red-500 p-4"), ui.Div(ui.ClassIf(true, "bg-blue-500")), ui.Div(ui.ClassMap(map[string]bool{"mt-2": true, "mt-96": false})), ui.Div(ui.Class(ui.Tw("px-2", "px-4"))))
}
`)
	out := filepath.Join(dir, "dist", "app.css")
	if err := BuildCSS(CSSOptions{Dir: dir, Out: out}); err != nil {
		t.Fatal(err)
	}
	b1, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	css := string(b1)
	for _, want := range []string{".text-red-500", ".p-4", ".bg-blue-500", ".mt-2", ".px-4", "color:rgb(239 68 68)", "padding:1rem"} {
		if !strings.Contains(css, want) {
			t.Fatalf("css missing %q:\n%s", want, css)
		}
	}
	if strings.Contains(css, ".mt-96") {
		t.Fatalf("unused class was not purged:\n%s", css)
	}
	if err := BuildCSS(CSSOptions{Dir: dir, Out: out}); err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatal("CSS output is not reproducible")
	}
}

func disableTailwindStandalone() func() {
	oldSHA := tailwindSHA256
	tailwindSHA256 = map[string]string{}
	return func() { tailwindSHA256 = oldSHA }
}

func TestBuildCSSNoTailwindConfigNoops(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "dist", "app.css")
	if err := BuildCSS(CSSOptions{Dir: dir, Out: out}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("expected no css output, stat err=%v", err)
	}
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
