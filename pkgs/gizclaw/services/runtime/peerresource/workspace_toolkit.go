package peerresource

import (
	"context"
	"errors"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// resolveWorkspaceToolkit translates Peer names at the adapter boundary. A
// present empty list stays present so persistence cannot turn opt-out into the
// omitted policy, which keeps the Workflow's full Tool list.
func (s *Server) resolveWorkspaceToolkit(ctx context.Context, policy *rpcapi.ToolkitPolicy, profile *apitypes.RuntimeProfile) (*apitypes.ToolkitPolicy, error) {
	if policy == nil || policy.ToolNames == nil {
		return nil, nil
	}
	names := append([]string{}, (*policy.ToolNames)...)
	if profile == nil {
		return nil, errors.New("runtime profile not configured")
	}
	normalized, err := toolkit.NormalizePolicy(&apitypes.ToolkitPolicy{ToolNames: &names})
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if profile.Spec.Resources.Tools == nil {
			return nil, toolkit.ErrToolNotFound
		}
		if _, ok := (*profile.Spec.Resources.Tools)[name]; !ok {
			return nil, toolkit.ErrToolNotFound
		}
	}
	return normalized, nil
}

// Preserve alias selections verbatim, including stale references and explicit
// empty lists. Discovery never substitutes an HTTP invoke_name for an alias.
func (s *Server) projectWorkspaceToolkit(ctx context.Context, policy *apitypes.ToolkitPolicy, profile *apitypes.RuntimeProfile) *rpcapi.ToolkitPolicy {
	if policy == nil {
		return nil
	}
	if policy.ToolNames != nil {
		names := append([]string{}, (*policy.ToolNames)...)
		return &rpcapi.ToolkitPolicy{ToolNames: &names}
	}
	if policy.ToolIds == nil {
		return nil
	}
	names := []string{}
	if profile != nil && profile.Spec.Resources.Tools != nil {
		for _, alias := range sortedBindingAliases(*profile.Spec.Resources.Tools) {
			binding := (*profile.Spec.Resources.Tools)[alias]
			if binding.ResourceId != "" && slices.Contains(*policy.ToolIds, binding.ResourceId) {
				names = append(names, alias)
			}
		}
	}
	return &rpcapi.ToolkitPolicy{ToolNames: &names}
}

func workspaceToolkitError(id string, err error) *rpcapi.RPCResponse {
	switch {
	case errors.Is(err, toolkit.ErrToolNotFound):
		return statusError(id, rpcapi.StatusCodeNotFound, "tool not found")
	case errors.Is(err, toolkit.ErrInvalidTool):
		return statusError(id, rpcapi.StatusCodeInvalidArgument, "invalid toolkit policy")
	default:
		return internalError(id, "toolkit resolution failed")
	}
}
