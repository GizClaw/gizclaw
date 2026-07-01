//go:build gizclaw_e2e

package connect_test

import (
	"net/http"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestPublicHTTPAuthUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "303-public-http-auth")

	h.CreateContext("device-http").MustSucceed(t)
	serverInfoResp, err := http.Get(h.PublicHTTPURL() + "/server-info")
	if err != nil {
		t.Fatalf("GET server-info: %v", err)
	}
	if serverInfoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", serverInfoResp.StatusCode)
	}
	_ = serverInfoResp.Body.Close()

	_ = h.PublicHTTPLogin("device-http")
}
