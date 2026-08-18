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
	owner, failure := s.apiKeyRPCOwner(ctx, req.Id)
	if failure != nil {
		return failure, nil
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

func (s *rpcServer) handleAPIKeyList(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsAPIKeyListRequest()
	if err != nil || params.Limit < 0 {
		return rpcInvalidParams(req.Id), nil
	}
	owner, failure := s.apiKeyRPCOwner(ctx, req.Id)
	if failure != nil {
		return failure, nil
	}
	limit := min(params.Limit, 100)
	result, err := s.apiKeys.ListOwner(ctx, owner, params.Cursor, int(limit))
	if errors.Is(err, apikey.ErrInvalidCursor) {
		return rpcInvalidParams(req.Id), nil
	}
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key listing failed"}.RPCResponse(), nil
	}
	items := make([]rpcapi.APIKey, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, rpcAPIKey(item))
	}
	response := rpcapi.APIKeyListResponse{Items: items, NextCursor: result.NextCursor}
	return newRPCResultResponse(req.Id, response, (*rpcapi.RPCPayload).FromAPIKeyListResponse)
}

func (s *rpcServer) handleAPIKeyRevoke(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsAPIKeyRevokeRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	owner, failure := s.apiKeyRPCOwner(ctx, req.Id)
	if failure != nil {
		return failure, nil
	}
	err = s.apiKeys.RevokeOwner(ctx, owner, params.Name)
	switch {
	case errors.Is(err, apikey.ErrInvalidName):
		return rpcInvalidParams(req.Id), nil
	case errors.Is(err, apikey.ErrNotFound):
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeNotFound, Message: "API key not found"}.RPCResponse(), nil
	case err != nil:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key revocation failed"}.RPCResponse(), nil
	}
	return newRPCResultResponse(req.Id, rpcapi.APIKeyRevokeResponse{}, (*rpcapi.RPCPayload).FromAPIKeyRevokeResponse)
}

func (s *rpcServer) apiKeyRPCOwner(ctx context.Context, requestID string) (string, *rpcapi.RPCResponse) {
	if s.apiKeys == nil || s.registrations == nil || s.validateAPIKeyOwner == nil || s.callerPublicKey.IsZero() {
		return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key service not configured"}.RPCResponse()
	}
	owner := s.callerPublicKey.String()
	if err := s.validateAPIKeyOwner(ctx, s.callerPublicKey); err != nil {
		switch {
		case errors.Is(err, peer.ErrPeerPendingDeletion):
			return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeConflict, Message: "Peer deletion is pending"}.RPCResponse()
		case errors.Is(err, peer.ErrPeerNotFound), errors.Is(err, peer.ErrPeerDeleted), errors.Is(err, errAPIKeyOwnerUnavailable):
			return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeForbidden, Message: "active Client registration required"}.RPCResponse()
		default:
			return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeInternalError, Message: "API key owner validation failed"}.RPCResponse()
		}
	}
	if _, err := s.registrations.ResolveOwnerProfile(ctx, owner); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeForbidden, Message: "device registration required"}.RPCResponse()
		}
		return "", rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeInternalError, Message: "RuntimeProfile lookup failed"}.RPCResponse()
	}
	return owner, nil
}

func rpcAPIKey(key apikey.Key) rpcapi.APIKey {
	return rpcapi.APIKey{
		Name: key.Name, DisplayName: key.DisplayName, Prefix: key.Prefix,
		ManageAPIKeys: key.ManageAPIKeys, CreatedAt: key.CreatedAt.Format(time.RFC3339Nano),
	}
}
