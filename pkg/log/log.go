package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// DebugEnabled reports whether GOFLEX_DEBUG=1 is set.
func DebugEnabled() bool { return os.Getenv("GOFLEX_DEBUG") == "1" }

// Debugf writes a stable debug line when GOFLEX_DEBUG=1.
func Debugf(w io.Writer, format string, args ...any) {
	if DebugEnabled() {
		_, _ = fmt.Fprintf(w, "level=debug msg=%q\n", fmt.Sprintf(format, args...))
	}
}

func New(w io.Writer, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
