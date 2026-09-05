package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type rpcRuntimeSettings interface {
	SetDebugMode(context.Context, giznet.PublicKey, string) error
}

func (s *rpcServer) handlePutRuntime(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsServerPutRuntimeRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	settings, ok := s.peerRun.(rpcRuntimeSettings)
	if !ok {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInternal, Message: "runtime settings unavailable"}.RPCResponse(), nil
	}
	if err := settings.SetDebugMode(ctx, s.callerPublicKey, params.DebugMode); err != nil {
		if errors.Is(err, peerrun.ErrInvalidDebugMode) {
			return rpcInvalidParams(req.Id), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInternal, Message: "cannot update runtime settings"}.RPCResponse(), nil
	}
	return newRPCResultResponse(req.Id, rpcapi.ServerPutRuntimeResponse{DebugMode: params.DebugMode}, (*rpcapi.RPCPayload).FromServerPutRuntimeResponse)
}
