//go:build integration

package build

import (
	"context"
	"strings"
	"testing"
)

func TestHelloWorldCompilesIntegration(t *testing.T) {
	if err := checkGo(context.Background()); err != nil {
		t.Skipf("GopherJS toolchain unavailable: %v", err)
	}
	res, err := Build(context.Background(), Options{Entry: "../../examples/hello", OutDir: t.TempDir(), SourceMap: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.SizeBytes <= 1024 {
		t.Fatalf("bundle too small: %d", res.SizeBytes)
	}
	if strings.Contains(strings.ToLower(res.Stderr), "error") {
		t.Fatalf("stderr contains error: %s", res.Stderr)
	}
}
