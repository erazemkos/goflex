package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// stableFrameworkVersion is the released goflex version that scaffolded apps
// depend on by default. Bump this when cutting a new release.
const stableFrameworkVersion = "v0.2.1"

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
			if cfg.Dev {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Using latest goflex main branch via GOPROXY=direct...")
				if err := runDevResolve(filepath.Clean(cfg.Name), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nNext steps:\n  cd %s\n  goflex dev\n", cfg.Name)
				return nil
			}
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
	cmd.Flags().BoolVar(&cfg.Dev, "dev", false, "resolve goflex from the latest main branch via GOPROXY=direct (for goflex framework development)")
}

// runDevResolve is the hook used by `goflex new --dev` to replace the
// placeholder goflex dependency with whatever the latest main commit
// resolves to. It is overridable in tests.
var runDevResolve = resolveDevFramework

func resolveDevFramework(dir string, stdout, stderr io.Writer) error {
	getCmd := exec.Command("go", "get", "github.com/erazemkos/goflex@main")
	getCmd.Dir = dir
	getCmd.Env = append(os.Environ(), "GOPROXY=direct")
	if out, err := getCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("goflex new --dev requires network access to GitHub; "+
			"`GOPROXY=direct go get github.com/erazemkos/goflex@main` failed: %w\n%s",
			err, strings.TrimSpace(string(out)))
	} else if len(out) > 0 {
		_, _ = stderr.Write(out)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w\n%s", err, strings.TrimSpace(string(out)))
	} else if len(out) > 0 {
		_, _ = stderr.Write(out)
	}
	return nil
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
		"go.mod":                   goModTemplate(module, cfg.Dev),
		"README.md":                appReadmeTemplate(module),
		"index.html":               indexHTMLTemplate,
		"tailwind.config.css":      tailwindTemplate,
		"cmd/server/main.go":       serverMainTemplate(module),
		"cmd/web/main.go":          webMainTemplate(module),
		"internal/web/ids.go":      webIDsTemplate,
		"internal/web/page.go":     webPageTemplate,
		"internal/api/greeting.go": apiTemplate(module),
		"shared/types.go":          sharedTypesTemplate,
		"assets/.gitkeep":          "",
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
const gomponentsVersion = "v1.3.0"

func goModTemplate(module string, dev bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", module)
	b.WriteString("go 1.23\n\n")
	if dev {
		// Dev mode: leave goflex out of go.mod entirely. `goflex new --dev`
		// immediately runs `GOPROXY=direct go get github.com/erazemkos/goflex@main`,
		// which adds the resolved pseudo-version. Keeping a placeholder here would
		// make Go try to resolve v0.0.0 first and fail before `go get` runs.
		b.WriteString("require (\n")
		fmt.Fprintf(&b, "\tgithub.com/gopherjs/gopherjs %s\n", gopherJSRuntimeVersion)
		fmt.Fprintf(&b, "\tmaragu.dev/gomponents %s\n", gomponentsVersion)
		b.WriteString(")\n")
		return b.String()
	}
	var replace, version string
	replace = localFrameworkReplace()
	if replace != "" {
		version = "v0.0.0"
	} else {
		version = stableFrameworkVersion
	}
	b.WriteString("require (\n")
	fmt.Fprintf(&b, "\tgithub.com/erazemkos/goflex %s\n", version)
	fmt.Fprintf(&b, "\tgithub.com/gopherjs/gopherjs %s\n", gopherJSRuntimeVersion)
	fmt.Fprintf(&b, "\tmaragu.dev/gomponents %s\n", gomponentsVersion)
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
	"%s/shared"

	"github.com/erazemkos/goflex/pkg/reactive"
	"github.com/gopherjs/gopherjs/js"
)

func main() {
	count := reactive.NewSignal(0)
	name := reactive.NewSignal("Gopher")
	greeting := reactive.NewSignal(shared.GreetingResponse{})
	loading := reactive.NewSignal(true)
	errText := reactive.NewSignal("")

	bindText(web.IDCount, func() string {
		return itoa(count.Get())
	})
	bindText(web.IDDoubleCount, func() string {
		return itoa(count.Get() * 2)
	})
	bindText(web.IDClickSummary, func() string {
		c := count.Get()
		return "You clicked " + itoa(c) + " " + plural(c, "time", "times") + "."
	})
	bindText(web.IDGreeting, func() string {
		if loading.Get() {
			return "Loading typed API response…"
		}
		if errText.Get() != "" {
			return "API error: " + errText.Get()
		}
		return greeting.Get().Message
	})
	bindText(web.IDNameLength, func() string {
		return itoa(greeting.Get().Length)
	})
	setText(web.IDAPIPath, "/api"+shared.GreetingPath)

	byID(web.IDIncrement).Call("addEventListener", "click", func() {
		count.Update(func(c int) int { return c + 1 })
	})
	byID(web.IDDecrement).Call("addEventListener", "click", func() {
		count.Update(func(c int) int { return c - 1 })
	})
	byID(web.IDReset).Call("addEventListener", "click", func() {
		count.Set(0)
	})
	byID(web.IDNameInput).Call("addEventListener", "input", func(event *js.Object) {
		name.Set(event.Get("target").Get("value").String())
		fetchGreeting(name, greeting, loading, errText)
	})

	fetchGreeting(name, greeting, loading, errText)
}

