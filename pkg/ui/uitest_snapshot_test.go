package ui_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/goflex/goflex/internal/uitest"
	"github.com/goflex/goflex/pkg/ui"
)

func TestRenderToMockRuntimeSnapshot(t *testing.T) {
	elem := ui.Div(
		ui.Class("x"),
		ui.H1(ui.Text("Hi")),
	)
	node := uitest.Render(elem)
	b, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	golden := filepath.Join("testdata", "basic_tree.json")
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(want) {
		t.Fatalf("snapshot mismatch\ngot:\n%s\nwant:\n%s", b, want)
	}
}

func TestMockRuntimeRawRoundTrip(t *testing.T) {
	raw := &struct{ ID int }{ID: 1}
	node := uitest.Render(ui.Raw(raw))
	if node.Raw != raw {
		t.Fatalf("raw=%#v", node.Raw)
	}
}

func TestMockRuntimeCustomComponent(t *testing.T) {
	greeting := ui.Component("Greeting", func(p ui.Props) ui.Element {
		return ui.Text("Hello " + p.String("name"))
	})
	node := uitest.Render(greeting(map[string]any{"name": "World"}))
	if node.Text != "Hello World" {
		t.Fatalf("node=%#v", node)
	}
}
