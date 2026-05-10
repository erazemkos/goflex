package main

import (
	"github.com/erazemkos/goflex/examples/todo/internal/web"
	"github.com/erazemkos/goflex/pkg/ui"
)

func main() {
	ui.Render(web.App(), nil)
}
