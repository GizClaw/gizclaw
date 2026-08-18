package apikey

import (
	"context"
	"errors"
)

type principalContextKey struct{}

// WithPrincipal attaches an authenticated API key to a request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated API key for a request.
func PrincipalFromContext(ctx context.Context) (Principal, error) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok || principal.Key.Name == "" {
		return Principal{}, errors.New("api key: principal missing from context")
	}
	return principal, nil
}
