package gizlog

import (
	"context"
	"log/slog"
	"strings"
)

type peerPublicKeyContextKey struct{}

// WithPeerPublicKey returns a child context carrying a Peer identity for logging.
func WithPeerPublicKey(ctx context.Context, publicKey string) context.Context {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, peerPublicKeyContextKey{}, publicKey)
}

// PeerPublicKey returns the Peer identity attached by WithPeerPublicKey, or
// an empty string when the context carries none.
func PeerPublicKey(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	publicKey, _ := ctx.Value(peerPublicKeyContextKey{}).(string)
	return publicKey
}

type contextHandler struct {
	root       slog.Handler
	scoped     slog.Handler
	operations []contextHandlerOperation
}

type contextHandlerOperation struct {
	attrs []slog.Attr
	group string
}

func newContextHandler(next slog.Handler, fixed []slog.Attr) *contextHandler {
	if len(fixed) > 0 {
		next = next.WithAttrs(fixed)
	}
	return &contextHandler{root: next, scoped: next}
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h != nil && h.scoped != nil && h.scoped.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if h == nil || h.scoped == nil {
		return nil
	}
	record = withoutLogContextAttrs(ctx, record)
	next := h.scoped
	if attrs := contextIdentityAttrs(ctx); len(attrs) > 0 {
		next = h.root.WithAttrs(attrs)
		for _, operation := range h.operations {
			if operation.group != "" {
				next = next.WithGroup(operation.group)
				continue
			}
			attrs := make([]slog.Attr, 0, len(operation.attrs))
			for _, attr := range operation.attrs {
				if correlation(ctx, correlationKey(attr.Key)) == "" {
					attrs = append(attrs, attr)
				}
			}
			next = next.WithAttrs(attrs)
		}
	}
	return next.Handle(ctx, record)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h == nil || h.scoped == nil {
		return (*contextHandler)(nil)
	}
	attrs = withoutReservedLogIdentityAttrs(attrs)
	if len(attrs) == 0 {
		return h
	}
	operations := append([]contextHandlerOperation(nil), h.operations...)
	operations = append(operations, contextHandlerOperation{attrs: attrs})
	return &contextHandler{root: h.root, scoped: h.scoped.WithAttrs(attrs), operations: operations}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	if h == nil || h.scoped == nil {
		return (*contextHandler)(nil)
	}
	if name == "" {
		return h
	}
	operations := append([]contextHandlerOperation(nil), h.operations...)
	operations = append(operations, contextHandlerOperation{group: name})
	return &contextHandler{root: h.root, scoped: h.scoped.WithGroup(name), operations: operations}
}

func withoutLogContextAttrs(ctx context.Context, record slog.Record) slog.Record {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if !isReservedLogIdentity(attr.Key) && correlation(ctx, correlationKey(attr.Key)) == "" {
			filtered.AddAttrs(attr)
		}
		return true
	})
	return filtered
}

func withoutReservedLogIdentityAttrs(attrs []slog.Attr) []slog.Attr {
	filtered := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		if !isReservedLogIdentity(attr.Key) {
			filtered = append(filtered, attr)
		}
	}
	return filtered
}

func isReservedLogIdentity(key string) bool {
	return key == "node_id" || key == "peer_public_key"
}
