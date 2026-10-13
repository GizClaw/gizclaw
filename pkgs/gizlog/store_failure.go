package gizlog

import (
	"context"
	"errors"
	"log/slog"
)

type storeFailureReportingHandler struct {
	handler slog.Handler
	queue   *storeQueue
	store   string
}

func newStoreFailureReportingHandler(handler slog.Handler, queue *storeQueue, store string) slog.Handler {
	return &storeFailureReportingHandler{handler: handler, queue: queue, store: store}
}

func (h *storeFailureReportingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *storeFailureReportingHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.handler.Handle(ctx, record)
	if err != nil && !errors.Is(err, ErrStoreQueueFull) && !errors.Is(err, errStoreQueueClosed) {
		h.queue.rejectProjection(h.store)
	}
	return err
}

func (h *storeFailureReportingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &storeFailureReportingHandler{handler: h.handler.WithAttrs(attrs), queue: h.queue, store: h.store}
}

func (h *storeFailureReportingHandler) WithGroup(name string) slog.Handler {
	return &storeFailureReportingHandler{handler: h.handler.WithGroup(name), queue: h.queue, store: h.store}
}
