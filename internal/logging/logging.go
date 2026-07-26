// Package logging configures the application-wide structured logger, which
// writes JSON records to both stdout and a log file on disk.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// New creates a slog.Logger that writes to both stdout and the file at path,
// creating parent directories as needed.
func New(path string) (*slog.Logger, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	writer := io.MultiWriter(os.Stdout, f)
	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), nil
}
