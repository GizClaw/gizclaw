package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
)

func (c *rpcClient) GetPeerInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.PeerGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.PeerGetInfoRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerInfoGet, params), rpcapi.RPCResponse_Result.AsPeerGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) PutPeerInfo(ctx context.Context, conn net.Conn, id string, info rpcapi.PeerPutInfoRequest) (*rpcapi.PeerPutInfoResponse, error) {
	params, err := newRPCRequestParams(info, (*rpcapi.RPCRequest_Params).FromPeerPutInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerInfoPut, params), rpcapi.RPCResponse_Result.AsPeerPutInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) GetPeerRuntime(ctx context.Context, conn net.Conn, id string) (*rpcapi.PeerGetRuntimeResponse, error) {
	params, err := newRPCRequestParams(rpcapi.PeerGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetRuntimeRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerRuntimeGet, params), rpcapi.RPCResponse_Result.AsPeerGetRuntimeResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer runtime", err)
	}
	return result, nil
}

func (c *rpcClient) GetPeerStatus(ctx context.Context, conn net.Conn, id string) (*rpcapi.PeerGetStatusResponse, error) {
	params, err := newRPCRequestParams(rpcapi.PeerGetStatusRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerStatusGet, params), rpcapi.RPCResponse_Result.AsPeerGetStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer status", err)
	}
	return result, nil
}

func (c *rpcClient) PutPeerStatus(ctx context.Context, conn net.Conn, id string, status rpcapi.PeerPutStatusRequest) (*rpcapi.PeerPutStatusResponse, error) {
	params, err := newRPCRequestParams(status, (*rpcapi.RPCRequest_Params).FromPeerPutStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerStatusPut, params), rpcapi.RPCResponse_Result.AsPeerPutStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer status", err)
	}
	return result, nil
}

func (c *rpcClient) GetPeerRunAgent(ctx context.Context, conn net.Conn, id string) (*rpcapi.PeerGetRunAgentResponse, error) {
	params, err := newRPCRequestParams(rpcapi.PeerGetRunAgentRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerRunAgentGet, params), rpcapi.RPCResponse_Result.AsPeerGetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (c *rpcClient) SetPeerRunAgent(ctx context.Context, conn net.Conn, id string, selection rpcapi.PeerSetRunAgentRequest) (*rpcapi.PeerSetRunAgentResponse, error) {
	params, err := newRPCRequestParams(selection, (*rpcapi.RPCRequest_Params).FromPeerSetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerRunAgentSet, params), rpcapi.RPCResponse_Result.AsPeerSetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (s *rpcServer) handleGetInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetInfoRequest); err != nil {
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
	result, err := convertRPCType[rpcapi.PeerGetInfoResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetInfoResponse)
}

func (s *rpcServer) handlePutInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsPeerPutInfoRequest()
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
	result, err := convertRPCType[rpcapi.PeerPutInfoResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerPutInfoResponse)
}

func (s *rpcServer) handleGetRuntime(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetRuntimeRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer service not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.PeerGetRuntimeResponse](s.peer.GetSelfRuntime(ctx, s.callerPublicKey))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetRuntimeResponse)
}

func (s *rpcServer) handleGetStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetStatusRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.GetStatus(ctx, s.callerPublicKey)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.PeerGetStatusResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetStatusResponse)
}

func (s *rpcServer) handlePutStatus(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsPeerPutStatusRequest()
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
	result, err := convertRPCType[rpcapi.PeerPutStatusResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerPutStatusResponse)
}

func (s *rpcServer) handleGetRunAgent(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetRunAgentRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if s.peerRun == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer run service not configured"}.RPCResponse(), nil
	}
	resp, err := s.peerRun.GetRunAgent(ctx, s.callerPublicKey)
	if err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: err.Error()}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.PeerGetRunAgentResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetRunAgentResponse)
}

func (s *rpcServer) handleSetRunAgent(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsPeerSetRunAgentRequest()
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
	result, err := convertRPCType[rpcapi.PeerSetRunAgentResponse](resp)
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerSetRunAgentResponse)
}