func fetchGreeting(
	name *reactive.Signal[string],
	greeting *reactive.Signal[shared.GreetingResponse],
	loading *reactive.Signal[bool],
	errText *reactive.Signal[string],
) {
	loading.Set(true)
	errText.Set("")
	encodedName := js.Global.Call("encodeURIComponent", fallback(name.Peek(), "Gopher")).String()
	js.Global.Call("fetch", "/api"+shared.GreetingPath+"?name="+encodedName).
		Call("then", func(resp *js.Object) {
			if !resp.Get("ok").Bool() {
				errText.Set("API returned " + resp.Get("status").String())
				loading.Set(false)
				return
			}
			resp.Call("json").Call("then", func(data *js.Object) {
				greeting.Set(shared.GreetingResponse{
					Message: data.Get("message").String(),
					Source:  data.Get("source").String(),
					Length:  data.Get("length").Int(),
				})
				errText.Set("")
				loading.Set(false)
			})
		}).
		Call("catch", func(err *js.Object) {
			errText.Set(err.String())
			loading.Set(false)
		})
}

func bindText(id web.ElementID, text func() string) reactive.DisposeFunc {
	return reactive.Effect(func() {
		setText(id, text())
	})
}

func byID(id web.ElementID) *js.Object {
	return js.Global.Get("document").Call("getElementById", string(id))
}

