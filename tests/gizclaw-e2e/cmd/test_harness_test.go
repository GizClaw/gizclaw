//go:build gizclaw_e2e

package clitest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

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

func TestHarnessSplitsClientAndAdminEndpoints(t *testing.T) {
	h := &Harness{
		ServerAddr: "127.0.0.1:19820",
		EdgeAddr:   "127.0.0.1:19821",
	}
	if got := h.clientEndpoint(); got != h.EdgeAddr {
		t.Fatalf("clientEndpoint = %q, want edge endpoint %q", got, h.EdgeAddr)
	}
	if got := h.adminEndpoint(); got != h.ServerAddr {
		t.Fatalf("adminEndpoint = %q, want server endpoint %q", got, h.ServerAddr)
	}
}
