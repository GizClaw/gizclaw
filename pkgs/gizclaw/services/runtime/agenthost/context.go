package agenthost

import (
	"context"
	"sort"
	"strings"
)

type accessContextKey struct{}

type accessContext struct {
	ownerPublicKey          string
	profileToolIDs          []string
	profileToolBindings     map[string]string
	profileWorkflowBindings map[string]string
}

// WithResourceAccess attaches the caller ownership and RuntimeProfile snapshot.
func WithResourceAccess(ctx context.Context, ownerPublicKey string, profileToolBindings, profileWorkflowBindings map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	access := accessContext{
		ownerPublicKey:          strings.TrimSpace(ownerPublicKey),
		profileToolBindings:     make(map[string]string, len(profileToolBindings)),
		profileWorkflowBindings: make(map[string]string, len(profileWorkflowBindings)),
	}
	aliases := make([]string, 0, len(profileToolBindings))
	for alias := range profileToolBindings {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		name := profileToolBindings[alias]
		access.profileToolBindings[alias] = name
		access.profileToolIDs = append(access.profileToolIDs, name)
	}
	for alias, name := range profileWorkflowBindings {
		access.profileWorkflowBindings[alias] = name
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
