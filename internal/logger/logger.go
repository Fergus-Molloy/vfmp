package logger

import (
	"context"
	"fmt"
	"log/slog"
)

// TeeHandler is a slog.Handler that forwards log records to multiple handlers.
// This allows writing logs to multiple destinations (e.g., stdout and a file).
type TeeHandler struct {
	one slog.Handler
	two slog.Handler
}

// NewTeeHandler creates a new TeeHandler that writes to the given handler and also a given [io.Writer].
func NewTeeHandler(handler slog.Handler, file slog.Handler) *TeeHandler {
	return &TeeHandler{
		one: handler,
		two: file,
	}
}

// Enabled returns true if either handler is enabled for the given level.
func (h *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.one.Enabled(ctx, level) || h.two.Enabled(ctx, level)
}

func (h *TeeHandler) Handle(ctx context.Context, record slog.Record) error {
	err1 := h.one.Handle(ctx, record)
	err2 := h.two.Handle(ctx, record)

	switch {
	case err1 != nil && err2 != nil:
		return fmt.Errorf("encountered multiple errors: %w | %w", err1, err2)
	case err1 != nil:
		return err1
	case err2 != nil:
		return err2
	default:
		return nil
	}
}

// WithAttrs returns a new TeeHandler with attributes added to both handlers.
func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TeeHandler{one: h.one.WithAttrs(attrs), two: h.two.WithAttrs(attrs)}
}

// WithGroup returns a new TeeHandler with a group added to both handlers.
func (h *TeeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &TeeHandler{one: h.one.WithGroup(name), two: h.two.WithGroup(name)}
}

// NewTeeLogger creates a new slog.Logger that writes to two different handlers
func NewTeeLogger(one slog.Handler, two slog.Handler) *slog.Logger {
	return slog.New(NewTeeHandler(one, two))
}
