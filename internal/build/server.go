package build

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

// ServeStatic serves a build directory for manual verification of index.html and app.js.
func ServeStatic(ctx context.Context, dir, addr string) (*http.Server, net.Addr, error) {
	if dir == "" {
		dir = "dist"
	}
	if addr == "" {
		addr = ":0"
	}
	mux := http.NewServeMux()
	files := http.FileServer(http.Dir(dir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			panic(fmt.Sprintf("goflex static server failed: %v", err))
		}
	}()
	return srv, ln.Addr(), nil
}
