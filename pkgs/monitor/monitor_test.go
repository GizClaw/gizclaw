package monitor

import (
	"context"
	"encoding/json"
	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	if response.StatusCode != 405 || response.Header.Get("Allow") != "GET" {
		t.Fatal("method contract mismatch")
	}
}
