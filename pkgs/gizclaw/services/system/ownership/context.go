// Package ownership carries the authenticated owner through internal service calls.
package ownership

import (
	"context"
	"strings"
)

type ownerKey struct{}

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
