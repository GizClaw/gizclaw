package gizedge

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/infraflow"
	"github.com/GizClaw/gizclaw-go/pkgs/monitor"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestOrderedUpstreamTransportRetriesSafeRequestOnNextUpstream(t *testing.T) {
	firstErr := errors.New("first upstream failed")
	secondErr := errors.New("second upstream failed")
	first := &failingGiznetConn{dialErr: firstErr, state: giznet.PeerStateEstablished}
	second := &failingGiznetConn{dialErr: secondErr, state: giznet.PeerStateEstablished}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{conn: first, connEpoch: 1},
		{conn: second, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw/server-info", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("RoundTrip error = %v, want both upstream errors", err)
	}
}

func TestOrderedTransportInfraFeedDoesNotResolveDeviceOwnership(t *testing.T) {
	conn := &routeRPCGiznetConn{failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished}, httpServerID: "feed"}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{{conn: conn, connEpoch: 1}}}
	request, err := http.NewRequest(http.MethodGet, "http://gizclaw"+infraflow.Path+"?environment=prod", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.Header.Get("X-Test-Server") != "feed" || conn.calls.Load() != 0 {
		t.Fatal("infra feed performed per-device route resolution")
	}
	if !isAPIKeyRoutedPath(infraflow.Path + "/other") {
		t.Fatal("feed routing exception widened")
	}
}

func TestAnonymousInfraFeedThroughEdgeIngressAndUpstream(t *testing.T) {
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	feed, err := infraflow.New(store)
	if err != nil {
		t.Fatal(err)
	}
	conn := &routeRPCGiznetConn{
		failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished},
		httpHandler:       infraflow.Handler(infraflow.Config{Store: "flows"}, feed, nil),
	}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{{conn: conn, connEpoch: 1}}}
	proxy := edgeIngressHandler(newPeerHTTPProxy("edge.example:9821", transport), nil)
	server := httptest.NewServer(monitor.Handler(monitor.Config{Token: "gizclaw_mk_" + strings.Repeat("m", 32)}, monitor.Node{Role: "edge"}, proxy))
	t.Cleanup(server.Close)
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", infraflow.Path + "?environment=prod", 200},
		{"GET", infraflow.Path + "?environment=trial", 200},
		{"GET", infraflow.Path + "?environment=unknown", 400},
		{"GET", infraflow.Path + "?environment=prod&cursor=5001", 400},
		{"OPTIONS", infraflow.Path, 204},
		{"POST", infraflow.Path, 405},
		{"HEAD", infraflow.Path, 405},
		{"DELETE", infraflow.Path, 405},
		{"GET", infraflow.Path + "/", 401},
		{"GET", infraflow.Path + "/other", 401},
		{"GET", "/gizclaw/v1/device/status", 401},
		{"GET", "/gizclaw/v1/api-keys/self", 401},
		{"GET", "/monitor/api/node", 401},
	} {
		request, err := http.NewRequestWithContext(t.Context(), test.method, server.URL+test.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "https://globe.example")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status {
			t.Fatalf("%s %s: %d %s err=%v", test.method, test.path, response.StatusCode, body, err)
		}
		if strings.Split(test.path, "?")[0] == infraflow.Path {
			if response.Header.Get("Access-Control-Allow-Methods") != "GET,OPTIONS" || response.Header.Get("Access-Control-Allow-Origin") != "https://globe.example" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Access-Control-Allow-Credentials") != "" {
				t.Fatalf("Edge replaced read-only CORS: %v", response.Header)
			}
		}
	}
	if conn.calls.Load() != 0 || conn.httpCalls.Load() != 4 {
		t.Fatalf("feed resolved owners or forwarded preflight/write: RPC=%d HTTP=%d", conn.calls.Load(), conn.httpCalls.Load())
	}
	// Caller-supplied invalid credentials must not invoke API-key or debug-owner resolution.
	for _, authorization := range []string{"Bearer gizclaw_sk_v1_invalid", "Bearer gizclaw_pk_invalid", "Bearer gizclaw_mk_old", "invalid"} {
		request := httptest.NewRequest(http.MethodGet, infraflow.Path+"?environment=prod", nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		proxy.ServeHTTP(response, request)
		if response.Code != 200 || conn.calls.Load() != 0 {
			t.Fatalf("public feed consulted credentials: %d RPC=%d", response.Code, conn.calls.Load())
		}
	}
}

func TestInfraFeedEdgeReadPolicyWhenUpstreamIsUnavailable(t *testing.T) {
	transport := &orderedUpstreamTransport{}
	server := httptest.NewServer(edgeIngressHandler(newPeerHTTPProxy("edge.example:9821", transport), nil))
	t.Cleanup(server.Close)
	for _, test := range []struct {
		method string
		status int
	}{{"GET", 502}, {"OPTIONS", 204}, {"POST", 405}} {
		request, err := http.NewRequestWithContext(t.Context(), test.method, server.URL+infraflow.Path+"?environment=prod", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != test.status || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Access-Control-Allow-Methods") != "GET,OPTIONS" {
			t.Fatalf("unavailable upstream %s: %d %v", test.method, response.StatusCode, response.Header)
		}
	}
}

func TestOrderedUpstreamTransportDoesNotReplayUnsafeRequest(t *testing.T) {
	firstErr := errors.New("first upstream failed")
	secondErr := errors.New("second upstream failed")
	first := &failingGiznetConn{dialErr: firstErr, state: giznet.PeerStateEstablished}
	second := &failingGiznetConn{dialErr: secondErr, state: giznet.PeerStateEstablished}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{
		{conn: first, connEpoch: 1},
		{conn: second, connEpoch: 1},
	}}
	request, err := http.NewRequest(http.MethodPost, "http://gizclaw/login", nil)
	if err != nil {
		t.Fatalf("http.NewRequest error = %v", err)
	}

	_, err = transport.RoundTrip(request)
	if !errors.Is(err, firstErr) {
		t.Fatalf("RoundTrip error = %v, want first upstream error", err)
	}
	if errors.Is(err, secondErr) {
		t.Fatalf("RoundTrip error = %v, unsafe request reached second upstream", err)
	}
}

func TestOrderedTransportAnonymousDirectory(t *testing.T) {
	conn := &routeRPCGiznetConn{failingGiznetConn: &failingGiznetConn{state: giznet.PeerStateEstablished}, httpServerID: "directory"}
	transport := &orderedUpstreamTransport{entries: []*upstreamTransport{{conn: conn, connEpoch: 1}}}
	for _, path := range []string{"/gizclaw/v1/peers/@findBySn/test", "/gizclaw/v1/peers/@findByImei/123/456"} {
		request, err := http.NewRequest(http.MethodGet, "http://gizclaw"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := transport.RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.Header.Get("X-Test-Server") != "directory" {
			t.Fatal("wrong directory")
		}
	}
	if conn.calls.Load() != 0 {
		t.Fatal("directory required authenticated route resolution")
	}
}
