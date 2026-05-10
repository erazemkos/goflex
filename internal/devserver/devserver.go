package devserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	frontendbuild "github.com/erazemkos/goflex/internal/build"
	"github.com/erazemkos/goflex/internal/gen"
)

type Options struct {
	Addr, Dir string
	Out       io.Writer
}

type BuildError struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
	Raw     string `json:"raw,omitempty"`
}

type Status struct {
	Restarts int64  `json:"restarts"`
	Reloads  int64  `json:"reloads"`
	CSS      int64  `json:"css"`
	Error    string `json:"error,omitempty"`
}

type changeKind uint8

const (
	changeFrontend changeKind = 1 << iota
	changeServer
	changeCSS
	changeShared
)

var (
	buildFrontend = frontendbuild.Build
	buildCSS      = frontendbuild.BuildCSS
	generateAPI   = gen.Generate
)

func Run(ctx context.Context, opts Options) error {
	if opts.Addr == "" {
		opts.Addr = ":3000"
	}
	if opts.Dir == "" {
		opts.Dir = "."
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}
	s := newServer(dir, opts.Out)
	if err := s.start(ctx, opts.Addr); err != nil {
		return err
	}
	return s.wait()
}

type devServer struct {
	dir        string
	out        io.Writer
	server     *http.Server
	listener   net.Listener
	broker     *broker
	lastErr    atomic.Value
	status     Status
	statusMu   sync.Mutex
	backendMu  sync.Mutex
	backendCmd *exec.Cmd
	done       chan error
}

func newServer(dir string, out io.Writer) *devServer {
	s := &devServer{dir: dir, out: out, broker: newBroker(), done: make(chan error, 1)}
	s.lastErr.Store((*BuildError)(nil))
	return s
}

func (s *devServer) start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/_goflex/events", s.handleEvents)
	mux.HandleFunc("/_goflex/error.json", s.handleError)
	mux.HandleFunc("/_goflex/runtime.js", s.handleRuntime)
	mux.HandleFunc("/_goflex/status.json", s.handleStatus)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { serveApp(s.dir, w, r) })
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if s.out != nil {
		_, _ = fmt.Fprintf(s.out, "goflex dev listening on %s\n", devURL(ln.Addr()))
	}
	go s.watch(ctx)
	go func() {
		<-ctx.Done()
		s.stopBackend()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
	}()
	go func() {
		err := s.server.Serve(ln)
		if err == http.ErrServerClosed {
			err = nil
		}
		s.done <- err
	}()
	return nil
}

func (s *devServer) wait() error { return <-s.done }

func devURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://" + addr.String()
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func (s *devServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, unsubscribe := s.broker.subscribe()
	defer unsubscribe()
	if be := s.currentError(); be != nil {
		writeSSE(w, "error", mustJSON(be))
	} else {
		writeSSE(w, "ok", `{}`)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	for {
		select {
		case ev := <-ch:
			writeSSE(w, ev.name, ev.data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *devServer) handleError(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if be := s.currentError(); be != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(be)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *devServer) handleRuntime(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = fmt.Fprint(w, runtimeJS)
}

func (s *devServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.statusMu.Lock()
	st := s.status
	s.statusMu.Unlock()
	if be := s.currentError(); be != nil {
		st.Error = be.Message
	}
	_ = json.NewEncoder(w).Encode(st)
}

func (s *devServer) currentError() *BuildError {
	be, _ := s.lastErr.Load().(*BuildError)
	return be
}

func (s *devServer) setError(err error) {
	be := parseBuildError(err)
	s.lastErr.Store(be)
	s.broker.publish("error", mustJSON(be))
}

func (s *devServer) clearError() {
	s.lastErr.Store((*BuildError)(nil))
	s.broker.publish("ok", `{}`)
}

func (s *devServer) inc(fn func(*Status)) {
	s.statusMu.Lock()
	fn(&s.status)
	s.statusMu.Unlock()
}

func (s *devServer) watch(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.setError(err)
		return
	}
	defer func() { _ = watcher.Close() }()
	if err := addWatchDirs(watcher, s.dir); err != nil {
		s.setError(err)
		return
	}
	pending := make(map[string]struct{})
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-watcher.Errors:
			if err != nil {
				s.setError(err)
			}
		case ev := <-watcher.Events:
			if ev.Name == "" || shouldIgnore(ev.Name) || ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() && !shouldIgnore(ev.Name) {
					_ = addWatchDirs(watcher, ev.Name)
				}
			}
			pending[ev.Name] = struct{}{}
			if timer == nil {
				timer = time.NewTimer(200 * time.Millisecond)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(200 * time.Millisecond)
			}
		case <-timerC:
			paths := make([]string, 0, len(pending))
			for p := range pending {
				paths = append(paths, p)
			}
			pending = make(map[string]struct{})
			timerC = nil
			timer = nil
			s.rebuild(ctx, paths)
		}
	}
}

