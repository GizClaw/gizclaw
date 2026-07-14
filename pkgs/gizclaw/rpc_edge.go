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
	case rpcapi.RPCMethodServerPeerLookup:
		return s.handleLookup(ctx, req), nil
	case rpcapi.RPCMethodServerPeerAssign:
		return s.handleAssign(ctx, req), nil
	case rpcapi.RPCMethodServerRouteResolve:
		return s.handleResolve(ctx, req), nil
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}

func (s *edgeRPCServer) handleLookup(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsServerPeerLookupRequest)
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
	return edgeRPCResult(req.Id, rpcapi.ServerPeerLookupResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromServerPeerLookupResponse)
}

func (s *edgeRPCServer) handleAssign(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsServerPeerAssignRequest)
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
	return edgeRPCResult(req.Id, rpcapi.ServerPeerAssignResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromServerPeerAssignResponse)
}

func (s *edgeRPCServer) handleResolve(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s == nil || s.routes == nil {
		return edgeRPCError(req.Id, peerroute.ErrStoreNil)
	}
	params, err := edgeRequiredParams(req, rpcapi.RPCPayload.AsServerRouteResolveRequest)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}.RPCResponse()
	}
	publicKey, err := peerroute.ParsePublicKey(params.TargetPeerPublicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	assignment, err := s.routes.Resolve(ctx, publicKey)
	if err != nil {
		return edgeRPCError(req.Id, err)
	}
	return edgeRPCResult(req.Id, rpcapi.ServerRouteResolveResponse{Assignment: peerroute.ToRPC(assignment)}, (*rpcapi.RPCPayload).FromServerRouteResolveResponse)
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
	case errors.Is(err, peerroute.ErrInvalidPublicKey), errors.Is(err, peerroute.ErrPeerNotAssignable):
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
