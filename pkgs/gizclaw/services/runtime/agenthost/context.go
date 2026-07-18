package agenthost

import (
	"context"
	"strings"
)

type accessContextKey struct{}

type accessContext struct {
	ownerPublicKey string
	profileToolIDs []string
}

// WithResourceAccess attaches the caller ownership and RuntimeProfile tool snapshot.
func WithResourceAccess(ctx context.Context, ownerPublicKey string, profileToolIDs []string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	access := accessContext{
		ownerPublicKey: strings.TrimSpace(ownerPublicKey),
		profileToolIDs: append([]string(nil), profileToolIDs...),
	}
	return context.WithValue(ctx, accessContextKey{}, access)
}

func resourceAccessFromContext(ctx context.Context) (accessContext, bool) {
	if ctx == nil {
		return accessContext{}, false
	}
	access, ok := ctx.Value(accessContextKey{}).(accessContext)
	if !ok || strings.TrimSpace(access.ownerPublicKey) == "" {
		return accessContext{}, false
	}
	return access, true
}
