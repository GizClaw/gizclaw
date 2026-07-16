package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type rpcPeerService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
	PutSelfInfo(context.Context, giznet.PublicKey, apitypes.DeviceInfo) (apitypes.DeviceInfo, error)
	GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime
}

type rpcPeerRunService interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
	GetRunAgent(context.Context, giznet.PublicKey) (apitypes.PeerRunAgent, error)
	SetRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error)
}

type rpcPeerRunRuntime interface {
	Reload(context.Context) (apitypes.PeerRunStatus, error)
	Status(context.Context) (apitypes.PeerRunStatus, error)
	Stop(context.Context) (apitypes.PeerRunStatus, error)
	WorkspaceState(context.Context) (apitypes.PeerRunWorkspaceState, error)
	ListWorkspaceHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	PlayWorkspaceHistory(context.Context, apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error)
	WorkspaceMemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error)
	WorkspaceRecall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error)
}

type rpcServerResourceService interface {
	Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error)
}

type rpcServerGenXService interface {
	Say(context.Context, peergenx.SayRequest) (peergenx.SayResponse, error)
}

type rpcServer struct {
	peer            rpcPeerService
	peerRun         rpcPeerRunService
	peerRunRuntime  rpcPeerRunRuntime
	serverResources rpcServerResourceService
	serverGenX      rpcServerGenXService
	callerPublicKey giznet.PublicKey
}

func (s *rpcServer) Handle(conn net.Conn) error {
	peerPublicKey := ""
	if !s.callerPublicKey.IsZero() {
		peerPublicKey = s.callerPublicKey.String()
	}
	return handleRPCWithStreamObserved(conn, s.dispatch, s.dispatchStream, &rpcObservationOptions{
		peerPublicKey: peerPublicKey,
	})
}

func (s *rpcServer) dispatchStream(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
	if req == nil {
		return false, nil
	}
	switch req.Method {
	case rpcapi.RPCMethodAllSpeedTestRun:
		return true, s.handleSpeedTest(ctx, stream, req)
	case rpcapi.RPCMethodServerFirmwareFilesDownload:
		return true, s.handleFirmwareBinDownload(ctx, stream, req)
	case rpcapi.RPCMethodServerPetPixaDownload:
		return true, s.handlePetPixaDownload(ctx, stream, req)
	case rpcapi.RPCMethodServerBadgeDefPixaDownload:
		return true, s.handleBadgeDefPixaDownload(ctx, stream, req)
	case rpcapi.RPCMethodServerWorkspaceHistoryAudioGet:
		return true, s.handleWorkspaceHistoryAudioGet(ctx, stream, req)
	default:
		return false, nil
	}
}

