package gizcli

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (c *rpcClient) GetServerInfo(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetInfoResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetInfoRequest{}, (*rpcapi.RPCPayload).FromServerGetInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerInfoGet, params), rpcapi.RPCPayload.AsServerGetInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) PutServerInfo(ctx context.Context, conn net.Conn, id string, info rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
	params, err := newRPCRequestParams(info, (*rpcapi.RPCPayload).FromServerPutInfoRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerInfoPut, params), rpcapi.RPCPayload.AsServerPutInfoResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer info", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRuntime(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetRuntimeResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetRuntimeRequest{}, (*rpcapi.RPCPayload).FromServerGetRuntimeRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRuntimeGet, params), rpcapi.RPCPayload.AsServerGetRuntimeResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer runtime", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerStatus(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetStatusResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetStatusRequest{}, (*rpcapi.RPCPayload).FromServerGetStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerStatusGet, params), rpcapi.RPCPayload.AsServerGetStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer status", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRunAgent(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetRunAgentResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetRunAgentRequest{}, (*rpcapi.RPCPayload).FromServerGetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunAgentGet, params), rpcapi.RPCPayload.AsServerGetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (c *rpcClient) SetServerRunAgent(ctx context.Context, conn net.Conn, id string, selection rpcapi.ServerSetRunAgentRequest) (*rpcapi.ServerSetRunAgentResponse, error) {
	params, err := newRPCRequestParams(selection, (*rpcapi.RPCPayload).FromServerSetRunAgentRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunAgentSet, params), rpcapi.RPCPayload.AsServerSetRunAgentResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run agent", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRunWorkspace(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerGetRunWorkspaceRequest{}, (*rpcapi.RPCPayload).FromServerGetRunWorkspaceRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceGet, params), rpcapi.RPCPayload.AsServerGetRunWorkspaceResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace", err)
	}
	return result, nil
}

func (c *rpcClient) SetServerRunWorkspace(ctx context.Context, conn net.Conn, id string, selection rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
	params, err := newRPCRequestParams(selection, (*rpcapi.RPCPayload).FromServerSetRunWorkspaceRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceSet, params), rpcapi.RPCPayload.AsServerSetRunWorkspaceResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace", err)
	}
	return result, nil
}

func (c *rpcClient) ReloadServerRunWorkspace(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerReloadRunWorkspaceRequest{}, (*rpcapi.RPCPayload).FromServerReloadRunWorkspaceRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceReload, params), rpcapi.RPCPayload.AsServerReloadRunWorkspaceResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace reload", err)
	}
	return result, nil
}

func (c *rpcClient) ListServerRunWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerListRunWorkspaceHistoryRequest) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerListRunWorkspaceHistoryRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceHistory, params), rpcapi.RPCPayload.AsServerListRunWorkspaceHistoryResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace history", err)
	}
	return result, nil
}

func (c *rpcClient) PlayServerRunWorkspaceHistory(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerPlayRunWorkspaceHistoryRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceHistoryPlay, params), rpcapi.RPCPayload.AsServerPlayRunWorkspaceHistoryResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace history play", err)
	}
	return result, nil
}

func (c *rpcClient) GetServerRunWorkspaceMemoryStats(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerGetRunWorkspaceMemoryStatsRequest) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerGetRunWorkspaceMemoryStatsRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceMemoryStats, params), rpcapi.RPCPayload.AsServerGetRunWorkspaceMemoryStatsResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace memory stats", err)
	}
	return result, nil
}

func (c *rpcClient) ServerRunWorkspaceRecall(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRunWorkspaceRecallRequest) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerRunWorkspaceRecallRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunWorkspaceRecall, params), rpcapi.RPCPayload.AsServerRunWorkspaceRecallResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run workspace recall", err)
	}
	return result, nil
}

func (c *rpcClient) ReloadServerRun(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerReloadRunResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerReloadRunRequest{}, (*rpcapi.RPCPayload).FromServerReloadRunRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunReload, params), rpcapi.RPCPayload.AsServerReloadRunResponse)
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
	params, err := newRPCRequestParams(req, (*rpcapi.RPCPayload).FromServerGetRunStatusRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunStatus, params), rpcapi.RPCPayload.AsServerGetRunStatusResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run status", err)
	}
	return result, nil
}

func (c *rpcClient) StopServerRun(ctx context.Context, conn net.Conn, id string) (*rpcapi.ServerStopRunResponse, error) {
	params, err := newRPCRequestParams(rpcapi.ServerStopRunRequest{}, (*rpcapi.RPCPayload).FromServerStopRunRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunStop, params), rpcapi.RPCPayload.AsServerStopRunResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run stop", err)
	}
	return result, nil
}

func (c *rpcClient) ServerRunSay(ctx context.Context, conn net.Conn, id string, request rpcapi.ServerRunSayRequest) (*rpcapi.ServerRunSayResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromServerRunSayRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodServerRunSay, params), rpcapi.RPCPayload.AsServerRunSayResponse)
	if err != nil {
		return nil, wrapRPCResultError("peer run say", err)
	}
	return result, nil
}
