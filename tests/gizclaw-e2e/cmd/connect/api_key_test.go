//go:build gizclaw_e2e

package connect_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAPIKeyManagementUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-api-key")
	h.CreateContext("api-key-device").MustSucceed(t)
	client := h.ConnectClientFromContext("api-key-device")
	defer func() { _ = client.Close() }()

	manager, err := client.CreateAPIKey(t.Context(), "manager", rpcapi.APIKeyCreateRequest{DisplayName: "manager", ManageAPIKeys: true})
	if err != nil {
		t.Fatalf("create management API key: %v", err)
	}
	created := apiKeyRequest(t, h, manager.APIKey, http.MethodPost, "/gizclaw/v1/api-keys", []byte(`{"display_name":"phone","manage_api_keys":false}`))
	if created.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(created.Body)
		_ = created.Body.Close()
		t.Fatalf("create ordinary API key status = %d body=%s", created.StatusCode, body)
	}
	var ordinary peerhttp.APIKeyCreateResult
	if err := json.NewDecoder(created.Body).Decode(&ordinary); err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	listed := apiKeyRequest(t, h, manager.APIKey, http.MethodGet, "/gizclaw/v1/api-keys", nil)
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list API keys status = %d", listed.StatusCode)
	}
	_ = listed.Body.Close()
	forbidden := apiKeyRequest(t, h, ordinary.ApiKey, http.MethodGet, "/gizclaw/v1/api-keys", nil)
	if forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("ordinary key list status = %d", forbidden.StatusCode)
	}
	_ = forbidden.Body.Close()
	revoked := apiKeyRequest(t, h, ordinary.ApiKey, http.MethodDelete, "/gizclaw/v1/api-keys/self", nil)
	if revoked.StatusCode != http.StatusNoContent {
		t.Fatalf("self revoke status = %d", revoked.StatusCode)
	}
	_ = revoked.Body.Close()
}

func apiKeyRequest(t *testing.T, h *clitest.Harness, secret, method, path string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, h.PublicHTTPURL()+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+secret)
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
