//go:build gizclaw_e2e

package connect_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPublicHTTPAuthUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "303-public-http-auth")

	h.CreateContext("device-http").MustSucceed(t)
	h.RegisterContext("device-http", "--sn", "connect-public-http-device-sn").MustSucceed(t)
	client := h.ConnectClientFromContext("device-http")
	defer func() { _ = client.Close() }()
	registrationToken := os.Getenv("GIZCLAW_TEST_REGISTRATION_TOKEN")
	if registrationToken == "" {
		t.Fatal("GIZCLAW_TEST_REGISTRATION_TOKEN is required")
	}
	if _, err := client.Register(t.Context(), "connect.public-http.register", registrationToken); err != nil {
		t.Fatalf("register public HTTP device: %v", err)
	}
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
	if !serverInfo.Ice.Udp || serverInfo.Ice.Tcp {
		t.Fatalf("server-info ice = %+v, want Edge udp=true tcp=false", serverInfo.Ice)
	}

	created, err := client.CreateAPIKey(t.Context(), "e2e-api-key", rpcapi.APIKeyCreateRequest{DisplayName: "e2e", ManageAPIKeys: true})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.PublicHTTPURL()+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+created.APIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET API key self status = %d", response.StatusCode)
	}
}
