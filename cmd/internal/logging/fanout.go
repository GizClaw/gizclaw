package logging

import (
	"context"
	"errors"
	"log/slog"
)

// FanoutHandler forwards records to a fixed set of slog handlers.
type FanoutHandler struct {
	handlers []slog.Handler
}

// NewFanoutHandler creates a handler that fans out to non-nil child handlers.
func NewFanoutHandler(handlers ...slog.Handler) *FanoutHandler {
	out := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			out = append(out, handler)
		}
	}
	return &FanoutHandler{handlers: out}
}

func (h *FanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if h == nil {
		return false
	}
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *FanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	if h == nil {
		return nil
	}
	var errs []error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (h *FanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h == nil {
		return (*FanoutHandler)(nil)
	}
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return &FanoutHandler{handlers: handlers}
}

func (h *FanoutHandler) WithGroup(name string) slog.Handler {
	if h == nil {
		return (*FanoutHandler)(nil)
	}
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return &FanoutHandler{handlers: handlers}
}
