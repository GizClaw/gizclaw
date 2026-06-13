package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peerrun"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
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
		status: apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateRunning, WorkspaceName: stringPtr("demo"), FriendOtp: stringPtr("123456")},
		stop:   apitypes.PeerRunStatus{State: apitypes.PeerRunStatusStateStopped, WorkspaceName: stringPtr("demo")},
	}
	friendOTPs := &fakeFriendOTPReporter{}
	serverGenX := &fakeRPCServerGenXService{}
	server := &rpcServer{peer: fake, peerRun: &peerrun.Server{Store: kv.NewMemory(nil)}, peerRunRuntime: runRuntime, friendOTPs: friendOTPs, serverGenX: serverGenX, callerPublicKey: publicKey}
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

	volume := 55
	status := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerPutStatusResponse, error) {
		return client.PutServerStatus(context.Background(), conn, "put-status", rpcapi.ServerPutStatusRequest{Volume: &volume})
	})
	if status.Volume == nil || *status.Volume != volume {
		t.Fatalf("PutStatus() = %+v", status)
	}
	gotStatus := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetStatusResponse, error) {
		return client.GetServerStatus(context.Background(), conn, "get-status")
	})
	if gotStatus.Volume == nil || *gotStatus.Volume != volume {
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
	if friendOTPs.peerID != publicKey.String() || friendOTPs.code != "123456" {
		t.Fatalf("friend OTP report = peer %q code %q", friendOTPs.peerID, friendOTPs.code)
	}
	runStatus = callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(context.Background(), conn, "run-status-report", rpcapi.ServerGetRunStatusRequest{FriendOtp: stringPtr("654321")})
	})
	if runStatus.FriendOtp == nil || *runStatus.FriendOtp != "654321" {
		t.Fatalf("GetServerRunStatus reported friend_otp = %+v", runStatus.FriendOtp)
	}
	if friendOTPs.peerID != publicKey.String() || friendOTPs.code != "654321" {
		t.Fatalf("request friend OTP report = peer %q code %q", friendOTPs.peerID, friendOTPs.code)
	}
	stopRun := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerStopRunResponse, error) {
		return client.StopServerRun(context.Background(), conn, "stop-run")
	})
	if stopRun.State != rpcapi.PeerRunStatusStateStopped {
		t.Fatalf("StopServerRun() = %+v", stopRun)
	}
	if runRuntime.reloadCalls != 1 || runRuntime.statusCalls != 2 || runRuntime.stopCalls != 1 {
		t.Fatalf("runtime calls reload=%d status=%d stop=%d", runRuntime.reloadCalls, runRuntime.statusCalls, runRuntime.stopCalls)
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

type fakeFriendOTPReporter struct {
	peerID string
	code   string
	err    error
}

func (f *fakeFriendOTPReporter) ReportFriendOTP(_ context.Context, peerID, code string) error {
	f.peerID = peerID
	f.code = code
	return f.err
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
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()

	server := &rpcServer{peer: &fakeRPCPeerService{waitPutInfoContext: true}}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Handle(serverSide)
	}()

	params := mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromServerPutInfoRequest)
	if err := rpcapi.WriteRequest(clientSide, newRPCRequest("put-info-cancel", rpcapi.RPCMethodServerInfoPut, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	_ = clientSide.Close()

	if err := <-errCh; !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Handle() error = %v, want %v or %v", err, io.EOF, io.ErrClosedPipe)
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
			request: newRPCRequest("put-400", rpcapi.RPCMethodServerInfoPut, mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromServerPutInfoRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "put info not found",
			server:  &rpcServer{peer: &fakeRPCPeerService{putInfoError: peer.ErrPeerNotFound}},
			request: newRPCRequest("put-404", rpcapi.RPCMethodServerInfoPut, mustRPCParams(rpcapi.ServerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromServerPutInfoRequest)),
			code:    404,
		},
		{
			name:    "runtime missing service",
			server:  &rpcServer{},
			request: newRPCRequest("runtime", rpcapi.RPCMethodServerRuntimeGet, mustRPCParams(rpcapi.ServerGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetRuntimeRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "run status missing runtime",
			server:  &rpcServer{},
			request: newRPCRequest("run-status", rpcapi.RPCMethodServerRunStatus, mustRPCParams(rpcapi.ServerGetRunStatusRequest{}, (*rpcapi.RPCRequest_Params).FromServerGetRunStatusRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "run reload runtime error",
			server:  &rpcServer{peerRunRuntime: &fakeRPCPeerRunRuntime{err: errors.New("boom")}},
			request: newRPCRequest("run-reload", rpcapi.RPCMethodServerRunReload, mustRPCParams(rpcapi.ServerReloadRunRequest{}, (*rpcapi.RPCRequest_Params).FromServerReloadRunRequest)),
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
	var invalidParamsReq rpcapi.RPCRequest
	if err := json.Unmarshal([]byte(`{"v":1,"id":"invalid","method":"server.info.get","params":[]}`), &invalidParamsReq); err != nil {
		t.Fatalf("unmarshal invalid params request: %v", err)
	}
	if resp, err := (&rpcServer{peer: &fakeRPCPeerService{}}).dispatch(context.Background(), &invalidParamsReq); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(invalid params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), newRPCRequest("audio", rpcapi.RPCMethodServerRunSay, nil)); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(audio missing params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), newRPCRequest("audio", rpcapi.RPCMethodServerRunSay, mustRPCParams(rpcapi.ServerRunSayRequest{Text: "hello", VoiceId: stringPtr("voice")}, (*rpcapi.RPCRequest_Params).FromServerRunSayRequest))); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInternalError {
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
		errCh <- server.Handle(serverSide)
	}()

	result, err := call(clientSide)
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
}

type fakeRPCServerInfoService struct {
	t             *testing.T
	wantPublicKey giznet.PublicKey
	info          apitypes.ServerInfo
	err           apitypes.ErrorResponse
}

func (f *fakeRPCServerInfoService) GetServerInfo(ctx context.Context, _ serverpublic.GetServerInfoRequestObject) (serverpublic.GetServerInfoResponseObject, error) {
	if f.t != nil && f.wantPublicKey != (giznet.PublicKey{}) {
		if got := serverpublic.CallerPublicKey(ctx); got != f.wantPublicKey {
			f.t.Fatalf("caller public key = %s, want %s", got, f.wantPublicKey)
		}
	}
	if f.err.Error.Code != "" {
		return serverpublic.GetServerInfo400JSONResponse(f.err), nil
	}
	return serverpublic.GetServerInfo200JSONResponse(f.info), nil
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
	reload apitypes.PeerRunStatus
	status apitypes.PeerRunStatus
	stop   apitypes.PeerRunStatus
	err    error

	reloadCalls int
	statusCalls int
	stopCalls   int
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

func mustRPCParams[T any](value T, encode func(*rpcapi.RPCRequest_Params, T) error) *rpcapi.RPCRequest_Params {
	params, err := newRPCRequestParams(value, encode)
	if err != nil {
		panic(err)
	}
	return params
}
