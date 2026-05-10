package main

import (
	"github.com/goflex/goflex/examples/todo/internal/web"
	"github.com/goflex/goflex/pkg/ui"
)

func main() {
	ui.Render(web.App(), nil)
}