func (s *rpcServer) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodAllPing:
		return handleRPCPing(ctx, req)
	case rpcapi.RPCMethodServerInfoGet:
		return s.handleGetInfo(ctx, req)
	case rpcapi.RPCMethodServerInfoPut:
		return s.handlePutInfo(ctx, req)
	case rpcapi.RPCMethodServerRuntimeGet:
		return s.handleGetRuntime(ctx, req)
	case rpcapi.RPCMethodServerStatusGet:
		return s.handleGetStatus(ctx, req)
	case rpcapi.RPCMethodServerRunAgentGet:
		return s.handleGetRunAgent(ctx, req)
	case rpcapi.RPCMethodServerRunAgentSet:
		return s.handleSetRunAgent(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceGet:
		return s.handleGetRunWorkspace(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceSet:
		return s.handleSetRunWorkspace(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceReload:
		return s.handleReloadRunWorkspace(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceHistory:
		return s.handleListRunWorkspaceHistory(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceHistoryPlay:
		return s.handlePlayRunWorkspaceHistory(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceMemoryStats:
		return s.handleGetRunWorkspaceMemoryStats(ctx, req)
	case rpcapi.RPCMethodServerRunWorkspaceRecall:
		return s.handleRunWorkspaceRecall(ctx, req)
	case rpcapi.RPCMethodServerRunReload:
		return s.handleReloadRun(ctx, req)
	case rpcapi.RPCMethodServerRunStatus:
		return s.handleGetRunStatus(ctx, req)
	case rpcapi.RPCMethodServerRunStop:
		return s.handleStopRun(ctx, req)
	case rpcapi.RPCMethodServerRunSay:
		return s.handleServerRunSay(ctx, req)
	default:
		if s.serverResources != nil {
			resp, handled, err := s.serverResources.Dispatch(ctx, req)
			if handled || err != nil {
				return resp, err
			}
		}
		if isPlannedServerMethod(req.Method) {
			return rpcNotImplemented(req.Id, req.Method), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}

func rpcNotImplemented(id string, method rpcapi.RPCMethod) *rpcapi.RPCResponse {
	return rpcapi.Error{
		RequestID: id,
		Code:      rpcapi.RPCErrorCodeMethodNotFound,
		Message:   fmt.Sprintf("method not implemented: %s", method),
	}.RPCResponse()
}

func isPlannedServerMethod(method rpcapi.RPCMethod) bool {
	switch method {
	case rpcapi.RPCMethodServerFirmwareList,
		rpcapi.RPCMethodServerFirmwareGet,
		rpcapi.RPCMethodServerFirmwareFilesDownload,
		rpcapi.RPCMethodServerWorkspaceList,
		rpcapi.RPCMethodServerWorkspaceGet,
		rpcapi.RPCMethodServerWorkspaceCreate,
		rpcapi.RPCMethodServerWorkspacePut,
		rpcapi.RPCMethodServerWorkspaceDelete,
		rpcapi.RPCMethodServerWorkflowList,
		rpcapi.RPCMethodServerWorkflowGet,
		rpcapi.RPCMethodServerModelList,
		rpcapi.RPCMethodServerModelGet,
		rpcapi.RPCMethodServerModelCreate,
		rpcapi.RPCMethodServerModelPut,
		rpcapi.RPCMethodServerModelDelete,
		rpcapi.RPCMethodServerCredentialList,
		rpcapi.RPCMethodServerCredentialGet,
		rpcapi.RPCMethodServerCredentialCreate,
		rpcapi.RPCMethodServerCredentialPut,
		rpcapi.RPCMethodServerCredentialDelete,
		rpcapi.RPCMethodServerContactList,
		rpcapi.RPCMethodServerContactGet,
		rpcapi.RPCMethodServerContactCreate,
		rpcapi.RPCMethodServerContactPut,
		rpcapi.RPCMethodServerContactDelete,
		rpcapi.RPCMethodServerFriendInviteTokenGet,
		rpcapi.RPCMethodServerFriendInviteTokenCreate,
		rpcapi.RPCMethodServerFriendInviteTokenClear,
		rpcapi.RPCMethodServerFriendAdd,
		rpcapi.RPCMethodServerFriendList,
		rpcapi.RPCMethodServerFriendDelete,
		rpcapi.RPCMethodServerFriendGroupList,
		rpcapi.RPCMethodServerFriendGroupGet,
		rpcapi.RPCMethodServerFriendGroupCreate,
		rpcapi.RPCMethodServerFriendGroupPut,
		rpcapi.RPCMethodServerFriendGroupDelete,
		rpcapi.RPCMethodServerFriendGroupInviteTokenGet,
		rpcapi.RPCMethodServerFriendGroupInviteTokenCreate,
		rpcapi.RPCMethodServerFriendGroupInviteTokenClear,
		rpcapi.RPCMethodServerFriendGroupJoin,
		rpcapi.RPCMethodServerFriendGroupMembersList,
		rpcapi.RPCMethodServerFriendGroupMembersAdd,
		rpcapi.RPCMethodServerFriendGroupMembersPut,
		rpcapi.RPCMethodServerFriendGroupMembersDelete,
		rpcapi.RPCMethodServerFriendGroupMessagesList,
		rpcapi.RPCMethodServerFriendGroupMessagesGet,
		rpcapi.RPCMethodServerFriendGroupMessagesSend,
		rpcapi.RPCMethodServerBadgeDefPixaDownload:
		return true
	default:
		return false
	}
}

func (s *rpcServer) handleGetInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerGetInfoRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetInfoResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerPutInfoResponse)
}

func (s *rpcServer) handleGetRuntime(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerGetRuntimeRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer service not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.ServerGetRuntimeResponse](s.peer.GetSelfRuntime(ctx, s.callerPublicKey))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetRuntimeResponse)
}

func (s *rpcServer) handleGetStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerGetStatusRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetStatusResponse)
}

func (s *rpcServer) handleGetRunAgent(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerGetRunAgentRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetRunAgentResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerSetRunAgentResponse)
}

func (s *rpcServer) handleGetRunWorkspace(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerGetRunWorkspaceRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetRunWorkspaceResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerSetRunWorkspaceResponse)
}

func (s *rpcServer) handleReloadRunWorkspace(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerReloadRunWorkspaceRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerReloadRunWorkspaceResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerListRunWorkspaceHistoryResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerPlayRunWorkspaceHistoryResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetRunWorkspaceMemoryStatsResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerRunWorkspaceRecallResponse)
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
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerReloadRunRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerReloadRunResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerGetRunStatusResponse)
}

func (s *rpcServer) handleStopRun(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCPayload.AsServerStopRunRequest); err != nil {
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerStopRunResponse)
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
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerRunSayResponse)
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
