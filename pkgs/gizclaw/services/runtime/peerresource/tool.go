package peerresource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	tools, err := s.Tools.ListToolsByOwner(ctx, s.Caller.String())
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	byID := make(map[string]toolkit.Tool, len(tools))
	owned := make([]string, 0, len(tools))
	for _, tool := range tools {
		byID[tool.ID] = tool
		if tool.OwnerPublicKey != nil && *tool.OwnerPublicKey == s.Caller.String() {
			owned = append(owned, tool.ID)
		}
	}
	ordered := orderedUnique(s.profileNames(profileTools), owned)
	cursor := strings.TrimSpace(valueOrZero(params.Cursor))
	limit := peerListLimit(params.Limit)
	items := make([]rpcapi.Tool, 0, limit)
	hasNext := false
	started := cursor == ""
	for _, id := range ordered {
		if !started {
			if id == cursor {
				started = true
			}
			continue
		}
		tool, ok := byID[id]
		if !ok {
			tool, err = s.Tools.GetTool(ctx, id)
			if errors.Is(err, toolkit.ErrToolNotFound) {
				continue
			}
			if err != nil {
				return internalError(req.Id, err.Error())
			}
		}
		if len(items) == limit {
			hasNext = true
			break
		}
		item, err := toolkit.ToRPC(tool)
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		items = append(items, item)
	}
	var nextCursor *string
	if hasNext && len(items) > 0 {
		next := items[len(items)-1].Id
		nextCursor = &next
	}
	return resultResponse(req.Id, rpcapi.ToolListResponse{Items: items, HasNext: hasNext, NextCursor: nextCursor}, (*rpcapi.RPCPayload).FromToolListResponse)
}

func (s *Server) handleToolGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolGetRequest)
	if !ok || strings.TrimSpace(params.Id) == "" {
		return invalidParams(req.Id)
	}
	tool, err := s.Tools.GetTool(ctx, params.Id)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, err.Error())
	}
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if !s.profileAllows(profileTools, params.Id) && (tool.OwnerPublicKey == nil || *tool.OwnerPublicKey != s.Caller.String()) {
		return statusError(req.Id, http.StatusNotFound, "tool not found")
	}
	item, err := toolkit.ToRPC(tool)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCPayload).FromToolGetResponse)
}

func (s *Server) handleToolCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	tool, err := s.peerDeviceTool(params)
	if err != nil {
		return statusError(req.Id, http.StatusBadRequest, err.Error())
	}
	if _, err := s.Tools.GetTool(ctx, tool.ID); err == nil {
		return statusError(req.Id, http.StatusConflict, "tool already exists")
	} else if !errors.Is(err, toolkit.ErrToolNotFound) {
		return internalError(req.Id, err.Error())
	}
	stored, err := s.Tools.PutTool(ctx, tool)
	if err != nil {
		return statusError(req.Id, http.StatusBadRequest, err.Error())
	}
	item, err := toolkit.ToRPC(stored)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCPayload).FromToolCreateResponse)
}

func (s *Server) handleToolPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolPutRequest)
	if !ok || strings.TrimSpace(params.Id) == "" || params.Body.Id != params.Id {
		return invalidParams(req.Id)
	}
	tool, err := s.peerDeviceTool(params.Body)
	if err != nil {
		return statusError(req.Id, http.StatusBadRequest, err.Error())
	}
	existing, err := s.Tools.GetTool(ctx, tool.ID)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, err.Error())
	} else if err != nil {
		return internalError(req.Id, err.Error())
	}
	if err := s.validateOwnedDeviceTool(existing); err != nil {
		return statusError(req.Id, http.StatusForbidden, err.Error())
	}
	stored, err := s.Tools.PutTool(ctx, tool)
	if err != nil {
		return statusError(req.Id, http.StatusBadRequest, err.Error())
	}
	item, err := toolkit.ToRPC(stored)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCPayload).FromToolPutResponse)
}

func (s *Server) handleToolDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsToolDeleteRequest)
	if !ok || strings.TrimSpace(params.Id) == "" {
		return invalidParams(req.Id)
	}
	stored, err := s.Tools.GetTool(ctx, params.Id)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, err.Error())
	}
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if err := s.validateOwnedDeviceTool(stored); err != nil {
		return statusError(req.Id, http.StatusForbidden, err.Error())
	}
	if err := s.Tools.DeleteTool(ctx, params.Id); err != nil {
		return internalError(req.Id, err.Error())
	}
	item, err := toolkit.ToRPC(stored)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, item, (*rpcapi.RPCPayload).FromToolDeleteResponse)
}

func (s *Server) peerDeviceTool(value rpcapi.Tool) (toolkit.Tool, error) {
	caller := s.Caller.String()
	if value.Source != rpcapi.ToolSourceDevice {
		return toolkit.Tool{}, errors.New("peer RPC may write only device Tools")
	}
	if !strings.HasPrefix(value.Id, "peer."+caller+".") {
		return toolkit.Tool{}, fmt.Errorf("tool id must use peer.%s. namespace", caller)
	}
	if value.OwnerPeer != nil && strings.TrimSpace(*value.OwnerPeer) != caller {
		return toolkit.Tool{}, errors.New("owner_peer must match the authenticated peer")
	}
	value.OwnerPeer = &caller
	if value.OwnerPublicKey != nil && strings.TrimSpace(*value.OwnerPublicKey) != caller {
		return toolkit.Tool{}, errors.New("owner_public_key must match the authenticated peer")
	}
	value.OwnerPublicKey = &caller
	if value.Executor.Kind != rpcapi.ToolExecutorKindDeviceRpc {
		return toolkit.Tool{}, errors.New("device Tool must use a device_rpc executor")
	}
	if value.Executor.PeerId != nil && strings.TrimSpace(*value.Executor.PeerId) != caller {
		return toolkit.Tool{}, errors.New("executor.peer_id must match the authenticated peer")
	}
	value.Executor.PeerId = &caller
	return toolkit.FromRPC(value)
}

func (s *Server) validateOwnedDeviceTool(tool toolkit.Tool) error {
	caller := s.Caller.String()
	if tool.Source != toolkit.ToolSourceDevice || !strings.HasPrefix(tool.ID, "peer."+caller+".") || tool.OwnerPublicKey == nil || *tool.OwnerPublicKey != caller {
		return errors.New("peer may modify only its own device Tools")
	}
	return nil
}
