package gizclaw

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRPCServerPeerMethods(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	publicKey := giznet.PublicKey{1, 2, 3}
	fake := &fakeRPCPeerService{
		t:               t,
		wantPublicKey:   publicKey,
		info:            apitypes.DeviceInfo{Name: stringPtr("peer-1")},
		runtime:         apitypes.Runtime{Online: true, LastSeenAt: now},
		putInfoResponse: apitypes.DeviceInfo{Name: stringPtr("peer-2")},
	}
	runRuntime := &fakeRPCPeerRunRuntime{
		reload: apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateRunning, WorkspaceName: stringPtr("demo")},
		status: apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateRunning, WorkspaceName: stringPtr("demo")},
		stop:   apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateStopped, WorkspaceName: stringPtr("demo")},
		workspaceState: apitypes.PeerRunWorkspaceState{
			RuntimeState:         apitypes.PeerRunStatusStateRunning,
			WorkspaceName:        "demo",
			HistoryAvailable:     boolPtr(true),
			MemoryStatsAvailable: boolPtr(true),
			RecallAvailable:      boolPtr(true),
		},
		history: apitypes.PeerRunHistoryListResponse{
			Available: true,
			Items:     []apitypes.PeerRunHistoryEntry{{Id: "h1", CreatedAt: now, ReplayAvailable: true}},
			HasNext:   false,
		},
		historyPlay: apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: "h1", State: "playing"},
		memoryStats: apitypes.PeerRunMemoryStatsResponse{Available: true, Enabled: true, ItemCount: 2, StorageBytes: 128},
		recall:      apitypes.PeerRunRecallResponse{Available: true, Hits: []apitypes.PeerRunRecallHit{{Id: "m1", Score: 0.9, Snippet: "hello"}}},
	}
	serverGenX := &fakeRPCServerGenXService{}
	peerRun := &peerrun.Server{Store: kv.NewMemory(nil)}
	server := &rpcServer{peer: fake, peerRun: peerRun, peerRunRuntime: runRuntime, serverGenX: serverGenX, callerPublicKey: publicKey}
	client := &rpcClient{}

	info := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetInfoResponse, error) {
		return client.GetServerInfo(context.Background(), conn, "info")
	})
	if info.Name == nil || *info.Name != "peer-1" {
		t.Fatalf("GetInfo() = %+v", info)
	}

	putInfo := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerPutInfoResponse, error) {
		return client.PutServerInfo(context.Background(), conn, "put-info", rpcapi.ServerPutInfoRequest{Name: stringPtr("peer-put")})
	})
	if putInfo.Name == nil || *putInfo.Name != "peer-2" {
		t.Fatalf("PutInfo() = %+v", putInfo)
	}
	if fake.lastPutInfo == nil || fake.lastPutInfo.Name == nil || *fake.lastPutInfo.Name != "peer-put" {
		t.Fatalf("PutInfo request = %+v", fake.lastPutInfo)
	}

	runtime := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRuntimeResponse, error) {
		return client.GetServerRuntime(context.Background(), conn, "runtime")
	})
	if !runtime.Online || !runtime.LastSeenAt.Equal(now) {
		t.Fatalf("GetRuntime() = %+v", runtime)
	}

	battery := 55
	if _, err := peerRun.PutStatus(context.Background(), publicKey, apitypes.PeerStatus{BatteryPercent: &battery}); err != nil {
		t.Fatalf("seed PutStatus() error = %v", err)
	}
	gotStatus := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetStatusResponse, error) {
		return client.GetServerStatus(context.Background(), conn, "get-status")
	})
	if gotStatus.BatteryPercent == nil || *gotStatus.BatteryPercent != battery {
		t.Fatalf("GetStatus() = %+v", gotStatus)
	}

	runAgent := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerSetRunAgentResponse, error) {
		return client.SetServerRunAgent(context.Background(), conn, "set-run-agent", rpcapi.ServerSetRunAgentRequest{WorkspaceName: "demo"})
	})
	if runAgent.Pending == nil || runAgent.Pending.WorkspaceName != "demo" || runAgent.Active != nil {
		t.Fatalf("SetRunAgent() = %+v", runAgent)
	}
	gotRunAgent := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRunAgentResponse, error) {
		return client.GetServerRunAgent(context.Background(), conn, "get-run-agent")
	})
	if gotRunAgent.Pending == nil || gotRunAgent.Pending.WorkspaceName != "demo" {
		t.Fatalf("GetRunAgent() = %+v", gotRunAgent)
	}
	runWorkspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
		return client.GetServerRunWorkspace(context.Background(), conn, "get-run-workspace")
	})
	if runWorkspace.WorkspaceName != "demo" || runWorkspace.ActiveWorkspaceName != nil || runWorkspace.PendingWorkspaceName == nil || *runWorkspace.PendingWorkspaceName != "demo" {
		t.Fatalf("GetServerRunWorkspace() = %+v", runWorkspace)
	}
	if runWorkspace.HistoryAvailable == nil || !*runWorkspace.HistoryAvailable || runWorkspace.MemoryStatsAvailable == nil || !*runWorkspace.MemoryStatsAvailable || runWorkspace.RecallAvailable == nil || !*runWorkspace.RecallAvailable {
		t.Fatalf("GetServerRunWorkspace() availability = %+v", runWorkspace)
	}
	setWorkspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
		return client.SetServerRunWorkspace(context.Background(), conn, "set-run-workspace", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: "alt"})
	})
	if setWorkspace.WorkspaceName != "alt" || setWorkspace.PendingWorkspaceName == nil || *setWorkspace.PendingWorkspaceName != "alt" {
		t.Fatalf("SetServerRunWorkspace() = %+v", setWorkspace)
	}

	reloadRun := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerReloadRunResponse, error) {
		return client.ReloadServerRun(context.Background(), conn, "reload-run")
	})
	if reloadRun.State != rpcapi.PeerRunStatusStateRunning || reloadRun.WorkspaceName == nil || *reloadRun.WorkspaceName != "demo" {
		t.Fatalf("ReloadServerRun() = %+v", reloadRun)
	}
	runStatus := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(context.Background(), conn, "run-status")
	})
	if runStatus.State != rpcapi.PeerRunStatusStateRunning {
		t.Fatalf("GetServerRunStatus() = %+v", runStatus)
	}
	stopRun := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerStopRunResponse, error) {
		return client.StopServerRun(context.Background(), conn, "stop-run")
	})
	if stopRun.State != rpcapi.PeerRunStatusStateStopped {
		t.Fatalf("StopServerRun() = %+v", stopRun)
	}
	if runRuntime.reloadCalls != 1 || runRuntime.statusCalls != 1 || runRuntime.stopCalls != 1 {
		t.Fatalf("runtime calls reload=%d status=%d stop=%d", runRuntime.reloadCalls, runRuntime.statusCalls, runRuntime.stopCalls)
	}
	reloadWorkspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
		return client.ReloadServerRunWorkspace(context.Background(), conn, "reload-run-workspace")
	})
	if reloadWorkspace.RuntimeState != rpcapi.PeerRunStatusStateRunning || reloadWorkspace.ActiveWorkspaceName == nil || *reloadWorkspace.ActiveWorkspaceName != "demo" {
		t.Fatalf("ReloadServerRunWorkspace() = %+v", reloadWorkspace)
	}
	history := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
		return client.ListServerRunWorkspaceHistory(context.Background(), conn, "run-workspace-history", rpcapi.ServerListRunWorkspaceHistoryRequest{Limit: intPtr(1)})
	})
	if !history.Available || len(history.Items) != 1 || history.Items[0].Id != "h1" || runRuntime.lastHistoryLimit == nil || *runRuntime.lastHistoryLimit != 1 {
		t.Fatalf("ListServerRunWorkspaceHistory() = %+v limit=%v", history, runRuntime.lastHistoryLimit)
	}
	play := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
		return client.PlayServerRunWorkspaceHistory(context.Background(), conn, "run-workspace-history-play", rpcapi.ServerPlayRunWorkspaceHistoryRequest{HistoryId: "h1"})
	})
	if !play.Accepted || runRuntime.lastHistoryPlayID != "h1" {
		t.Fatalf("PlayServerRunWorkspaceHistory() = %+v id=%q", play, runRuntime.lastHistoryPlayID)
	}
	memory := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
		return client.GetServerRunWorkspaceMemoryStats(context.Background(), conn, "run-workspace-memory", rpcapi.ServerGetRunWorkspaceMemoryStatsRequest{})
	})
	if !memory.Available || memory.ItemCount != 2 {
		t.Fatalf("GetServerRunWorkspaceMemoryStats() = %+v", memory)
	}
	recall := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
		return client.ServerRunWorkspaceRecall(context.Background(), conn, "run-workspace-recall", rpcapi.ServerRunWorkspaceRecallRequest{Query: "hello"})
	})
	if !recall.Available || len(recall.Hits) != 1 || runRuntime.lastRecallQuery != "hello" {
		t.Fatalf("ServerRunWorkspaceRecall() = %+v query=%q", recall, runRuntime.lastRecallQuery)
	}

	audio := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerRunSayResponse, error) {
		return client.ServerRunSay(context.Background(), conn, "audio-say", rpcapi.ServerRunSayRequest{Text: "hello", VoiceId: stringPtr("voice-1")})
	})
	if !audio.Accepted {
		t.Fatalf("ServerRunSay() = %+v", audio)
	}
	if serverGenX.lastSay.Text != "hello" || serverGenX.lastSay.VoiceID != "voice-1" {
		t.Fatalf("ServerRunSay request = %+v", serverGenX.lastSay)
	}
}

