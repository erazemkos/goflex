package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIHandlersDoNotExposeInternalModels(t *testing.T) {
	root := filepath.Join("..", "..")
	var checked int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !strings.Contains(filepath.ToSlash(path), "/internal/api/") {
			return nil
		}
		checked++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(importPath, "/internal/models") {
				t.Fatalf("%s imports server model package %s; return shared DTOs instead", path, importPath)
			}
		}
		ast.Inspect(file, func(ast.Node) bool { return false })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Log("no internal/api handlers found yet")
	}
}
