package gizlog

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"time"
)

type storeFailureReportingHandler struct {
	handler  slog.Handler
	fallback slog.Handler
	store    string
}

func newStoreFailureReportingHandler(handler, fallback slog.Handler, store string) slog.Handler {
	return &storeFailureReportingHandler{handler: handler, fallback: fallback, store: store}
}

func (h *storeFailureReportingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *storeFailureReportingHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.handler.Handle(ctx, record)
	if err == nil {
		return nil
	}
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	failure := slog.NewRecord(time.Now(), slog.LevelError, "system log store sink failed", pcs[0])
	failure.AddAttrs(slog.String("store", h.store))
	return errors.Join(err, h.fallback.Handle(context.Background(), failure))
}

func (h *storeFailureReportingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &storeFailureReportingHandler{handler: h.handler.WithAttrs(attrs), fallback: h.fallback, store: h.store}
}

func (h *storeFailureReportingHandler) WithGroup(name string) slog.Handler {
	return &storeFailureReportingHandler{handler: h.handler.WithGroup(name), fallback: h.fallback, store: h.store}
}
