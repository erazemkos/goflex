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

func main() {
	root := js.Global.Get("document").Call("getElementById", "root")
	root.Set("innerHTML", web.Markup)
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

const Markup = "" +
	"<main class=\"min-h-screen flex flex-col items-center justify-center gap-6 bg-slate-950 text-white p-8\">" +
	"<h1 class=\"text-5xl font-bold tracking-tight\">GoFlex</h1>" +
	"<p class=\"max-w-xl text-center text-lg text-slate-300\">A Reflex-like full-stack web framework for Go, built around typed contracts and scalable package boundaries.</p>" +
	"<a href=\"https://github.com/erazemkos/goflex\" class=\"rounded bg-blue-500 px-5 py-3 font-semibold text-white hover:bg-blue-600\">View on GitHub</a>" +
	"</main>"
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

A basic GoFlex app.

## Run

`+"```sh"+`
go mod tidy
goflex dev
`+"```"+`

Open the URL printed by `+"`goflex dev`"+`.

## Build

`+"```sh"+`
goflex build --out ./bin/app
PORT=8080 ./bin/app
`+"```"+`
`, module)
}