func TestRPCServerSetRunWorkspaceDoesNotRequireRuntime(t *testing.T) {
	publicKey := giznet.PublicKey{1, 2, 3}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	server := &rpcServer{peerRun: store, callerPublicKey: publicKey}
	client := &rpcClient{}

	setWorkspace := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
		return client.SetServerRunWorkspace(context.Background(), conn, "set-run-workspace", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: "demo"})
	})
	if setWorkspace.RuntimeState != rpcapi.PeerRunStatusStateStopped || setWorkspace.WorkspaceName != "demo" || setWorkspace.PendingWorkspaceName == nil || *setWorkspace.PendingWorkspaceName != "demo" {
		t.Fatalf("SetServerRunWorkspace() = %+v", setWorkspace)
	}
	agent, err := store.GetRunAgent(context.Background(), publicKey)
	if err != nil {
		t.Fatalf("GetRunAgent() error = %v", err)
	}
	if agent.Pending == nil || agent.Pending.WorkspaceName != "demo" || agent.Active != nil {
		t.Fatalf("run agent after set = %+v", agent)
	}
}

func TestRPCServerPeerErrorResponse(t *testing.T) {
	server := &rpcServer{peer: &fakeRPCPeerService{getInfoError: peer.ErrPeerNotFound}}
	client := &rpcClient{}
	_, err := callRPCPairErr(server, func(conn net.Conn) (*rpcapi.ServerGetInfoResponse, error) {
		return client.GetServerInfo(context.Background(), conn, "info-error")
	})
	if err == nil || err.Error() != "rpc: peer: peer not found" {
		t.Fatalf("GetInfo(error) err = %v", err)
	}
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("GetInfo(error) err = %T, want rpcapi.Error", err)
	}
	if rpcErr.Code != 404 || rpcErr.RequestID != "info-error" {
		t.Fatalf("GetInfo(error) rpc error = %+v", rpcErr)
	}
}

