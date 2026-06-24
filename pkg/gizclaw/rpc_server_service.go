package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/peerrun"
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

func (s *rpcServer) handleGetRunWorkspace(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerGetRunWorkspaceRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	state, resp := s.runWorkspaceState(ctx, req.Id, nil, nil)
	if resp != nil {
		return resp, nil
	}
	result, err := convertRPCType[rpcapi.ServerGetRunWorkspaceResponse](state)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetRunWorkspaceResponse)
}

func (s *rpcServer) handleSetRunWorkspace(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerSetRunWorkspaceRequest()
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
	agent, err := s.peerRun.SetRunAgent(ctx, s.callerPublicKey, selection)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	state, resp := s.runWorkspaceState(ctx, req.Id, &agent, nil)
	if resp != nil {
		return resp, nil
	}
	result, err := convertRPCType[rpcapi.ServerSetRunWorkspaceResponse](state)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerSetRunWorkspaceResponse)
}

func (s *rpcServer) handleReloadRunWorkspace(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsServerReloadRunWorkspaceRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	status, err := s.peerRunRuntime.Reload(ctx)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	state, resp := s.runWorkspaceState(ctx, req.Id, nil, &status)
	if resp != nil {
		return resp, nil
	}
	result, err := convertRPCType[rpcapi.ServerReloadRunWorkspaceResponse](state)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerReloadRunWorkspaceResponse)
}

func (s *rpcServer) handleListRunWorkspaceHistory(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsServerListRunWorkspaceHistoryRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	request, err := convertRPCType[apitypes.PeerRunHistoryListRequest](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.peerRunRuntime.ListWorkspaceHistory(ctx, request)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerListRunWorkspaceHistoryResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerListRunWorkspaceHistoryResponse)
}

func (s *rpcServer) handlePlayRunWorkspaceHistory(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerPlayRunWorkspaceHistoryRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	request, err := convertRPCType[apitypes.PeerRunHistoryPlayRequest](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.peerRunRuntime.PlayWorkspaceHistory(ctx, request)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerPlayRunWorkspaceHistoryResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerPlayRunWorkspaceHistoryResponse)
}

func (s *rpcServer) handleGetRunWorkspaceMemoryStats(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcInvalidParams(req.Id), nil
	}
	params, err := req.Params.AsServerGetRunWorkspaceMemoryStatsRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	request, err := convertRPCType[apitypes.PeerRunMemoryStatsRequest](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.peerRunRuntime.WorkspaceMemoryStats(ctx, request)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetRunWorkspaceMemoryStatsResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerGetRunWorkspaceMemoryStatsResponse)
}

func (s *rpcServer) handleRunWorkspaceRecall(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsServerRunWorkspaceRecallRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	request, err := convertRPCType[apitypes.PeerRunRecallRequest](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.peerRunRuntime.WorkspaceRecall(ctx, request)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerRunWorkspaceRecallResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromServerRunWorkspaceRecallResponse)
}

func (s *rpcServer) runWorkspaceState(ctx context.Context, requestID string, agent *apitypes.PeerRunAgent, status *apitypes.PeerRunStatus) (apitypes.PeerRunWorkspaceState, *rpcapi.RPCResponse) {
	if s.peerRun == nil {
		return apitypes.PeerRunWorkspaceState{}, rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse()
	}
	if agent == nil {
		got, err := s.peerRun.GetRunAgent(ctx, s.callerPublicKey)
		if err != nil {
			return apitypes.PeerRunWorkspaceState{}, rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse()
		}
		agent = &got
	}
	state := apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateStopped}
	if s.peerRunRuntime != nil {
		runtimeState, err := s.peerRunRuntime.WorkspaceState(ctx)
		if err != nil {
			if errors.Is(err, peerrun.ErrRunAgentNotConfigured) {
				state = apitypes.PeerRunWorkspaceState{RuntimeState: apitypes.PeerRunStatusStateStopped}
			} else {
				return apitypes.PeerRunWorkspaceState{}, rpcapi.Error{RequestID: requestID, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse()
			}
		} else {
			state = runtimeState
			if state.RuntimeState == "" {
				state.RuntimeState = apitypes.PeerRunStatusStateStopped
			}
		}
	}
	if status != nil {
		mergeRunWorkspaceStatus(&state, *status)
	}
	mergeRunWorkspaceAgent(&state, *agent)
	return state, nil
}

func mergeRunWorkspaceAgent(state *apitypes.PeerRunWorkspaceState, agent apitypes.PeerRunAgent) {
	if agent.Active != nil {
		value := strings.TrimSpace(agent.Active.WorkspaceName)
		if value != "" {
			state.ActiveWorkspaceName = &value
		}
	}
	if agent.Pending != nil {
		value := strings.TrimSpace(agent.Pending.WorkspaceName)
		if value != "" {
			state.PendingWorkspaceName = &value
		}
	}
	selected := ""
	if state.PendingWorkspaceName != nil {
		selected = *state.PendingWorkspaceName
	}
	if selected == "" && state.ActiveWorkspaceName != nil {
		selected = *state.ActiveWorkspaceName
	}
	if selected == "" {
		selected = strings.TrimSpace(state.WorkspaceName)
	}
	state.WorkspaceName = selected
	if selected != "" {
		state.SelectedWorkspaceName = &selected
	}
}

func mergeRunWorkspaceStatus(state *apitypes.PeerRunWorkspaceState, status apitypes.PeerRunStatus) {
	if status.State != "" {
		state.RuntimeState = status.State
	}
	if status.WorkspaceName != nil && strings.TrimSpace(*status.WorkspaceName) != "" {
		active := strings.TrimSpace(*status.WorkspaceName)
		state.ActiveWorkspaceName = &active
		if state.WorkspaceName == "" {
			state.WorkspaceName = active
		}
	}
	if status.Message != nil {
		state.Message = status.Message
	}
	if status.StartedAt != nil {
		state.StartedAt = status.StartedAt
	}
	if status.UpdatedAt != nil {
		state.UpdatedAt = status.UpdatedAt
	}
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
	if _, err := req.Params.AsServerGetRunStatusRequest(); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRunRuntime == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run runtime not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRunRuntime.Status(ctx)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeBadRequest, Message: err.Error()}.RPCResponse(), nil
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
