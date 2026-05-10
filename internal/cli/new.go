package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCmd() *cobra.Command {
	cfg := NewConfig{Template: "default"}
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "scaffold a new app",
		Args:  requireArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			if cfg.Module == "" {
				cfg.Module = filepath.Base(filepath.Clean(cfg.Name))
			}
			lastNew = cfg
			if err := scaffoldNewApp(cfg); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "created GoFlex app %s\n\n", cfg.Name)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Next steps:\n  cd %s\n  go mod tidy\n  goflex dev\n", cfg.Name)
			return nil
		},
	}
	newFlags(cmd, &cfg)
	return cmd
}

func newFlags(cmd *cobra.Command, cfg *NewConfig) {
	cmd.Flags().StringVar(&cfg.Template, "template", "default", "template name")
	cmd.Flags().StringVar(&cfg.Module, "module", "", "module path")
}

func scaffoldNewApp(cfg NewConfig) error {
	if cfg.Template != "default" && cfg.Template != "basic" {
		return fmt.Errorf("unknown template %q", cfg.Template)
	}
	target := filepath.Clean(cfg.Name)
	if target == "." || target == string(filepath.Separator) {
		return fmt.Errorf("invalid project name %q", cfg.Name)
	}
	if st, err := os.Stat(target); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("%s already exists and is not a directory", target)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("%s already exists and is not empty", target)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
	} else {
		return err
	}
	module := strings.TrimSpace(cfg.Module)
	if module == "" {
		module = filepath.Base(target)
	}
	files := map[string]string{
		"go.mod":                goModTemplate(module),
		"README.md":             appReadmeTemplate(module),
		"index.html":            indexHTMLTemplate,
		"tailwind.config.css":   tailwindTemplate,
		"cmd/server/main.go":    serverMainTemplate,
		"cmd/web/main.go":       webMainTemplate(module),
		"internal/web/app.go":   webAppTemplate,
		"assets/.gitkeep":       "",
		"shared/.gitkeep":       "",
		"internal/api/.gitkeep": "",
	}
	for rel, content := range files {
		if err := writeScaffoldFile(filepath.Join(target, rel), content); err != nil {
			return err
		}
	}
	return nil
}

func writeScaffoldFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

const gopherJSRuntimeVersion = "v1.20.2"

func goModTemplate(module string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", module)
	b.WriteString("go 1.23\n\n")
	replace := localFrameworkReplace()
	frameworkVersion := "main"
	if replace != "" {
		frameworkVersion = "v0.0.0"
	}
	b.WriteString("require (\n")
	fmt.Fprintf(&b, "\tgithub.com/erazemkos/goflex %s\n", frameworkVersion)
	fmt.Fprintf(&b, "\tgithub.com/gopherjs/gopherjs %s\n", gopherJSRuntimeVersion)
	b.WriteString(")\n")
	if replace != "" {
		fmt.Fprintf(&b, "\nreplace github.com/erazemkos/goflex => %s\n", filepath.ToSlash(replace))
	}
	return b.String()
}