func setText(id web.ElementID, text string) {
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
`, module, module)
}

func serverMainTemplate(module string) string {
	return fmt.Sprintf(`package main

import (
	"io/fs"
	"log"
	"os"

	"%s/internal/api"
	"%s/internal/web"
	"github.com/erazemkos/goflex/pkg/server"
)

func main() {
	app := server.New(server.Config{
		Env:       env(),
		StaticFS:  staticFS(),
		IndexHTML: []byte(web.Shell()),
	})
	app.API("", api.RegisterRoutes)
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
	return nil
}
`, module, module)
}

const webIDsTemplate = `package web

// ElementID is the typed selector for every DOM node the browser-side Go code
// touches. Keep the frontend and the template in sync by editing the constants
// below instead of stringly-typed IDs. This file is compiled by both the
// backend server (Go) and the browser entrypoint (GopherJS).
type ElementID string

const (
	IDIncrement    ElementID = "increment"
	IDDecrement    ElementID = "decrement"
	IDReset        ElementID = "reset"
	IDCount        ElementID = "count"
	IDDoubleCount  ElementID = "double-count"
	IDClickSummary ElementID = "click-summary"
	IDNameInput    ElementID = "name-input"
	IDGreeting     ElementID = "greeting"
	IDAPIPath      ElementID = "api-path"
	IDNameLength   ElementID = "name-length"
)
`

// webPageTemplate contains the gomponents-based HTML renderer. It is guarded
// by //go:build !js so it is only compiled for the backend binary -
// gomponents depends on html/template, which is not supported by GopherJS.
const webPageTemplate = `//go:build !js

package web

import (
	"bytes"

	g "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

func id(v ElementID) g.Node    { return ID(string(v)) }
func forID(v ElementID) g.Node { return For(string(v)) }

// Shell renders the full HTML document served at "/". The client entrypoint
// (compiled by GopherJS) attaches event listeners to the elements below and
// updates fine-grained reactive text bindings in place, so there is no
// client-side tree diffing.
func Shell() string {
	var buf bytes.Buffer
	_ = HTML5(HTML5Props{
		Title:    "GoFlex",
		Language: "en",
		Head: []g.Node{
			Link(Rel("stylesheet"), Href("/dist/app.css")),
		},
		Body: []g.Node{
			page(),
			Script(Src("/dist/app.js")),
		},
	}).Render(&buf)
	return buf.String()
}

// Page renders just the <main> content. Useful for tests and for embedding the
// starter page inside a custom shell.
func Page() string {
	var buf bytes.Buffer
	_ = page().Render(&buf)
	return buf.String()
}

func page() g.Node {
	return Main(Class("min-h-screen bg-slate-950 text-white"),
		Section(Class("mx-auto flex min-h-screen max-w-4xl flex-col items-center justify-center gap-8 px-6 py-12"),
			Div(Class("text-center"),
				P(Class("mb-3 text-sm font-semibold uppercase tracking-[0.3em] text-blue-300"), g.Text("GoFlex starter")),
				H1(Class("text-5xl font-bold tracking-tight sm:text-6xl"), g.Text("GoFlex")),
				P(Class("mt-5 max-w-2xl text-lg text-slate-300"), g.Text("A Reflex-like full-stack web framework for Go, built around typed contracts and scalable package boundaries.")),
			),
			Div(Class("w-full rounded-3xl border border-white/10 bg-white/5 p-6 shadow-2xl shadow-blue-950/40 backdrop-blur"),
				Div(Class("mb-6 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between"),
					Div(
						P(Class("text-sm font-semibold uppercase tracking-[0.25em] text-blue-300"), g.Text("Typed client + API demo")),
						H2(Class("mt-2 text-2xl font-bold"), g.Text("Everything below is compiled Go")),
					),
					A(Href("https://github.com/erazemkos/goflex"), Class("text-sm font-semibold text-blue-300 hover:text-blue-200"), g.Text("View on GitHub →")),
				),
				Div(Class("grid gap-4 md:grid-cols-2"),
					counterCard(),
					apiCard(),
				),
			),
		),
	)
}

func counterCard() g.Node {
	return Div(Class("rounded-2xl bg-slate-900/80 p-5"),
		P(Class("text-sm text-slate-400"), g.Text("Typed selectors + fine-grained signals")),
		Div(Class("my-5 flex items-center justify-center gap-4"),
			Button(id(IDDecrement), Class("h-12 w-12 rounded-full bg-slate-800 text-2xl font-bold hover:bg-slate-700"), g.Text("−")),
			Div(id(IDCount), Class("min-w-20 text-center text-6xl font-black text-blue-300"), g.Text("0")),
			Button(id(IDIncrement), Class("h-12 w-12 rounded-full bg-blue-500 text-2xl font-bold hover:bg-blue-400"), g.Text("+")),
		),
		P(id(IDClickSummary), Class("text-center text-slate-300"), Aria("live", "polite"), g.Text("You clicked 0 times.")),
		P(Class("mt-2 text-center text-sm text-slate-500"), g.Text("Derived value: "), Span(id(IDDoubleCount), g.Text("0"))),
		Button(id(IDReset), Class("mt-5 w-full rounded-xl border border-white/10 px-4 py-2 font-semibold text-slate-200 hover:bg-white/10"), g.Text("Reset")),
	)
}

func apiCard() g.Node {
	return Div(Class("rounded-2xl bg-slate-900/80 p-5"),
		Label(forID(IDNameInput), Class("text-sm text-slate-400"), g.Text("Typed DTO shared by frontend and backend")),
		Input(id(IDNameInput), Value("Gopher"), Class("mt-3 w-full rounded-xl border border-white/10 bg-slate-950 px-4 py-3 text-white outline-none ring-blue-400/30 focus:ring-4")),
		P(id(IDGreeting), Class("mt-6 text-3xl font-bold text-white"), Aria("live", "polite"), g.Text("Loading typed API response…")),
		P(Class("mt-3 text-slate-400"), g.Text("The browser imports shared.GreetingResponse and calls "), Code(id(IDAPIPath), g.Text("/api/greeting")), g.Text(". Name length from the typed response: "), Span(id(IDNameLength), Class("font-semibold text-blue-300"), g.Text("0")), g.Text(".")),
	)
}
`
const sharedTypesTemplate = `package shared

const GreetingPath = "/greeting"

type GreetingRequest struct {
	Name string ` + "`" + `query:"name" json:"name"` + "`" + `
}

type GreetingResponse struct {
	Message string ` + "`" + `json:"message"` + "`" + `
	Source  string ` + "`" + `json:"source"` + "`" + `
	Length  int    ` + "`" + `json:"length"` + "`" + `
}
`

func apiTemplate(module string) string {
	return fmt.Sprintf(`package api

import (
	"net/http"

	"%s/shared"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.RouterGroup) {
	r.GET(shared.GreetingPath, greeting)
}

func greeting(c *gin.Context) {
	req := shared.GreetingRequest{Name: c.Query("name")}
	if req.Name == "" {
		req.Name = "Gopher"
	}
	c.JSON(http.StatusOK, shared.GreetingResponse{
		Message: "Hello, " + req.Name + "!",
		Source:  "/api" + shared.GreetingPath,
		Length:  len(req.Name),
	})
}
`, module)
}

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

A basic GoFlex app with typed HTML, typed DOM selectors, fine-grained browser-side reactive signals, and a shared frontend/backend API DTO.

## Run

`+"```sh"+`
go mod tidy
goflex dev
`+"```"+`

Open the URL printed by `+"`goflex dev`"+`. `+"`internal/web/app.go`"+` builds HTML with a small typed DSL, `+"`cmd/web/main.go`"+` uses reactive signals to update only the DOM text nodes that depend on changed state, and `+"`shared/types.go`"+` defines the API DTO used by both frontend and backend.

## Build

`+"```sh"+`
goflex build --out ./bin/app
PORT=8080 ./bin/app
`+"```"+`
`, module)
}
