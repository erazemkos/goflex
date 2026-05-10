package build

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
)

type ProductionOptions struct {
	Dir, Out, Target string
	Minify           bool
}

type Manifest map[string]string

type DistResult struct {
	Dir      string
	Manifest Manifest
	JSName   string
	CSSName  string
}

var (
	productionBuildFrontend = Build
	productionBuildCSS      = BuildCSS
	productionCopyAssets    = CopyAssets
	productionCommand       = exec.CommandContext
	productionGitSHA        = gitSHA
)

func Production(ctx context.Context, opts ProductionOptions) error {
	res, err := BuildDist(ctx, opts)
	if err != nil {
		return err
	}
	targets := splitTargets(opts.Target)
	if len(targets) == 0 {
		targets = []string{""}
	}
	for _, target := range targets {
		out := opts.Out
		if out == "" {
			out = filepath.Join(opts.Dir, "bin", "app")
		}
		if len(targets) > 1 {
			out = targetOut(out, target)
		}
		compileOpts := opts
		compileOpts.Out = out
		compileOpts.Target = target
		if err := compileServer(ctx, compileOpts, res.Dir); err != nil {
			return err
		}
	}
	return nil
}

func BuildDist(ctx context.Context, opts ProductionOptions) (DistResult, error) {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return DistResult{}, err
	}
	dist := filepath.Join(dir, "dist")
	if err := os.RemoveAll(dist); err != nil {
		return DistResult{}, err
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return DistResult{}, err
	}
	if _, err := productionBuildFrontend(ctx, Options{Entry: FrontendEntry(dir), OutDir: dist, Minify: true, SourceMap: false}); err != nil {
		return DistResult{}, err
	}
	if err := productionBuildCSS(CSSOptions{Dir: dir, Out: filepath.Join(dist, "app.css"), Minify: true}); err != nil {
		return DistResult{}, err
	}
	assetManifest, err := productionCopyAssets(AssetOptions{Dir: dir, Out: dist})
	if err != nil {
		return DistResult{}, err
	}
	js, err := os.ReadFile(filepath.Join(dist, "app.js"))
	if err != nil {
		return DistResult{}, fmt.Errorf("production frontend did not produce app.js: %w", err)
	}
	css, err := os.ReadFile(filepath.Join(dist, "app.css"))
	if err != nil {
		css = []byte{}
	}
	manifest := Manifest{}
	jsName, err := writeFingerprinted(dist, "app.js", js)
	if err != nil {
		return DistResult{}, err
	}
	cssName, err := writeFingerprinted(dist, "app.css", css)
	if err != nil {
		return DistResult{}, err
	}
	manifest["app.js"] = jsName
	manifest["app.css"] = cssName
	for k, v := range assetManifest {
		manifest[k] = v
	}
	if err := writeCompressed(filepath.Join(dist, jsName), js); err != nil {
		return DistResult{}, err
	}
	if err := writeCompressed(filepath.Join(dist, cssName), css); err != nil {
		return DistResult{}, err
	}
	_ = os.Remove(filepath.Join(dist, "app.js"))
	_ = os.Remove(filepath.Join(dist, "app.css"))
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		index = []byte(defaultProductionHTML)
	}
	idx := renderIndex(string(index), manifest)
	if err := writeReproducible(filepath.Join(dist, "index.html"), []byte(idx), 0o644); err != nil {
		return DistResult{}, err
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return DistResult{}, err
	}
	mb = append(mb, '\n')
	if err := writeReproducible(filepath.Join(dist, "manifest.json"), mb, 0o644); err != nil {
		return DistResult{}, err
	}
	return DistResult{Dir: dist, Manifest: manifest, JSName: jsName, CSSName: cssName}, nil
}

const defaultProductionHTML = `<!doctype html><html><head><title>GoFlex App</title></head><body><div id="root"></div></body></html>`

