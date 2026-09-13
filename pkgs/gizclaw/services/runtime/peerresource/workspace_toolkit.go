package peerresource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// resolveWorkspaceToolkit translates Peer names at the adapter boundary. A
// present empty list stays present so persistence cannot turn opt-out into inherit.
func (s *Server) resolveWorkspaceToolkit(ctx context.Context, policy *rpcapi.ToolkitPolicy, profile *apitypes.RuntimeProfile) (*apitypes.ToolkitPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	out := &apitypes.ToolkitPolicy{}
	if policy.ToolNames == nil {
		return out, nil
	}
	ids := make([]string, 0, len(*policy.ToolNames))
	out.ToolIds = &ids
	if len(*policy.ToolNames) == 0 {
		return out, nil
	}
	if profile == nil || s.Tools == nil {
		return nil, errors.New("toolkit service or runtime profile not configured")
	}
	bindings := bindingMap(profile.Spec.Resources.Tools)
	for _, name := range *policy.ToolNames {
		if name == "" || strings.TrimSpace(name) != name {
			return nil, fmt.Errorf("%w: invalid Tool name", toolkit.ErrInvalidTool)
		}
		var id string
		if binding, ok := bindings[name]; ok {
			tool, err := s.Tools.GetToolByID(ctx, binding.ResourceId)
			if err != nil {
				return nil, err
			}
			id = tool.ID
		} else {
			tool, err := s.Tools.GetTool(ctx, name)
			if err != nil {
				return nil, err
			}
			for _, binding := range bindings {
				if binding.ResourceId == tool.ID {
					id = tool.ID
					break
				}
			}
			if id == "" {
				return nil, toolkit.ErrToolNotFound
			}
		}
		ids = append(ids, id)
	}
	return toolkit.NormalizePolicy(out)
}

func projectWorkspaceToolkit(policy *apitypes.ToolkitPolicy, profile *apitypes.RuntimeProfile) *rpcapi.ToolkitPolicy {
	if policy == nil {
		return nil
	}
	out := &rpcapi.ToolkitPolicy{}
	if policy.ToolIds == nil {
		return out
	}
	names := make([]string, 0, len(*policy.ToolIds))
	if profile != nil {
		bindings := bindingMap(profile.Spec.Resources.Tools)
		for _, alias := range sortedBindingAliases(bindings) {
			if slices.Contains(*policy.ToolIds, bindings[alias].ResourceId) {
				names = append(names, alias)
			}
		}
	}
	out.ToolNames = &names
	return out
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
