package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

type rpcClient struct {
	peer *Client
}

func (c *rpcClient) Handle(conn net.Conn) error {
	return handleRPC(conn, c.dispatch)
}

func (c *rpcClient) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodPeerInfoGet:
		return c.handleGetPeerInfo(ctx, req)
	case rpcapi.RPCMethodPeerIdentifiersGet:
		return c.handleGetPeerIdentifiers(ctx, req)
	case rpcapi.RPCMethodPeerPing:
		return handleRPCPing(ctx, req)
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "unsupported method: " + string(req.Method)}.RPCResponse(), nil
	}
}

func (c *rpcClient) Ping(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	return callRPCPing(ctx, conn, id)
}

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

func (c *rpcClient) GetPeerIdentifiers(ctx context.Context, conn net.Conn, id string) (*rpcapi.PeerGetIdentifiersResponse, error) {
	params, err := newRPCRequestParams(rpcapi.PeerGetIdentifiersRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetIdentifiersRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerIdentifiersGet, params), rpcapi.RPCResponse_Result.AsPeerGetIdentifiersResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer identifiers", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetInfoRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerInfoGet, params), rpcapi.RPCResponse_Result.AsServerGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("server info", err)
	}
	return result, nil
}

func (c *rpcClient) handleGetPeerInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetInfoRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.PeerGetInfoResponse](gearDeviceToPeerRefreshInfo(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetInfoResponse)
}

func (c *rpcClient) handleGetPeerIdentifiers(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsPeerGetIdentifiersRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.peer == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInternalError, Message: "peer client not configured"}.RPCResponse(), nil
	}
	result, err := convertRPCType[rpcapi.PeerGetIdentifiersResponse](gearDeviceToPeerRefreshIdentifiers(c.peer.Device))
	if err != nil {
		return nil, err
	}
	return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPeerGetIdentifiersResponse)
}

func (c *rpcClient) GetConfig(ctx context.Context, conn net.Conn, id string) (*rpcapi.GearGetConfigResponse, error) {
	params, err := newRPCRequestParams(rpcapi.GearGetConfigRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetConfigRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearConfigGet, params), rpcapi.RPCResponse_Result.AsGearGetConfigResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear config", err)
	}
	return result, nil
}

func (c *rpcClient) GetInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.GearGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.GearGetInfoRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearInfoGet, params), rpcapi.RPCResponse_Result.AsGearGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear info", err)
	}
	return result, nil
}

func (c *rpcClient) PutInfo(ctx context.Context, conn net.Conn, id string, info rpcapi.GearPutInfoRequest) (*rpcapi.GearPutInfoResponse, error) {
	params, err := newRPCRequestParams(info, (*rpcapi.RPCRequest_Params).FromGearPutInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearInfoPut, params), rpcapi.RPCResponse_Result.AsGearPutInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear info", err)
	}
	return result, nil
}

func (c *rpcClient) GetRegistration(ctx context.Context, conn net.Conn, id string) (*rpcapi.GearGetRegistrationResponse, error) {
	params, err := newRPCRequestParams(rpcapi.GearGetRegistrationRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetRegistrationRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearRegistrationGet, params), rpcapi.RPCResponse_Result.AsGearGetRegistrationResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear registration", err)
	}
	return result, nil
}

func (c *rpcClient) RegisterGear(ctx context.Context, conn net.Conn, id string, request rpcapi.GearRegisterRequest) (*rpcapi.GearRegisterResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromGearRegisterRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearRegistrationRegister, params), rpcapi.RPCResponse_Result.AsGearRegisterResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear registration", err)
	}
	return result, nil
}

func (c *rpcClient) GetRuntime(ctx context.Context, conn net.Conn, id string) (*rpcapi.GearGetRuntimeResponse, error) {
	params, err := newRPCRequestParams(rpcapi.GearGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetRuntimeRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearRuntimeGet, params), rpcapi.RPCResponse_Result.AsGearGetRuntimeResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear runtime", err)
	}
	return result, nil
}
