package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	monitorapi "github.com/GizClaw/gizclaw-go/pkgs/monitor/api"
)

func TestNodeAuthorizationAndIsolation(t *testing.T) {
	token := "gizclaw_mk_" + strings.Repeat("x", 32)
	handler := Handler(Config{Token: token}, Node{Role: "server", PublicKey: "local-key", Version: "0.9.1", Commit: "abc123"}, http.NotFoundHandler())
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
			if snapshot.Version != "0.9.1" || snapshot.BuildCommit != "abc123" {
				t.Fatalf("build = %q %q", snapshot.Version, snapshot.BuildCommit)
			}
			if !strings.Contains(out.Body.String(), `"inbound_service_channels":0`) {
				t.Fatal("missing inbound service channel count")
			}
			if strings.Contains(out.Body.String(), `"logs"`) {
				t.Fatal("node snapshot must not contain cached logs")
			}
			if strings.Contains(out.Body.String(), token) {
				t.Fatal("token leaked")
			}
		}
	}
	disabled := httptest.NewRecorder()
	Handler(Config{}, Node{Role: "edge", PublicKey: "edge-key"}, http.NotFoundHandler()).ServeHTTP(disabled, httptest.NewRequest("GET", "/monitor/api/node", nil))
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
	server := httptest.NewServer(Handler(Config{Token: token}, Node{Role: "edge", PublicKey: "local-node"}, http.NotFoundHandler()))
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
	// Unversioned builds report "dev" for both build fields.
	if snapshot := authorized.JSON200; snapshot == nil || snapshot.PublicKey != "local-node" || snapshot.Version != "dev" || snapshot.BuildCommit != "dev" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	disabledServer := httptest.NewServer(Handler(Config{}, Node{Role: "edge", PublicKey: "local-node"}, http.NotFoundHandler()))
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
	handler := Handler(Config{Token: token}, Node{Role: "server", PublicKey: "local-key"}, http.NotFoundHandler())
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

func TestEmbeddedConsole(t *testing.T) {
	// An unrelated working directory proves serving does not read web/console/dist.
	t.Chdir(t.TempDir())
	handler := Handler(Config{}, Node{Role: "edge", PublicKey: "local-key"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	for path, want := range map[string]int{
		"/monitor":                   308,
		"/monitor/":                  200,
		"/monitor/api/node":          503,
		"/monitor/api/missing":       404,
		"/monitor/assets/missing.js": 404,
		"/unrelated":                 418,
	} {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != want {
			t.Fatalf("%s: status %d, want %d", path, out.Code, want)
		}
		if path == "/monitor" && out.Header().Get("Location") != "/monitor/" {
			t.Fatal("missing canonical redirect")
		}
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest("GET", "/monitor/", nil))
	if !strings.Contains(page.Body.String(), `<div id="root">`) {
		t.Fatal("console application missing")
	}
	references := regexp.MustCompile(`(?:src|href)="\./(assets/[^\"]+)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(references) < 2 {
		t.Fatal("missing built JavaScript and CSS references")
	}
	for _, ref := range references {
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest("GET", "/monitor/"+ref[1], nil))
		if out.Code != 200 || out.Body.Len() == 0 {
			t.Fatalf("asset %s: status %d", ref[1], out.Code)
		}
		if strings.Contains(out.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("asset %s returned HTML", ref[1])
		}
	}
}
