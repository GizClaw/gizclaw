package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	"github.com/GizClaw/gizclaw-go/pkgs/monitor"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
)

func TestInfraFlowConfigurationAndKVBinding(t *testing.T) {
	parsed, err := parseConfigData([]byte("infra_flow:\n  store: flow-windows\n"))
	if err != nil || parsed.InfraFlow.Store != "flow-windows" {
		t.Fatalf("infra_flow config: %+v %v", parsed.InfraFlow, err)
	}
	cfg := validLayeredConfig(t.TempDir())
	cfg.WebRTC.Listen = "127.0.0.1:0"
	cfg.HTTP.Listeners[0].Listen = cfg.WebRTC.Listen
	cfg.Stores["flow-windows"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "flow-windows"}
	cfg.InfraFlow = parsed.InfraFlow
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if server.InfraFlowHandler == nil {
		t.Fatal("KV feed handler not wired into Server")
	}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	monitorToken := "gizclaw_mk_" + strings.Repeat("m", 32)
	httpServer := httptest.NewServer(monitor.Handler(monitor.Config{Token: monitorToken}, monitor.Node{Role: "server"}, server))
	t.Cleanup(httpServer.Close)
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", infraflow.Path + "?environment=prod", 200},
		{"OPTIONS", infraflow.Path, 204},
		{"GET", "/monitor/api/node", 401},
		{"GET", infraflow.Path + "/", 403},
		{"GET", "/gizclaw/v1/device/status", 403},
		{"GET", "/gizclaw/v1/api-keys/self", 403},
	} {
		request, err := http.NewRequestWithContext(t.Context(), test.method, httpServer.URL+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "https://globe.example")
		response, err := httpServer.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatalf("%s %s: %d want %d", test.method, test.path, response.StatusCode, test.status)
		}
		if strings.HasPrefix(test.path, infraflow.Path) && !strings.HasSuffix(test.path, "/") &&
			(response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Access-Control-Allow-Methods") != "GET,OPTIONS") {
			t.Fatalf("public flow policy changed: %v", response.Header)
		}
	}
	if isProtectedPublicHTTPRoute(infraflow.Path) {
		t.Fatal("public feed blocked at Server ingress")
	}
	for _, path := range []string{infraflow.Path + "/", infraflow.Path + "/users", "/gizclaw/v1/device/status"} {
		if !isProtectedPublicHTTPRoute(path) {
			t.Fatalf("feed exception widened to %q", path)
		}
	}
}

func TestInfraFlowLegacyConfigurationDoesNotRequireReadToken(t *testing.T) {
	for _, token := range []string{"", "old-value", "${UNSET_INFRA_FLOW_TEST_TOKEN}", "gizclaw_mk_" + strings.Repeat("x", 32), strings.Repeat("x", 513)} {
		parsed, err := parseConfigData([]byte("infra_flow:\n  store: flow-windows\n  read_token: '" + token + "'\n"))
		if err != nil {
			t.Fatal(err)
		}
		cfg := validLayeredConfig(t.TempDir())
		cfg.Stores["flow-windows"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "flow-windows"}
		cfg.InfraFlow = parsed.InfraFlow
		server, err := New(cfg)
		if err != nil {
			t.Fatalf("legacy read_token blocked startup: %v", err)
		}
		response := httptest.NewRecorder()
		server.InfraFlowHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, infraflow.Path+"?environment=prod", nil))
		if response.Code != 200 {
			t.Fatalf("legacy configuration blocked anonymous read: %d %s", response.Code, response.Body.String())
		}
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInfraFlowStoreMustBeExplicitAndKeyValue(t *testing.T) {
	for _, store := range []string{"missing-store", "workspaces", " flow-windows "} {
		cfg := validLayeredConfig(t.TempDir())
		cfg.InfraFlow = infraflow.Config{Store: store}
		if server, err := New(cfg); err == nil {
			_ = server.Close()
			t.Fatalf("accepted invalid KV reference %q", store)
		}
	}
	cfg := validLayeredConfig(t.TempDir())
	cfg.InfraFlow.ReadToken = "ignored-token-only"
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	response := httptest.NewRecorder()
	server.InfraFlowHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, infraflow.Path+"?environment=prod", nil))
	if response.Code != 503 || !strings.Contains(response.Body.String(), "FLOW_FEED_DISABLED") {
		t.Fatalf("missing Store enabled implicit feed: %d %s", response.Code, response.Body.String())
	}
}
