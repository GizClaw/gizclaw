package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/stores"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/pion/webrtc/v4"
)

func TestCmdServerServeHTTPNilServerReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	(*CmdServer)(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nil server status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCmdServerPrivateIngressRequiresAuthorizedSession(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	adminKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(admin) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	srv, err := New(Config{
		KeyPair:  serverKey,
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer srv.Close()

	peers := &peer.Server{Store: kv.Prefixed(srv.Server.PeerStore, kv.Key{"peers"})}
	for _, item := range []apitypes.Peer{
		{
			PublicKey:     adminKey.Public.String(),
			Role:          apitypes.PeerRoleAdmin,
			Status:        apitypes.PeerRegistrationStatusActive,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		},
		{
			PublicKey:     clientKey.Public.String(),
			Role:          apitypes.PeerRoleClient,
			Status:        apitypes.PeerRegistrationStatusActive,
			Device:        apitypes.DeviceInfo{},
			Configuration: apitypes.Configuration{},
		},
	} {
		if _, err := peers.SavePeer(context.Background(), item); err != nil {
			t.Fatalf("SavePeer(%s) error = %v", item.PublicKey, err)
		}
	}

	if err := srv.Listen(); err != nil {
		t.Fatalf("Listen() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/server-info", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusUnauthorized, "INVALID_SESSION")

	srv.Server.WebRTCSignalingHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("WebRTC offer Authorization = %q, want signed offer without bearer session", r.Header.Get("Authorization"))
			http.Error(w, "unexpected bearer session", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	req = httptest.NewRequest(http.MethodPost, gizwebrtc.SignalingPath, nil)
	req.Header.Set("X-Giznet-Public-Key", clientKey.Public.String())
	req.Header.Set("X-Giznet-Timestamp", "1")
	req.Header.Set("X-Giznet-Nonce", "nonce")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d body=%s, want %d", gizwebrtc.SignalingPath, rec.Code, rec.Body.String(), http.StatusOK)
	}

	clientLogin := cmdServerTestLogin(t, srv, serverKey.Public, clientKey)
	assertHTTPError(t, clientLogin, http.StatusUnauthorized, "INVALID_ASSERTION")

	adminLogin := cmdServerTestLogin(t, srv, serverKey.Public, adminKey)
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin POST /login status = %d body=%s", adminLogin.Code, adminLogin.Body.String())
	}
	var session publiclogin.LoginResponse
	if err := json.Unmarshal(adminLogin.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode admin login response: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, clientKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusUnauthorized, "PUBLIC_KEY_MISMATCH")

	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized GET /server-info status = %d body=%s", rec.Code, rec.Body.String())
	}

	if _, err := peers.SavePeer(context.Background(), apitypes.Peer{
		PublicKey:     adminKey.Public.String(),
		Role:          apitypes.PeerRoleClient,
		Status:        apitypes.PeerRegistrationStatusActive,
		Device:        apitypes.DeviceInfo{},
		Configuration: apitypes.Configuration{},
	}); err != nil {
		t.Fatalf("SavePeer(demoted admin) error = %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/server-info", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publiclogin.PublicKeyHeader, adminKey.Public.String())
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	assertHTTPError(t, rec, http.StatusForbidden, "PRIVATE_INGRESS_DENIED")
}

func assertHTTPError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), status)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code = %q body=%s, want %q", body.Error.Code, rec.Body.String(), code)
	}
}

func cmdServerTestLogin(t *testing.T, srv *CmdServer, serverPublicKey giznet.PublicKey, keyPair *giznet.KeyPair) *httptest.ResponseRecorder {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(keyPair, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.Header.Set(publiclogin.PublicKeyHeader, keyPair.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestNewWithOptionsWiresLegacyMetricsStore(t *testing.T) {
	srv, err := newWithOptions(Config{
		Listen:   "127.0.0.1:0",
		Endpoint: "127.0.0.1:0",
		Stores: map[string]stores.Config{
			defaultPeersStore: {Kind: stores.KindKeyValue, Backend: "memory"},
			defaultMetricsStore: {
				Kind: stores.KindMetrics,
				Prometheus: &metrics.PrometheusConfig{
					RemoteWriteURL: "http://127.0.0.1:1/api/v1/write",
					QueryURL:       "http://127.0.0.1:1",
				},
			},
		},
	}, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()
	if srv.Server.MetricsStore == nil {
		t.Fatal("MetricsStore is nil")
	}
}

func TestConfigListenAddrs(t *testing.T) {
	cfg := Config{Listen: "0.0.0.0:9820", Endpoint: "192.168.1.20:9820"}
	if got := cfg.PublicAPIListenAddr(); got != "0.0.0.0:9820" {
		t.Fatalf("PublicAPIListenAddr = %q", got)
	}
	if got := cfg.ICEListenAddr(); got != "0.0.0.0:9820" {
		t.Fatalf("ICEListenAddr = %q", got)
	}
}

func TestWebRTCListenConfigUsesListenAndPublicEndpoint(t *testing.T) {
	policy := testSecurityPolicy{}
	handler := testPeerEventHandler{}
	iceTCPListener := &testListener{addr: testAddr("0.0.0.0:9820")}
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "192.168.1.20:19820"}, gizclaw.PeerListenerOptions{
		SecurityPolicy:   policy,
		PeerEventHandler: handler,
	}, iceTCPListener)

	if cfg.ICEUDPAddr != "0.0.0.0:9820" || cfg.ICETCPAddr != "" {
		t.Fatalf("ICE addrs = %q, %q", cfg.ICEUDPAddr, cfg.ICETCPAddr)
	}
	if cfg.ICETCPListener != iceTCPListener {
		t.Fatal("ICETCPListener not preserved")
	}
	if cfg.PublicICEUDPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICEUDPAddr = %q", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICETCPAddr = %q", cfg.PublicICETCPAddr)
	}
	if len(cfg.NAT1To1IPs) != 0 {
		t.Fatalf("NAT1To1IPs = %#v", cfg.NAT1To1IPs)
	}
	if cfg.ICELite {
		t.Fatal("ICELite = true, want false")
	}
	if cfg.SecurityPolicy != policy {
		t.Fatal("SecurityPolicy not preserved")
	}
	if cfg.PeerEventHandler != handler {
		t.Fatal("PeerEventHandler not preserved")
	}
}

