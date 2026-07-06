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
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "192.168.1.20:19820"}, gizclaw.PeerListenerOptions{
		SecurityPolicy:   policy,
		PeerEventHandler: handler,
	})

	if cfg.ICEUDPAddr != "0.0.0.0:9820" || cfg.ICETCPAddr != "" {
		t.Fatalf("ICE addrs = %q, %q", cfg.ICEUDPAddr, cfg.ICETCPAddr)
	}
	if cfg.PublicICEUDPAddr != "192.168.1.20:19820" {
		t.Fatalf("PublicICEUDPAddr = %q", cfg.PublicICEUDPAddr)
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
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "0.0.0.0:9820"}, gizclaw.PeerListenerOptions{})
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
	}
}

func TestWebRTCListenConfigSkipsHostnamePublicEndpoint(t *testing.T) {
	cfg := webRTCListenConfig(Config{Listen: "0.0.0.0:9820", Endpoint: "example.com:9820"}, gizclaw.PeerListenerOptions{})
	if cfg.PublicICEUDPAddr != "" {
		t.Fatalf("PublicICEUDPAddr = %q, want empty", cfg.PublicICEUDPAddr)
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
