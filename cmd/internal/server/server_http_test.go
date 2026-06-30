package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCmdServerServeHTTPNilServerReturnsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	(*CmdServer)(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nil server status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestConfigTransportListenAddrs(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", PublicAPIPort: 9820, NoiseUDPPort: 9822, ICEPort: 9821}
	if got := cfg.PublicAPIListenAddr(); got != "127.0.0.1:9820" {
		t.Fatalf("PublicAPIListenAddr = %q", got)
	}
	if got := cfg.NoiseUDPListenAddr(); got != "127.0.0.1:9822" {
		t.Fatalf("NoiseUDPListenAddr = %q", got)
	}
	if got := cfg.ICEListenAddr(); got != "127.0.0.1:9821" {
		t.Fatalf("ICEListenAddr = %q", got)
	}
	cfg.ListenAddr = "127.0.0.1:9999"
	if got := cfg.PublicAPIListenAddr(); got != "127.0.0.1:9999" {
		t.Fatalf("PublicAPIListenAddr legacy = %q", got)
	}
	if got := cfg.NoiseUDPListenAddr(); got != "127.0.0.1:9999" {
		t.Fatalf("NoiseUDPListenAddr legacy = %q", got)
	}
}
