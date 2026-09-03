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
	record = withoutLogIdentityAttrs(record)
	next := h.scoped
	if publicKey, _ := ctx.Value(peerPublicKeyContextKey{}).(string); publicKey != "" {
		next = h.root.WithAttrs([]slog.Attr{slog.String("peer_public_key", publicKey)})
		for _, operation := range h.operations {
			if operation.group != "" {
				next = next.WithGroup(operation.group)
				continue
			}
			next = next.WithAttrs(operation.attrs)
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

func withoutLogIdentityAttrs(record slog.Record) slog.Record {
	filtered := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		if !isReservedLogIdentity(attr.Key) {
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
