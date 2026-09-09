package gizlog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
)

type correlationKey string

const (
	requestIDKey  correlationKey = "request_id"
	sessionIDKey  correlationKey = "session_id"
	apiKeyNameKey correlationKey = "api_key_name"
	streamIDKey   correlationKey = "stream_id"
)

// NewID returns a server-generated, 128-bit random correlation identifier.
func NewID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func withCorrelation(ctx context.Context, key correlationKey, value string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, key, strings.TrimSpace(value))
}
func correlation(ctx context.Context, key correlationKey) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key).(string)
	return value
}

// WithRequestID attaches the trusted ingress request identifier to a child context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return withCorrelation(ctx, requestIDKey, id)
}

// RequestID returns the trusted ingress request identifier.
func RequestID(ctx context.Context) string { return correlation(ctx, requestIDKey) }

// WithSessionID attaches the owning device connection's session identifier.
func WithSessionID(ctx context.Context, id string) context.Context {
	return withCorrelation(ctx, sessionIDKey, id)
}

// SessionID returns the owning device connection's session identifier.
func SessionID(ctx context.Context) string { return correlation(ctx, sessionIDKey) }

// WithAPIKeyName attaches the authenticated credential's public name, never its secret.
func WithAPIKeyName(ctx context.Context, name string) context.Context {
	return withCorrelation(ctx, apiKeyNameKey, name)
}

// APIKeyName returns the authenticated credential's public name.
func APIKeyName(ctx context.Context) string { return correlation(ctx, apiKeyNameKey) }

// WithStreamID attaches the stream being processed by this child operation.
func WithStreamID(ctx context.Context, id string) context.Context {
	return withCorrelation(ctx, streamIDKey, id)
}

// StreamID returns the stream associated with this operation.
func StreamID(ctx context.Context) string { return correlation(ctx, streamIDKey) }

func contextIdentityAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 5)
	if peer := PeerPublicKey(ctx); peer != "" {
		attrs = append(attrs, slog.String("peer_public_key", peer))
	}
	for _, key := range []correlationKey{sessionIDKey, requestIDKey, apiKeyNameKey, streamIDKey} {
		if value := correlation(ctx, key); value != "" {
			attrs = append(attrs, slog.String(string(key), value))
		}
	}
	return attrs
}

// AuthenticatedPeerHeader carries Server-authenticated identity back to an
// authenticated Edge HTTP proxy. Edge strips it before sending the response.
const AuthenticatedPeerHeader = "X-Gizclaw-Log-Peer"

// APIKeyNameHeader carries the authenticated credential name to Edge, never
// its bearer secret. Public ingress must not trust a client-supplied value.
const APIKeyNameHeader = "X-Gizclaw-Log-Key-Name"
