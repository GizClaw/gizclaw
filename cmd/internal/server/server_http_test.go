package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestCmdServerServeHTTPNilServerReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	(*CmdServer)(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nil server status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestConfigEndpointListenAddrs(t *testing.T) {
	cfg := Config{Endpoint: "127.0.0.1:9820"}
	if got := cfg.PublicAPIListenAddr(); got != "127.0.0.1:9820" {
		t.Fatalf("PublicAPIListenAddr = %q", got)
	}
	if got := cfg.ICEListenAddr(); got != "127.0.0.1:9820" {
		t.Fatalf("ICEListenAddr = %q", got)
	}
}

func TestWebRTCListenConfigReadsRuntimeEnvAtServerBoundary(t *testing.T) {
	t.Setenv("GIZCLAW_WEBRTC_ICE_TCP_ADDR", "0.0.0.0:9821")
	t.Setenv("GIZCLAW_WEBRTC_NAT1TO1_IPS", " 198.51.100.1, ,203.0.113.2 ")
	t.Setenv("GIZCLAW_WEBRTC_ICE_LITE", "yes")

	policy := testSecurityPolicy{}
	handler := testPeerEventHandler{}
	cfg := webRTCListenConfig(Config{Endpoint: "0.0.0.0:9820"}, gizclaw.PeerListenerOptions{
		SecurityPolicy:   policy,
		PeerEventHandler: handler,
	})

	if cfg.ICEUDPAddr != "0.0.0.0:9820" || cfg.ICETCPAddr != "0.0.0.0:9821" {
		t.Fatalf("ICE addrs = %q, %q", cfg.ICEUDPAddr, cfg.ICETCPAddr)
	}
	if len(cfg.NAT1To1IPs) != 2 || cfg.NAT1To1IPs[0] != "198.51.100.1" || cfg.NAT1To1IPs[1] != "203.0.113.2" {
		t.Fatalf("NAT1To1IPs = %#v", cfg.NAT1To1IPs)
	}
	if !cfg.ICELite {
		t.Fatal("ICELite = false, want true")
	}
	if cfg.SecurityPolicy != policy {
		t.Fatal("SecurityPolicy not preserved")
	}
	if cfg.PeerEventHandler != handler {
		t.Fatal("PeerEventHandler not preserved")
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
