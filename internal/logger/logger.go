// Package logger wraps the stdlib slog to produce structured logs in
// either JSON or plain text. Keep this thin — slog handles everything.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a *slog.Logger configured for the given level + format.
// Both inputs are validated by config.Validate; this function is permissive
// (falls back to info+json) so it can be called from tests without setup.
func New(level, format string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: l}
	if strings.ToLower(format) == "text" {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}
