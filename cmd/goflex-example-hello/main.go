package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/erazemkos/goflex/pkg/server"
)

func main() {
	addr := os.Getenv("GOFLEX_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	staticDir := os.Getenv("GOFLEX_STATIC_DIR")
	if staticDir == "" {
		staticDir = "examples/hello/dist"
	}
	if err := ensureIndex(staticDir); err != nil {
		log.Printf("warning: could not prepare index.html: %v", err)
	}
	s := server.New(server.Config{Env: "prod", StaticFS: os.DirFS(staticDir)})
	log.Printf("serving GoFlex hello on http://localhost%s from %s", addr, staticDir)
	if err := s.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func ensureIndex(staticDir string) error {
	index := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(index); err == nil {
		return nil
	}
	source := filepath.Join(filepath.Dir(staticDir), "index.html")
	if _, err := os.Stat(source); err != nil {
		return err
	}
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(index)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	return err
}
