package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizmetrics"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
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

func TestCmdServerRejectsDirectProtectedRoutesBeforeAuthentication(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg := validLayeredConfig(t.TempDir())
	cfg.KeyPair = serverKey
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer srv.Close()

	for _, path := range []string{"/gizclaw/v1/api-keys/self", "/openai/v1/models"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer invalid")
		srv.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("GET %s status = %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}
func TestNewWithOptionsWiresPrometheusMetricsStore(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Storage["prometheus"] = storage.PrometheusConfig{
		RemoteWriteURL: "http://127.0.0.1:1/api/v1/write",
		QueryURL:       "http://127.0.0.1:1",
	}
	cfg.Stores["test-metrics"] = stores.Config{Kind: stores.KindMetrics, Storage: "prometheus"}
	cfg.Services.Metrics = &SingleStoreConfig{Store: "test-metrics"}
	srv, err := newWithOptions(cfg, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()
	if srv.Server.MetricsStore == nil {
		t.Fatal("MetricsStore is nil")
	}
}

func TestNewWithOptionsWiresServerInfoMetadata(t *testing.T) {
	originalVersion, originalCommit := buildinfo.Version, buildinfo.Commit
	buildinfo.Version, buildinfo.Commit = "0.2.5", "deadbeef"
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = originalVersion, originalCommit
	})

	cfg := validLayeredConfig(t.TempDir())
	srv, err := newWithOptions(cfg, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()
	if srv.Server.BuildVersion != "0.2.5" || srv.Server.BuildCommit != "deadbeef" {
		t.Fatalf("server info metadata = version %q commit %q",
			srv.Server.BuildVersion, srv.Server.BuildCommit)
	}
}

func TestNewWithOptionsInstallsAndFlushesMetricsRecorder(t *testing.T) {
	cfg := validLayeredConfig(t.TempDir())
	cfg.Stores["test-metrics"] = stores.Config{Kind: stores.KindMetrics, Storage: "memory"}
	cfg.Services.Metrics = &SingleStoreConfig{Store: "test-metrics"}
	srv, err := newWithOptions(cfg, newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	gizmetrics.AddCounter(context.Background(), "gizclaw_server_test_total", 2,
		gizmetrics.Label{Name: "result", Value: "ok"},
	)
	if err := srv.shutdownMetrics(context.Background()); err != nil {
		t.Fatalf("shutdownMetrics() error = %v", err)
	}
	series, err := srv.Server.MetricsStore.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{
			Name: "gizclaw_server_test_total",
			Matchers: []metrics.LabelMatcher{
				{Name: "result", Op: metrics.MatchEqual, Value: "ok"},
			},
		},
		At:       time.Now(),
		Lookback: time.Minute,
	})
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 2 {
		t.Fatalf("Latest() = %#v, want one sample with value 2", series)
	}
}

func TestNewWithOptionsWithoutMetricsStoreLeavesRecorderDisabled(t *testing.T) {
	srv, err := newWithOptions(validLayeredConfig(t.TempDir()), newServerOptions{})
	if err != nil {
		t.Fatalf("newWithOptions() error = %v", err)
	}
	defer srv.Close()

	if srv.metricsShutdown != nil {
		t.Fatal("metricsShutdown is configured without a metrics store")
	}
	gizmetrics.AddCounter(context.Background(), "gizclaw_server_no_store_total", 1)
}

func TestCmdServerCloseStopsServerBeforeMetricsRecorder(t *testing.T) {
	srv := &CmdServer{Server: &gizclaw.Server{}}
	srv.metricsShutdown = func(context.Context) error {
		if srv.Server != nil {
			return errors.New("metrics recorder stopped before gizclaw server")
		}
		return nil
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewWithOptionsConcurrentMetricsInstallPreservesExistingRecorder(t *testing.T) {
	existing := metrics.NewMemoryStore()
	shutdown, err := gizmetrics.InstallStore(existing, gizmetrics.WithFlushInterval(time.Hour))
	if err != nil {
		t.Fatalf("InstallStore(existing) error = %v", err)
	}
	t.Cleanup(func() {
		_ = shutdown(context.Background())
		_ = existing.Close()
	})

	cfg := validLayeredConfig(t.TempDir())
	cfg.Stores["test-metrics"] = stores.Config{Kind: stores.KindMetrics, Storage: "memory"}
	cfg.Services.Metrics = &SingleStoreConfig{Store: "test-metrics"}
	_, err = newWithOptions(cfg, newServerOptions{})
	if !errors.Is(err, gizmetrics.ErrAlreadyInstalled) {
		t.Fatalf("newWithOptions() error = %v, want %v", err, gizmetrics.ErrAlreadyInstalled)
	}

	gizmetrics.AddCounter(context.Background(), "gizclaw_existing_recorder_total", 1)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown(existing) error = %v", err)
	}
	series, err := existing.Latest(context.Background(), metrics.LatestQuery{
		Selector: metrics.Selector{Name: "gizclaw_existing_recorder_total"},
		At:       time.Now(),
		Lookback: time.Minute,
	})
	if err != nil {
		t.Fatalf("Latest(existing) error = %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 1 {
		t.Fatalf("existing recorder series = %#v", series)
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
	policy := testGatewaySCTPSecurityPolicy{}
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
	if cfg.GatewaySCTPPeer == nil || !cfg.GatewaySCTPPeer(context.Background(), giznet.PublicKey{}) {
		t.Fatal("GatewaySCTPPeer policy not preserved")
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

type testGatewaySCTPSecurityPolicy struct{ testSecurityPolicy }

func (testGatewaySCTPSecurityPolicy) AllowGatewaySCTP(giznet.PublicKey) bool {
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
