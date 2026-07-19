//go:build gizclaw_e2e

package clitest

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

func TestRetryableRefreshReconnectError(t *testing.T) {
	retryable := errors.New("gizclaw: dial: gizwebrtc: wait for packet channel: context deadline exceeded")
	if !isRetryableRefreshReconnectError(retryable) {
		t.Fatalf("isRetryableRefreshReconnectError(%q) = false", retryable)
	}
	for _, err := range []error{
		nil,
		errors.New("gizclaw: dial: context deadline exceeded"),
		errors.New("gizclaw: dial: gizwebrtc: authentication failed"),
	} {
		if isRetryableRefreshReconnectError(err) {
			t.Fatalf("isRetryableRefreshReconnectError(%v) = true", err)
		}
	}
}

func TestFetchE2EServerInfoIncludesICEServers(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"public_key": %q,
			"protocol": "gizclaw-webrtc",
			"signaling_path": "/webrtc/v1/offer",
			"ice_servers": [{"urls":["turn:edge.example.com:3478"],"username":"edge","credential":"secret"}]
		}`, serverKey.Public.String())
	}))
	defer server.Close()

	info, err := fetchE2EServerInfo(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("fetchE2EServerInfo error = %v", err)
	}
	if !info.PublicKey.Equal(serverKey.Public) {
		t.Fatalf("PublicKey = %v, want %v", info.PublicKey, serverKey.Public)
	}
	if info.SignalingURL != server.URL+"/webrtc/v1/offer" {
		t.Fatalf("SignalingURL = %q", info.SignalingURL)
	}
	if len(info.ICEServers) != 1 || len(info.ICEServers[0].URLs) != 1 || info.ICEServers[0].URLs[0] != "turn:edge.example.com:3478" {
		t.Fatalf("ICEServers = %+v", info.ICEServers)
	}
	if info.ICEServers[0].Username != "edge" || info.ICEServers[0].Credential != "secret" {
		t.Fatalf("ICE server credentials = %+v", info.ICEServers[0])
	}
}

func TestFetchE2EServerInfoRetriesTransientFailure(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "stale upstream", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"protocol":"gizclaw-webrtc","public_key":%q}`, serverKey.Public.String())
	}))
	defer server.Close()

	info, err := fetchE2EServerInfo(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("fetchE2EServerInfo error = %v", err)
	}
	if !info.PublicKey.Equal(serverKey.Public) {
		t.Fatalf("PublicKey = %v, want %v", info.PublicKey, serverKey.Public)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestHarnessSplitsClientAndAdminEndpoints(t *testing.T) {
	h := NewPersistentHarnessForRoot(t, "tests/gizclaw-e2e/cmd", "endpoint-split", t.TempDir())
	h.ServerAddr = "127.0.0.1:19820"
	h.EdgeAddr = "127.0.0.1:19821"
	h.ServerPublicKey = "test"

	if got := h.clientEndpoint(); got != h.EdgeAddr {
		t.Fatalf("clientEndpoint = %q, want edge endpoint %q", got, h.EdgeAddr)
	}
	if got := h.adminEndpoint(); got != h.ServerAddr {
		t.Fatalf("adminEndpoint = %q, want server endpoint %q", got, h.ServerAddr)
	}

	client := h.CreateContext("client-a")
	client.MustSucceed(t)
	admin := h.CreateAdminContext("admin-a")
	admin.MustSucceed(t)

	if got := readTestContextEndpoint(t, h, "client-a"); got != h.EdgeAddr {
		t.Fatalf("client endpoint = %q, want edge endpoint %q", got, h.EdgeAddr)
	}
	if got := readTestContextEndpoint(t, h, "admin-a"); got != h.ServerAddr {
		t.Fatalf("admin endpoint = %q, want server endpoint %q", got, h.ServerAddr)
	}
}

func readTestContextEndpoint(t *testing.T, h *Harness, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(h.contextRoot(), name, "config.yaml"))
	if err != nil {
		t.Fatalf("read context %s config: %v", name, err)
	}
	var cfg cliContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse context %s config: %v", name, err)
	}
	return cliContextDialAddr(cfg)
}