func addWatchDirs(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && shouldIgnore(path) {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

func shouldIgnore(path string) bool {
	clean := filepath.Clean(path)
	base := filepath.Base(clean)
	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".tmp") {
		return true
	}
	if strings.HasSuffix(base, ".generated.go") {
		return true
	}
	for _, part := range strings.Split(clean, string(os.PathSeparator)) {
		switch part {
		case ".git", "node_modules", "dist", "vendor", ".goflex":
			return true
		}
	}
	return false
}

func classifyChange(root string, paths []string) changeKind {
	var k changeKind
	for _, p := range paths {
		if shouldIgnore(p) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(p))
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch ext {
		case ".css":
			k |= changeCSS
		case ".go":
			k |= changeCSS
			if strings.Contains(rel, "/shared/") || strings.HasPrefix(rel, "shared/") {
				k |= changeShared | changeFrontend | changeServer
				continue
			}
			if isServerPath(rel) {
				k |= changeServer
			} else {
				k |= changeFrontend
			}
		case ".html", ".js":
			k |= changeFrontend
		}
	}
	return k
}

func isServerPath(rel string) bool {
	return strings.HasPrefix(rel, "internal/api/") || strings.HasPrefix(rel, "internal/models/") || strings.HasPrefix(rel, "cmd/server/") || strings.Contains(rel, "/cmd/server/")
}

func (s *devServer) rebuild(ctx context.Context, paths []string) {
	kind := classifyChange(s.dir, paths)
	if kind == 0 {
		return
	}
	if kind&changeShared != 0 {
		if _, err := generateAPI(s.dir, "api"); err != nil {
			s.setError(err)
			return
		}
	}
	if kind&changeServer != 0 {
		if err := s.rebuildServer(ctx); err != nil {
			s.setError(err)
			return
		}
		s.inc(func(st *Status) { st.Restarts++ })
	}
	if kind&changeFrontend != 0 {
		if _, err := buildFrontend(ctx, frontendbuild.Options{Entry: frontendbuild.FrontendEntry(s.dir), OutDir: filepath.Join(s.dir, "dist"), SourceMap: true}); err != nil {
			s.setError(err)
			return
		}
		s.inc(func(st *Status) { st.Reloads++ })
		s.broker.publish("reload", `{}`)
	}
	if kind&changeCSS != 0 {
		if err := buildCSS(frontendbuild.CSSOptions{Dir: s.dir, Out: filepath.Join(s.dir, "dist", "app.css")}); err != nil {
			s.setError(err)
			return
		}
		s.inc(func(st *Status) { st.CSS++ })
		s.broker.publish("css", `{}`)
	}
	s.clearError()
}

func (s *devServer) rebuildServer(ctx context.Context) error {
	entry := findServerEntry(s.dir)
	if entry == "" {
		return nil
	}
	work := filepath.Join(s.dir, ".goflex", "dev")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	bin := filepath.Join(work, "server")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, entry)
	cmd.Dir = s.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("backend build failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	s.stopBackend()
	serverCtx, cancel := context.WithCancel(ctx)
	cmd = exec.CommandContext(serverCtx, bin)
	cmd.Dir = s.dir
	cmd.Env = append(os.Environ(), "GOFLEX_DEV_BACKEND=1")
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	s.backendMu.Lock()
	s.backendCmd = cmd
	s.backendMu.Unlock()
	go func() {
		_ = cmd.Wait()
		cancel()
	}()
	return nil
}

func (s *devServer) stopBackend() {
	s.backendMu.Lock()
	cmd := s.backendCmd
	s.backendCmd = nil
	s.backendMu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		_ = cmd.Process.Kill()
	}
}

func findServerEntry(dir string) string {
	candidates := []string{filepath.Join(dir, "cmd", "server"), filepath.Join(dir, "cmd", "app"), dir}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return relOrAbs(dir, c)
		}
	}
	return ""
}

func relOrAbs(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return "./" + filepath.ToSlash(rel)
	}
	return path
}

type event struct{ name, data string }

type broker struct {
	mu      sync.Mutex
	clients map[chan event]struct{}
}

func newBroker() *broker { return &broker{clients: map[chan event]struct{}{}} }

func (b *broker) subscribe() (<-chan event, func()) {
	ch := make(chan event, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		close(ch)
		b.mu.Unlock()
	}
}

func (b *broker) publish(name, data string) {
	b.mu.Lock()
	clients := make([]chan event, 0, len(b.clients))
	for ch := range b.clients {
		clients = append(clients, ch)
	}
	b.mu.Unlock()
	for _, ch := range clients {
		select {
		case ch <- event{name: name, data: data}:
		default:
		}
	}
}

func writeSSE(w io.Writer, name, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\n", name)
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		_, _ = fmt.Fprintf(w, "data: %s\n", scanner.Text())
	}
	_, _ = fmt.Fprint(w, "\n")
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

var buildErrRe = regexp.MustCompile(`(?m)([^\s:]+\.go):(\d+):(\d+):\s*(.+)`)

