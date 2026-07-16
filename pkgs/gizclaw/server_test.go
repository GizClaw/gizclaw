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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
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

func TestServerInitKeepsLegacyPeerPrefixWithMetricsStore(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	baseStore := kv.NewMemory(nil)
	server := &Server{
		LocalStatic:  *keyPair,
		PeerStore:    baseStore,
		MetricsStore: metrics.NewMemoryStore(),
	}
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}

	peerKeyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(peer) error = %v", err)
	}
	if _, err := server.manager.Peers.EnsureConnectedPeer(context.Background(), peerKeyPair.Public); err != nil {
		t.Fatalf("EnsureConnectedPeer() error = %v", err)
	}
	if _, err := baseStore.Get(context.Background(), kv.Key{"peers", "by-pubkey", peerKeyPair.Public.String()}); err != nil {
		t.Fatalf("prefixed peer key missing: %v", err)
	}
	if _, err := baseStore.Get(context.Background(), kv.Key{"by-pubkey", peerKeyPair.Public.String()}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("root peer key error = %v, want ErrNotFound", err)
	}
}

func TestServerInitKeepsLegacyPeerPrefixWithObjectStores(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Server, objectstore.ObjectStore)
	}{
		{name: "peer", configure: func(server *Server, store objectstore.ObjectStore) { server.PeerAssets = store }},
		{name: "workspace", configure: func(server *Server, store objectstore.ObjectStore) { server.WorkspaceAssets = store }},
		{name: "workflow", configure: func(server *Server, store objectstore.ObjectStore) { server.WorkflowAssets = store }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}
			baseStore := kv.NewMemory(nil)
			server := &Server{LocalStatic: *keyPair, PeerStore: baseStore}
			tc.configure(server, objectstore.Dir(t.TempDir()))
			if err := server.init(); err != nil {
				t.Fatalf("init() error = %v", err)
			}

			peerKeyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair(peer) error = %v", err)
			}
			if _, err := server.manager.Peers.EnsureConnectedPeer(context.Background(), peerKeyPair.Public); err != nil {
				t.Fatalf("EnsureConnectedPeer() error = %v", err)
			}
			if _, err := baseStore.Get(context.Background(), kv.Key{"peers", "by-pubkey", peerKeyPair.Public.String()}); err != nil {
				t.Fatalf("prefixed peer key missing: %v", err)
			}
			if _, err := baseStore.Get(context.Background(), kv.Key{"by-pubkey", peerKeyPair.Public.String()}); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("root peer key error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestServerInitPreservesExistingObjectStorePeerLayout(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*Server, objectstore.ObjectStore)
	}{
		{name: "agent host", configure: func(server *Server, store objectstore.ObjectStore) { server.AgentHostStore = store }},
		{name: "friend group message", configure: func(server *Server, store objectstore.ObjectStore) { server.FriendGroupMessageAssets = store }},
		{name: "gameplay", configure: func(server *Server, store objectstore.ObjectStore) { server.GameplayAssets = store }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair() error = %v", err)
			}
			baseStore := kv.NewMemory(nil)
			server := &Server{LocalStatic: *keyPair, PeerStore: baseStore}
			tc.configure(server, objectstore.Dir(t.TempDir()))
			if err := server.init(); err != nil {
				t.Fatalf("init() error = %v", err)
			}

			peerKeyPair, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatalf("GenerateKeyPair(peer) error = %v", err)
			}
			if _, err := server.manager.Peers.EnsureConnectedPeer(context.Background(), peerKeyPair.Public); err != nil {
				t.Fatalf("EnsureConnectedPeer() error = %v", err)
			}
			if _, err := baseStore.Get(context.Background(), kv.Key{"by-pubkey", peerKeyPair.Public.String()}); err != nil {
				t.Fatalf("root peer key missing: %v", err)
			}
			if _, err := baseStore.Get(context.Background(), kv.Key{"peers", "by-pubkey", peerKeyPair.Public.String()}); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("prefixed peer key error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestServerInitWiresDefaultPeerView(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	server := &Server{
		LocalStatic:     *keyPair,
		PeerStore:       mustBadgerInMemory(t, nil),
		DefaultPeerView: "default-client",
	}
	if err := server.init(); err != nil {
		t.Fatalf("init() error = %v", err)
	}

	peerKeyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(peer) error = %v", err)
	}
	created, err := server.manager.Peers.EnsureConnectedPeer(context.Background(), peerKeyPair.Public)
	if err != nil {
		t.Fatalf("EnsureConnectedPeer() error = %v", err)
	}
	if created.Configuration.View == nil || *created.Configuration.View != "default-client" {
		t.Fatalf("created view = %v, want default-client", created.Configuration.View)
	}
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