func TestRPCServerHandleClosedConn(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	_ = clientSide.Close()
	if err := (&rpcServer{}).Handle(serverSide); err != nil {
		t.Fatalf("Handle(closed conn) error = %v", err)
	}
}

func TestRPCServerContextCancelsWhenConnCloses(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()

	started := make(chan struct{})
	server := &rpcServer{peer: &fakeRPCPeerService{waitPutInfoContext: true, putInfoStarted: started}}
	errCh := make(chan error, 1)
	go func() {
		defer serverSide.Close()
		errCh <- server.Handle(serverSide)
	}()

	params := mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCPayload).FromServerPutInfoRequest)
	if err := rpcapi.WriteRequest(clientSide, newRPCRequest("put-info-cancel", rpcapi.RPCMethodServerInfoPut, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	if err := rpcapi.WriteFrame(clientSide, rpcapi.Frame{Type: rpcapi.FrameTypeEOS}); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	<-started
	_ = clientSide.Close()

	if err := <-errCh; !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Handle() error = %v, want %v or %v", err, io.EOF, io.ErrClosedPipe)
	}
	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "WARN" || attrs["result"] != "canceled" || attrs["operation"] != string(rpcapi.RPCMethodServerInfoPut) || attrs["request_id"] != "put-info-cancel" {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if attrs["rpc_code"] != int64(rpcapi.RPCErrorCodeInternalError) {
		t.Fatalf("rpc_code = %#v, want existing internal error response code", attrs["rpc_code"])
	}
	if _, ok := attrs["peer_public_key"]; ok {
		t.Fatalf("zero caller key was logged: %#v", attrs)
	}
}

func TestRPCAPIErrorUsesStatusText(t *testing.T) {
	resp := rpcAPIError("status-text", http.StatusNotFound, apitypes.ErrorResponse{})
	if resp.Error == nil || resp.Error.Message != http.StatusText(http.StatusNotFound) {
		t.Fatalf("rpcAPIError() = %+v", resp)
	}
}

func TestRPCServerDispatchErrorPaths(t *testing.T) {
	if resp, err := (&rpcServer{}).dispatch(context.Background(), nil); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidRequest {
		t.Fatalf("dispatch(nil) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "unknown", Method: rpcapi.RPCMethod("bad")}); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeMethodNotFound {
		t.Fatalf("dispatch(unknown) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "ping-missing", Method: rpcapi.RPCMethodAllPing}); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(ping missing params) = %+v, %v", resp, err)
	}

	for _, tc := range []struct {
		name    string
		server  *rpcServer
		request *rpcapi.RPCRequest
		code    rpcapi.RPCErrorCode
	}{
		{
			name:    "put info internal error",
			server:  &rpcServer{peer: &fakeRPCPeerService{putInfoError: errors.New("boom")}},
			request: newRPCRequest("put-400", rpcapi.RPCMethodServerInfoPut, mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCPayload).FromServerPutInfoRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "put info not found",
			server:  &rpcServer{peer: &fakeRPCPeerService{putInfoError: peer.ErrPeerNotFound}},
			request: newRPCRequest("put-404", rpcapi.RPCMethodServerInfoPut, mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCPayload).FromServerPutInfoRequest)),
			code:    404,
		},
		{
			name:    "runtime missing service",
			server:  &rpcServer{},
			request: newRPCRequest("runtime", rpcapi.RPCMethodServerRuntimeGet, mustRPCParams(rpcapi.ServerGetRuntimeRequest{}, (*rpcapi.RPCPayload).FromServerGetRuntimeRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "run status missing runtime",
			server:  &rpcServer{},
			request: newRPCRequest("run-status", rpcapi.RPCMethodServerRunStatus, mustRPCParams(rpcapi.ServerGetRunStatusRequest{}, (*rpcapi.RPCPayload).FromServerGetRunStatusRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "run reload runtime error",
			server:  &rpcServer{peerRunRuntime: &fakeRPCPeerRunRuntime{err: errors.New("boom")}},
			request: newRPCRequest("run-reload", rpcapi.RPCMethodServerRunReload, mustRPCParams(rpcapi.ServerReloadRunRequest{}, (*rpcapi.RPCPayload).FromServerReloadRunRequest)),
			code:    rpcapi.RPCErrorCodeBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.server.dispatch(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("dispatch() error = %v", err)
			}
			if resp.Error == nil || resp.Error.Code != tc.code {
				t.Fatalf("dispatch() = %+v, want error code %d", resp, tc.code)
			}
		})
	}

	if resp, err := (&rpcServer{peer: &fakeRPCPeerService{}}).dispatch(context.Background(), newRPCRequest("put-missing", rpcapi.RPCMethodServerInfoPut, nil)); err != nil || resp.Error == nil || resp.Error.Message != "missing params" {
		t.Fatalf("dispatch(put missing params) = %+v, %v", resp, err)
	}
	invalidParamsReq := newRPCRequest("invalid", rpcapi.RPCMethodServerInfoGet, &rpcapi.RPCPayload{})
	if resp, err := (&rpcServer{peer: &fakeRPCPeerService{}}).dispatch(context.Background(), invalidParamsReq); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(invalid params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), newRPCRequest("audio", rpcapi.RPCMethodServerRunSay, nil)); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(audio missing params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), newRPCRequest("audio", rpcapi.RPCMethodServerRunSay, mustRPCParams(rpcapi.ServerRunSayRequest{Text: "hello", VoiceId: stringPtr("voice")}, (*rpcapi.RPCPayload).FromServerRunSayRequest))); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("dispatch(audio missing service) = %+v, %v", resp, err)
	}
}

