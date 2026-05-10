package meta

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoLayout(t *testing.T) {
	dirs := []string{"cmd/goflex", "pkg/ui", "pkg/hooks", "pkg/router", "pkg/api", "pkg/query", "pkg/form", "pkg/auth", "pkg/server", "internal/build", "internal/gen", "internal/devserver", "templates/new-app", "examples/todo", "plan"}
	root := filepath.Join("..", "..")
	for _, d := range dirs {
		if st, err := os.Stat(filepath.Join(root, d)); err != nil || !st.IsDir() {
			t.Fatalf("missing directory %s: %v", d, err)
		}
	}
}
