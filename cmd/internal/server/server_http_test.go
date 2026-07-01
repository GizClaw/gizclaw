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

func TestConfigEndpointListenAddrs(t *testing.T) {
	cfg := Config{Endpoint: "127.0.0.1:9820"}
	if got := cfg.PublicAPIListenAddr(); got != "127.0.0.1:9820" {
		t.Fatalf("PublicAPIListenAddr = %q", got)
	}
	if got := cfg.ICEListenAddr(); got != "127.0.0.1:9820" {
		t.Fatalf("ICEListenAddr = %q", got)
	}
}