func callRPCPair[T any](t *testing.T, server *rpcServer, call func(net.Conn) (*T, error)) *T {
	t.Helper()
	result, err := callRPCPairErr(server, call)
	if err != nil {
		t.Fatalf("RPC call error = %v", err)
	}
	return result
}

func callRPCPairErr[T any](server *rpcServer, call func(net.Conn) (*T, error)) (*T, error) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	errCh := make(chan error, 1)
	go func() {
		defer serverSide.Close()
		errCh <- server.Handle(serverSide)
	}()

	result, err := call(clientSide)
	_ = clientSide.Close()
	if serverErr := <-errCh; serverErr != nil {
		return nil, serverErr
	}
	return result, err
}

type fakeRPCPeerService struct {
	t             *testing.T
	wantPublicKey giznet.PublicKey

	info            apitypes.DeviceInfo
	putInfoResponse apitypes.DeviceInfo
	runtime         apitypes.Runtime

	lastPutInfo        *apitypes.DeviceInfo
	getInfoError       error
	putInfoError       error
	waitPutInfoContext bool
	putInfoStarted     chan struct{}
}

type fakeRPCServerInfoService struct {
	t             *testing.T
	wantPublicKey giznet.PublicKey
	info          apitypes.ServerInfo
	err           apitypes.ErrorResponse
}

