//go:build gizclaw_e2e

package connect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestSideControlPublicHTTPUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "304-side-control-public-http")
	h.CreateContext("side-target").MustSucceed(t)
	targetClient := h.ConnectClientFromContext("side-target")
	defer func() { _ = targetClient.Close() }()
	h.CreateContext("side-controller").MustSucceed(t)

	targetLogin := h.PublicHTTPLogin("side-target")
	deviceToken := createSideControlDeviceToken(t, h, "side-target", targetLogin.AccessToken)
	sideLogin := exchangeSideControlDeviceToken(t, h, "side-controller", deviceToken.Token)

	response := sideControlRequest(t, h, "side-controller", sideLogin.AccessToken, http.MethodGet, "/side-control/info", nil)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("GET side-control info status = %d body=%s", response.StatusCode, body)
	}
	_ = response.Body.Close()

	response = sideControlRequest(t, h, "side-controller", sideLogin.AccessToken, http.MethodPost, "/side-control/contacts", []byte(`{"display_name":"Side Contact"}`))
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("POST side-control contact status = %d body=%s", response.StatusCode, body)
	}
	_ = response.Body.Close()

	response = primarySideControlRequest(t, h, "side-target", targetLogin.AccessToken, http.MethodGet, "/me/side-control/sessions", nil)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("GET side-control sessions status = %d body=%s", response.StatusCode, body)
	}
	var sessions peerhttp.SideControlSessionList
	if err := json.NewDecoder(response.Body).Decode(&sessions); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode sessions: %v", err)
	}
	_ = response.Body.Close()
	if len(sessions.Items) != 1 || sessions.Items[0].ControllerPublicKey != h.ContextPublicKey("side-controller") {
		t.Fatalf("sessions = %+v", sessions.Items)
	}

	response = primarySideControlRequest(t, h, "side-target", targetLogin.AccessToken, http.MethodDelete, "/me/side-control/sessions/"+sessions.Items[0].Id, nil)
	if response.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("DELETE side-control session status = %d body=%s", response.StatusCode, body)
	}
	_ = response.Body.Close()

	response = sideControlRequest(t, h, "side-controller", sideLogin.AccessToken, http.MethodGet, "/side-control/info", nil)
	if response.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("revoked side session status = %d body=%s", response.StatusCode, body)
	}
	_ = response.Body.Close()
}

func createSideControlDeviceToken(t *testing.T, h *clitest.Harness, contextName, accessToken string) peerhttp.SideControlDeviceToken {
	t.Helper()
	response := primarySideControlRequest(t, h, contextName, accessToken, http.MethodPost, "/me/side-control/device-tokens", nil)
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("POST device token status = %d body=%s", response.StatusCode, body)
	}
	defer response.Body.Close()
	var token peerhttp.SideControlDeviceToken
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		t.Fatalf("decode device token: %v", err)
	}
	return token
}

func exchangeSideControlDeviceToken(t *testing.T, h *clitest.Harness, contextName, deviceToken string) peerhttp.LoginResult {
	t.Helper()
	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(h.ServerPublicKey)); err != nil {
		t.Fatalf("parse server public key: %v", err)
	}
	assertion, err := publiclogin.NewLoginAssertion(h.ContextKeyPair(contextName), serverPublicKey, time.Minute)
	if err != nil {
		t.Fatalf("create controller assertion: %v", err)
	}
	body, err := json.Marshal(peerhttp.LoginRequest{GrantType: peerhttp.SideControl, DeviceToken: deviceToken})
	if err != nil {
		t.Fatalf("encode side login: %v", err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, h.PublicHTTPURL()+"/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create side login request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(publiclogin.PublicKeyHeader, h.ContextPublicKey(contextName))
	request.Header.Set("Authorization", "Bearer "+assertion)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("side login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("side login status = %d body=%s", response.StatusCode, body)
	}
	var result peerhttp.LoginResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode side login: %v", err)
	}
	return result
}

func primarySideControlRequest(t *testing.T, h *clitest.Harness, contextName, accessToken, method, path string, body []byte) *http.Response {
	t.Helper()
	return authenticatedSideControlRequest(t, h, contextName, accessToken, method, path, body)
}

func sideControlRequest(t *testing.T, h *clitest.Harness, contextName, accessToken, method, path string, body []byte) *http.Response {
	t.Helper()
	return authenticatedSideControlRequest(t, h, contextName, accessToken, method, path, body)
}

func authenticatedSideControlRequest(t *testing.T, h *clitest.Harness, contextName, accessToken, method, path string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, h.PublicHTTPURL()+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create %s %s request: %v", method, path, err)
	}
	request.Header.Set(publiclogin.PublicKeyHeader, h.ContextPublicKey(contextName))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}