func TestWebRTCListenConfigSkipsUnspecifiedPublicEndpoint(t *testing.T) {
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "0.0.0.0:9820"}, gizclaw.PeerListenerOptions{}, nil)
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "" {
		t.Fatalf("PublicICETCPAddr = %q, want empty", cfg.PublicICETCPAddr)
	}
}

func TestWebRTCListenConfigSkipsHostnamePublicEndpoint(t *testing.T) {
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "example.com:9820"}, gizclaw.PeerListenerOptions{}, nil)
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "" {
		t.Fatalf("PublicICETCPAddr = %q, want empty", cfg.PublicICETCPAddr)
	}
}

func TestWebRTCListenConfigUsesRelayOnlyWithICEServers(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:       []string{"turn:edge.example.com:3478?transport=udp"},
			Username:   "edge",
			Credential: "secret",
		}},
	}, gizclaw.PeerListenerOptions{}, nil)

	if cfg.ICETransportPolicy != webrtc.ICETransportPolicyRelay {
		t.Fatalf("ICETransportPolicy = %s, want relay", cfg.ICETransportPolicy)
	}
}

func TestWebRTCListenConfigKeepsTURNRESTCredentialsForPerAnswerMinting(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
			Username:       "edge",
			Credential:     "long-term-secret",
			CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
		}},
	}, gizclaw.PeerListenerOptions{}, nil)
	if len(cfg.ICEServers) != 1 {
		t.Fatalf("ICEServers len = %d, want 1", len(cfg.ICEServers))
	}
	got := cfg.ICEServers[0]
	if got.Username != "edge" || got.Credential != "long-term-secret" || got.CredentialMode != gizwebrtc.ICECredentialModeTURNREST {
		t.Fatalf("ICEServers[0] = %+v, want raw TURN REST config", got)
	}
}

func TestWebRTCListenConfigRejectsEmptyTURNRESTSecret(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
			Username:       "edge",
			CredentialMode: gizwebrtc.ICECredentialModeTURNREST,
		}},
	}, gizclaw.PeerListenerOptions{}, nil)
	if _, err := cfg.Listen(testKeyPair(t, 0x44)); err == nil {
		t.Fatal("Listen error = nil, want empty TURN REST credential rejection")
	}
}

func TestWebRTCListenConfigKeepsDefaultPolicyWithSTUNOnlyICEServers(t *testing.T) {
	cfg := webRTCListenConfig(Config{
		Listen:   "0.0.0.0:9820",
		Endpoint: "192.168.1.20:19820",
		ICEServers: []gizwebrtc.ICEServer{{
			URLs: []string{"stun:edge.example.com:3478"},
		}},
	}, gizclaw.PeerListenerOptions{}, nil)

	if cfg.ICETransportPolicy != webrtc.ICETransportPolicyAll {
		t.Fatalf("ICETransportPolicy = %s, want all", cfg.ICETransportPolicy)
	}
	if cfg.PublicICEUDPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICEUDPAddr = %q, want endpoint", cfg.PublicICEUDPAddr)
	}
	if cfg.PublicICETCPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICETCPAddr = %q, want endpoint", cfg.PublicICETCPAddr)
	}
}

type testSecurityPolicy struct{}

func (testSecurityPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (testSecurityPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

type testPeerEventHandler struct{}

func (testPeerEventHandler) HandlePeerEvent(giznet.PeerEvent) {}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type testListener struct {
	addr testAddr
}

func (l *testListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *testListener) Close() error              { return nil }
func (l *testListener) Addr() net.Addr            { return l.addr }