func (f *fakeRPCServerInfoService) GetServerInfo(ctx context.Context, _ peerhttp.GetServerInfoRequestObject) (peerhttp.GetServerInfoResponseObject, error) {
	if f.t != nil && f.wantPublicKey != (giznet.PublicKey{}) {
		if got := peerhttp.CallerPublicKey(ctx); got != f.wantPublicKey {
			f.t.Fatalf("caller public key = %s, want %s", got, f.wantPublicKey)
		}
	}
	if f.err.Error.Code != "" {
		return peerhttp.GetServerInfo400JSONResponse(f.err), nil
	}
	return peerhttp.GetServerInfo200JSONResponse(f.info), nil
}

func (f *fakeRPCPeerService) GetSelfInfo(_ context.Context, publicKey giznet.PublicKey) (apitypes.DeviceInfo, error) {
	f.checkPublicKey(publicKey)
	if f.getInfoError != nil {
		return apitypes.DeviceInfo{}, f.getInfoError
	}
	return f.info, nil
}

func (f *fakeRPCPeerService) PutSelfInfo(ctx context.Context, publicKey giznet.PublicKey, info apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	f.checkPublicKey(publicKey)
	f.lastPutInfo = &info
	if f.putInfoStarted != nil {
		close(f.putInfoStarted)
	}
	if f.waitPutInfoContext {
		<-ctx.Done()
		return apitypes.DeviceInfo{}, ctx.Err()
	}
	if f.putInfoError != nil {
		return apitypes.DeviceInfo{}, f.putInfoError
	}
	return f.putInfoResponse, nil
}

func (f *fakeRPCPeerService) GetSelfRuntime(_ context.Context, publicKey giznet.PublicKey) apitypes.Runtime {
	f.checkPublicKey(publicKey)
	return f.runtime
}

