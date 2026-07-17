//go:build gizclaw_e2e

package connect_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPublicHTTPAuthUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "303-public-http-auth")

	h.CreateContext("device-http").MustSucceed(t)
	client := h.ConnectClientFromContext("device-http")
	defer func() { _ = client.Close() }()
	serverInfoResp, err := http.Get(h.PublicHTTPURL() + "/server-info")
	if err != nil {
		t.Fatalf("GET server-info: %v", err)
	}
	if serverInfoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", serverInfoResp.StatusCode)
	}
	var serverInfo apitypes.ServerInfo
	if err := json.NewDecoder(serverInfoResp.Body).Decode(&serverInfo); err != nil {
		_ = serverInfoResp.Body.Close()
		t.Fatalf("decode server-info: %v", err)
	}
	if err := serverInfoResp.Body.Close(); err != nil {
		t.Fatalf("close server-info body: %v", err)
	}
	if !serverInfo.Ice.Udp || !serverInfo.Ice.Tcp {
		t.Fatalf("server-info ice = %+v, want udp=true tcp=true", serverInfo.Ice)
	}

	_ = h.PublicHTTPLogin("device-http")
}
