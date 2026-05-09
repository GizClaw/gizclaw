package publichttpauth_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	clitest "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/cmd"
)

func TestPublicHTTPAuthUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "303-public-http-auth")
	h.StartServerFromFixture("server_config.yaml")

	h.CreateContext("device-http").MustSucceed(t)
	serverInfoResp, err := http.Get(h.PublicHTTPURL() + "/api/public/server-info")
	if err != nil {
		t.Fatalf("GET server-info: %v", err)
	}
	if serverInfoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", serverInfoResp.StatusCode)
	}
	_ = serverInfoResp.Body.Close()

	downloadURL := h.PublicHTTPURL() + "/api/gear/download/firmware/fw.bin"
	resp, err := http.Get(downloadURL)
	if err != nil {
		t.Fatalf("GET unauth firmware download: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("unauth firmware download status = %d body=%s", resp.StatusCode, string(body))
	}
	_ = resp.Body.Close()

	session := h.PublicHTTPLogin("device-http")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("create firmware download request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET firmware download: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET firmware download status = %d body=%s", resp.StatusCode, string(body))
	}
}
