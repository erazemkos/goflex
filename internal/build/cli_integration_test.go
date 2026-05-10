//go:build integration

package build

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLIBuildCommandWorksIntegration(t *testing.T) {
	if _, err := lookPath("gopherjs"); err != nil {
		t.Skipf("gopherjs unavailable: %v", err)
	}
	if err := checkGo(context.Background()); err != nil {
		t.Skipf("GopherJS toolchain unavailable: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "goflex")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/goflex").CombinedOutput(); err != nil {
		t.Fatalf("go build cli: %v: %s", err, out)
	}
	outDir := t.TempDir()
	cmd := exec.Command(bin, "build", "--out", outDir)
	cmd.Dir = filepath.Join("..", "..", "examples", "hello")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("goflex build: %v: %s", err, out)
	}
	if st, err := os.Stat(filepath.Join(outDir, "app.js")); err != nil || st.Size() == 0 {
		t.Fatalf("missing app.js: %v", err)
	}
}
