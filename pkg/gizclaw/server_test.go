package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

type testGiznetSecurityPolicy struct {
	allowService func(giznet.PublicKey, uint64) bool
}

func (p testGiznetSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (p testGiznetSecurityPolicy) AllowService(pk giznet.PublicKey, service uint64) bool {
	if p.allowService == nil {
		return service == 0
	}
	return p.allowService(pk, service)
}

func TestServerListenRequiresPeerStore(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	server := &Server{LocalStatic: *keyPair}
	err = server.Listen()
	if err == nil || !strings.Contains(err.Error(), "nil peer store") {
		t.Fatalf("Listen error = %v, want nil peer store", err)
	}
}

func TestServerListenValidatesReceiverAndLocalStatic(t *testing.T) {
	t.Run("nil server", func(t *testing.T) {
		var server *Server
		if err := server.Listen(); err == nil || !strings.Contains(err.Error(), "nil server") {
			t.Fatalf("Listen() err = %v", err)
		}
	})

	t.Run("nil key pair", func(t *testing.T) {
		server := &Server{}
		if err := server.Listen(); err == nil || !strings.Contains(err.Error(), "empty local static private key") {
			t.Fatalf("Listen() empty local static private key err = %v", err)
		}
	})
}

func TestServerServeReturnsNilAfterClose(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	server := &Server{
		LocalStatic: *keyPair,
		ListenAddr:  "127.0.0.1:0",
		PeerStore:   mustBadgerInMemory(t, nil),
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve()
	}()

	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() after Close() error = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not return after Close()")
	}
}

func TestServerPublicKeyAndPeerServiceAccessors(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	service := &PeerService{}
	server := &Server{LocalStatic: *keyPair, peerService: service}
	if got := server.PublicKey(); got != keyPair.Public {
		t.Fatalf("PublicKey() = %v, want %v", got, keyPair.Public)
	}
	if got := server.PeerService(); got != service {
		t.Fatalf("PeerService() = %v, want %v", got, service)
	}
}

func TestServerInitConfiguresPeerRunService(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}
	if server.manager == nil || server.manager.PeerRun == nil {
		t.Fatalf("manager peer run service not configured: %+v", server.manager)
	}
	conn := &GearConn{Service: server.peerService}
	conn.initRPC()
	if conn.rpc == nil || conn.rpc.peerRun != server.manager.PeerRun {
		t.Fatalf("GearConn rpc peerRun = %+v, want %+v", conn.rpc, server.manager.PeerRun)
	}
}

func TestServerServeHTTPLoginRegisterAndGearAPI(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}
	server := &Server{
		LocalStatic: *serverKey,
		PeerStore:   mustBadgerInMemory(t, nil),
		BuildCommit: "test-build",
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}
	ts := httptest.NewServer(server)
	defer ts.Close()

	infoResp, err := http.Get(ts.URL + "/api/public/server-info")
	if err != nil {
		t.Fatalf("GET server-info error = %v", err)
	}
	if infoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", infoResp.StatusCode)
	}
	_ = infoResp.Body.Close()

	_ = publicHTTPTestLogin(t, ts.URL, serverKey.Public, deviceKey)
}

func publicHTTPTestLogin(t *testing.T, baseURL string, serverPublicKey giznet.PublicKey, deviceKey *giznet.KeyPair) publiclogin.LoginResponse {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(deviceKey, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/api/public/login", nil)
	if err != nil {
		t.Fatalf("NewRequest login error = %v", err)
	}
	req.Header.Set(publiclogin.PublicKeyHeader, deviceKey.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST login error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST login status = %d", resp.StatusCode)
	}
	var result publiclogin.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return result
}

func TestServerSecurityPolicyAllowServiceUsesGearPolicy(t *testing.T) {
	var nilServer *Server
	if (*ServerSecurityPolicy)(nilServer).AllowService(giznet.PublicKey{}, ServiceRPC) {
		t.Fatal("nil server should deny all services")
	}

	gearKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair gear error = %v", err)
	}
	adminKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair admin error = %v", err)
	}
	gearsServer := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := gearsServer.SaveGear(context.Background(), apitypes.Gear{
		PublicKey:     gearKey.Public.String(),
		Role:          apitypes.GearRoleGear,
		Status:        apitypes.GearStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SaveGear gear error = %v", err)
	}
	if _, err := gearsServer.SaveGear(context.Background(), apitypes.Gear{
		PublicKey:     adminKey.Public.String(),
		Role:          apitypes.GearRoleAdmin,
		Status:        apitypes.GearStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SaveGear admin error = %v", err)
	}
	server := &Server{manager: NewManager(gearsServer)}
	policy := (*ServerSecurityPolicy)(server)
	if !policy.AllowService(gearKey.Public, ServiceRPC) {
		t.Fatal("gear should allow rpc")
	}
	if !policy.AllowService(gearKey.Public, ServiceServerPublic) {
		t.Fatal("gear should allow server public")
	}
	if policy.AllowService(gearKey.Public, ServiceAdmin) {
		t.Fatal("non-admin gear should not allow admin")
	}
	if !policy.AllowService(adminKey.Public, ServiceAdmin) {
		t.Fatal("active admin gear should allow admin")
	}
	configuredKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair configured error = %v", err)
	}
	server.SecurityPolicy = testGiznetSecurityPolicy{
		allowService: func(publicKey giznet.PublicKey, service uint64) bool {
			return service == ServiceAdmin && publicKey == configuredKey.Public
		},
	}
	if !policy.AllowService(configuredKey.Public, ServiceAdmin) {
		t.Fatal("configured security policy should allow admin")
	}
	server.SecurityPolicy = nil
	if policy.AllowService(configuredKey.Public, ServiceAdmin) {
		t.Fatal("missing configured security policy should not allow admin")
	}
}

func TestServerPeerEventHandlerMarksManagerOffline(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{manager: &Manager{}}
	server.manager.SetPeerUp(keyPair.Public, &giznet.Conn{})

	(*serverPeerEventHandler)(server).HandlePeerEvent(giznet.PeerEvent{PublicKey: keyPair.Public, State: giznet.PeerStateOffline})
	runtime := server.manager.PeerRuntime(context.Background(), keyPair.Public)
	if runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after offline event = %+v", runtime)
	}
}
