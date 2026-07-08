package gizcli

import (
	"context"
	"net"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCServerServiceClientWrappers(t *testing.T) {
	client := &rpcClient{}

	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerInfoGet, rpcapi.ServerGetInfoResponse{}, (*rpcapi.RPCResponse_Result).FromServerGetInfoResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetInfoResponse, error) {
		return client.GetServerInfo(ctx, conn, "server-info-get")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerInfoPut, rpcapi.ServerPutInfoResponse{}, (*rpcapi.RPCResponse_Result).FromServerPutInfoResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerPutInfoResponse, error) {
		return client.PutServerInfo(ctx, conn, "server-info-put", rpcapi.ServerPutInfoRequest{})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRuntimeGet, rpcapi.ServerGetRuntimeResponse{}, (*rpcapi.RPCResponse_Result).FromServerGetRuntimeResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetRuntimeResponse, error) {
		return client.GetServerRuntime(ctx, conn, "server-runtime-get")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerStatusGet, rpcapi.ServerGetStatusResponse{}, (*rpcapi.RPCResponse_Result).FromServerGetStatusResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetStatusResponse, error) {
		return client.GetServerStatus(ctx, conn, "server-status-get")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunAgentGet, rpcapi.ServerGetRunAgentResponse{}, (*rpcapi.RPCResponse_Result).FromServerGetRunAgentResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetRunAgentResponse, error) {
		return client.GetServerRunAgent(ctx, conn, "server-run-agent-get")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunAgentSet, rpcapi.ServerSetRunAgentResponse{}, (*rpcapi.RPCResponse_Result).FromServerSetRunAgentResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerSetRunAgentResponse, error) {
		return client.SetServerRunAgent(ctx, conn, "server-run-agent-set", rpcapi.ServerSetRunAgentRequest{})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceGet, rpcapi.ServerGetRunWorkspaceResponse{RuntimeState: rpcapi.PeerRunStatusStateStopped}, (*rpcapi.RPCResponse_Result).FromServerGetRunWorkspaceResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
		return client.GetServerRunWorkspace(ctx, conn, "server-run-workspace-get")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceSet, rpcapi.ServerSetRunWorkspaceResponse{RuntimeState: rpcapi.PeerRunStatusStateStopped}, (*rpcapi.RPCResponse_Result).FromServerSetRunWorkspaceResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
		return client.SetServerRunWorkspace(ctx, conn, "server-run-workspace-set", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: "demo"})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceReload, rpcapi.ServerReloadRunWorkspaceResponse{RuntimeState: rpcapi.PeerRunStatusStateRunning}, (*rpcapi.RPCResponse_Result).FromServerReloadRunWorkspaceResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
		return client.ReloadServerRunWorkspace(ctx, conn, "server-run-workspace-reload")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceHistory, rpcapi.ServerListRunWorkspaceHistoryResponse{Available: true, Items: []rpcapi.PeerRunHistoryEntry{}, HasNext: false}, (*rpcapi.RPCResponse_Result).FromServerListRunWorkspaceHistoryResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
		return client.ListServerRunWorkspaceHistory(ctx, conn, "server-run-workspace-history", rpcapi.ServerListRunWorkspaceHistoryRequest{})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceHistoryPlay, rpcapi.ServerPlayRunWorkspaceHistoryResponse{Accepted: true, HistoryId: "h1", State: "playing"}, (*rpcapi.RPCResponse_Result).FromServerPlayRunWorkspaceHistoryResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
		return client.PlayServerRunWorkspaceHistory(ctx, conn, "server-run-workspace-history-play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: "h1"})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceMemoryStats, rpcapi.ServerGetRunWorkspaceMemoryStatsResponse{Available: true, Enabled: true, ItemCount: 1, StorageBytes: 2}, (*rpcapi.RPCResponse_Result).FromServerGetRunWorkspaceMemoryStatsResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
		return client.GetServerRunWorkspaceMemoryStats(ctx, conn, "server-run-workspace-memory", rpcapi.ServerGetRunWorkspaceMemoryStatsRequest{})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunWorkspaceRecall, rpcapi.ServerRunWorkspaceRecallResponse{Available: true, Hits: []rpcapi.PeerRunRecallHit{}}, (*rpcapi.RPCResponse_Result).FromServerRunWorkspaceRecallResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
		return client.ServerRunWorkspaceRecall(ctx, conn, "server-run-workspace-recall", rpcapi.ServerRunWorkspaceRecallRequest{Query: "hello"})
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunReload, rpcapi.ServerReloadRunResponse{}, (*rpcapi.RPCResponse_Result).FromServerReloadRunResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerReloadRunResponse, error) {
		return client.ReloadServerRun(ctx, conn, "server-run-reload")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunStatus, rpcapi.ServerGetRunStatusResponse{}, (*rpcapi.RPCResponse_Result).FromServerGetRunStatusResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(ctx, conn, "server-run-status")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunStop, rpcapi.ServerStopRunResponse{}, (*rpcapi.RPCResponse_Result).FromServerStopRunResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerStopRunResponse, error) {
		return client.StopServerRun(ctx, conn, "server-run-stop")
	})
	runRPCResultWrapperTest(t, rpcapi.RPCMethodServerRunSay, rpcapi.ServerRunSayResponse{Accepted: true}, (*rpcapi.RPCResponse_Result).FromServerRunSayResponse, func(ctx context.Context, conn net.Conn) (*rpcapi.ServerRunSayResponse, error) {
		return client.ServerRunSay(ctx, conn, "server-run-say", rpcapi.ServerRunSayRequest{Text: "hello"})
	})
}
