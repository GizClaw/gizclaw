package peerresource

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
)

func (s *Server) toolSelection(ctx context.Context, profile apitypes.RuntimeProfile, workflowName, workspaceName *string) (*[]string, error) {
	if workflowName != nil && workspaceName != nil {
		return nil, errors.New("choose Workflow or Workspace")
	}
	if workspaceName != nil {
		service, ok := s.Workspaces.(interface {
			GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error)
		})
		if !ok {
			return nil, errors.New("workspace service unavailable")
		}
		ws, err := service.GetWorkspaceByName(s.ownerContext(ctx), *workspaceName)
		if err != nil {
			return nil, err
		}
		if ws.OwnerPublicKey == nil || *ws.OwnerPublicKey != s.Caller.String() {
			return nil, errors.New("Workspace owner mismatch")
		}
		alias := ""
		if ws.Labels != nil {
			alias = (*ws.Labels)["workflow_name"]
		}
		names, err := toolcatalog.Selection(profile, alias, ws.WorkflowId, ws.Toolkit)
		return &names, err
	}
	if workflowName != nil {
		binding, ok := profile.Spec.Workflows[*workflowName]
		if !ok {
			return nil, errors.New("Workflow binding unavailable")
		}
		names, err := toolcatalog.Selection(profile, *workflowName, binding.ResourceId, nil)
		return &names, err
	}
	return nil, nil
}

func (s *Server) catalog() *toolcatalog.Catalog {
	if s.ToolCatalog != nil {
		return s.ToolCatalog
	}
	return &toolcatalog.Catalog{Tools: s.Tools}
}

func (s *Server) handleToolList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsToolListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	selection, err := s.toolSelection(ctx, *profile, params.WorkflowName, params.WorkspaceName)
	if err != nil {
		return statusError(req.Id, rpcapi.StatusCodeInvalidArgument, "invalid Tool scope")
	}
	tools, err := s.catalog().Resolve(ctx, s.Caller.String(), *profile, selection)
	if err != nil {
		return internalError(req.Id, "Tool catalog resolution failed")
	}
	aliases := make([]string, 0, len(tools))
	byName := make(map[string]toolcatalog.Tool, len(tools))
	for _, tool := range tools {
		aliases = append(aliases, tool.Alias)
		byName[tool.Alias] = tool
	}
	scope, _ := json.Marshal([]*string{params.WorkflowName, params.WorkspaceName})
	digest := sha256.Sum256(scope)
	page, hasNext, nextCursor, conflict := pageAliases(aliases, params.Cursor, params.Limit, fmt.Sprintf("%s:%x", profile.Revision, digest[:8]))
	if conflict {
		return statusError(req.Id, rpcapi.StatusCodeAborted, "runtime profile or scope changed")
	}
	items := make([]rpcapi.Tool, 0, len(page))
	for _, alias := range page {
		items = append(items, projectCatalogTool(byName[alias]))
	}
	return resultResponse(req.Id, rpcapi.ToolListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor, RuntimeProfileName: profile.Id, RuntimeProfileRevision: profile.Revision}, (*rpcapi.RPCPayload).FromToolListResponse)
}

func (s *Server) handleToolGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolGetRequest)
	if !ok || strings.TrimSpace(params.Name) != params.Name || params.Name == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	selection, err := s.toolSelection(ctx, *profile, params.WorkflowName, params.WorkspaceName)
	if err != nil {
		return statusError(req.Id, rpcapi.StatusCodeInvalidArgument, "invalid Tool scope")
	}
	if selection != nil && !slices.Contains(*selection, params.Name) {
		return statusError(req.Id, rpcapi.StatusCodeNotFound, "tool not found")
	}
	names := []string{params.Name}
	tools, err := s.catalog().Resolve(ctx, s.Caller.String(), *profile, &names)
	if err != nil {
		return internalError(req.Id, "Tool catalog resolution failed")
	}
	for _, tool := range tools {
		if tool.Alias != params.Name {
			continue
		}
		return resultResponse(req.Id, rpcapi.ToolGetResponse{Value: projectCatalogTool(tool), RuntimeProfileName: profile.Id, RuntimeProfileRevision: profile.Revision}, (*rpcapi.RPCPayload).FromToolGetResponse)
	}
	return statusError(req.Id, rpcapi.StatusCodeNotFound, "tool not found")
}

func projectCatalogTool(tool toolcatalog.Tool) rpcapi.Tool {
	return rpcapi.Tool{Name: tool.Alias, InvokeName: tool.FunctionName, I18n: projectBindingI18n(tool.Binding.I18n), InputSchema: tool.Schema, Source: tool.Source, Target: tool.Target, Supported: tool.Supported, Online: tool.Online, Available: tool.Available, UnavailableReason: tool.Reason}
}

func sortedBindingAliases[T any](bindings map[string]T) []string {
	aliases := make([]string, 0, len(bindings))
	for alias := range bindings {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	return aliases
}