func (f *fakeRPCPeerService) checkPublicKey(publicKey giznet.PublicKey) {
	if f.t == nil || f.wantPublicKey == (giznet.PublicKey{}) {
		return
	}
	if publicKey != f.wantPublicKey {
		f.t.Fatalf("caller public key = %s, want %s", publicKey, f.wantPublicKey)
	}
}

type fakeRPCPeerRunRuntime struct {
	reload         apitypes.PeerRunStatus
	status         apitypes.PeerRunStatus
	stop           apitypes.PeerRunStatus
	workspaceState apitypes.PeerRunWorkspaceState
	history        apitypes.PeerRunHistoryListResponse
	historyPlay    apitypes.PeerRunHistoryPlayResponse
	memoryStats    apitypes.PeerRunMemoryStatsResponse
	recall         apitypes.PeerRunRecallResponse
	err            error

	reloadCalls       int
	statusCalls       int
	stopCalls         int
	workspaceCalls    int
	historyCalls      int
	historyPlayCalls  int
	memoryStatsCalls  int
	recallCalls       int
	lastHistoryLimit  *int
	lastHistoryPlayID string
	lastRecallQuery   string
}

func (f *fakeRPCPeerRunRuntime) Reload(context.Context) (apitypes.PeerRunStatus, error) {
	f.reloadCalls++
	if f.err != nil {
		return apitypes.PeerRunStatus{}, f.err
	}
	return f.reload, nil
}

func (f *fakeRPCPeerRunRuntime) Status(context.Context) (apitypes.PeerRunStatus, error) {
	f.statusCalls++
	if f.err != nil {
		return apitypes.PeerRunStatus{}, f.err
	}
	return f.status, nil
}

func (f *fakeRPCPeerRunRuntime) Stop(context.Context) (apitypes.PeerRunStatus, error) {
	f.stopCalls++
	if f.err != nil {
		return apitypes.PeerRunStatus{}, f.err
	}
	return f.stop, nil
}

func (f *fakeRPCPeerRunRuntime) WorkspaceState(context.Context) (apitypes.PeerRunWorkspaceState, error) {
	f.workspaceCalls++
	if f.err != nil {
		return apitypes.PeerRunWorkspaceState{}, f.err
	}
	return f.workspaceState, nil
}

func (f *fakeRPCPeerRunRuntime) ListWorkspaceHistory(_ context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	f.historyCalls++
	f.lastHistoryLimit = req.Limit
	if f.err != nil {
		return apitypes.PeerRunHistoryListResponse{}, f.err
	}
	return f.history, nil
}

func (f *fakeRPCPeerRunRuntime) PlayWorkspaceHistory(_ context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	f.historyPlayCalls++
	f.lastHistoryPlayID = req.HistoryId
	if f.err != nil {
		return apitypes.PeerRunHistoryPlayResponse{}, f.err
	}
	return f.historyPlay, nil
}

func (f *fakeRPCPeerRunRuntime) WorkspaceMemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	f.memoryStatsCalls++
	if f.err != nil {
		return apitypes.PeerRunMemoryStatsResponse{}, f.err
	}
	return f.memoryStats, nil
}

func (f *fakeRPCPeerRunRuntime) WorkspaceRecall(_ context.Context, req apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	f.recallCalls++
	f.lastRecallQuery = req.Query
	if f.err != nil {
		return apitypes.PeerRunRecallResponse{}, f.err
	}
	return f.recall, nil
}

type fakeRPCServerGenXService struct {
	lastSay peergenx.SayRequest
	err     error
}

func (f *fakeRPCServerGenXService) Say(_ context.Context, request peergenx.SayRequest) (peergenx.SayResponse, error) {
	f.lastSay = request
	if f.err != nil {
		return peergenx.SayResponse{}, f.err
	}
	return peergenx.SayResponse{Accepted: true}, nil
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func mustRPCParams[T any](value T, encode func(*rpcapi.RPCPayload, T) error) *rpcapi.RPCPayload {
	params, err := newRPCRequestParams(value, encode)
	if err != nil {
		panic(err)
	}
	return params
}
