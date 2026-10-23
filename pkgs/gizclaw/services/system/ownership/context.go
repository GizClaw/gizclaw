// Package ownership carries the authenticated owner through internal service calls.
package ownership

import (
	"context"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ownerKey struct{}
type callerKey struct{}

// WithCaller records the already-authenticated invoking Peer. Resource owner
// changes must preserve it; declarations and device reports cannot set it.
func WithCaller(ctx context.Context, publicKey giznet.PublicKey) context.Context {
	return context.WithValue(ctx, callerKey{}, publicKey)
}

// Caller returns the invoking Peer independently of resource ownership.
func Caller(ctx context.Context) giznet.PublicKey {
	if ctx == nil {
		return giznet.PublicKey{}
	}
	value, _ := ctx.Value(callerKey{}).(giznet.PublicKey)
	return value
}

func WithOwner(ctx context.Context, publicKey string) context.Context {
	return context.WithValue(ctx, ownerKey{}, strings.TrimSpace(publicKey))
}

func FromContext(ctx context.Context) (string, bool) {
	owner, ok := ctx.Value(ownerKey{}).(string)
	return owner, ok && owner != ""
}

func Matches(ctx context.Context, owner *string) bool {
	want, ok := FromContext(ctx)
	return ok && owner != nil && *owner == want
}
