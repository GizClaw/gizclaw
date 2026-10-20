package gizclaw

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/monitor"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerAndTrustedEdgeHTTPInfraFeedArePublicAndIsolated(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	feed, err := infraflow.New(store)
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().Truncate(infraflow.Window).UnixMilli()
	if err := feed.Append(t.Context(), infraapi.Frame{
		Id: fmt.Sprint(end), Environment: infraapi.Prod, WindowStart: end - 5000, WindowEnd: end,
		Flows: []infraapi.Flow{{Source: "edge", Target: "server", Kind: infraapi.Request, Count: 12}},
	}); err != nil {
		t.Fatal(err)
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := completeTestServer(t, &Server{
		LocalStatic: *key, InfraFlowHandler: infraflow.Handler(infraflow.Config{Store: "flows"}, feed, nil),
	})
	if err := server.init(); err != nil {
		t.Fatal(err)
	}
	for _, surface := range []struct {
		name    string
		handler http.Handler
	}{
		{"server", server},
		{"trusted-edge", server.peerService.edgeHTTPHandlerForPeer(server.apiKeys, "trusted-edge")},
	} {
		t.Run(surface.name, func(t *testing.T) {
			// Match the process composition: Monitor remains independently protected.
			handler := monitor.Handler(monitor.Config{Token: "gizclaw_mk_" + strings.Repeat("m", 32)}, monitor.Node{Role: "server"}, surface.handler)
			httpServer := httptest.NewServer(handler)
			t.Cleanup(httpServer.Close)
			for _, test := range []struct {
				method, path string
				status       int
				body         string
			}{
				{"GET", infraflow.Path + "?environment=prod", 200, `"source":"edge"`},
				{"OPTIONS", infraflow.Path, 204, ""},
				{"GET", infraflow.Path + "?environment=invalid", 400, "INVALID_FLOW_QUERY"},
				{"POST", infraflow.Path, 405, ""},
				{"GET", infraflow.Path + "/", 401, "INVALID_API_KEY"},
				{"GET", infraflow.Path + "/other", 401, "INVALID_API_KEY"},
				{"GET", "/gizclaw/v1/device/status", 401, "INVALID_API_KEY"},
				{"GET", "/gizclaw/v1/api-keys/self", 401, "INVALID_API_KEY"},
				{"GET", "/openai/v1/models", 401, "INVALID_API_KEY"},
				{"GET", "/monitor/api/node", 401, "INVALID_MONITOR_TOKEN"},
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
				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != test.status || !strings.Contains(string(body), test.body) {
					t.Fatalf("%s %s: %d %s err=%v", test.method, test.path, response.StatusCode, body, err)
				}
				if strings.Split(test.path, "?")[0] == infraflow.Path &&
					(response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Access-Control-Allow-Origin") != "https://globe.example" || response.Header.Get("Access-Control-Allow-Methods") != "GET,OPTIONS") {
					t.Fatalf("public feed headers: %v", response.Header)
				}
			}
		})
	}
}

func TestUnconfiguredInfraFeedDoesNotFallThroughToDeviceAuthentication(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	server := completeTestServer(t, &Server{LocalStatic: *key})
	if err := server.init(); err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.Handler{server, server.peerService.edgeHTTPHandlerForPeer(server.apiKeys, "trusted-edge")} {
		for _, test := range []struct {
			method string
			status int
		}{{"GET", 503}, {"OPTIONS", 204}, {"POST", 405}} {
			request := httptest.NewRequest(test.method, infraflow.Path+"?environment=prod", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("WWW-Authenticate") != "" {
				t.Fatalf("unconfigured %s: %d %s", test.method, response.Code, response.Body.String())
			}
			if test.method == "GET" && !strings.Contains(response.Body.String(), "FLOW_FEED_DISABLED") {
				t.Fatalf("unconfigured feed fell into device authentication: %s", response.Body.String())
			}
		}
	}
}
