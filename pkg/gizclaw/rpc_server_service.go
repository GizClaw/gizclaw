package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
)

func (s *rpcServer) handleGetInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerGetInfoRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peer.GetSelfInfo(ctx, s.callerPublicKey)
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return rpcAPIError(req.Id, http.StatusNotFound, apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetInfoResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetInfoResponse)
}

func (s *rpcServer) handlePutInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerPutInfoRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	body, err := convertRPCType[apitypes.DeviceInfo](params)
	if err != nil {
		return nil, err
	}
	if s.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peer.PutSelfInfo(ctx, s.callerPublicKey, body)
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return rpcAPIError(req.Id, http.StatusNotFound, apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerPutInfoResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerPutInfoResponse)
}

func (s *rpcServer) handleGetRuntime(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerGetRuntimeRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer service not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetRuntimeResponse](s.peer.GetSelfRuntime(ctx, s.callerPublicKey))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetRuntimeResponse)
}

func (s *rpcServer) handleGetStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerGetStatusRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.GetStatus(ctx, s.callerPublicKey)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetStatusResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetStatusResponse)
}

func (s *rpcServer) handlePutStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerPutStatusRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	body, err := convertRPCType[apitypes.PeerStatus](params)
	if err != nil {
		return nil, err
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.PutStatus(ctx, s.callerPublicKey, body)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerPutStatusResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerPutStatusResponse)
}

func (s *rpcServer) handleGetRunAgent(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerGetRunAgentRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.GetRunAgent(ctx, s.callerPublicKey)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetRunAgentResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetRunAgentResponse)
}

func (s *rpcServer) handleSetRunAgent(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerSetRunAgentRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	selection, err := convertRPCType[apitypes.AgentSelection](params)
	if err != nil {
		return nil, err
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.SetRunAgent(ctx, s.callerPublicKey, selection)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerSetRunAgentResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerSetRunAgentResponse)
}

func (s *rpcServer) handleReloadRun(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerReloadRunRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRunRuntime.Reload(ctx)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerReloadRunResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerReloadRunResponse)
}

func (s *rpcServer) handleGetRunStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsServerGetRunStatusRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRunRuntime.Status(ctx)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	friendOTP := resp.FriendOtp
	if params.FriendOtp != nil {
		friendOTP = params.FriendOtp
		resp.FriendOtp = params.FriendOtp
	}
	if s.friendOTPs != nil && friendOTP != nil {
		if err := s.friendOTPs.ReportFriendOTP(ctx, s.callerPublicKey.String(), *friendOTP); err != nil {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
		}
	}
	result, err := convertRPCType[rpcapi.ServerGetRunStatusResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetRunStatusResponse)
}

func (s *rpcServer) handleStopRun(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerStopRunRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRunRuntime.Stop(ctx)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerStopRunResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerStopRunResponse)
}

func (s *rpcServer) handleServerRunSay(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerRunSayRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.serverGenX == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peergenx service not configured"}.RPCResponse(), nil
	}
	resp, err := s.serverGenX.Say(ctx, peergenx.SayRequest{
		Text:           params.Text,
		VoiceID:        stringPtrValue(params.VoiceId),
		ModelID:        stringPtrValue(params.ModelId),
		CredentialName: stringPtrValue(params.CredentialName),
	})
	if err != nil {
		switch {
		case errors.Is(err, peergenx.ErrDenied):
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeForbidden, Message: err.Error()}.RPCResponse(), nil
		case errors.Is(err, peergenx.ErrInvalid):
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: err.Error()}.RPCResponse(), nil
		case errors.Is(err, peergenx.ErrNotConfigured):
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
		}
	}
	result := rpcapi.ServerRunSayResponse{Accepted: resp.Accepted}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerRunSayResponse)
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