func TestPeerHTTPWebRTCSignalingUsesGeneratedRoute(t *testing.T) {
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

func TestPeerHTTPWebRTCSignalingUnavailable(t *testing.T) {
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

func TestPeerHTTPWebRTCSignalingPreservesContentType(t *testing.T) {
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
	if _, err := server.manager.EnsurePeer(context.Background(), deviceKey.Public); err != nil {
		t.Fatalf("EnsurePeer error = %v", err)
	}
	ts := httptest.NewServer(server)
	defer ts.Close()

	infoResp, err := http.Get(ts.URL + "/server-info")
	if err != nil {
		t.Fatalf("GET server-info error = %v", err)
	}
	if infoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", infoResp.StatusCode)
	}
	_ = infoResp.Body.Close()

	oldInfoResp, err := http.Get(ts.URL + "/api/public/server-info")
	if err != nil {
		t.Fatalf("GET old server-info error = %v", err)
	}
	if oldInfoResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET old server-info status = %d, want %d", oldInfoResp.StatusCode, http.StatusNotFound)
	}
	_ = oldInfoResp.Body.Close()

	session := publicHTTPTestLogin(t, ts.URL, serverKey.Public, deviceKey)

	openAIPreflightReq, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, ts.URL+"/openai/v1/models", nil)
	if err != nil {
		t.Fatalf("NewRequest OPTIONS /openai/v1/models error = %v", err)
	}
	openAIPreflightReq.Header.Set("Origin", "wails://wails.localhost")
	openAIPreflightReq.Header.Set("Access-Control-Request-Method", http.MethodGet)
	openAIPreflightReq.Header.Set("Access-Control-Request-Headers", "authorization,x-public-key")
	openAIPreflightResp, err := http.DefaultClient.Do(openAIPreflightReq)
	if err != nil {
		t.Fatalf("OPTIONS /openai/v1/models error = %v", err)
	}
	defer openAIPreflightResp.Body.Close()
	if openAIPreflightResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(openAIPreflightResp.Body)
		t.Fatalf("OPTIONS /openai/v1/models status = %d body=%s", openAIPreflightResp.StatusCode, string(body))
	}
	if got := openAIPreflightResp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") || !strings.Contains(got, publiclogin.PublicKeyHeader) {
		t.Fatalf("Access-Control-Allow-Headers = %q, want session headers", got)
	}

	unauthMe, err := http.Get(ts.URL + "/me")
	if err != nil {
		t.Fatalf("GET unauth /me error = %v", err)
	}
	if unauthMe.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET unauth /me status = %d, want %d", unauthMe.StatusCode, http.StatusUnauthorized)
	}
	_ = unauthMe.Body.Close()

	missingKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(missing) error = %v", err)
	}
	missingSession := publicHTTPTestLogin(t, ts.URL, serverKey.Public, missingKey)
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/me/status"},
		{method: http.MethodPut, path: "/me/status", body: `{"battery_percent":66}`},
		{method: http.MethodGet, path: "/me/runtime"},
	} {
		req, err := http.NewRequestWithContext(context.Background(), tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("NewRequest missing peer %s %s error = %v", tc.method, tc.path, err)
		}
		req.Header.Set("Authorization", "Bearer "+missingSession.AccessToken)
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("missing peer %s %s error = %v", tc.method, tc.path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			t.Fatalf("missing peer %s %s status = %d body=%s", tc.method, tc.path, resp.StatusCode, string(body))
		}
		_ = resp.Body.Close()
	}

	getMeReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/me", nil)
	if err != nil {
		t.Fatalf("NewRequest /me error = %v", err)
	}
	getMeReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	getMeResp, err := http.DefaultClient.Do(getMeReq)
	if err != nil {
		t.Fatalf("GET /me error = %v", err)
	}
	defer getMeResp.Body.Close()
	if getMeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getMeResp.Body)
		t.Fatalf("GET /me status = %d body=%s", getMeResp.StatusCode, string(body))
	}
	var me struct {
		PublicKey          string `json:"public_key"`
		RegistrationStatus string `json:"registration_status"`
	}
	if err := json.NewDecoder(getMeResp.Body).Decode(&me); err != nil {
		t.Fatalf("decode /me response: %v", err)
	}
	if me.PublicKey != deviceKey.Public.String() || me.RegistrationStatus != string(apitypes.PeerRegistrationStatusActive) {
		t.Fatalf("/me response = %+v", me)
	}

	putStatusReq, err := http.NewRequestWithContext(context.Background(), http.MethodPut, ts.URL+"/me/status", strings.NewReader(`{"battery_percent":77}`))
	if err != nil {
		t.Fatalf("NewRequest PUT /me/status error = %v", err)
	}
	putStatusReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	putStatusReq.Header.Set("Content-Type", "application/json")
	putStatusResp, err := http.DefaultClient.Do(putStatusReq)
	if err != nil {
		t.Fatalf("PUT /me/status error = %v", err)
	}
	defer putStatusResp.Body.Close()
	if putStatusResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(putStatusResp.Body)
		t.Fatalf("PUT /me/status status = %d body=%s", putStatusResp.StatusCode, string(body))
	}

	getStatusReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/me/status", nil)
	if err != nil {
		t.Fatalf("NewRequest GET /me/status error = %v", err)
	}
	getStatusReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	getStatusResp, err := http.DefaultClient.Do(getStatusReq)
	if err != nil {
		t.Fatalf("GET /me/status error = %v", err)
	}
	defer getStatusResp.Body.Close()
	if getStatusResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getStatusResp.Body)
		t.Fatalf("GET /me/status status = %d body=%s", getStatusResp.StatusCode, string(body))
	}
	var status apitypes.PeerStatus
	if err := json.NewDecoder(getStatusResp.Body).Decode(&status); err != nil {
		t.Fatalf("decode /me/status response: %v", err)
	}
	if status.BatteryPercent == nil || *status.BatteryPercent != 77 {
		t.Fatalf("/me/status = %+v", status)
	}

	getRuntimeReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/me/runtime", nil)
	if err != nil {
		t.Fatalf("NewRequest GET /me/runtime error = %v", err)
	}
	getRuntimeReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	getRuntimeResp, err := http.DefaultClient.Do(getRuntimeReq)
	if err != nil {
		t.Fatalf("GET /me/runtime error = %v", err)
	}
	defer getRuntimeResp.Body.Close()
	if getRuntimeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getRuntimeResp.Body)
		t.Fatalf("GET /me/runtime status = %d body=%s", getRuntimeResp.StatusCode, string(body))
	}

	openAIReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/openai/v1/models", nil)
	if err != nil {
		t.Fatalf("NewRequest GET /openai/v1/models error = %v", err)
	}
	openAIReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	openAIResp, err := http.DefaultClient.Do(openAIReq)
	if err != nil {
		t.Fatalf("GET /openai/v1/models error = %v", err)
	}
	defer openAIResp.Body.Close()
	if openAIResp.StatusCode == http.StatusNotFound || openAIResp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(openAIResp.Body)
		t.Fatalf("GET /openai/v1/models status = %d body=%s", openAIResp.StatusCode, string(body))
	}
}

