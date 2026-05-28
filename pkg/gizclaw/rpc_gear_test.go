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
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestRPCGearPeerMethods(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	publicKey := giznet.PublicKey{1, 2, 3}
	fake := &fakeRPCPeerService{
		t:               t,
		wantPublicKey:   publicKey,
		info:            apitypes.DeviceInfo{Name: stringPtr("gear-1")},
		runtime:         apitypes.Runtime{Online: true, LastSeenAt: now},
		putInfoResponse: apitypes.DeviceInfo{Name: stringPtr("gear-2")},
	}
	server := &rpcServer{peer: fake, callerPublicKey: publicKey}
	client := &rpcClient{}

	info := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.PeerGetInfoResponse, error) {
		return client.GetPeerInfo(context.Background(), conn, "info")
	})
	if info.Name == nil || *info.Name != "gear-1" {
		t.Fatalf("GetInfo() = %+v", info)
	}

	putInfo := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.PeerPutInfoResponse, error) {
		return client.PutPeerInfo(context.Background(), conn, "put-info", rpcapi.PeerPutInfoRequest{Name: stringPtr("gear-put")})
	})
	if putInfo.Name == nil || *putInfo.Name != "gear-2" {
		t.Fatalf("PutInfo() = %+v", putInfo)
	}
	if fake.lastPutInfo == nil || fake.lastPutInfo.Name == nil || *fake.lastPutInfo.Name != "gear-put" {
		t.Fatalf("PutInfo request = %+v", fake.lastPutInfo)
	}

	runtime := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.PeerGetRuntimeResponse, error) {
		return client.GetPeerRuntime(context.Background(), conn, "runtime")
	})
	if !runtime.Online || !runtime.LastSeenAt.Equal(now) {
		t.Fatalf("GetRuntime() = %+v", runtime)
	}
}

func TestRPCServerPeerErrorResponse(t *testing.T) {
	server := &rpcServer{peer: &fakeRPCPeerService{getInfoError: peer.ErrPeerNotFound}}
	client := &rpcClient{}
	_, err := callRPCPairErr(server, func(conn net.Conn) (*rpcapi.PeerGetInfoResponse, error) {
		return client.GetPeerInfo(context.Background(), conn, "info-error")
	})
	if err == nil || err.Error() != "rpc: gear: gear not found" {
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

	params := mustRPCParams(rpcapi.PeerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromPeerPutInfoRequest)
	if err := rpcapi.WriteRequest(clientSide, newRPCRequest("put-info-cancel", rpcapi.RPCMethodPeerInfoPut, params)); err != nil {
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
	if resp, err := (&rpcServer{}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "ping-missing", Method: rpcapi.RPCMethodPeerPing}); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
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
			request: newRPCRequest("put-400", rpcapi.RPCMethodPeerInfoPut, mustRPCParams(rpcapi.PeerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromPeerPutInfoRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "put info not found",
			server:  &rpcServer{peer: &fakeRPCPeerService{putInfoError: peer.ErrPeerNotFound}},
			request: newRPCRequest("put-404", rpcapi.RPCMethodPeerInfoPut, mustRPCParams(rpcapi.PeerPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromPeerPutInfoRequest)),
			code:    404,
		},
		{
			name:    "runtime missing service",
			server:  &rpcServer{},
			request: newRPCRequest("runtime", rpcapi.RPCMethodPeerRuntimeGet, mustRPCParams(rpcapi.PeerGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromPeerGetRuntimeRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
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

	if resp, err := (&rpcServer{peer: &fakeRPCPeerService{}}).dispatch(context.Background(), newRPCRequest("put-missing", rpcapi.RPCMethodPeerInfoPut, nil)); err != nil || resp.Error == nil || resp.Error.Message != "missing params" {
		t.Fatalf("dispatch(put missing params) = %+v, %v", resp, err)
	}
	var invalidParamsReq rpcapi.RPCRequest
	if err := json.Unmarshal([]byte(`{"v":1,"id":"invalid","method":"peer.info.get","params":[]}`), &invalidParamsReq); err != nil {
		t.Fatalf("unmarshal invalid params request: %v", err)
	}
	if resp, err := (&rpcServer{peer: &fakeRPCPeerService{}}).dispatch(context.Background(), &invalidParamsReq); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("dispatch(invalid params) = %+v, %v", resp, err)
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
