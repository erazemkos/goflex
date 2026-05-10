package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for _, file := range []string{"go.mod", "index.html", "tailwind.config.css", "cmd/server/main.go", "cmd/web/main.go", "internal/web/app.go", "internal/api/greeting.go", "shared/types.go", "assets/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(tmp, "myapp", file)); err != nil {
			t.Fatalf("missing %s: %v", file, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(tmp, "myapp", "internal", "web", "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "GoFlex") || !strings.Contains(string(b), "Typed client + API demo") || !strings.Contains(string(b), "https://github.com/erazemkos/goflex") {
		t.Fatalf("bad app template:\n%s", b)
	}
	webMain, err := os.ReadFile(filepath.Join(tmp, "myapp", "cmd", "web", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(webMain), "addEventListener") || !strings.Contains(string(webMain), "shared.GreetingResponse") || !strings.Contains(string(webMain), "fetchGreeting(state)") {
		t.Fatalf("bad web main template:\n%s", webMain)
	}
	mod, err := os.ReadFile(filepath.Join(tmp, "myapp", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modText := string(mod)
	if !strings.Contains(modText, "module example.com/myapp") ||
		!strings.Contains(modText, "github.com/erazemkos/goflex v0.0.0") ||
		!strings.Contains(modText, "github.com/gopherjs/gopherjs v1.20.2") ||
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
	if !strings.Contains(modText, "github.com/erazemkos/goflex v0.1.0") {
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