func publicHTTPTestLogin(t *testing.T, baseURL string, serverPublicKey giznet.PublicKey, deviceKey *giznet.KeyPair) peerhttp.LoginResult {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(deviceKey, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/login", nil)
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
	var result peerhttp.LoginResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return result
}

func TestServerSecurityPolicyAllowServiceUsesPeerPolicy(t *testing.T) {
	var nilServer *Server
	if (*ServerSecurityPolicy)(nilServer).AllowService(giznet.PublicKey{}, ServicePeerRPC) {
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
	if !policy.AllowService(peerKey.Public, ServicePeerRPC) {
		t.Fatal("peer should allow rpc")
	}
	if !policy.AllowService(peerKey.Public, ServicePeerHTTP) {
		t.Fatal("peer should allow server public")
	}
	if policy.AllowService(peerKey.Public, ServiceAdminHTTP) {
		t.Fatal("non-admin peer should not allow admin")
	}
	if !policy.AllowService(adminKey.Public, ServiceAdminHTTP) {
		t.Fatal("active admin peer should allow admin")
	}
	configuredKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair configured error = %v", err)
	}
	server.SecurityPolicy = testGiznetSecurityPolicy{
		allowService: func(publicKey giznet.PublicKey, service uint64) bool {
			return service == ServiceAdminHTTP && publicKey == configuredKey.Public
		},
	}
	if !policy.AllowService(configuredKey.Public, ServiceAdminHTTP) {
		t.Fatal("configured security policy should allow admin")
	}
	server.SecurityPolicy = nil
	if policy.AllowService(configuredKey.Public, ServiceAdminHTTP) {
		t.Fatal("missing configured security policy should not allow admin")
	}
}

func TestServerPeerEventHandlerDoesNotClearActivePeer(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := &Server{manager: &Manager{}}
	server.manager.SetPeerUp(keyPair.Public, &testGiznetConn{})

	(*serverPeerEventHandler)(server).HandlePeerEvent(giznet.PeerEvent{PublicKey: keyPair.Public, State: giznet.PeerStateOffline})
	runtime := server.manager.PeerRuntime(context.Background(), keyPair.Public)
	if !runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("runtime after offline event = %+v, want active peer unchanged", runtime)
	}
}
