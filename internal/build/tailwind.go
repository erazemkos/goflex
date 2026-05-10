package build

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const TailwindVersion = "v4.1.13"

type CSSOptions struct {
	Dir      string
	Out      string
	Config   string
	CacheDir string
	Minify   bool
}

type TailwindOptions struct {
	Version  string
	CacheDir string
	GOOS     string
	GOARCH   string
}

var (
	tailwindDownload     = downloadFile
	tailwindCommand      = exec.CommandContext
	tailwindUserCacheDir = os.UserCacheDir
	tailwindSHA256       = map[string]string{
		TailwindVersion + "/linux/amd64":   "b9ed9f8f640d3323711f9f68608aa266dff3adbc42e867c38ea2d009b973be11",
		TailwindVersion + "/linux/arm64":   "c90529475a398adbf3315898721c0f9fe85f434a2b3ea3eafada68867641819a",
		TailwindVersion + "/darwin/amd64":  "c3b230bdbfaa46c94cad8db44da1f82773f10bac54f56fa196c8977d819c09e4",
		TailwindVersion + "/darwin/arm64":  "c47681e9948db20026a913a4aca4ee0269b4c0d4ef3f71343cb891dfdc1e97c9",
		TailwindVersion + "/windows/amd64": "ad16a528e13111e5df4e771b4b4981bd4b73e69140fa021f4102f46f02eeb86d",
	}
)

func BuildCSS(opts CSSOptions) error {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}
	if opts.Out == "" {
		opts.Out = filepath.Join(dir, "dist", "app.css")
	} else if !filepath.IsAbs(opts.Out) {
		opts.Out = filepath.Join(dir, opts.Out)
	}
	config := opts.Config
	if config == "" {
		config = findTailwindConfig(dir)
	} else if !filepath.IsAbs(config) {
		config = filepath.Join(dir, config)
	}
	if config == "" {
		return nil
	}
	classes, err := ExtractClasses(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.Out), 0o755); err != nil {
		return err
	}
	if err := writeClassesFile(dir, classes); err != nil {
		return err
	}
	if bin, err := EnsureTailwind(context.Background(), TailwindOptions{CacheDir: opts.CacheDir}); err == nil {
		if err := runTailwind(context.Background(), bin, dir, config, opts.Out, opts.Minify); err == nil {
			return nil
		}
	}
	return writeFallbackCSS(opts.Out, classes)
}

func EnsureTailwind(ctx context.Context, opts TailwindOptions) (string, error) {
	if opts.Version == "" {
		opts.Version = TailwindVersion
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	asset, key, err := tailwindAsset(opts.GOOS, opts.GOARCH)
	if err != nil {
		return "", err
	}
	expected := expectedTailwindSHA(opts.Version, key)
	if expected == "" {
		return "", fmt.Errorf("no pinned Tailwind SHA256 for %s %s", opts.Version, key)
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		base, err := tailwindUserCacheDir()
		if err != nil {
			return "", err
		}
		cacheDir = filepath.Join(base, "goflex")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	bin := filepath.Join(cacheDir, "tailwindcss-"+opts.Version+"-"+asset)
	if ok, err := fileSHA256Matches(bin, expected); err == nil && ok {
		return bin, nil
	}
	_ = os.Remove(bin)
	tmp := bin + ".tmp"
	_ = os.Remove(tmp)
	url := "https://github.com/tailwindlabs/tailwindcss/releases/download/" + opts.Version + "/" + asset
	if err := tailwindDownload(ctx, url, tmp); err != nil {
		return "", err
	}
	if ok, err := fileSHA256Matches(tmp, expected); err != nil || !ok {
		_ = os.Remove(tmp)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("downloaded Tailwind binary SHA256 mismatch")
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, bin); err != nil {
		return "", err
	}
	return bin, nil
}

func tailwindAsset(goos, goarch string) (asset, key string, err error) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "tailwindcss-linux-x64", "linux/amd64", nil
	case "linux/arm64":
		return "tailwindcss-linux-arm64", "linux/arm64", nil
	case "darwin/amd64":
		return "tailwindcss-macos-x64", "darwin/amd64", nil
	case "darwin/arm64":
		return "tailwindcss-macos-arm64", "darwin/arm64", nil
	case "windows/amd64":
		return "tailwindcss-windows-x64.exe", "windows/amd64", nil
	case "windows/arm64":
		return "tailwindcss-windows-arm64.exe", "windows/arm64", nil
	default:
		return "", "", fmt.Errorf("unsupported Tailwind platform %s/%s", goos, goarch)
	}
}

func expectedTailwindSHA(version, key string) string {
	if override := os.Getenv("GOFLEX_TAILWIND_SHA256"); override != "" {
		return override
	}
	return tailwindSHA256[version+"/"+key]
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fileSHA256Matches(path, want string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == strings.ToLower(want), nil
}

func runTailwind(ctx context.Context, bin, dir, config, out string, minify bool) error {
	input := filepath.Join(dir, ".goflex-tailwind.input.css")
	inputCSS := "@import \"tailwindcss\";\n@source \"./**/*.go\";\n@source \"./.goflex/classes.txt\";\n"
	args := []string{"-i", relPath(dir, input), "-o", relPath(dir, out)}
	if strings.HasSuffix(config, ".css") {
		b, err := os.ReadFile(config)
		if err != nil {
			return err
		}
		inputCSS = string(b) + "\n@source \"./.goflex/classes.txt\";\n"
	} else if strings.HasSuffix(config, ".js") || strings.HasSuffix(config, ".cjs") || strings.HasSuffix(config, ".mjs") {
		args = append(args, "--config", relPath(dir, config))
	}
	if err := os.WriteFile(input, []byte(inputCSS), 0o644); err != nil {
		return err
	}
	defer func() { _ = os.Remove(input) }()
	if minify {
		args = append(args, "--minify")
	}
	cmd := tailwindCommand(ctx, bin, args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailwindcss failed: %w: %s", err, strings.TrimSpace(string(b)))
	}
	return nil
}

func relPath(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}
	return filepath.ToSlash(rel)
}

