package monitor

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
)

func TestNodeAuthorizationAndIsolation(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	handler := Handler(Config{Token: token}, "server", "local-key", http.NotFoundHandler())
	for _, tc := range []struct {
		auth string
		want int
	}{{"", 401}, {"Bearer gizclaw_pk_other", 401}, {"Bearer " + token, 200}} {
		req := httptest.NewRequest("GET", "/monitor/api/node", nil)
		req.Header.Set("Authorization", tc.auth)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != tc.want || out.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d headers=%v", out.Code, out.Header())
		}
		if out.Code == 200 {
			var snapshot monitorapi.NodeSnapshot
			if err := json.Unmarshal(out.Body.Bytes(), &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.PublicKey != "local-key" || snapshot.Role != "server" {
				t.Fatal("wrong node")
			}
			if !strings.Contains(out.Body.String(), `"inbound_service_channels":0`) {
				t.Fatal("missing inbound service channel count")
			}
			if strings.Contains(out.Body.String(), token) {
				t.Fatal("token leaked")
			}
		}
	}
	disabled := httptest.NewRecorder()
	Handler(Config{}, "edge", "edge-key", http.NotFoundHandler()).ServeHTTP(disabled, httptest.NewRequest("GET", "/monitor/api/node", nil))
	if disabled.Code != 503 {
		t.Fatal(disabled.Code)
	}
	// Public assets must not grant access to a similarly named private route.
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest("GET", "/monitor/api/node/secret", nil))
	if unknown.Code != 404 {
		t.Fatal(unknown.Code)
	}
}
func TestMonitorTokenConfig(t *testing.T) {
	for _, token := range []string{"gizclaw_pk_" + strings.Repeat("x", 32), "gizclaw_mk_" + strings.Repeat("x", 31)} {
		cfg := Config{Token: token}
		if cfg.Validate() == nil {
			t.Fatal("invalid token accepted")
		}
	}
	t.Setenv("MONITOR_TEST_TOKEN", "gizclaw_mk_"+strings.Repeat("x", 32))
	cfg := Config{Token: "${MONITOR_TEST_TOKEN}"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedMonitorClientContract(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("t", 32)
	server := httptest.NewServer(Handler(Config{Token: token}, "edge", "local-node", http.NotFoundHandler()))
	defer server.Close()
	client, err := monitorapi.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized, err := client.GetNodeMonitorWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if unauthorized.JSON401 == nil || unauthorized.JSON401.Error != monitorapi.INVALIDMONITORTOKEN {
		t.Fatalf("unexpected unauthorized response: %+v", unauthorized)
	}
	authorized, err := client.GetNodeMonitorWithResponse(context.Background(), func(_ context.Context, r *http.Request) error {
		r.Header.Set("Authorization", "Bearer "+token)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorized.JSON200 == nil || authorized.JSON200.PublicKey != "local-node" || authorized.JSON200.Logs == nil {
		t.Fatalf("unexpected snapshot: %+v", authorized)
	}
	disabledServer := httptest.NewServer(Handler(Config{}, "edge", "local-node", http.NotFoundHandler()))
	defer disabledServer.Close()
	disabledClient, err := monitorapi.NewClientWithResponses(disabledServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := disabledClient.GetNodeMonitorWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if disabled.JSON503 == nil || disabled.JSON503.Error != monitorapi.MONITORDISABLED {
		t.Fatalf("unexpected disabled response: %+v", disabled)
	}
	response, err := http.Post(server.URL+"/monitor/api/node", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 405 || response.Header.Get("Allow") != "GET,OPTIONS" {
		t.Fatal("method contract mismatch")
	}
}

func TestNodeMonitorCORS(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	handler := Handler(Config{Token: token}, "server", "local-key", http.NotFoundHandler())
	preflight := httptest.NewRecorder()
	options := httptest.NewRequest("OPTIONS", "/monitor/api/node", nil)
	options.Header.Set("Origin", "https://console.example.com")
	handler.ServeHTTP(preflight, options)
	if preflight.Code != 204 {
		t.Fatalf("preflight status %d", preflight.Code)
	}
	if got := preflight.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
	if got := preflight.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Fatalf("Access-Control-Allow-Headers = %q", got)
	}
	if got := preflight.Header().Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("Vary = %q", got)
	}
	snapshot := httptest.NewRecorder()
	get := httptest.NewRequest("GET", "/monitor/api/node", nil)
	get.Header.Set("Origin", "https://console.example.com")
	get.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(snapshot, get)
	if snapshot.Code != 200 || snapshot.Header().Get("Access-Control-Allow-Origin") != "https://console.example.com" {
		t.Fatalf("status=%d headers=%v", snapshot.Code, snapshot.Header())
	}
	unauthorized := httptest.NewRecorder()
	denied := httptest.NewRequest("GET", "/monitor/api/node", nil)
	denied.Header.Set("Origin", "https://console.example.com")
	handler.ServeHTTP(unauthorized, denied)
	if unauthorized.Code != 401 || unauthorized.Header().Get("Access-Control-Allow-Origin") != "https://console.example.com" {
		t.Fatalf("status=%d headers=%v", unauthorized.Code, unauthorized.Header())
	}
}

func TestNodeSnapshotCarriesStructuredLogFields(t *testing.T) {
	logger, cleanup, err := gizlog.NewLogger(gizlog.Config{Level: "info"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
	}()
	logger.LogAttrs(
		gizlog.WithPeerPublicKey(context.Background(), "peer-key"),
		slog.LevelInfo,
		"gizclaw: request completed",
		slog.String("request_id", "req-monitor-1"),
		slog.String("operation", "getPeerRuntime"),
		slog.Int("status", 200),
	)
	server := &nodeServer{role: "server", publicKey: "local-key", started: time.Now()}
	response, err := server.GetNodeMonitor(context.Background(), monitorapi.GetNodeMonitorRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := response.(monitorapi.GetNodeMonitor200JSONResponse)
	if !ok {
		t.Fatalf("unexpected response %T", response)
	}
	for _, entry := range snapshot.Logs {
		if entry.Fields == nil {
			continue
		}
		if (*entry.Fields)["request_id"] == "req-monitor-1" && (*entry.Fields)["status"] == "200" {
			return
		}
	}
	t.Fatal("structured log fields missing from the node snapshot")
}