func localFrameworkReplace() string {
	if p := strings.TrimSpace(os.Getenv("GOFLEX_FRAMEWORK_PATH")); p != "" {
		if abs, err := filepath.Abs(p); err == nil && isFrameworkRoot(abs) {
			return abs
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if isFrameworkRoot(wd) {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return ""
		}
		wd = parent
	}
}

func isFrameworkRoot(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && strings.Contains(string(b), "module github.com/erazemkos/goflex")
}

func webMainTemplate(module string) string {
	return fmt.Sprintf(`package main

import (
	"%s/internal/web"

	"github.com/gopherjs/gopherjs/js"
)

type appState struct {
	count int
	name  string
}

func main() {
	state := &appState{name: "Gopher"}
	root := byID("root")
	root.Set("innerHTML", web.Markup)

	byID("increment").Call("addEventListener", "click", func() {
		state.count++
		render(state)
	})
	byID("decrement").Call("addEventListener", "click", func() {
		state.count--
		render(state)
	})
	byID("reset").Call("addEventListener", "click", func() {
		state.count = 0
		render(state)
	})
	byID("name-input").Call("addEventListener", "input", func(event *js.Object) {
		state.name = event.Get("target").Get("value").String()
		render(state)
	})

	render(state)
}

func render(state *appState) {
	count := itoa(state.count)
	setText("count", count)
	setText("double-count", itoa(state.count*2))
	setText("click-summary", "You clicked " + count + " " + plural(state.count, "time", "times") + ".")
	setText("greeting", "Hello, " + fallback(state.name, "Gopher") + "!")
	byID("name-preview").Set("textContent", fallback(state.name, "Gopher"))
}

func byID(id string) *js.Object {
	return js.Global.Get("document").Call("getElementById", id)
}

func setText(id, text string) {
	byID(id).Set("textContent", text)
}

func fallback(value, otherwise string) string {
	if value == "" {
		return otherwise
	}
	return value
}

func plural(n int, one, many string) string {
	if n == 1 || n == -1 {
		return one
	}
	return many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
`, module)
}

const serverMainTemplate = `package main

import (
	"io/fs"
	"log"
	"os"

	"github.com/erazemkos/goflex/pkg/server"
)

func main() {
	app := server.New(server.Config{Env: env(), StaticFS: staticFS()})
	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	log.Fatal(app.Run(addr))
}

func env() string {
	if v := os.Getenv("GOFLEX_ENV"); v != "" {
		return v
	}
	return "dev"
}

func staticFS() fs.FS {
	if _, err := os.Stat("dist"); err == nil {
		return os.DirFS("dist")
	}
	return os.DirFS(".")
}
`

const webAppTemplate = `package web

const Markup = ` + "`" + `
<main class="min-h-screen bg-slate-950 text-white">
  <section class="mx-auto flex min-h-screen max-w-4xl flex-col items-center justify-center gap-8 px-6 py-12">
    <div class="text-center">
      <p class="mb-3 text-sm font-semibold uppercase tracking-[0.3em] text-blue-300">GoFlex starter</p>
      <h1 class="text-5xl font-bold tracking-tight sm:text-6xl">GoFlex</h1>
      <p class="mt-5 max-w-2xl text-lg text-slate-300">A Reflex-like full-stack web framework for Go, built around typed contracts and scalable package boundaries.</p>
    </div>

    <div class="w-full rounded-3xl border border-white/10 bg-white/5 p-6 shadow-2xl shadow-blue-950/40 backdrop-blur">
      <div class="mb-6 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p class="text-sm font-semibold uppercase tracking-[0.25em] text-blue-300">Client-side reactivity</p>
          <h2 class="mt-2 text-2xl font-bold">Stateful UI written in Go</h2>
        </div>
        <a href="https://github.com/erazemkos/goflex" class="text-sm font-semibold text-blue-300 hover:text-blue-200">View on GitHub →</a>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="rounded-2xl bg-slate-900/80 p-5">
          <p class="text-sm text-slate-400">Counter state</p>
          <div class="my-5 flex items-center justify-center gap-4">
            <button id="decrement" class="h-12 w-12 rounded-full bg-slate-800 text-2xl font-bold hover:bg-slate-700">−</button>
            <div id="count" class="min-w-20 text-center text-6xl font-black text-blue-300">0</div>
            <button id="increment" class="h-12 w-12 rounded-full bg-blue-500 text-2xl font-bold hover:bg-blue-400">+</button>
          </div>
          <p id="click-summary" class="text-center text-slate-300" aria-live="polite">You clicked 0 times.</p>
          <p class="mt-2 text-center text-sm text-slate-500">Derived value: <span id="double-count">0</span></p>
          <button id="reset" class="mt-5 w-full rounded-xl border border-white/10 px-4 py-2 font-semibold text-slate-200 hover:bg-white/10">Reset</button>
        </div>

        <div class="rounded-2xl bg-slate-900/80 p-5">
          <label for="name-input" class="text-sm text-slate-400">Reactive input</label>
          <input id="name-input" value="Gopher" class="mt-3 w-full rounded-xl border border-white/10 bg-slate-950 px-4 py-3 text-white outline-none ring-blue-400/30 focus:ring-4" />
          <p id="greeting" class="mt-6 text-3xl font-bold text-white" aria-live="polite">Hello, Gopher!</p>
          <p class="mt-3 text-slate-400">The browser app keeps state in Go, listens to DOM events, and re-renders the changed text nodes. Current name: <span id="name-preview" class="font-semibold text-blue-300">Gopher</span>.</p>
        </div>
      </div>
    </div>
  </section>
</main>
` + "`" + `
`

const indexHTMLTemplate = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>GoFlex App</title>
    <link rel="stylesheet" href="/dist/app.css">
  </head>
  <body>
    <div id="root"></div>
    <script src="/dist/app.js"></script>
  </body>
</html>
`

const tailwindTemplate = `@import "tailwindcss";
@source "./**/*.go";
`

func appReadmeTemplate(module string) string {
	return fmt.Sprintf(`# %s

A basic GoFlex app with a small client-side reactivity demo written in Go.

## Run

`+"```sh"+`
go mod tidy
goflex dev
`+"```"+`

Open the URL printed by `+"`goflex dev`"+`. The browser entrypoint in `+"`cmd/web/main.go`"+` shows how to keep state in Go, listen to DOM events, and update the page without a full reload.

## Build

`+"```sh"+`
goflex build --out ./bin/app
PORT=8080 ./bin/app
`+"```"+`
`, module)
}
