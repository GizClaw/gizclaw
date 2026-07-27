package agenthost

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type accessContextKey struct{}
type toolExecutionContextKey struct{}

// ClientToolInvoker invokes a Tool handler on the Peer that owns the current
// AgentHost run. Implementations must not route by a caller supplied Peer ID.
type ClientToolInvoker interface {
	InvokeClientTool(context.Context, string, []byte) ([]byte, error)
}

type toolExecutionContext struct {
	profileTools []string
	client       ClientToolInvoker
}

type accessContext struct {
	ownerPublicKey          string
	profileToolIDs          []string
	profileToolBindings     map[string]string
	profileWorkflowBindings map[string]string
	profileFingerprint      string
}

// WithResourceAccess attaches the caller ownership and RuntimeProfile snapshot.
func WithResourceAccess(ctx context.Context, ownerPublicKey string, profileToolBindings, profileWorkflowBindings map[string]string, profileFingerprints ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	access := accessContext{
		ownerPublicKey:          strings.TrimSpace(ownerPublicKey),
		profileToolBindings:     make(map[string]string, len(profileToolBindings)),
		profileWorkflowBindings: make(map[string]string, len(profileWorkflowBindings)),
	}
	if len(profileFingerprints) > 0 {
		access.profileFingerprint = strings.TrimSpace(profileFingerprints[0])
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
	maps.Copy(access.profileWorkflowBindings, profileWorkflowBindings)
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

// WithToolExecution attaches the current connected Peer's immutable
// RuntimeProfile Tool snapshot and its connection-scoped client RPC invoker.
func WithToolExecution(
	ctx context.Context,
	bindings *map[string]apitypes.RuntimeProfileBinding,
	client ClientToolInvoker,
) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if bindings == nil {
		return context.WithValue(ctx, toolExecutionContextKey{}, toolExecutionContext{client: client}), nil
	}
	aliases := make([]string, 0, len(*bindings))
	for alias := range *bindings {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	names := make([]string, 0, len(aliases))
	seen := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		name := strings.TrimSpace((*bindings)[alias].ResourceId)
		if name == "" {
			return nil, fmt.Errorf("agenthost: runtime Tool alias %q has an empty resource name", alias)
		}
		if previous, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf(
				"agenthost: runtime Tool aliases %q and %q bind the same canonical Tool %q",
				previous, alias, name,
			)
		}
		seen[name] = alias
		names = append(names, name)
	}
	sort.Strings(names)
	return context.WithValue(ctx, toolExecutionContextKey{}, toolExecutionContext{
		profileTools: names,
		client:       client,
	}), nil
}

func toolExecutionFromContext(ctx context.Context) (toolExecutionContext, bool) {
	if ctx == nil {
		return toolExecutionContext{}, false
	}
	value, ok := ctx.Value(toolExecutionContextKey{}).(toolExecutionContext)
	return value, ok
}
