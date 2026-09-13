package peerresource

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// resolveWorkspaceToolkit translates Peer names at the adapter boundary. A
// present empty list stays present so persistence cannot turn opt-out into inherit.
func (s *Server) resolveWorkspaceToolkit(ctx context.Context, policy *rpcapi.ToolkitPolicy, profile *apitypes.RuntimeProfile) (*apitypes.ToolkitPolicy, error) {
	if policy == nil || policy.ToolNames == nil {
		return nil, nil
	}
	out := &apitypes.ToolkitPolicy{}
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

// projectWorkspaceToolkit returns representable names without letting stale
// toolkit data fail Workspace reads or responses after a successful mutation.
func (s *Server) projectWorkspaceToolkit(ctx context.Context, policy *apitypes.ToolkitPolicy, profile *apitypes.RuntimeProfile) *rpcapi.ToolkitPolicy {
	if policy == nil || policy.ToolIds == nil {
		return nil
	}
	bindings := map[string]apitypes.RuntimeProfileBinding{}
	if profile != nil {
		bindings = bindingMap(profile.Spec.Resources.Tools)
	}
	aliases := map[string]string{}
	for _, alias := range sortedBindingAliases(bindings) {
		id := bindings[alias].ResourceId
		if _, exists := aliases[id]; !exists {
			aliases[id] = alias
		}
	}
	names := make([]string, 0, len(*policy.ToolIds))
	for _, id := range *policy.ToolIds {
		name, bound := aliases[id]
		if !bound {
			if s.Tools == nil {
				continue
			}
			tool, err := s.Tools.GetToolByID(ctx, id)
			if err != nil {
				continue
			}
			name = tool.InvokeName
			if binding, collision := bindings[name]; collision && binding.ResourceId != id {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
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
