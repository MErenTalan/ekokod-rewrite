// Package logging configures the application's structured logger: JSON or text,
// context fields propagated automatically, and secret-looking attributes
// redacted before they reach the output.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New builds a logger. level is debug|info|warn|error, format is json|text.
func New(level, format string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var base slog.Handler
	if strings.EqualFold(format, "text") {
		base = slog.NewTextHandler(w, opts)
	} else {
		base = slog.NewJSONHandler(w, opts)
	}
	return slog.New(redactHandler{inner: base})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
