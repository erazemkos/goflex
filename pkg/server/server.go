package server

import (
	"context"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/erazemkos/goflex/pkg/httperr"
	flexlog "github.com/erazemkos/goflex/pkg/log"
	"github.com/erazemkos/goflex/pkg/version"
)

const (
	requestIDHeader = "X-Request-Id"
	loggerKey       = "goflex.logger"
	requestIDKey    = "goflex.request_id"
)

type Config struct {
	Env            string
	StaticFS       fs.FS
	IndexPath      string
	IndexHTML      []byte // When set, served for all browser HTML page requests instead of reading IndexPath from StaticFS.
	APIPrefix      string
	CORSOrigins    []string
	TrustedProxies []string
	Logger         *slog.Logger
}

type Server struct {
	cfg      Config
	engine   *gin.Engine
	draining atomic.Bool
}

func New(cfg Config) *Server {
	if cfg.APIPrefix == "" {
		cfg.APIPrefix = "/api"
	}
	cfg.APIPrefix = normalizePrefix(cfg.APIPrefix)
	if cfg.IndexPath == "" {
		cfg.IndexPath = "index.html"
	}
	if cfg.Logger == nil {
		cfg.Logger = flexlog.New(os.Stderr, cfg.Env == "dev")
	}
	if cfg.Env == "dev" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	e := gin.New()
	if err := e.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		cfg.Logger.Warn("invalid trusted proxies", "error", err)
	}
	s := &Server{cfg: cfg, engine: e}
	e.Use(s.requestID(), s.cors(), s.accessLog(), s.rejectWhileDraining())
	e.GET("/healthz", s.health)
	e.GET(path.Join(s.cfg.APIPrefix, "healthz"), s.health)
	e.NoRoute(s.noRoute)
	return s
}

func (s *Server) Use(middleware ...gin.HandlerFunc) { s.engine.Use(middleware...) }

func (s *Server) API(prefix string, fn func(r *gin.RouterGroup)) {
	if prefix == "" {
		prefix = s.cfg.APIPrefix
	}
	fn(s.engine.Group(normalizePrefix(prefix)))
}

func (s *Server) Static(fsys fs.FS) { s.cfg.StaticFS = fsys }

// Engine returns the underlying Gin engine for registering custom routes
// outside the API prefix (for example, server-rendered HTML pages).
func (s *Server) Engine() *gin.Engine { return s.engine }

// GET registers a GET handler at the given path on the root engine,
// bypassing the API prefix. Use this for server-rendered HTML pages.
func (s *Server) GET(path string, h gin.HandlerFunc) { s.engine.GET(path, h) }

func (s *Server) Run(addr string) error {
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: s.engine, ReadHeaderTimeout: 5 * time.Second}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	return s.serveHTTP(srv, srv.ListenAndServe, signals)
}

func (s *Server) Handler() http.Handler { return s.engine }

func (s *Server) health(c *gin.Context) {
	if s.draining.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining", "version": version.Version()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version.Version()})
}

func (s *Server) serveHTTP(srv *http.Server, serve func() error, signals <-chan os.Signal) error {
	shutdownDone := make(chan struct{})
	serveDone := make(chan struct{})
	if signals != nil {
		go func() {
			defer close(shutdownDone)
			select {
			case _, ok := <-signals:
				if !ok {
					return
				}
			case <-serveDone:
				return
			}
			s.draining.Store(true)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				s.cfg.Logger.Error("graceful shutdown failed", "error", err)
			}
		}()
	} else {
		close(shutdownDone)
	}

	err := serve()
	close(serveDone)
	if err == http.ErrServerClosed {
		<-shutdownDone
		return nil
	}
	return err
}

func (s *Server) requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(requestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
			c.Request.Header.Set(requestIDHeader, rid)
		}
		c.Header(requestIDHeader, rid)
		c.Set(requestIDKey, rid)
		c.Set(loggerKey, s.cfg.Logger.With("request_id", rid))
		c.Next()
	}
}

