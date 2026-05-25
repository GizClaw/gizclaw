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
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

func TestRPCClientServerGearMethods(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	publicKey := giznet.PublicKey{1, 2, 3}
	fake := &fakeRPCGearService{
		t:               t,
		wantPublicKey:   publicKey,
		config:          apitypes.Configuration{},
		info:            apitypes.DeviceInfo{Name: stringPtr("gear-1")},
		registration:    testRPCRegistration(publicKey, now),
		runtime:         apitypes.Runtime{Online: true, LastSeenAt: now},
		registerResult:  testRPCRegistrationResult(publicKey, now),
		putInfoResponse: apitypes.DeviceInfo{Name: stringPtr("gear-2")},
	}
	serverPublicKey := giznet.PublicKey{9, 8, 7}
	serverInfo := &fakeRPCServerInfoService{
		t:             t,
		wantPublicKey: publicKey,
		info: apitypes.ServerInfo{
			PublicKey:   serverPublicKey.String(),
			ServerTime:  123,
			BuildCommit: "test",
		},
	}
	server := &rpcServer{gear: fake, serverInfo: serverInfo, callerPublicKey: publicKey}
	client := &rpcClient{}

	infoResp := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.ServerGetInfoResponse, error) {
		return client.GetServerInfo(context.Background(), conn, "server-info")
	})
	if infoResp.PublicKey != serverPublicKey.String() || infoResp.ServerTime != 123 || infoResp.BuildCommit != "test" {
		t.Fatalf("GetServerInfo() = %+v", infoResp)
	}

	if got := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearGetConfigResponse, error) {
		return client.GetConfig(context.Background(), conn, "config")
	}); got == nil {
		t.Fatal("GetConfig() returned nil")
	}

	info := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearGetInfoResponse, error) {
		return client.GetInfo(context.Background(), conn, "info")
	})
	if info.Name == nil || *info.Name != "gear-1" {
		t.Fatalf("GetInfo() = %+v", info)
	}

	putInfo := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearPutInfoResponse, error) {
		return client.PutInfo(context.Background(), conn, "put-info", rpcapi.GearPutInfoRequest{Name: stringPtr("gear-put")})
	})
	if putInfo.Name == nil || *putInfo.Name != "gear-2" {
		t.Fatalf("PutInfo() = %+v", putInfo)
	}
	if fake.lastPutInfo == nil || fake.lastPutInfo.Name == nil || *fake.lastPutInfo.Name != "gear-put" {
		t.Fatalf("PutInfo request = %+v", fake.lastPutInfo)
	}

	registration := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearGetRegistrationResponse, error) {
		return client.GetRegistration(context.Background(), conn, "registration")
	})
	if registration.PublicKey != publicKey.String() {
		t.Fatalf("GetRegistration() = %+v", registration)
	}

	register := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearRegisterResponse, error) {
		return client.RegisterGear(context.Background(), conn, "register", rpcapi.GearRegisterRequest{Device: rpcapi.DeviceInfo{Name: stringPtr("gear-register")}})
	})
	if register.Registration.PublicKey != publicKey.String() {
		t.Fatalf("RegisterGear() = %+v", register)
	}
	if fake.lastRegister == nil || fake.lastRegister.Device.Name == nil || *fake.lastRegister.Device.Name != "gear-register" {
		t.Fatalf("RegisterGear request = %+v", fake.lastRegister)
	}

	runtime := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.GearGetRuntimeResponse, error) {
		return client.GetRuntime(context.Background(), conn, "runtime")
	})
	if !runtime.Online || !runtime.LastSeenAt.Equal(now) {
		t.Fatalf("GetRuntime() = %+v", runtime)
	}
}

func TestRPCServerPingHandle(t *testing.T) {
	server := &rpcServer{}
	client := &rpcClient{}
	ping := callRPCPair(t, server, func(conn net.Conn) (*rpcapi.PingResponse, error) {
		return client.Ping(context.Background(), conn, "ping")
	})
	if ping.ServerTime <= 0 {
		t.Fatalf("Ping() = %+v", ping)
	}
}

