package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
)

func TestInfraFlowConfigurationAndKVBinding(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	parsed, err := parseConfigData([]byte("infra_flow:\n  store: flow-windows\n  read_token: " + token + "\n"))
	if err != nil || parsed.InfraFlow.Store != "flow-windows" || parsed.InfraFlow.ReadToken != token {
		t.Fatalf("infra_flow config: %+v %v", parsed.InfraFlow, err)
	}
	t.Setenv("TEST_FLOW_READ_TOKEN", token)
	cfg := validLayeredConfig(t.TempDir())
	cfg.Stores["flow-windows"] = stores.Config{Kind: stores.KindKeyValue, Storage: "memory", Prefix: "flow-windows"}
	cfg.InfraFlow = infraflow.Config{Store: "flow-windows", ReadToken: "${TEST_FLOW_READ_TOKEN}"}
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if server.InfraFlowHandler == nil {
		t.Fatal("KV feed handler not wired into Server")
	}
	request := httptest.NewRequest(http.MethodGet, infraflow.Path+"?environment=prod", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.InfraFlowHandler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"frames":[]`) {
		t.Fatalf("empty configured feed: %d %s", response.Code, response.Body.String())
	}
	if isProtectedPublicHTTPRoute(infraflow.Path) {
		t.Fatal("independently authenticated feed blocked at AP ingress")
	}
	for _, path := range []string{infraflow.Path + "/", infraflow.Path + "/users", "/gizclaw/v1/device/status"} {
		if !isProtectedPublicHTTPRoute(path) {
			t.Fatalf("feed exception widened to %q", path)
		}
	}
}
