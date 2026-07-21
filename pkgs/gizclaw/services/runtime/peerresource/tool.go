package peerresource

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

func (s *Server) handleToolList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsToolListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	bindings := bindingMap(profile.Spec.Resources.Tools)
	aliases := sortedBindingAliases(bindings)
	page, hasNext, nextCursor, conflict := pageAliases(aliases, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, http.StatusConflict, "runtime profile revision changed")
	}
	items := make([]rpcapi.Tool, 0, len(page))
	for _, alias := range page {
		binding := bindings[alias]
		tool, err := s.Tools.GetTool(ctx, binding.ResourceId)
		if errors.Is(err, toolkit.ErrToolNotFound) {
			continue
		}
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		items = append(items, projectTool(alias, binding, tool))
	}
	return resultResponse(req.Id, rpcapi.ToolListResponse{
		Items: items, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromToolListResponse)
}

func (s *Server) handleToolGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolGetRequest)
	if !ok || strings.TrimSpace(params.Alias) == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	binding, ok := bindingMap(profile.Spec.Resources.Tools)[strings.TrimSpace(params.Alias)]
	if !ok {
		return statusError(req.Id, http.StatusNotFound, "tool not found")
	}
	tool, err := s.Tools.GetTool(ctx, binding.ResourceId)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, "tool not found")
	}
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcapi.ToolGetResponse{
		Value: projectTool(params.Alias, binding, tool), RuntimeProfileName: profile.Name,
		RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromToolGetResponse)
}

func projectTool(alias string, binding apitypes.RuntimeProfileBinding, tool toolkit.Tool) rpcapi.Tool {
	return rpcapi.Tool{
		Alias: alias, I18n: bindingI18n(binding),
		InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
	}
}

func sortedBindingAliases(bindings map[string]apitypes.RuntimeProfileBinding) []string {
	aliases := make([]string, 0, len(bindings))
	for alias := range bindings {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
