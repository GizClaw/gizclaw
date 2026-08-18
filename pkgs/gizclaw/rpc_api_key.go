package gizclaw

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *rpcServer) handleAPIKeyCreate(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsAPIKeyCreateRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.apiKeys == nil || s.registrations == nil || s.validateAPIKeyOwner == nil || s.callerPublicKey.IsZero() {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key service not configured"}.RPCResponse(), nil
	}
	owner := s.callerPublicKey.String()
	if err := s.validateAPIKeyOwner(ctx, s.callerPublicKey); err != nil {
		switch {
		case errors.Is(err, peer.ErrPeerPendingDeletion):
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeConflict, Message: "Peer deletion is pending"}.RPCResponse(), nil
		case errors.Is(err, peer.ErrPeerNotFound), errors.Is(err, peer.ErrPeerDeleted), errors.Is(err, errAPIKeyOwnerUnavailable):
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeForbidden, Message: "active Client registration required"}.RPCResponse(), nil
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key owner validation failed"}.RPCResponse(), nil
		}
	}
	if _, err := s.registrations.ResolveOwnerProfile(ctx, owner); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeForbidden, Message: "device registration required"}.RPCResponse(), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "RuntimeProfile lookup failed"}.RPCResponse(), nil
	}
	created, err := s.apiKeys.Create(ctx, owner, params.DisplayName, params.ManageAPIKeys)
	if errors.Is(err, apikey.ErrInvalidDisplayName) {
		return rpcInvalidParams(req.Id), nil
	}
	if errors.Is(err, apikey.ErrOwnerRetired) {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeConflict, Message: "Peer is deleted"}.RPCResponse(), nil
	}
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key creation failed"}.RPCResponse(), nil
	}
	response := rpcapi.APIKeyCreateResponse{
		Value: &rpcapi.APIKey{
			Name: created.Key.Name, DisplayName: created.Key.DisplayName,
			Prefix: created.Key.Prefix, ManageAPIKeys: created.Key.ManageAPIKeys,
			CreatedAt: created.Key.CreatedAt.Format(time.RFC3339Nano),
		},
		APIKey: created.Secret,
	}
	return newRPCResultResponse(req.Id, response, (*rpcapi.RPCPayload).FromAPIKeyCreateResponse)
}
