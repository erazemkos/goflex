package build

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGopherJSMissing(t *testing.T) {
	restore := resetTestHooks()
	defer restore()
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	_, err := Build(context.Background(), Options{Entry: ".", OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), gopherJSInstallCommand) {
		t.Fatalf("err=%v", err)
	}
}

func TestSourceMapGenerationWithMockedGopherJS(t *testing.T) {
	restore := resetTestHooks()
	defer restore()
	lookPath = func(string) (string, error) { return "gopherjs", nil }
	goVersion = func(context.Context) (string, error) { return "go version go1.20.14 darwin/arm64", nil }
	gopherJSVersion = func(context.Context) (string, error) { return "GopherJS 1.20.2+go1.20.14", nil }
	execCommand = fakeGopherJSCommand
	out := t.TempDir()
	res, err := Build(context.Background(), Options{Entry: "./examples/hello", OutDir: out, SourceMap: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.JSPath != filepath.Join(out, "app.js") || res.MapPath != filepath.Join(out, "app.js.map") {
		t.Fatalf("bad paths: %+v", res)
	}
	if res.SizeBytes <= 1024 {
		t.Fatalf("size=%d", res.SizeBytes)
	}
	if _, err := os.Stat(res.MapPath); err != nil {
		t.Fatal(err)
	}
}

func TestIncompatibleGoVersionWarning(t *testing.T) {
	restore := resetTestHooks()
	defer restore()
	lookPath = func(string) (string, error) { return "gopherjs", nil }
	goVersion = func(context.Context) (string, error) { return "go version go1.26.3 darwin/arm64", nil }
	gopherJSVersion = func(context.Context) (string, error) { return "GopherJS 1.20.2+go1.20.14", nil }
	execCommand = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("gopherjs should not be invoked")
		return nil
	}
	_, err := Build(context.Background(), Options{Entry: ".", OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "unsupported Go version") {
		t.Fatalf("err=%v", err)
	}
}

func fakeGopherJSCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{"-test.run=TestHelperProcess", "--", name}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	sep := 0
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == 0 || sep+1 >= len(args) || args[sep+1] != "gopherjs" {
		os.Exit(2)
	}
	var out string
	wantMap := false
	for i := sep + 2; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 < len(args) {
				out = args[i+1]
				i++
			}
		case "--source_map=true":
			wantMap = true
		}
	}
	if out == "" {
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(out, []byte(strings.Repeat("console.log('hello');\n", 80)), 0o644); err != nil {
		panic(err)
	}
	if wantMap {
		if err := os.WriteFile(out+".map", []byte(`{"version":3,"sources":["main.go"]}`), 0o644); err != nil {
			panic(err)
		}
	}
	os.Exit(0)
}
