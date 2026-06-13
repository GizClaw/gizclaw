package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func (c *rpcClient) GetServerInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetInfoRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerInfoGet, params), rpcapi.RPCResponse_Result.AsServerGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) PutServerInfo(ctx context.Context, conn net.Conn, id string, info rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
	params, err := newRPCRequestParams(info, (*rpcapi.RPCRequest_Params).FromServerPutInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerInfoPut, params), rpcapi.RPCResponse_Result.AsServerPutInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRuntime(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetRuntimeResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetRuntimeRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRuntimeGet, params), rpcapi.RPCResponse_Result.AsServerGetRuntimeResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer runtime", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerStatus(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetStatusResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetStatusRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerStatusGet, params), rpcapi.RPCResponse_Result.AsServerGetStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer status", err)
	}
	return result, nil
}

func (c *rpcClient) PutServerStatus(ctx context.Context, conn net.Conn, id string, status rpcapi.ServerPutStatusRequest) (*rpcapi.ServerPutStatusResponse, error) {
	params, err := newRPCRequestParams(status, (*rpcapi.RPCRequest_Params).FromServerPutStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerStatusPut, params), rpcapi.RPCResponse_Result.AsServerPutStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer status", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRunAgent(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetRunAgentResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetRunAgentRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunAgentGet, params), rpcapi.RPCResponse_Result.AsServerGetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (c *rpcClient) SetServerRunAgent(ctx context.Context, conn net.Conn, id string, selection rpcapi.ServerSetRunAgentRequest) (*rpcapi.ServerSetRunAgentResponse, error) {
	params, err := newRPCRequestParams(selection, (*rpcapi.RPCRequest_Params).FromServerSetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunAgentSet, params), rpcapi.RPCResponse_Result.AsServerSetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (c *rpcClient) ReloadServerRun(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerReloadRunResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerReloadRunRequest{}, (*rpcapi.RPCRequest_Params).FromServerReloadRunRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunReload, params), rpcapi.RPCResponse_Result.AsServerReloadRunResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run reload", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRunStatus(ctx context.Context, conn net.Conn, id string, request ...rpcapi.ServerGetRunStatusRequest) (*rpcapi.ServerGetRunStatusResponse, error) {
	req := rpcapi.ServerGetRunStatusRequest{}
	if len(request) > 0 {
		req = request[0]
	}
	params, err := newRPCRequestParams(req, (*rpcapi.RPCRequest_Params).FromServerGetRunStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunStatus, params), rpcapi.RPCResponse_Result.AsServerGetRunStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run status", err)
	}
	return result, nil
}

func (c *rpcClient) StopServerRun(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerStopRunResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerStopRunRequest{}, (*rpcapi.RPCRequest_Params).FromServerStopRunRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunStop, params), rpcapi.RPCResponse_Result.AsServerStopRunResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run stop", err)
	}
	return result, nil
}

func (c *rpcClient) ServerRunSay(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRunSayRequest) (*rpcapi.ServerRunSayResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromServerRunSayRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunSay, params), rpcapi.RPCResponse_Result.AsServerRunSayResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run say", err)
	}
	return result, nil
}
