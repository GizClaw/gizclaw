package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type edgeRPCServer struct {
	routes *peerroute.Server
}

func (s *edgeRPCServer) Handle(conn net.Conn) error {
	return handleRPCWithStream(conn, s.dispatch, nil)
}

func (s *edgeRPCServer) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodEdgePeerLookup:
		return s.handleLookup(ctx, req), nil
	case rpcapi.RPCMethodEdgePeerAssign:
		return s.handleAssign(ctx, req), nil
	case rpcapi.RPCMethodEdgeRouteResolve:
		return s.handleResolve(ctx, req), nil
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}

func (s *edgeRPCServer) handleLookup(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsEdgePeerLookupRequest)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}.RPCResponse()
	}
	publicKey, err := peerroute.ParsePublicKey(params.PeerPublicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	assignment, err := s.routes.Lookup(ctx, publicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	return edgeRPCResult(req.Id, rpcapi.EdgePeerLookupResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromEdgePeerLookupResponse)
}

func (s *edgeRPCServer) handleAssign(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsEdgePeerAssignRequest)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}.RPCResponse()
	}
	publicKey, err := peerroute.ParsePublicKey(params.PeerPublicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	assignment, err := s.routes.Assign(ctx, publicKey, params.ExpectedVersion)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	return edgeRPCResult(req.Id, rpcapi.EdgePeerAssignResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromEdgePeerAssignResponse)
}

func (s *edgeRPCServer) handleResolve(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsEdgeRouteResolveRequest)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}.RPCResponse()
	}
	if params.CallerPublicKey != nil && *params.CallerPublicKey != "" {
		if _, err := peerroute.ParsePublicKey(*params.CallerPublicKey); err != nil {
			return edgeRPCError(req.Id, err)
		}
	}
	publicKey, err := peerroute.ParsePublicKey(params.TargetPeerPublicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	assignment, err := s.routes.Resolve(ctx, publicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	return edgeRPCResult(req.Id, rpcapi.EdgeRouteResolveResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromEdgeRouteResolveResponse)
}

func edgeRequiredParams[T any](req *rpcapi.RPCRequest, decode func(rpcapi.RPCPayload) (T, error)) (T, error) {
	var zero T
	if req == nil || req.Params == nil {
		return zero, errors.New("params required")
	}
	return decode(*req.Params)
}

func edgeRPCResult[T any](id string, value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCResponse {
	var body rpcapi.RPCPayload
	if err := encode(&body, value); err != nil {
		return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse()
	}
	return &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: id, Result: &body}
}

func edgeRPCError(id string, err error) *rpcapi.RPCResponse {
	code := rpcapi.RPCErrorCodeInternalError
	switch {
	case errors.Is(err, peerroute.ErrInvalidPublicKey):
		code = rpcapi.RPCErrorCodeInvalidParams
	case errors.Is(err, peerroute.ErrAssignmentNotFound), errors.Is(err, peerroute.ErrPeerInactive), errors.Is(err, peer.ErrPeerNotFound), errors.Is(err, kv.ErrNotFound):
		code = rpcapi.RPCErrorCodeNotFound
	case errors.Is(err, peerroute.ErrVersionConflict):
		code = rpcapi.RPCErrorCodeConflict
	case errors.Is(err, peerroute.ErrMissingRoute), errors.Is(err, peerroute.ErrPeerStoreNil), errors.Is(err, peerroute.ErrStoreNil):
		code = rpcapi.RPCErrorCodeInternalError
	default:
		code = rpcapi.RPCErrorCodeInternalError
	}
	return rpcapi.Error{RequestID: id, Code: code, Message: err.Error()}.RPCResponse()
}