func splitTargets(target string) []string {
	var out []string
	for _, part := range strings.Split(target, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func targetOut(out, target string) string {
	if target == "" {
		return out
	}
	ext := filepath.Ext(out)
	stem := strings.TrimSuffix(out, ext)
	return stem + "-" + strings.ReplaceAll(target, "/", "-") + ext
}

func writeFingerprinted(dir, logical string, b []byte) (string, error) {
	ext := filepath.Ext(logical)
	base := strings.TrimSuffix(filepath.Base(logical), ext)
	s := sha256.Sum256(b)
	name := fmt.Sprintf("%s.%s%s", base, hex.EncodeToString(s[:])[:8], ext)
	return name, writeReproducible(filepath.Join(dir, name), b, 0o644)
}

func writeCompressed(filename string, b []byte) error {
	gz, err := gzipBytes(b)
	if err != nil {
		return err
	}
	if err := writeReproducible(filename+".gz", gz, 0o644); err != nil {
		return err
	}
	br := brotliBytes(b)
	return writeReproducible(filename+".br", br, 0o644)
}

func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	zw.Name = ""
	zw.ModTime = sourceDateEpoch()
	if _, err := zw.Write(b); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func brotliBytes(b []byte) []byte {
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, brotli.BestCompression)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

func sourceDateEpoch() time.Time {
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		var sec int64
		if _, err := fmt.Sscanf(v, "%d", &sec); err == nil {
			return time.Unix(sec, 0).UTC()
		}
	}
	return time.Unix(0, 0).UTC()
}

func writeReproducible(name string, b []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(name, b, perm); err != nil {
		return err
	}
	return os.Chtimes(name, sourceDateEpoch(), sourceDateEpoch())
}

func renderIndex(index string, manifest Manifest) string {
	css := "/dist/" + manifest["app.css"]
	js := "/dist/" + manifest["app.js"]
	replacements := map[string]string{
		"{{.CSS}}":     css,
		"{{.JS}}":      js,
		"{{.CSSHash}}": strings.TrimSuffix(strings.TrimPrefix(manifest["app.css"], "app."), ".css"),
		"{{.JSHash}}":  strings.TrimSuffix(strings.TrimPrefix(manifest["app.js"], "app."), ".js"),
	}
	out := index
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	if !strings.Contains(out, css) {
		link := `<link rel="stylesheet" href="` + css + `">`
		if strings.Contains(out, "</head>") {
			out = strings.Replace(out, "</head>", link+"</head>", 1)
		} else {
			out = link + out
		}
	}
	if !strings.Contains(out, js) {
		script := `<script src="` + js + `"></script>`
		if strings.Contains(out, "</body>") {
			out = strings.Replace(out, "</body>", script+"</body>", 1)
		} else {
			out += script
		}
	}
	return out
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		return writeReproducible(out, b, 0o644)
	})
}

func GenerateDistEmbed(workDir, dist string) error {
	webDir := filepath.Join(workDir, "internal", "web")
	if err := os.RemoveAll(filepath.Join(webDir, "dist")); err != nil {
		return err
	}
	if err := copyTree(dist, filepath.Join(webDir, "dist")); err != nil {
		return err
	}
	src := `package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFS embed.FS

func DistFS() fs.FS { return distFS }
`
	return writeReproducible(filepath.Join(webDir, "dist_embed.go"), []byte(src), 0o644)
}