func parseBuildError(err error) *BuildError {
	if err == nil {
		return nil
	}
	raw := err.Error()
	m := buildErrRe.FindStringSubmatch(raw)
	if len(m) == 5 {
		return &BuildError{File: m[1], Line: atoi(m[2]), Column: atoi(m[3]), Message: strings.TrimSpace(m[4]), Raw: raw}
	}
	return &BuildError{Message: raw, Raw: raw}
}

func atoi(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

func serveApp(dir string, w http.ResponseWriter, r *http.Request) {
	requestPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if requestPath == "." {
		requestPath = "index.html"
	}
	candidates := staticCandidates(dir, requestPath)
	for _, full := range candidates {
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			if filepath.Base(full) == "index.html" {
				serveIndex(full, w)
				return
			}
			http.ServeFile(w, r, full)
			return
		}
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") || r.URL.Path == "/" {
		index := filepath.Join(dir, "index.html")
		if _, err := os.Stat(index); errors.Is(err, os.ErrNotExist) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(injectRuntime([]byte(defaultHTML)))
			return
		}
		serveIndex(index, w)
		return
	}
	http.NotFound(w, r)
}

func staticCandidates(dir, requestPath string) []string {
	var out []string
	if requestPath == "app.js" || requestPath == "app.js.map" {
		out = append(out, filepath.Join(dir, "dist", requestPath))
	}
	out = append(out, filepath.Join(dir, requestPath))
	if strings.HasPrefix(filepath.ToSlash(requestPath), "dist/") {
		out = append(out, filepath.Join(dir, requestPath))
	}
	return out
}

func serveIndex(path string, w http.ResponseWriter) {
	b, err := os.ReadFile(path)
	if err != nil {
		b = []byte(defaultHTML)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(injectRuntime(b))
}

func injectRuntime(b []byte) []byte {
	s := string(b)
	tag := `<script src="/_goflex/runtime.js"></script>`
	if strings.Contains(s, tag) {
		return b
	}
	if strings.Contains(s, "</body>") {
		return []byte(strings.Replace(s, "</body>", tag+"</body>", 1))
	}
	return []byte(s + tag)
}

func ignoredForTest(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if shouldIgnore(p) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

const defaultHTML = `<!doctype html><html><body><div id="root"><h1>Hello from GoFlex</h1></div></body></html>`

const runtimeJS = `(() => {
  const KEY = 'goflex:reload-state';
  function snapshot(){
    const inputs = Array.from(document.querySelectorAll('input,textarea,select')).map((el, i) => ({i, value: el.type === 'checkbox' ? el.checked : el.value, type: el.type}));
    sessionStorage.setItem(KEY, JSON.stringify({t: Date.now(), scroll:[scrollX, scrollY], inputs}));
  }
  function restore(){
    try {
      const raw = sessionStorage.getItem(KEY); if (!raw) return;
      const state = JSON.parse(raw); if (Date.now() - state.t > 30000) { sessionStorage.removeItem(KEY); return; }
      for (const item of state.inputs || []) { const el = document.querySelectorAll('input,textarea,select')[item.i]; if (el) { if (item.type === 'checkbox') el.checked = item.value; else el.value = item.value; } }
      if (state.scroll) scrollTo(state.scroll[0], state.scroll[1]);
    } catch (_) {}
  }
  function overlay(data){
    let el = document.getElementById('goflex-error-overlay');
    if (!el) { el = document.createElement('div'); el.id = 'goflex-error-overlay'; document.body.appendChild(el); }
    const loc = [data.file, data.line, data.column].filter(Boolean).join(':');
    el.style.cssText = 'position:fixed;inset:0;z-index:2147483647;background:rgba(20,0,0,.94);color:#fee;padding:24px;font:14px ui-monospace,monospace;white-space:pre-wrap;overflow:auto';
    el.innerHTML = '<h2 style="margin-top:0;color:#fca5a5">GoFlex build failed</h2><a id="goflex-error-loc" style="color:#93c5fd;cursor:pointer">' + (loc || 'build error') + '</a><pre></pre>';
    el.querySelector('pre').textContent = data.raw || data.message || String(data);
    el.querySelector('a').onclick = () => { if (data.file) window.open('vscode://file/' + data.file + (data.line ? ':' + data.line : '') + (data.column ? ':' + data.column : '')); };
  }
  function clearOverlay(){ const el = document.getElementById('goflex-error-overlay'); if (el) el.remove(); }
  function reload(){ snapshot(); location.reload(); }
  function reloadCSS(){
    for (const link of document.querySelectorAll('link[rel="stylesheet"]')) { const u = new URL(link.href); u.searchParams.set('goflex', Date.now()); link.href = u.href; }
  }
  addEventListener('load', restore);
  const es = new EventSource('/_goflex/events');
  es.addEventListener('reload', reload);
  es.addEventListener('css', reloadCSS);
  es.addEventListener('ok', clearOverlay);
  es.addEventListener('error', e => { try { overlay(JSON.parse(e.data)); } catch (_) { overlay({message:e.data, raw:e.data}); } });
})();`
