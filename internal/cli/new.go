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
const stableFrameworkVersion = "v0.1.0"

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
		"internal/web/app.go":      webAppTemplate,
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

func goModTemplate(module string, dev bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", module)
	b.WriteString("go 1.23\n\n")
	if dev {
		// Dev mode: leave goflex out of go.mod entirely. `goflex new --dev`
		// immediately runs `GOPROXY=direct go get github.com/erazemkos/goflex@main`,
		// which adds the resolved pseudo-version. Keeping a placeholder here would
		// make Go try to resolve v0.0.0 first and fail before `go get` runs.
		fmt.Fprintf(&b, "require github.com/gopherjs/gopherjs %s\n", gopherJSRuntimeVersion)
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

	"github.com/gopherjs/gopherjs/js"
)

type appState struct {
	count    int
	name     string
	greeting shared.GreetingResponse
	loading  bool
	err      string
}

func main() {
	state := &appState{name: "Gopher"}
	byID(web.IDRoot).Set("innerHTML", web.Page())

	byID(web.IDIncrement).Call("addEventListener", "click", func() {
		state.count++
		render(state)
	})
	byID(web.IDDecrement).Call("addEventListener", "click", func() {
		state.count--
		render(state)
	})
	byID(web.IDReset).Call("addEventListener", "click", func() {
		state.count = 0
		render(state)
	})
	byID(web.IDNameInput).Call("addEventListener", "input", func(event *js.Object) {
		state.name = event.Get("target").Get("value").String()
		fetchGreeting(state)
		render(state)
	})

	fetchGreeting(state)
	render(state)
}

func fetchGreeting(state *appState) {
	state.loading = true
	state.err = ""
	name := js.Global.Call("encodeURIComponent", fallback(state.name, "Gopher")).String()
	js.Global.Call("fetch", "/api"+shared.GreetingPath+"?name="+name).
		Call("then", func(resp *js.Object) {
			if !resp.Get("ok").Bool() {
				state.loading = false
				state.err = "API returned " + resp.Get("status").String()
				render(state)
				return
			}
			resp.Call("json").Call("then", func(data *js.Object) {
				state.greeting = shared.GreetingResponse{
					Message: data.Get("message").String(),
					Source:  data.Get("source").String(),
					Length:  data.Get("length").Int(),
				}
				state.loading = false
				state.err = ""
				render(state)
			})
		}).
		Call("catch", func(err *js.Object) {
			state.loading = false
			state.err = err.String()
			render(state)
		})
}

func render(state *appState) {
	count := itoa(state.count)
	setText(web.IDCount, count)
	setText(web.IDDoubleCount, itoa(state.count*2))
	setText(web.IDClickSummary, "You clicked "+count+" "+plural(state.count, "time", "times")+".")
	if state.loading {
		setText(web.IDGreeting, "Loading typed API response…")
	} else if state.err != "" {
		setText(web.IDGreeting, "API error: "+state.err)
	} else {
		setText(web.IDGreeting, state.greeting.Message)
	}
	setText(web.IDAPIPath, "/api"+shared.GreetingPath)
	setText(web.IDNameLength, itoa(state.greeting.Length))
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
	"github.com/erazemkos/goflex/pkg/server"
)

func main() {
	app := server.New(server.Config{Env: env(), StaticFS: staticFS()})
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
	return os.DirFS(".")
}
`, module)
}

const webAppTemplate = `package web

type ElementID string

const (
	IDRoot         ElementID = "root"
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

type Attr struct{ name, value string }
type Node struct {
	tag      string
	text     string
	attrs    []Attr
	children []Node
}

func Page() string {
	return Main(A("class", "min-h-screen bg-slate-950 text-white"),
		Section(A("class", "mx-auto flex min-h-screen max-w-4xl flex-col items-center justify-center gap-8 px-6 py-12"),
			Div(A("class", "text-center"),
				P(A("class", "mb-3 text-sm font-semibold uppercase tracking-[0.3em] text-blue-300"), Text("GoFlex starter")),
				H1(A("class", "text-5xl font-bold tracking-tight sm:text-6xl"), Text("GoFlex")),
				P(A("class", "mt-5 max-w-2xl text-lg text-slate-300"), Text("A Reflex-like full-stack web framework for Go, built around typed contracts and scalable package boundaries.")),
			),
			Div(A("class", "w-full rounded-3xl border border-white/10 bg-white/5 p-6 shadow-2xl shadow-blue-950/40 backdrop-blur"),
				Div(A("class", "mb-6 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between"),
					Div(nil,
						P(A("class", "text-sm font-semibold uppercase tracking-[0.25em] text-blue-300"), Text("Typed client + API demo")),
						H2(A("class", "mt-2 text-2xl font-bold"), Text("Everything below is compiled Go")),
					),
					Link("https://github.com/erazemkos/goflex", A("class", "text-sm font-semibold text-blue-300 hover:text-blue-200"), Text("View on GitHub →")),
				),
				Div(A("class", "grid gap-4 md:grid-cols-2"),
					CounterCard(),
					APICard(),
				),
			),
		),
	).HTML()
}

func CounterCard() Node {
	return Div(A("class", "rounded-2xl bg-slate-900/80 p-5"),
		P(A("class", "text-sm text-slate-400"), Text("Typed selectors + local client state")),
		Div(A("class", "my-5 flex items-center justify-center gap-4"),
			Button(ID(IDDecrement), A("class", "h-12 w-12 rounded-full bg-slate-800 text-2xl font-bold hover:bg-slate-700"), Text("−")),
			Div(ID(IDCount), A("class", "min-w-20 text-center text-6xl font-black text-blue-300"), Text("0")),
			Button(ID(IDIncrement), A("class", "h-12 w-12 rounded-full bg-blue-500 text-2xl font-bold hover:bg-blue-400"), Text("+")),
		),
		P(ID(IDClickSummary), A("class", "text-center text-slate-300"), A("aria-live", "polite"), Text("You clicked 0 times.")),
		P(A("class", "mt-2 text-center text-sm text-slate-500"), Text("Derived value: "), Span(ID(IDDoubleCount), Text("0"))),
		Button(ID(IDReset), A("class", "mt-5 w-full rounded-xl border border-white/10 px-4 py-2 font-semibold text-slate-200 hover:bg-white/10"), Text("Reset")),
	)
}

func APICard() Node {
	return Div(A("class", "rounded-2xl bg-slate-900/80 p-5"),
		Label(IDFor(IDNameInput), A("class", "text-sm text-slate-400"), Text("Typed DTO shared by frontend and backend")),
		Input(ID(IDNameInput), A("value", "Gopher"), A("class", "mt-3 w-full rounded-xl border border-white/10 bg-slate-950 px-4 py-3 text-white outline-none ring-blue-400/30 focus:ring-4")),
		P(ID(IDGreeting), A("class", "mt-6 text-3xl font-bold text-white"), A("aria-live", "polite"), Text("Loading typed API response…")),
		P(A("class", "mt-3 text-slate-400"), Text("The browser imports shared.GreetingResponse and calls "), Code(ID(IDAPIPath), Text("/api/greeting")), Text(". Name length from the typed response: "), Span(ID(IDNameLength), A("class", "font-semibold text-blue-300"), Text("0")), Text(".")),
	)
}

func A(name, value string) Attr { return Attr{name: name, value: value} }
func ID(id ElementID) Attr      { return A("id", string(id)) }
func IDFor(id ElementID) Attr   { return A("for", string(id)) }

func Text(value string) Node { return Node{text: value} }
func El(tag string, args ...any) Node {
	node := Node{tag: tag}
	for _, arg := range args {
		switch v := arg.(type) {
		case nil:
		case Attr:
			node.attrs = append(node.attrs, v)
		case Node:
			node.children = append(node.children, v)
		case string:
			node.children = append(node.children, Text(v))
		}
	}
	return node
}

func Main(args ...any) Node    { return El("main", args...) }
func Section(args ...any) Node { return El("section", args...) }
func Div(args ...any) Node     { return El("div", args...) }
func P(args ...any) Node       { return El("p", args...) }
func H1(args ...any) Node      { return El("h1", args...) }
func H2(args ...any) Node      { return El("h2", args...) }
func Span(args ...any) Node    { return El("span", args...) }
func Code(args ...any) Node    { return El("code", args...) }
func Label(args ...any) Node   { return El("label", args...) }
func Button(args ...any) Node  { return El("button", args...) }
func Input(args ...any) Node   { return El("input", args...) }
func Link(href string, args ...any) Node {
	return El("a", append([]any{A("href", href)}, args...)...)
}

func (n Node) HTML() string {
	if n.tag == "" {
		return n.text
	}
	out := "<" + n.tag
	for _, attr := range n.attrs {
		out += " " + attr.name + "=\"" + attr.value + "\""
	}
	out += ">"
	for _, child := range n.children {
		out += child.HTML()
	}
	out += "</" + n.tag + ">"
	return out
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

A basic GoFlex app with typed HTML, typed DOM selectors, browser-side Go state, and a shared frontend/backend API DTO.

## Run

`+"```sh"+`
go mod tidy
goflex dev
`+"```"+`

Open the URL printed by `+"`goflex dev`"+`. `+"`internal/web/app.go`"+` builds HTML with a small typed DSL, `+"`cmd/web/main.go`"+` keeps state in the browser with Go compiled to JavaScript, and `+"`shared/types.go`"+` defines the API DTO used by both frontend and backend.

## Build

`+"```sh"+`
goflex build --out ./bin/app
PORT=8080 ./bin/app
`+"```"+`
`, module)
}
