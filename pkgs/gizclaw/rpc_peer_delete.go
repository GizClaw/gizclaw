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
	if s.peer == nil && s.deletePeerSelf == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "peer service not configured")
	}
	var err error
	if s.deletePeerSelf != nil {
		err = s.deletePeerSelf(ctx)
	} else {
		err = s.peer.DeleteSelf(ctx, s.callerPublicKey)
	}
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeNotFound, err.Error())
		}
		if errors.Is(err, ErrPeerConnNotActive) {
			return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeConflict, err.Error())
		}
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, err.Error())
	}
	if s.onPeerRetiring != nil {
		s.onPeerRetiring()
	}
	if s.onPeerDeleted != nil {
		defer s.onPeerDeleted()
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
	return nil
}
