package eventbus

import (
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
)

// slogAdapter adapts the application's slog.Logger to watermill's
// LoggerAdapter interface, so the Redis Streams library's own internal
// logging (consumer group creation, pending message claiming, etc.) flows
// through the same log file as the rest of the app.
type slogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter wraps logger for use as a watermill.LoggerAdapter.
func NewSlogAdapter(logger *slog.Logger) watermill.LoggerAdapter {
	return &slogAdapter{logger: logger}
}

func (a *slogAdapter) args(fields watermill.LogFields) []any {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return args
}

func (a *slogAdapter) Error(msg string, err error, fields watermill.LogFields) {
	a.logger.Error(msg, append(a.args(fields), "error", err)...)
}

func (a *slogAdapter) Info(msg string, fields watermill.LogFields) {
	a.logger.Info(msg, a.args(fields)...)
}

func (a *slogAdapter) Debug(msg string, fields watermill.LogFields) {
	a.logger.Debug(msg, a.args(fields)...)
}

func (a *slogAdapter) Trace(msg string, fields watermill.LogFields) {
	a.logger.Debug(msg, a.args(fields)...)
}

func (a *slogAdapter) With(fields watermill.LogFields) watermill.LoggerAdapter {
	return &slogAdapter{logger: a.logger.With(a.args(fields)...)}
}
