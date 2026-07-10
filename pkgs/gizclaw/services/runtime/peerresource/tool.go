package peerresource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

const toolOwnerRole = "tool-owner"

type ToolACLService interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
}

func (s *Server) handleToolList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Tools == nil {
		return internalError(req.Id, "toolkit service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsToolListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	tools, err := s.Tools.ListTools(ctx)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	cursor := strings.TrimSpace(valueOrZero(params.Cursor))
	limit := peerListLimit(params.Limit)
	items := make([]rpcapi.Tool, 0, limit)
	hasNext := false
	for _, tool := range tools {
		if tool.ID <= cursor {
			continue
		}
		err := s.authorizeErr(ctx, acl.ToolResource(tool.ID), apitypes.ACLPermissionRead)
		if errors.Is(err, acl.ErrDenied) {
			continue
		}
		if err != nil {
			return authError(req.Id, err)
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
	if resp := s.authorizeResponse(ctx, req.Id, acl.ToolResource(params.Id), apitypes.ACLPermissionRead); resp != nil {
		return resp
	}
	tool, err := s.Tools.GetTool(ctx, params.Id)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, err.Error())
	}
	if err != nil {
		return internalError(req.Id, err.Error())
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
	if resp := s.authorizeResponse(ctx, req.Id, acl.CollectionResource(acl.ResourceKindTool), apitypes.ACLPermissionCreate); resp != nil {
		return resp
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
	if err := s.grantToolOwner(ctx, stored.ID); err != nil {
		_ = s.Tools.DeleteTool(context.WithoutCancel(ctx), stored.ID)
		return internalError(req.Id, err.Error())
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
	if resp := s.authorizeResponse(ctx, req.Id, acl.ToolResource(params.Id), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
	}
	tool, err := s.peerDeviceTool(params.Body)
	if err != nil {
		return statusError(req.Id, http.StatusBadRequest, err.Error())
	}
	if _, err := s.Tools.GetTool(ctx, tool.ID); errors.Is(err, toolkit.ErrToolNotFound) {
		return statusError(req.Id, http.StatusNotFound, err.Error())
	} else if err != nil {
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
	if resp := s.authorizeResponse(ctx, req.Id, acl.ToolResource(params.Id), apitypes.ACLPermissionAdmin); resp != nil {
		return resp
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
	if s.ToolACL != nil {
		if _, err := s.ToolACL.DeletePolicyBinding(context.WithoutCancel(ctx), toolOwnerBindingID(params.Id, s.Caller.String())); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
			return internalError(req.Id, err.Error())
		}
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
	if tool.Source != toolkit.ToolSourceDevice || !strings.HasPrefix(tool.ID, "peer."+caller+".") || tool.OwnerPeer == nil || *tool.OwnerPeer != caller {
		return errors.New("peer may modify only its own device Tools")
	}
	return nil
}

func (s *Server) grantToolOwner(ctx context.Context, toolID string) error {
	if s.ToolACL == nil {
		return errors.New("tool ACL service not configured")
	}
	permissions := apitypes.ACLPermissionList{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse, apitypes.ACLPermissionAdmin}
	if _, err := s.ToolACL.PutRole(ctx, toolOwnerRole, permissions); err != nil {
		return err
	}
	caller := s.Caller.String()
	_, err := s.ToolACL.PutPolicyBinding(ctx, toolOwnerBindingID(toolID, caller), 0, apitypes.ACLPolicy{
		Subject:  acl.PublicKeySubject(caller),
		Resource: acl.ToolResource(toolID),
		Role:     toolOwnerRole,
	})
	return err
}

func toolOwnerBindingID(toolID, owner string) string {
	return "tool-owner:" + url.PathEscape(toolID) + ":" + url.PathEscape(owner)
}
