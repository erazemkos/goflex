package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const gopherJSInstallCommand = "go install github.com/gopherjs/gopherjs@latest"

// Options configures a GopherJS frontend build.
type Options struct {
	Entry     string
	OutDir    string
	Minify    bool
	SourceMap bool
}

// Artifacts describes files and diagnostics produced by a frontend build.
type Artifacts struct {
	JSPath     string
	MapPath    string
	SizeBytes  int64
	DurationMS int64
	Stdout     string
	Stderr     string
}

// BuildResult is kept as the public result name used in the roadmap.
type BuildResult = Artifacts

type runner func(context.Context, string, ...string) *exec.Cmd

var execCommand runner = exec.CommandContext
var lookPath = exec.LookPath
var goVersion = func(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "version")
	b, err := cmd.Output()
	return string(b), err
}
var gopherJSVersion = func(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gopherjs", "version")
	b, err := cmd.Output()
	return string(b), err
}

// FrontendEntry returns the conventional GopherJS frontend package for an app.
func FrontendEntry(dir string) string {
	if dir == "" {
		dir = "."
	}
	candidate := filepath.Join(dir, "cmd", "web")
	if st, err := os.Stat(candidate); err == nil && st.IsDir() {
		return candidate
	}
	return dir
}

// Build compiles opts.Entry into opts.OutDir/app.js using the gopherjs binary.
func Build(ctx context.Context, opts Options) (Artifacts, error) {
	if opts.Entry == "" {
		opts.Entry = "."
	}
	if opts.OutDir == "" {
		opts.OutDir = "dist"
	}
	if _, err := lookPath("gopherjs"); err != nil {
		return Artifacts{}, fmt.Errorf("gopherjs not found; install with: %s", gopherJSInstallCommand)
	}
	if err := checkGo(ctx); err != nil {
		return Artifacts{}, err
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return Artifacts{}, err
	}

	jsPath := filepath.Join(opts.OutDir, "app.js")
	args := []string{"build", "-o", jsPath}
	if opts.Minify {
		args = append(args, "--minify")
	}
	if opts.SourceMap {
		args = append(args, "--source_map=true")
	} else {
		args = append(args, "--source_map=false")
	}
	args = append(args, opts.Entry)

	start := time.Now()
	cmd := execCommand(ctx, "gopherjs", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Artifacts{JSPath: jsPath, Stdout: stdout.String(), Stderr: stderr.String(), DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		return res, fmt.Errorf("gopherjs build failed: %w: %s", err, strings.TrimSpace(res.Stderr))
	}
	st, statErr := os.Stat(jsPath)
	if statErr != nil {
		return res, statErr
	}
	res.SizeBytes = st.Size()
	if opts.SourceMap {
		res.MapPath = jsPath + ".map"
		if _, err := os.Stat(res.MapPath); err != nil {
			return res, fmt.Errorf("source map requested but %s was not produced: %w", res.MapPath, err)
		}
	}
	return res, nil
}

func checkGo(ctx context.Context) error {
	v, err := goVersion(ctx)
	if err != nil {
		return err
	}
	major, minor, ok := parseGoVersion(v)
	if !ok {
		return fmt.Errorf("unsupported Go version for GopherJS: %s", strings.TrimSpace(v))
	}
	requiredMajor, requiredMinor := 1, 20
	if gv, err := gopherJSVersion(ctx); err == nil {
		if m, n, ok := parseGoVersion(gv); ok {
			requiredMajor, requiredMinor = m, n
		}
	}
	if major != requiredMajor || minor != requiredMinor {
		return fmt.Errorf("unsupported Go version for GopherJS: %s (installed GopherJS requires Go %d.%d.x)", strings.TrimSpace(v), requiredMajor, requiredMinor)
	}
	return nil
}

func parseGoVersion(v string) (int, int, bool) {
	m := regexp.MustCompile(`go(\d+)\.(\d+)`).FindStringSubmatch(v)
	if len(m) != 3 {
		return 0, 0, false
	}
	var major, minor int
	if _, err := fmt.Sscanf(m[0], "go%d.%d", &major, &minor); err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func resetTestHooks() func() {
	oldExec := execCommand
	oldLook := lookPath
	oldGo := goVersion
	oldGopherJS := gopherJSVersion
	return func() { execCommand, lookPath, goVersion, gopherJSVersion = oldExec, oldLook, oldGo, oldGopherJS }
}
