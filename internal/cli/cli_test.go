package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	frontendbuild "github.com/erazemkos/goflex/internal/build"
	"github.com/erazemkos/goflex/internal/devserver"
	"github.com/erazemkos/goflex/pkg/db/migrate"
	"github.com/spf13/cobra"
)

func TestHelpOutputWorks(t *testing.T) {
	stdout, stderr, code := runCLI("--help")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"new", "dev", "build", "generate", "db", "version"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q in %s", want, stdout)
		}
	}

	stdout, stderr, code = runCLI("db", "--help")
	if code != 0 {
		t.Fatalf("db help code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"migrate", "rollback", "create", "status"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("db help missing %q in %s", want, stdout)
		}
	}
}

func TestUnknownCommandReturnsUsageError(t *testing.T) {
	_, stderr, code := runCLI("bogus")
	if code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, `unknown command "bogus"`) {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestStubsRunAndVersionPrints(t *testing.T) {
	restoreDev := fakeDevForTest()
	defer restoreDev()
	stdout, stderr, code := runCLI("dev")
	if code != 0 {
		t.Fatalf("dev code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "dev started") {
		t.Fatalf("dev stdout=%s", stdout)
	}

	stdout, stderr, code = runCLI("version")
	if code != 0 {
		t.Fatalf("version code=%d stderr=%s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" || strings.Contains(stdout, "not yet implemented") {
		t.Fatalf("bad version stdout=%q", stdout)
	}
}

func TestNewCommandScaffoldsBasicApp(t *testing.T) {
	tmp := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOFLEX_FRAMEWORK_PATH", root)
	withCwd(t, tmp)
	stdout, stderr, code := runCLI("new", "myapp", "--module", "example.com/myapp")
	if code != 0 {
		t.Fatalf("new code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "created GoFlex app myapp") {
		t.Fatalf("stdout=%s", stdout)
	}
	for _, file := range []string{"go.mod", "index.html", "tailwind.config.css", "cmd/server/main.go", "cmd/web/main.go", "internal/web/ids.go", "internal/web/page.go", "internal/api/greeting.go", "shared/types.go", "assets/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(tmp, "myapp", file)); err != nil {
			t.Fatalf("missing %s: %v", file, err)
		}
	}
	ids, err := os.ReadFile(filepath.Join(tmp, "myapp", "internal", "web", "ids.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ids), "type ElementID string") || !strings.Contains(string(ids), "IDNameInput") {
		t.Fatalf("bad ids template:\n%s", ids)
	}
	page, err := os.ReadFile(filepath.Join(tmp, "myapp", "internal", "web", "page.go"))
	if err != nil {
		t.Fatal(err)
	}
	pageText := string(page)
	if !strings.HasPrefix(pageText, "//go:build !js\n") {
		t.Fatalf("page.go must start with //go:build !js to keep gomponents out of the GopherJS bundle:\n%s", pageText)
	}
	if !strings.Contains(pageText, "maragu.dev/gomponents") || !strings.Contains(pageText, "Typed client + API demo") || !strings.Contains(pageText, "https://github.com/erazemkos/goflex") {
		t.Fatalf("bad page template:\n%s", pageText)
	}
	b := pageText
	_ = b
	webMain, err := os.ReadFile(filepath.Join(tmp, "myapp", "cmd", "web", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	webMainText := string(webMain)
	for _, want := range []string{"addEventListener", "shared.GreetingResponse", "reactive.NewSignal", "bindText", "fetchGreeting(name, greeting, loading, errText)"} {
		if !strings.Contains(webMainText, want) {
			t.Fatalf("bad web main template, missing %q:\n%s", want, webMain)
		}
	}
	if strings.Contains(webMainText, "func render(") {
		t.Fatalf("web main should use fine-grained reactive bindings instead of a global render function:\n%s", webMain)
	}
	mod, err := os.ReadFile(filepath.Join(tmp, "myapp", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modText := string(mod)
	if !strings.Contains(modText, "module example.com/myapp") ||
		!strings.Contains(modText, "github.com/erazemkos/goflex v0.0.0") ||
		!strings.Contains(modText, "github.com/gopherjs/gopherjs v1.20.2") ||
		!strings.Contains(modText, "maragu.dev/gomponents v1.3.0") ||
		!strings.Contains(modText, "replace github.com/erazemkos/goflex =>") {
		t.Fatalf("bad go.mod:\n%s", mod)
	}
}

func TestNewCommandUsesStableVersionWithoutLocalReplace(t *testing.T) {
	t.Setenv("GOFLEX_FRAMEWORK_PATH", "")
	tmp := t.TempDir()
	withCwd(t, tmp)
	_, stderr, code := runCLI("new", "myapp")
	if code != 0 {
		t.Fatalf("new code=%d stderr=%s", code, stderr)
	}
	mod, err := os.ReadFile(filepath.Join(tmp, "myapp", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modText := string(mod)
	if !strings.Contains(modText, "github.com/erazemkos/goflex v0.2.1") {
		t.Fatalf("default mode should pin stable version, got:\n%s", mod)
	}
	if strings.Contains(modText, "github.com/erazemkos/goflex main") {
		t.Fatalf("default mode must not use main, got:\n%s", mod)
	}
	if strings.Contains(modText, "replace github.com/erazemkos/goflex =>") {
		t.Fatalf("default mode should not include a replace directive, got:\n%s", mod)
	}
}

func TestNewCommandDevModeResolvesMainPseudoVersion(t *testing.T) {
	t.Setenv("GOFLEX_FRAMEWORK_PATH", "")
	tmp := t.TempDir()
	withCwd(t, tmp)
	restore := fakeDevResolve(func(dir string, _, _ io.Writer) error {
		modPath := filepath.Join(dir, "go.mod")
		b, err := os.ReadFile(modPath)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "github.com/erazemkos/goflex") {
			t.Errorf("dev mode should not pin goflex before go get runs, got:\n%s", b)
		}
		if strings.Contains(string(b), "github.com/erazemkos/goflex main") {
			t.Errorf("dev mode must not leave `main` in go.mod, got:\n%s", b)
		}
		updated := string(b) + "\nrequire github.com/erazemkos/goflex v0.1.1-0.20260510150422-a1c0feb2ee84\n"
		return os.WriteFile(modPath, []byte(updated), 0o644)
	})
	defer restore()
	stdout, stderr, code := runCLI("new", "myapp", "--dev")
	if code != 0 {
		t.Fatalf("new --dev code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Using latest goflex main branch via GOPROXY=direct...") {
		t.Fatalf("expected dev mode banner in stdout, got=%s", stdout)
	}
	if !ParsedNewConfig().Dev {
		t.Fatalf("parsed config should record Dev=true, got %+v", ParsedNewConfig())
	}
	mod, err := os.ReadFile(filepath.Join(tmp, "myapp", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modText := string(mod)
	if strings.Contains(modText, "github.com/erazemkos/goflex main") {
		t.Fatalf("dev mode must resolve to a pseudo-version, not leave `main`:\n%s", mod)
	}
	if !strings.Contains(modText, "github.com/erazemkos/goflex v0.1.1-0.20260510150422-a1c0feb2ee84") {
		t.Fatalf("dev mode should leave go.mod pinned to resolved pseudo-version, got:\n%s", mod)
	}
}

func TestNewCommandDevModePropagatesResolveError(t *testing.T) {
	t.Setenv("GOFLEX_FRAMEWORK_PATH", "")
	tmp := t.TempDir()
	withCwd(t, tmp)
	restore := fakeDevResolve(func(string, io.Writer, io.Writer) error {
		return errors.New("goflex new --dev requires network access to GitHub; fake resolve failed")
	})
	defer restore()
	_, stderr, code := runCLI("new", "myapp", "--dev")
	if code == 0 {
		t.Fatalf("expected non-zero exit, stderr=%s", stderr)
	}
	if !strings.Contains(stderr, "requires network access to GitHub") {
		t.Fatalf("expected network-access hint in stderr, got=%s", stderr)
	}
}

func TestNewCommandRejectsNonEmptyDirectory(t *testing.T) {
	tmp := t.TempDir()
	withCwd(t, tmp)
	if err := os.Mkdir("myapp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("myapp", "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI("new", "myapp")
	if code == 0 || !strings.Contains(stderr, "not empty") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

func TestDBCommandsInvokeMigrationTooling(t *testing.T) {
	restore := fakeDBForTest()
	defer restore()
	stdout, stderr, code := runCLI("db", "create", "add_todos", "--dir", "custom/migrations")
	if code != 0 {
		t.Fatalf("create code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "001_add_todos.up.sql") || ParsedDBConfig().Name != "add_todos" || ParsedDBConfig().Dir != "custom/migrations" {
		t.Fatalf("create stdout=%s cfg=%+v", stdout, ParsedDBConfig())
	}
	stdout, stderr, code = runCLI("db", "migrate", "--dsn", ":memory:", "--driver", "sqlite")
	if code != 0 || !strings.Contains(stdout, "migrations applied") {
		t.Fatalf("migrate code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI("db", "rollback", "--step", "2")
	if code != 0 || !strings.Contains(stdout, "rolled back 2") || ParsedDBConfig().Step != 2 {
		t.Fatalf("rollback code=%d stdout=%s stderr=%s cfg=%+v", code, stdout, stderr, ParsedDBConfig())
	}
	stdout, stderr, code = runCLI("db", "status")
	if code != 0 || !strings.Contains(stdout, "3 migrations, 1 applied, 2 pending") {
		t.Fatalf("status code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestBuildCommandInvokesPipeline(t *testing.T) {
	restore := fakeBuildForTest()
	defer restore()
	stdout, stderr, code := runCLI("build", "--out", t.TempDir(), "--minify")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "built") {
		t.Fatalf("stdout=%s", stdout)
	}
	if ParsedBuildConfig().Out == "" || !ParsedBuildConfig().Minify {
		t.Fatalf("cfg=%+v", ParsedBuildConfig())
	}
}

func TestGenerateCommandInvokesGenerator(t *testing.T) {
	restore := fakeGenerateForTest(true)
	defer restore()
	stdout, stderr, code := runCLI("generate", "--only", "api")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "generated api files") || ParsedGenerateConfig().Only != "api" {
		t.Fatalf("stdout=%s cfg=%+v", stdout, ParsedGenerateConfig())
	}

	restore = fakeGenerateForTest(false)
	defer restore()
	stdout, stderr, code = runCLI("generate", "--only", "api")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "no changes") {
		t.Fatalf("stdout=%s", stdout)
	}
}

func TestFlagsParseCorrectly(t *testing.T) {
	restoreDev := fakeDevForTest()
	defer restoreDev()
	_, _, code := runCLI("dev", "--addr", ":4000")
	if code != 0 || ParsedDevConfig().Addr != ":4000" {
		t.Fatalf("dev code=%d cfg=%+v", code, ParsedDevConfig())
	}
	restoreBuild := fakeBuildForTest()
	defer restoreBuild()
	_, _, code = runCLI("build", "--out", "./out", "--minify")
	if code != 0 || ParsedBuildConfig().Out != "./out" || !ParsedBuildConfig().Minify {
		t.Fatalf("build code=%d cfg=%+v", code, ParsedBuildConfig())
	}
	restoreDB := fakeDBForTest()
	defer restoreDB()
	_, _, code = runCLI("db", "migrate", "--step", "2")
	if code != 0 || ParsedDBConfig().Step != 2 {
		t.Fatalf("db migrate code=%d cfg=%+v", code, ParsedDBConfig())
	}
}

func TestMissingRequiredArgs(t *testing.T) {
	_, stderr, code := runCLI("new")
	if code != 2 || !strings.Contains(stderr, "requires exactly 1 argument") {
		t.Fatalf("new code=%d stderr=%s", code, stderr)
	}
	_, stderr, code = runCLI("db", "create")
	if code != 2 || !strings.Contains(stderr, "requires exactly 1 argument") {
		t.Fatalf("db create code=%d stderr=%s", code, stderr)
	}
}

func TestDebugMode(t *testing.T) {
	t.Setenv("GOFLEX_DEBUG", "1")
	_, stderr, code := runCLI("version")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "level=debug") {
		t.Fatalf("missing debug line: %s", stderr)
	}

	t.Setenv("GOFLEX_DEBUG", "")
	_, stderr, code = runCLI("version")
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if strings.Contains(stderr, "level=debug") {
		t.Fatalf("unexpected debug line: %s", stderr)
	}
}

func TestSubcommandsExposeExpectedFlags(t *testing.T) {
	root := NewRootCommand()
	mustFindFlag(t, root, []string{"new"}, "template")
	mustFindFlag(t, root, []string{"new"}, "module")
	mustFindFlag(t, root, []string{"new"}, "dev")
	mustFindFlag(t, root, []string{"dev"}, "addr")
	mustFindFlag(t, root, []string{"dev"}, "no-open")
	mustFindFlag(t, root, []string{"build"}, "out")
	mustFindFlag(t, root, []string{"build"}, "minify")
	mustFindFlag(t, root, []string{"build"}, "target")
	mustFindFlag(t, root, []string{"generate"}, "only")
	mustFindFlag(t, root, []string{"db", "migrate"}, "step")
	mustFindFlag(t, root, []string{"db", "migrate"}, "dsn")
	mustFindFlag(t, root, []string{"db", "migrate"}, "driver")
	mustFindFlag(t, root, []string{"db", "migrate"}, "dir")
	mustFindFlag(t, root, []string{"db", "migrate"}, "auto")
	mustFindFlag(t, root, []string{"db", "rollback"}, "step")
	mustFindFlag(t, root, []string{"db", "status"}, "dsn")
}

// TestNewCommandGeneratedAppBuildsAndServes scaffolds a new app using the
// --dev flag (the real framework-developer workflow), compiles it against
// the local framework checkout, starts the server binary, and asserts that:
//
//   - GET /     returns gomponents-rendered HTML with expected content
//   - GET /api/greeting?name=Pi  returns the typed JSON response
//
// The test is skipped in -short mode because it compiles real Go binaries.
func TestNewCommandGeneratedAppBuildsAndServes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping generated-app integration test in -short mode")
	}

	// Find the framework root so the fake resolver can wire a local replace.
	// This must happen before withCwd changes the working directory.
	frameworkRoot := findFrameworkRoot(t)

	// Intercept the --dev resolver so the test stays hermetic: instead of
	// running `GOPROXY=direct go get github.com/erazemkos/goflex@main` it
	// appends a local replace directive pointing at the current checkout.
	// This faithfully exercises the --dev code path (the generated go.mod
	// starts with no goflex require, just like --dev) while avoiding any
	// network access or dependency on a published commit.
	restore := fakeDevResolve(func(dir string, _, _ io.Writer) error {
		modPath := filepath.Join(dir, "go.mod")
		b, err := os.ReadFile(modPath)
		if err != nil {
			return err
		}
		updated := strings.TrimRight(string(b), "\n") + "\n\n" +
			"require github.com/erazemkos/goflex v0.0.0\n\n" +
			"replace github.com/erazemkos/goflex => " + filepath.ToSlash(frameworkRoot) + "\n"
		return os.WriteFile(modPath, []byte(updated), 0o644)
	})
	defer restore()

	tmp := t.TempDir()
	withCwd(t, tmp)

	stdout, stderr, code := runCLI("new", "myapp", "--dev")
	if code != 0 {
		t.Fatalf("new --dev code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Using latest goflex main branch via GOPROXY=direct...") {
		t.Fatalf("expected --dev banner, got: %s", stdout)
	}

	appDir := filepath.Join(tmp, "myapp")
	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = appDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}

	run("go", "mod", "tidy")

	// Compile the server binary.
	serverBin := filepath.Join(tmp, "server-bin")
	run("go", "build", "-o", serverBin, "./cmd/server")

	// Verify the GopherJS entrypoint compiles as plain Go.
	// This catches gomponents or other packages leaking into the JS bundle.
	run("go", "build", "./cmd/web")

	// Start the server on a free port.
	port := freePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	srv := exec.CommandContext(ctx, serverBin)
	srv.Dir = appDir
	srv.Env = append(os.Environ(), "PORT="+port, "GOFLEX_ENV=test")
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })

	base := "http://127.0.0.1:" + port
	waitForHTTP(t, base+"/healthz", 10*time.Second)

	// GET / must return gomponents-rendered HTML.
	resp := mustGET(t, base+"/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status=%d", resp.StatusCode)
	}
	body := readBody(t, resp)
	for _, want := range []string{
		"<!doctype html>",
		"<title>GoFlex</title>",
		"/dist/app.js",
		`id="increment"`,
		`id="decrement"`,
		`id="greeting"`,
		"Typed client",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / missing %q (first 600 bytes):\n%s", want, body[:min(600, len(body))])
		}
	}

	// GET /api/greeting must return the shared DTO.
	resp = mustGET(t, base+"/api/greeting?name=Pi")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/greeting status=%d", resp.StatusCode)
	}
	body = readBody(t, resp)
	for _, want := range []string{`"message":"Hello, Pi!"`, `"length":2`} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /api/greeting missing %q in: %s", want, body)
		}
	}
}

// ---- helpers used only by the integration test ----

func findFrameworkRoot(t *testing.T) string {
	t.Helper()
	// Prefer the environment variable (set by CI or the developer).
	if p := os.Getenv("GOFLEX_FRAMEWORK_PATH"); p != "" {
		return p
	}
	// Fall back to walking up from the current working directory, which is the
	// package directory when `go test ./internal/cli` is invoked directly.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if b, e := os.ReadFile(filepath.Join(dir, "go.mod")); e == nil &&
			strings.Contains(string(b), "module github.com/erazemkos/goflex") {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Skip("cannot locate framework root; set GOFLEX_FRAMEWORK_PATH")
		}
	}
	return ""
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()
	return port
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %s", url, timeout)
}

func mustGET(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fakeDevResolve(fn func(string, io.Writer, io.Writer) error) func() {
	old := runDevResolve
	runDevResolve = fn
	return func() { runDevResolve = old }
}

func fakeDevForTest() func() {
	oldDev := runDevServer
	oldCSS := runCSSBuild
	runDevServer = func(_ context.Context, opts devserver.Options) error {
		_, _ = opts.Out.Write([]byte("dev started\n"))
		return nil
	}
	runCSSBuild = func(frontendbuild.CSSOptions) error { return nil }
	return func() { runDevServer, runCSSBuild = oldDev, oldCSS }
}

func fakeBuildForTest() func() {
	oldBuild := runFrontendBuild
	oldCSS := runCSSBuild
	oldAssets := runAssetCopy
	oldProduction := runProductionBuild
	runFrontendBuild = func(_ context.Context, opts frontendbuild.Options) (frontendbuild.Artifacts, error) {
		return frontendbuild.Artifacts{JSPath: filepath.Join(opts.OutDir, "app.js"), SizeBytes: 1234}, nil
	}
	runCSSBuild = func(frontendbuild.CSSOptions) error { return nil }
	runAssetCopy = func(frontendbuild.AssetOptions) (frontendbuild.AssetManifest, error) { return nil, nil }
	runProductionBuild = func(_ context.Context, opts frontendbuild.ProductionOptions) error {
		lastBuild.Out = opts.Out
		lastBuild.Minify = opts.Minify
		lastBuild.Target = opts.Target
		return nil
	}
	return func() {
		runFrontendBuild, runCSSBuild, runAssetCopy, runProductionBuild = oldBuild, oldCSS, oldAssets, oldProduction
	}
}

func fakeGenerateForTest(changed bool) func() {
	old := runGenerate
	runGenerate = func(_, only string) (bool, error) {
		lastGenerate.Only = only
		return changed, nil
	}
	return func() { runGenerate = old }
}

func fakeDBForTest() func() {
	oldCreate, oldMigrate, oldRollback, oldStatus := runDBCreate, runDBMigrate, runDBRollback, runDBStatus
	runDBCreate = func(dir, name string) ([]string, error) {
		return []string{filepath.Join(dir, "001_"+name+".up.sql"), filepath.Join(dir, "001_"+name+".down.sql")}, nil
	}
	runDBMigrate = func(migrate.Config) error { return nil }
	runDBRollback = func(migrate.Config, int) error { return nil }
	runDBStatus = func(migrate.Config) (migrate.StatusInfo, error) {
		return migrate.StatusInfo{Total: 3, Applied: 1, Pending: 2}, nil
	}
	return func() {
		runDBCreate, runDBMigrate, runDBRollback, runDBStatus = oldCreate, oldMigrate, oldRollback, oldStatus
	}
}

func withCwd(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func runCLI(args ...string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := Execute(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func mustFindFlag(t *testing.T, root interface {
	Find([]string) (*cobra.Command, []string, error)
}, path []string, flag string) {
	t.Helper()
	cmd, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %v: %v", path, err)
	}
	if cmd.Flags().Lookup(flag) == nil {
		t.Fatalf("%v missing flag %s", path, flag)
	}
}