func TestRPCServerPingClientHandle(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{}).Handle(clientSide)
	}()

	ping, err := (&rpcServer{}).Ping(context.Background(), serverSide, "server-ping")
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if ping.ServerTime <= 0 {
		t.Fatalf("Ping() = %+v", ping)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestRPCClientHandlePeerInfoMethods(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	name := "main"
	device := &Client{Device: apitypes.DeviceInfo{
		Name: stringPtr("gear-1"),
		Sn:   stringPtr("sn-1"),
		Hardware: &apitypes.HardwareInfo{
			Manufacturer: stringPtr("Acme"),
			Model:        stringPtr("M1"),
			Imeis: &[]apitypes.GearIMEI{{
				Name:   &name,
				Tac:    "12345678",
				Serial: "0000001",
			}},
		},
	}}

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	caller := &rpcClient{}
	info, err := caller.GetPeerInfo(context.Background(), serverSide, "peer-info")
	if err != nil {
		t.Fatalf("GetPeerInfo() error = %v", err)
	}
	if info.Name == nil || *info.Name != "gear-1" || info.Manufacturer == nil || *info.Manufacturer != "Acme" {
		t.Fatalf("GetPeerInfo() = %+v", info)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(info) error = %v", err)
	}

	serverSide, clientSide = net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	errCh = make(chan error, 1)
	go func() {
		errCh <- (&rpcClient{peer: device}).Handle(clientSide)
	}()

	identifiers, err := caller.GetPeerIdentifiers(context.Background(), serverSide, "peer-identifiers")
	if err != nil {
		t.Fatalf("GetPeerIdentifiers() error = %v", err)
	}
	if identifiers.Sn == nil || *identifiers.Sn != "sn-1" || identifiers.Imeis == nil || len(*identifiers.Imeis) != 1 {
		t.Fatalf("GetPeerIdentifiers() = %+v", identifiers)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Handle(identifiers) error = %v", err)
	}
}

func TestRPCServerGearErrorResponse(t *testing.T) {
	server := &rpcServer{gear: &fakeRPCGearService{getInfoError: apitypes.NewErrorResponse("GEAR_NOT_FOUND", "missing")}}
	client := &rpcClient{}
	_, err := callRPCPairErr(server, func(conn net.Conn) (*rpcapi.GearGetInfoResponse, error) {
		return client.GetInfo(context.Background(), conn, "info-error")
	})
	if err == nil || err.Error() != "rpc: missing" {
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

	server := &rpcServer{gear: &fakeRPCGearService{waitRuntimeContext: true}}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Handle(serverSide)
	}()

	params := mustRPCParams(rpcapi.GearGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetRuntimeRequest)
	if err := rpcapi.WriteRequest(clientSide, newRPCRequest("runtime-cancel", rpcapi.RPCMethodGearRuntimeGet, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	_ = clientSide.Close()

	if err := <-errCh; !errors.Is(err, io.EOF) {
		t.Fatalf("Handle() error = %v, want %v", err, io.EOF)
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

	errResp := apitypes.NewErrorResponse("ERR", "boom")
	for _, tc := range []struct {
		name    string
		server  *rpcServer
		request *rpcapi.RPCRequest
		code    rpcapi.RPCErrorCode
	}{
		{
			name:    "config not found",
			server:  &rpcServer{gear: &fakeRPCGearService{configError: errResp}},
			request: newRPCRequest("config", rpcapi.RPCMethodGearConfigGet, mustRPCParams(rpcapi.GearGetConfigRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetConfigRequest)),
			code:    404,
		},
		{
			name:    "put info bad request",
			server:  &rpcServer{gear: &fakeRPCGearService{putInfo400: errResp}},
			request: newRPCRequest("put-400", rpcapi.RPCMethodGearInfoPut, mustRPCParams(rpcapi.GearPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromGearPutInfoRequest)),
			code:    rpcapi.RPCErrorCode(400),
		},
		{
			name:    "put info not found",
			server:  &rpcServer{gear: &fakeRPCGearService{putInfo404: errResp}},
			request: newRPCRequest("put-404", rpcapi.RPCMethodGearInfoPut, mustRPCParams(rpcapi.GearPutInfoRequest{}, (*rpcapi.RPCRequest_Params).FromGearPutInfoRequest)),
			code:    404,
		},
		{
			name:    "registration not found",
			server:  &rpcServer{gear: &fakeRPCGearService{registrationError: errResp}},
			request: newRPCRequest("registration", rpcapi.RPCMethodGearRegistrationGet, mustRPCParams(rpcapi.GearGetRegistrationRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetRegistrationRequest)),
			code:    404,
		},
		{
			name:    "register bad request",
			server:  &rpcServer{gear: &fakeRPCGearService{register400: errResp}},
			request: newRPCRequest("register-400", rpcapi.RPCMethodGearRegistrationRegister, mustRPCParams(rpcapi.GearRegisterRequest{}, (*rpcapi.RPCRequest_Params).FromGearRegisterRequest)),
			code:    400,
		},
		{
			name:    "register conflict",
			server:  &rpcServer{gear: &fakeRPCGearService{register409: errResp}},
			request: newRPCRequest("register-409", rpcapi.RPCMethodGearRegistrationRegister, mustRPCParams(rpcapi.GearRegisterRequest{}, (*rpcapi.RPCRequest_Params).FromGearRegisterRequest)),
			code:    409,
		},
		{
			name:    "runtime bad request",
			server:  &rpcServer{gear: &fakeRPCGearService{runtimeError: errResp}},
			request: newRPCRequest("runtime", rpcapi.RPCMethodGearRuntimeGet, mustRPCParams(rpcapi.GearGetRuntimeRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetRuntimeRequest)),
			code:    400,
		},
		{
			name:    "run test unimplemented",
			server:  &rpcServer{},
			request: newRPCRequest("run-test", rpcapi.RPCMethodGearTestRun, mustRPCParams(rpcapi.GearRunTestRequest{Mode: rpcapi.GearTestModeAudioPlay}, (*rpcapi.RPCRequest_Params).FromGearRunTestRequest)),
			code:    rpcapi.RPCErrorCodeInternalError,
		},
		{
			name:    "unexpected response",
			server:  &rpcServer{gear: &fakeRPCGearService{unexpectedConfig: true}},
			request: newRPCRequest("unexpected", rpcapi.RPCMethodGearConfigGet, mustRPCParams(rpcapi.GearGetConfigRequest{}, (*rpcapi.RPCRequest_Params).FromGearGetConfigRequest)),
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

	if resp, err := (&rpcServer{gear: &fakeRPCGearService{}}).dispatch(context.Background(), newRPCRequest("put-missing", rpcapi.RPCMethodGearInfoPut, nil)); err != nil || resp.Error == nil || resp.Error.Message != "missing params" {
		t.Fatalf("dispatch(put missing params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{gear: &fakeRPCGearService{}}).dispatch(context.Background(), newRPCRequest("register-missing", rpcapi.RPCMethodGearRegistrationRegister, nil)); err != nil || resp.Error == nil || resp.Error.Message != "missing params" {
		t.Fatalf("dispatch(register missing params) = %+v, %v", resp, err)
	}
	if resp, err := (&rpcServer{}).dispatch(context.Background(), newRPCRequest("run-test-missing", rpcapi.RPCMethodGearTestRun, nil)); err != nil || resp.Error == nil || resp.Error.Message != "missing params" {
		t.Fatalf("dispatch(run test missing params) = %+v, %v", resp, err)
	}

	var invalidParamsReq rpcapi.RPCRequest
	if err := json.Unmarshal([]byte(`{"v":1,"id":"invalid","method":"gear.config.get","params":[]}`), &invalidParamsReq); err != nil {
		t.Fatalf("unmarshal invalid params request: %v", err)
	}
	if resp, err := (&rpcServer{gear: &fakeRPCGearService{}}).dispatch(context.Background(), &invalidParamsReq); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
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

type fakeRPCGearService struct {
	t             *testing.T
	wantPublicKey giznet.PublicKey

	config          apitypes.Configuration
	info            apitypes.DeviceInfo
	putInfoResponse apitypes.DeviceInfo
	registration    apitypes.Registration
	registerResult  gearservice.RegistrationResult
	runtime         apitypes.Runtime

	lastPutInfo        *gearservice.PutInfoJSONRequestBody
	lastRegister       *gearservice.RegisterGearJSONRequestBody
	configError        apitypes.ErrorResponse
	getInfoError       apitypes.ErrorResponse
	putInfo400         apitypes.ErrorResponse
	putInfo404         apitypes.ErrorResponse
	registrationError  apitypes.ErrorResponse
	register400        apitypes.ErrorResponse
	register409        apitypes.ErrorResponse
	runtimeError       apitypes.ErrorResponse
	unexpectedConfig   bool
	waitRuntimeContext bool
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

func (f *fakeRPCGearService) GetConfig(ctx context.Context, _ gearservice.GetConfigRequestObject) (gearservice.GetConfigResponseObject, error) {
	f.checkPublicKey(ctx)
	if f.unexpectedConfig {
		return nil, nil
	}
	if f.configError.Error.Code != "" {
		return gearservice.GetConfig404JSONResponse(f.configError), nil
	}
	return gearservice.GetConfig200JSONResponse(f.config), nil
}

func (f *fakeRPCGearService) GetInfo(ctx context.Context, _ gearservice.GetInfoRequestObject) (gearservice.GetInfoResponseObject, error) {
	f.checkPublicKey(ctx)
	if f.getInfoError.Error.Code != "" {
		return gearservice.GetInfo404JSONResponse(f.getInfoError), nil
	}
	return gearservice.GetInfo200JSONResponse(f.info), nil
}

func (f *fakeRPCGearService) PutInfo(ctx context.Context, request gearservice.PutInfoRequestObject) (gearservice.PutInfoResponseObject, error) {
	f.checkPublicKey(ctx)
	f.lastPutInfo = request.Body
	if f.putInfo400.Error.Code != "" {
		return gearservice.PutInfo400JSONResponse(f.putInfo400), nil
	}
	if f.putInfo404.Error.Code != "" {
		return gearservice.PutInfo404JSONResponse(f.putInfo404), nil
	}
	return gearservice.PutInfo200JSONResponse(f.putInfoResponse), nil
}

func (f *fakeRPCGearService) GetRegistration(ctx context.Context, _ gearservice.GetRegistrationRequestObject) (gearservice.GetRegistrationResponseObject, error) {
	f.checkPublicKey(ctx)
	if f.registrationError.Error.Code != "" {
		return gearservice.GetRegistration404JSONResponse(f.registrationError), nil
	}
	return gearservice.GetRegistration200JSONResponse(f.registration), nil
}

func (f *fakeRPCGearService) RegisterGear(ctx context.Context, request gearservice.RegisterGearRequestObject) (gearservice.RegisterGearResponseObject, error) {
	f.checkPublicKey(ctx)
	f.lastRegister = request.Body
	if f.register400.Error.Code != "" {
		return gearservice.RegisterGear400JSONResponse(f.register400), nil
	}
	if f.register409.Error.Code != "" {
		return gearservice.RegisterGear409JSONResponse(f.register409), nil
	}
	return gearservice.RegisterGear200JSONResponse(f.registerResult), nil
}

func (f *fakeRPCGearService) GetRuntime(ctx context.Context, _ gearservice.GetRuntimeRequestObject) (gearservice.GetRuntimeResponseObject, error) {
	f.checkPublicKey(ctx)
	if f.waitRuntimeContext {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.runtimeError.Error.Code != "" {
		return gearservice.GetRuntime400JSONResponse(f.runtimeError), nil
	}
	return gearservice.GetRuntime200JSONResponse(f.runtime), nil
}

func (f *fakeRPCGearService) checkPublicKey(ctx context.Context) {
	if f.t == nil || f.wantPublicKey == (giznet.PublicKey{}) {
		return
	}
	if got := gearservice.CallerPublicKey(ctx); got != f.wantPublicKey {
		f.t.Fatalf("caller public key = %s, want %s", got, f.wantPublicKey)
	}
}

func testRPCRegistration(publicKey giznet.PublicKey, now time.Time) apitypes.Registration {
	return apitypes.Registration{
		PublicKey: publicKey.String(),
		Role:      apitypes.GearRoleGear,
		Status:    apitypes.GearStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testRPCRegistrationResult(publicKey giznet.PublicKey, now time.Time) gearservice.RegistrationResult {
	registration := testRPCRegistration(publicKey, now)
	return gearservice.RegistrationResult{
		Gear: apitypes.Gear{
			PublicKey:     publicKey.String(),
			Role:          apitypes.GearRoleGear,
			Status:        apitypes.GearStatusActive,
			Device:        apitypes.DeviceInfo{Name: stringPtr("gear-registered")},
			Configuration: apitypes.Configuration{},
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		Registration: registration,
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
