package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

type rpcClient struct{}

func (c *rpcClient) Handle(conn net.Conn) error {
	return handleRPC(conn, c.dispatch)
}

func (c *rpcClient) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodPeerPing:
		return handleRPCPing(ctx, req)
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: "unsupported method: " + string(req.Method)}.RPCResponse(), nil
	}
}

func (c *rpcClient) Ping(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	return callRPCPing(ctx, conn, id)
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

func (c *rpcClient) GetOTA(ctx context.Context, conn net.Conn, id string) (*rpcapi.GearGetOTAResponse, error) {
	params, err := newRPCRequestParams(rpcapi.GearGetOTARequest{}, (*rpcapi.RPCRequest_Params).FromGearGetOTARequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodGearOtaGet, params), rpcapi.RPCResponse_Result.AsGearGetOTAResponse)
	if err != nil {
		return nil, wrapRPCResultError("gear ota", err)
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
