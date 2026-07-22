package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
)

func (s *rpcServer) handlePeerDelete(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerPeerDeleteRequest); err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	if err := stream.ReadEOS(); err != nil {
		return err
	}
	if s.peer == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "peer service not configured")
	}
	if err := s.peer.DeleteSelf(ctx, s.callerPublicKey); err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeNotFound, err.Error())
		}
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	resp, err := newRPCResultResponse(req.Id, rpcapi.ServerPeerDeleteResponse{}, (*rpcapi.RPCPayload).FromServerPeerDeleteResponse)
	if err != nil {
		return err
	}
	if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
		return err
	}
	if err := stream.WriteEOS(); err != nil {
		return err
	}
	if s.onPeerDeleted != nil {
		_ = s.onPeerDeleted()
	}
	return nil
}