func compileServer(ctx context.Context, opts ProductionOptions, dist string) error {
	if opts.Dir == "" {
		opts.Dir = "."
	}
	if opts.Out == "" {
		opts.Out = filepath.Join(opts.Dir, "bin", "app")
	}
	work := filepath.Join(opts.Dir, ".goflex", "build")
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := GenerateDistEmbed(work, dist); err != nil {
		return err
	}
	mainSrc := generatedServerMain()
	if err := writeReproducible(filepath.Join(work, "main.go"), []byte(mainSrc), 0o644); err != nil {
		return err
	}
	if err := writeReproducible(filepath.Join(work, "go.mod"), []byte("module goflexapp\n\ngo 1.22\n"), 0o644); err != nil {
		return err
	}
	outPath, err := filepath.Abs(opts.Out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	ldflags := "-buildid= -X main.buildVersion=" + productionGitSHA(ctx, opts.Dir)
	args := []string{"build", "-trimpath", "-ldflags", ldflags, "-o", outPath, "."}
	cmd := productionCommand(ctx, "go", args...)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if opts.Target != "" {
		parts := strings.Split(opts.Target, "/")
		if len(parts) == 2 {
			cmd.Env = append(cmd.Env, "GOOS="+parts[0], "GOARCH="+parts[1])
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build production server: %w: %s", err, out)
	}
	return nil
}

func gitSHA(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--short=12", "HEAD")
	cmd.Dir = dir
	b, err := cmd.Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(b))
}

func generatedServerMain() string {
	return `package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode"

	"goflexapp/internal/web"
)

var buildVersion = "dev"

func main() {
	port := os.Getenv("PORT")
	if port == "" { port = "8080" }
	dist, err := fs.Sub(web.DistFS(), "dist")
	if err != nil { log.Fatal(err) }
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/api/healthz", health)
	mux.HandleFunc("/dist/", func(w http.ResponseWriter, r *http.Request) { serveStatic(dist, strings.TrimPrefix(r.URL.Path, "/dist/"), w, r) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." { name = "index.html" }
		if strings.HasPrefix(name, "dist/") { serveStatic(dist, strings.TrimPrefix(name, "dist/"), w, r); return }
		if b, err := fs.ReadFile(dist, name); err == nil { writeStatic(w, r, name, b); return }
		if looksAsset(name) { http.NotFound(w, r); return }
		if r.URL.Path == "/" || strings.Contains(r.Header.Get("Accept"), "text/html") {
			b, _ := fs.ReadFile(dist, "index.html")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	})
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status":"ok","version":buildVersion})
}

func serveStatic(fsys fs.FS, name string, w http.ResponseWriter, r *http.Request) {
	name = path.Clean(strings.TrimPrefix(name, "/"))
	if name == "." || name == "" { name = "index.html" }
	if name == "index.html" {
		b, err := fs.ReadFile(fsys, "index.html"); if err != nil { http.NotFound(w,r); return }
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b); return
	}
	encoding := ""
	readName := name
	accept := r.Header.Get("Accept-Encoding")
	if strings.Contains(accept, "br") { if b, err := fs.ReadFile(fsys, name+".br"); err == nil { writeStaticEncoded(w, r, name, b, "br"); return } }
	if strings.Contains(accept, "gzip") { if b, err := fs.ReadFile(fsys, name+".gz"); err == nil { writeStaticEncoded(w, r, name, b, "gzip"); return } }
	_ = encoding
	b, err := fs.ReadFile(fsys, readName); if err != nil { http.NotFound(w,r); return }
	writeStatic(w, r, name, b)
}

func writeStaticEncoded(w http.ResponseWriter, r *http.Request, name string, b []byte, enc string) {
	w.Header().Set("Content-Encoding", enc)
	w.Header().Set("Vary", "Accept-Encoding")
	writeStatic(w, r, name, b)
}

func writeStatic(w http.ResponseWriter, _ *http.Request, name string, b []byte) {
	if isHashedAsset(name) { w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") } else { w.Header().Set("Cache-Control", "no-cache") }
	w.Header().Set("Content-Type", contentType(name))
	_, _ = w.Write(b)
}

func contentType(name string) string {
	if strings.HasSuffix(name, ".js") { return "application/javascript" }
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" { return ct }
	return "application/octet-stream"
}

func looksAsset(name string) bool {
	switch strings.ToLower(path.Ext(name)) { case ".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".eot": return true; default: return false }
}

func isHashedAsset(name string) bool {
	base := path.Base(name); ext := path.Ext(base); stem := strings.TrimSuffix(base, ext)
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	for _, part := range parts { if len(part) >= 8 && isHashLike(part) { return true } }
	return false
}
func isHashLike(s string) bool { for _, r := range s { if !unicode.IsDigit(r) && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') { return false } }; return true }
`
}

func walkFSNames(fsys fs.FS) ([]string, error) {
	var names []string
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, path.Clean(name))
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}
