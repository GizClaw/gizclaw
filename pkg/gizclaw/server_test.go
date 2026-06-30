package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/publiclogin"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
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
		LocalStatic:   *keyPair,
		PeerStore:     mustBadgerInMemory(t, nil),
		PeerListeners: []giznet.Listener{newTestGiznetListener()},
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

func TestServerCanListenAgainAfterClose(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}

	first := newTestGiznetListener()
	second := newTestGiznetListener()
	server := &Server{
		LocalStatic:   *keyPair,
		PeerStore:     mustBadgerInMemory(t, nil),
		PeerListeners: []giznet.Listener{first},
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("first Listen() error = %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	server.PeerListeners = []giznet.Listener{second}
	if err := server.Listen(); err != nil {
		t.Fatalf("second Listen() error = %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve()
	}()
	if err := second.Close(); err != nil {
		t.Fatalf("second listener Close() error = %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() after second listener close error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve() did not use second listener")
	}
}

func TestServerPublicWebRTCSignalingUsesGeneratedRoute(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	var gotBody []byte
	var gotContentType string
	var gotPublicKey string
	var gotTimestamp string
	var gotNonce string
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
		WebRTCSignalingHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("signaling method = %q", r.Method)
			}
			if r.URL.Path != gizwebrtc.SignalingPath {
				t.Fatalf("signaling path = %q", r.URL.Path)
			}
			gotContentType = r.Header.Get("Content-Type")
			gotPublicKey = r.Header.Get("X-Giznet-Public-Key")
			gotTimestamp = r.Header.Get("X-Giznet-Timestamp")
			gotNonce = r.Header.Get("X-Giznet-Nonce")
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("encrypted-answer"))
		}),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, bytes.NewReader([]byte("encrypted-offer")))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "encrypted-answer" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if gotContentType != "application/octet-stream" {
		t.Fatalf("forwarded content-type = %q", gotContentType)
	}
	if gotPublicKey != "peer-public" || gotTimestamp != "123456789" || gotNonce != "nonce" {
		t.Fatalf("forwarded headers public=%q ts=%q nonce=%q", gotPublicKey, gotTimestamp, gotNonce)
	}
	if string(gotBody) != "encrypted-offer" {
		t.Fatalf("forwarded body = %q", string(gotBody))
	}
}

func TestServerPublicWebRTCSignalingUnavailable(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, strings.NewReader("encrypted-offer"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Error != "webrtc_signaling_listener_unavailable" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestServerPublicWebRTCSignalingPreservesContentType(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic: *keyPair,
		PeerStore:   mustBadgerInMemory(t, nil),
		WebRTCSignalingHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("forwarded content-type = %q", r.Header.Get("Content-Type"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"error":"unsupported_media_type"}`))
		}),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, strings.NewReader(`{"offer":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Giznet-Public-Key", "peer-public")
	req.Header.Set("X-Giznet-Timestamp", "123456789")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if payload.Error != "unsupported_media_type" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestServerServeWithoutListenStillRequiresListenerAfterClose(t *testing.T) {
	server := &Server{}
	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := server.Serve(); !errors.Is(err, giznet.ErrNilListener) {
		t.Fatalf("Serve() error = %v, want %v", err, giznet.ErrNilListener)
	}
}

func TestServerCloseWaitsForCleanupLoop(t *testing.T) {
	server := &Server{
		FriendGroupMessageCleanup: time.Hour,
		manager: &Manager{
			FriendGroups: &friendgroup.Server{Messages: kv.NewMemory(nil)},
		},
	}
	server.startCleanup()
	if server.cleanupStop == nil || server.cleanupDone == nil {
		t.Fatal("startCleanup did not start cleanup loop")
	}

	done := make(chan error, 1)
	go func() {
		done <- server.Close()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not wait for cleanup loop to exit")
	}
	if server.cleanupStop != nil || server.cleanupDone != nil {
		t.Fatal("Close() did not clear cleanup state")
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
	if server.manager == nil || server.manager.PeerRun == nil || server.manager.AgentHost == nil || server.manager.Voices == nil || server.manager.ProviderTenants == nil {
		t.Fatalf("manager peer run runtime services not configured: %+v", server.manager)
	}
	conn := &PeerConn{Service: server.peerService}
	conn.initRPC()
	if conn.rpc == nil || conn.rpc.peerRun != server.manager.PeerRun {
		t.Fatalf("PeerConn rpc peerRun = %+v, want %+v", conn.rpc, server.manager.PeerRun)
	}
}

func TestServerInitConfiguresWorkspaceRuntimeStore(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{
		LocalStatic:    *keyPair,
		PeerStore:      mustBadgerInMemory(t, nil),
		AgentHostStore: objectstore.Dir(t.TempDir()),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}
	workspaces, ok := server.manager.Workspaces.(*workspace.Server)
	if server.manager == nil || !ok || workspaces.RuntimeStore == nil {
		t.Fatalf("workspace runtime store not configured: %+v", server.manager)
	}
}

func TestServerServeHTTPLoginRegisterAndPeerAPI(t *testing.T) {
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

func TestServerSecurityPolicyAllowServiceUsesPeerPolicy(t *testing.T) {
	var nilServer *Server
	if (*ServerSecurityPolicy)(nilServer).AllowService(giznet.PublicKey{}, ServiceRPC) {
		t.Fatal("nil server should deny all services")
	}

	peerKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair peer error = %v", err)
	}
	adminKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair admin error = %v", err)
	}
	peersServer := &peer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     peerKey.Public.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer peer error = %v", err)
	}
	if _, err := peersServer.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     adminKey.Public.String(),
		Role:          apitypes.PeerRoleAdmin,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer admin error = %v", err)
	}
	server := &Server{manager: NewManager(peersServer)}
	policy := (*ServerSecurityPolicy)(server)
	if !policy.AllowService(peerKey.Public, ServiceRPC) {
		t.Fatal("peer should allow rpc")
	}
	if !policy.AllowService(peerKey.Public, ServiceServerPublic) {
		t.Fatal("peer should allow server public")
	}
	if policy.AllowService(peerKey.Public, ServiceAdmin) {
		t.Fatal("non-admin peer should not allow admin")
	}
	if !policy.AllowService(adminKey.Public, ServiceAdmin) {
		t.Fatal("active admin peer should allow admin")
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
	server.manager.SetPeerUp(keyPair.Public, &testGiznetConn{})

	(*serverPeerEventHandler)(server).HandlePeerEvent(giznet.PeerEvent{PublicKey: keyPair.Public, State: giznet.PeerStateOffline})
	runtime := server.manager.PeerRuntime(context.Background(), keyPair.Public)
	if runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after offline event = %+v", runtime)
	}
}
