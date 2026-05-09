package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/depotstore"
)

func TestPublicHTTPLoginRegisterAndGearAPI(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	deviceKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(device) error = %v", err)
	}
	server := &Server{
		KeyPair:            serverKey,
		GearStore:          mustBadgerInMemory(t, nil),
		DepotStore:         depotstore.Dir(t.TempDir()),
		BuildCommit:        "test-build",
		ServerPublicKey:    serverKey.Public,
		DepotMetadataStore: mustBadgerInMemory(t, nil),
	}
	handler, err := server.PublicHTTPHandler()
	if err != nil {
		t.Fatalf("PublicHTTPHandler error = %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	infoResp, err := http.Get(ts.URL + "/api/public/server-info")
	if err != nil {
		t.Fatalf("GET server-info error = %v", err)
	}
	if infoResp.StatusCode != http.StatusOK {
		t.Fatalf("GET server-info status = %d", infoResp.StatusCode)
	}
	_ = infoResp.Body.Close()

	downloadURL := ts.URL + "/api/gear/download/firmware/fw.bin"
	downloadResp, err := http.Get(downloadURL)
	if err != nil {
		t.Fatalf("GET unauth download error = %v", err)
	}
	if downloadResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth download status = %d", downloadResp.StatusCode)
	}
	_ = downloadResp.Body.Close()

	session := publicHTTPTestLogin(t, ts.URL, serverKey.Public, deviceKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("NewRequest download error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET download error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET download status = %d", resp.StatusCode)
	}

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		t.Fatalf("NewRequest download mismatch error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set(publicKeyHeader, serverKey.Public.String())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET download mismatch error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET download mismatch status = %d", resp.StatusCode)
	}
}

func publicHTTPTestLogin(t *testing.T, baseURL string, serverPublicKey giznet.PublicKey, deviceKey *giznet.KeyPair) publiclogin.LoginResponse {
	t.Helper()
	assertion, err := publiclogin.NewLoginAssertion(deviceKey, serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginAssertion error = %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/api/public/login", nil)
	if err != nil {
		t.Fatalf("NewRequest login error = %v", err)
	}
	req.Header.Set(publicKeyHeader, deviceKey.Public.String())
	req.Header.Set("Authorization", "Bearer "+assertion)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST login error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST login status = %d", resp.StatusCode)
	}
	var result publiclogin.LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return result
}
