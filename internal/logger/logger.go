// Package logger initialises the application-wide slog logger.
// In production (LOG_FORMAT=json) it outputs JSON. Otherwise text (dev-friendly).
package logger

import (
	"log/slog"
	"os"
)

// Init sets the global slog logger based on LOG_FORMAT env var.
func Init() {
	var h slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	slog.SetDefault(slog.New(h))
}
