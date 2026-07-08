package peerhttp

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type callerPublicKeyContextKey string

const callerPublicKeyKey callerPublicKeyContextKey = "caller_public_key"

func WithCallerPublicKey(ctx context.Context, publicKey giznet.PublicKey) context.Context {
	return context.WithValue(ctx, callerPublicKeyKey, publicKey)
}

func CallerPublicKey(ctx context.Context) giznet.PublicKey {
	value, _ := ctx.Value(callerPublicKeyKey).(giznet.PublicKey)
	return value
}