func (s *Server) cors() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(s.cfg.CORSOrigins))
	for _, origin := range s.cfg.CORSOrigins {
		allowed[origin] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", allowedHeaders(c))
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger := s.logger(c)
		logger.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

func (s *Server) rejectWhileDraining() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.draining.Load() && c.Request.URL.Path != "/healthz" {
			httperr.Write(c, http.StatusServiceUnavailable, httperr.New("draining", "server is shutting down", nil))
			return
		}
		c.Next()
	}
}

func (s *Server) noRoute(c *gin.Context) {
	p := c.Request.URL.Path
	if isAPIRoute(p, s.cfg.APIPrefix) {
		httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
		return
	}
	if s.cfg.StaticFS != nil {
		name := strings.TrimPrefix(path.Clean(p), "/")
		if name == "" || name == "." {
			name = s.cfg.IndexPath
		}
		// When IndexHTML is explicitly set, let it win for the index page;
		// still serve other static assets (app.js, app.css …) from StaticFS.
		if !(name == s.cfg.IndexPath && len(s.cfg.IndexHTML) > 0) {
			if s.tryWriteStatic(c, name) {
				return
			}
		}
	}
	if looksAsset(p) {
		httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
		return
	}
	if acceptsHTML(c) {
		if len(s.cfg.IndexHTML) > 0 {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", s.cfg.IndexHTML)
			return
		}
		if s.cfg.StaticFS != nil {
			if b, err := fs.ReadFile(s.cfg.StaticFS, s.cfg.IndexPath); err == nil {
				c.Header("Cache-Control", "no-cache")
				c.Data(http.StatusOK, "text/html; charset=utf-8", b)
				return
			}
		}
	}
	httperr.Write(c, http.StatusNotFound, httperr.New("not_found", "not found", nil))
}

func (s *Server) tryWriteStatic(c *gin.Context, name string) bool {
	if name != s.cfg.IndexPath {
		accept := c.GetHeader("Accept-Encoding")
		if strings.Contains(accept, "br") {
			if b, err := fs.ReadFile(s.cfg.StaticFS, name+".br"); err == nil {
				c.Header("Content-Encoding", "br")
				c.Header("Vary", "Accept-Encoding")
				s.writeStatic(c, name, b)
				return true
			}
		}
		if strings.Contains(accept, "gzip") {
			if b, err := fs.ReadFile(s.cfg.StaticFS, name+".gz"); err == nil {
				c.Header("Content-Encoding", "gzip")
				c.Header("Vary", "Accept-Encoding")
				s.writeStatic(c, name, b)
				return true
			}
		}
	}
	if b, err := fs.ReadFile(s.cfg.StaticFS, name); err == nil {
		s.writeStatic(c, name, b)
		return true
	}
	return false
}

func (s *Server) writeStatic(c *gin.Context, name string, b []byte) {
	ct := contentType(name)
	if isHashedAsset(name) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
	c.Data(http.StatusOK, ct, b)
}

func (s *Server) logger(c *gin.Context) *slog.Logger {
	if v, ok := c.Get(loggerKey); ok {
		if logger, ok := v.(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	return s.cfg.Logger
}

func allowedHeaders(c *gin.Context) string {
	if requested := c.GetHeader("Access-Control-Request-Headers"); requested != "" {
		return requested
	}
	return "Content-Type, Authorization, X-CSRF-Token, X-Request-Id"
}

func acceptsHTML(c *gin.Context) bool {
	if c.Request.URL.Path == "/" {
		return true
	}
	return strings.Contains(c.GetHeader("Accept"), "text/html")
}

func contentType(name string) string {
	if strings.HasSuffix(name, ".js") {
		return "application/javascript"
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func looksAsset(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".woff", ".woff2", ".ttf", ".eot":
		return true
	default:
		return false
	}
}

func isHashedAsset(name string) bool {
	base := path.Base(name)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '.' || r == '-' || r == '_' })
	for _, part := range parts {
		if len(part) >= 8 && isHashLike(part) {
			return true
		}
	}
	return false
}

func isHashLike(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func isAPIRoute(p, prefix string) bool {
	prefix = normalizePrefix(prefix)
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

func normalizePrefix(prefix string) string {
	if prefix == "" {
		return "/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 {
		prefix = strings.TrimRight(prefix, "/")
	}
	return prefix
}