func findTailwindConfig(dir string) string {
	for _, name := range []string{"tailwind.config.css", "tailwind.config.js", "tailwind.config.cjs", "tailwind.config.mjs"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func writeClassesFile(dir string, classes []string) error {
	if err := os.MkdirAll(filepath.Join(dir, ".goflex"), 0o755); err != nil {
		return err
	}
	return writeLines(filepath.Join(dir, ".goflex", "classes.txt"), classes)
}

func writeLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return w.Flush()
}

func ExtractClasses(dir string) ([]string, error) {
	set := map[string]struct{}{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".goflex", "dist", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		classes, err := extractClassesFromGo(path)
		if err != nil {
			return err
		}
		for _, c := range classes {
			set[c] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

func extractClassesFromGo(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := selectorName(call.Fun)
		switch name {
		case "ui.Class", "ui.ClassIf", "ui.Tw", "Class", "ClassIf", "Tw":
			for _, arg := range call.Args {
				for _, c := range stringLiteralClasses(arg) {
					set[c] = struct{}{}
				}
			}
		case "ui.ClassMap", "ClassMap":
			for _, c := range classMapKeys(call.Args) {
				set[c] = struct{}{}
			}
		}
		return true
	})
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

func selectorName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		if id, ok := v.X.(*ast.Ident); ok {
			return id.Name + "." + v.Sel.Name
		}
	case *ast.Ident:
		return v.Name
	}
	return ""
}

func stringLiteralClasses(expr ast.Expr) []string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return nil
	}
	return strings.Fields(s)
}

func classMapKeys(args []ast.Expr) []string {
	var out []string
	for _, arg := range args {
		lit, ok := arg.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok || !boolLiteral(kv.Value) {
				continue
			}
			out = append(out, stringLiteralClasses(kv.Key)...)
		}
	}
	return out
}

func boolLiteral(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "true"
}

func writeFallbackCSS(out string, classes []string) error {
	var sb strings.Builder
	sb.WriteString("/* Generated by GoFlex fallback Tailwind renderer. */\n")
	for _, class := range classes {
		sb.WriteString(".")
		sb.WriteString(cssEscapeClass(class))
		sb.WriteString("{")
		sb.WriteString(fallbackDecl(class))
		sb.WriteString("}\n")
	}
	return os.WriteFile(out, []byte(sb.String()), 0o644)
}

func cssEscapeClass(class string) string {
	var sb strings.Builder
	for _, r := range class {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			sb.WriteRune(r)
			continue
		}
		sb.WriteByte('\\')
		sb.WriteRune(r)
	}
	return sb.String()
}

func fallbackDecl(class string) string {
	base := class
	if i := strings.LastIndex(base, ":"); i >= 0 {
		base = base[i+1:]
	}
	if strings.HasPrefix(base, "text-") {
		if color, ok := fallbackColors[strings.TrimPrefix(base, "text-")]; ok {
			return "color:" + color + ";"
		}
	}
	if strings.HasPrefix(base, "bg-") {
		if color, ok := fallbackColors[strings.TrimPrefix(base, "bg-")]; ok {
			return "background-color:" + color + ";"
		}
	}
	if decl := fallbackSpacing(base); decl != "" {
		return decl
	}
	return ""
}

var fallbackColors = map[string]string{
	"red-500":    "rgb(239 68 68)",
	"blue-500":   "rgb(59 130 246)",
	"green-500":  "rgb(34 197 94)",
	"yellow-500": "rgb(234 179 8)",
	"gray-500":   "rgb(107 114 128)",
	"white":      "rgb(255 255 255)",
	"black":      "rgb(0 0 0)",
}

func fallbackSpacing(base string) string {
	for _, item := range []struct{ prefix, decl string }{
		{"px-", "padding-left:%s;padding-right:%s;"}, {"py-", "padding-top:%s;padding-bottom:%s;"},
		{"pt-", "padding-top:%s;"}, {"pr-", "padding-right:%s;"}, {"pb-", "padding-bottom:%s;"}, {"pl-", "padding-left:%s;"}, {"p-", "padding:%s;"},
		{"mx-", "margin-left:%s;margin-right:%s;"}, {"my-", "margin-top:%s;margin-bottom:%s;"},
		{"mt-", "margin-top:%s;"}, {"mr-", "margin-right:%s;"}, {"mb-", "margin-bottom:%s;"}, {"ml-", "margin-left:%s;"}, {"m-", "margin:%s;"},
	} {
		if strings.HasPrefix(base, item.prefix) {
			if v := spacingValue(strings.TrimPrefix(base, item.prefix)); v != "" {
				if strings.Count(item.decl, "%s") == 2 {
					return fmt.Sprintf(item.decl, v, v)
				}
				return fmt.Sprintf(item.decl, v)
			}
		}
	}
	return ""
}

func spacingValue(scale string) string {
	if scale == "0" {
		return "0px"
	}
	if n, err := strconv.Atoi(scale); err == nil {
		return fmt.Sprintf("%grem", float64(n)*0.25)
	}
	return ""
}
