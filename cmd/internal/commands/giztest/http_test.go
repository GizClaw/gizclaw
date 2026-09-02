package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestInvokeHTTPSendsBearerBodyAndDecodesJSON(t *testing.T) {
	var got struct {
		method, path, auth, contentType string
		body                            map[string]any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.auth, got.contentType = r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":{"volume":35,"muted":true}}`))
	}))
	defer server.Close()
	vars := &variables{values: map[string]value{"api_key": {data: "gizclaw_sk_v1_test"}, "level": {data: float64(35)}}}
	step := Step{ID: "volume", Client: "peer", HTTP: &HTTPOperation{
		Method: http.MethodPut, Path: "/gizclaw/v1/device/volume?x=1",
		Headers: map[string]string{"Authorization": "Bearer ${api_key}"},
		Body:    map[string]any{"level": "${level}", "muted": true},
		Status:  http.StatusOK,
	}}
	result, err := invokeHTTP(context.Background(), strings.TrimPrefix(server.URL, "http://"), step, vars)
	if err != nil {
		t.Fatal(err)
	}
	if got.method != http.MethodPut || got.path != "/gizclaw/v1/device/volume?x=1" || got.auth != "Bearer gizclaw_sk_v1_test" || got.contentType != "application/json" {
		t.Fatalf("request = %+v", got)
	}
	if got.body["level"] != float64(35) || got.body["muted"] != true {
		t.Fatalf("body = %v", got.body)
	}
	body, ok := result.body.(map[string]any)
	if !ok || body["status"].(map[string]any)["volume"] != float64(35) {
		t.Fatalf("decoded body = %#v", result.body)
	}
	if result.evidence["status"] != http.StatusOK {
		t.Fatalf("evidence = %v", result.evidence)
	}
	if value, ok := jsonPointer(result.body, "/status/volume"); !ok || value != float64(35) {
		t.Fatalf("json pointer = %v, %v", value, ok)
	}
}

func TestInvokeHTTPStatusExpectations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"DEVICE_OFFLINE","message":"device is offline"}}`))
	}))
	defer server.Close()
	vars := &variables{values: map[string]value{}}
	endpoint := strings.TrimPrefix(server.URL, "http://")

	result, err := invokeHTTP(context.Background(), endpoint, Step{HTTP: &HTTPOperation{Method: http.MethodGet, Path: "/gizclaw/v1/device/wifi"}}, vars)
	var failure *assertionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("unexpected 4xx without status expectation must be an assertion failure: %v", err)
	}
	if value, ok := jsonPointer(result.body, "/error/code"); !ok || value != "DEVICE_OFFLINE" {
		t.Fatalf("error body kept for evidence: %#v", result.body)
	}
	if _, err := invokeHTTP(context.Background(), endpoint, Step{HTTP: &HTTPOperation{Method: http.MethodGet, Path: "/gizclaw/v1/device/wifi", Status: http.StatusConflict}}, vars); err != nil {
		t.Fatalf("expected status must pass: %v", err)
	}
	if _, err := invokeHTTP(context.Background(), endpoint, Step{HTTP: &HTTPOperation{Method: http.MethodGet, Path: "/gizclaw/v1/device/wifi", Status: http.StatusOK}}, vars); !errors.As(err, &failure) {
		t.Fatalf("mismatched status must fail: %v", err)
	}
	if _, err := invokeHTTP(context.Background(), "", Step{HTTP: &HTTPOperation{Method: http.MethodGet, Path: "/x"}}, vars); err == nil {
		t.Fatal("empty endpoint accepted")
	}
	if _, err := invokeHTTP(context.Background(), endpoint, Step{HTTP: &HTTPOperation{Method: http.MethodGet, Path: "relative"}}, vars); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestHTTPStepDocumentValidation(t *testing.T) {
	doc := `# User Story:
# As a GizClaw API key holder,
# I want to read my device status over Public HTTP,
# So that the device contract is verified end to end.
version: gizclaw.test/v1alpha1
name: server.device.status.get
clients:
  peer:
    identity: ephemeral
    connection: webrtc
    access_point: ${endpoint}
variables:
  endpoint: {direction: input, type: string, env: GIZCLAW_TEST_ENDPOINT}
  api_key: {direction: output, type: string, secret: true}
steps:
  - id: create_key
    client: peer
    rpc: {method: server.api_key.create, request: {display_name: phone}}
    capture: {api_key: /api_key}
  - id: read_status
    client: peer
    http:
      method: GET
      path: /gizclaw/v1/device/status
      headers: {Authorization: "Bearer ${api_key}"}
      status: 200
    expect:
      /volume: {present: true}
  - id: volume_provider
    client: peer
    client_rpc:
      method: client.device.volume.set
      response: {battery_percent: 88}
      expect_calls: 1
`
	load := func(text string) (*Document, error) {
		path := filepath.Join(t.TempDir(), "device.giztest.yaml")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return loadDocument(path)
	}
	parsed, err := load(doc)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Steps[1].operation() != "http" || parsed.Steps[1].HTTP.Status != 200 {
		t.Fatalf("http step = %+v", parsed.Steps[1])
	}
	for name, mutated := range map[string]string{
		"relative path":      strings.Replace(doc, "path: /gizclaw/v1/device/status", "path: gizclaw/v1/device/status", 1),
		"unknown method":     strings.Replace(doc, "method: GET", "method: PATCH", 1),
		"unknown client rpc": strings.Replace(doc, "method: client.device.volume.set", "method: client.device.unknown", 1),
	} {
		if _, err := load(mutated); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestInstallDeviceControlScriptsProviders(t *testing.T) {
	var handlers gizcli.DeviceControlHandlers
	if err := installDeviceControl(&handlers, "client.device.volume.set", map[string]any{"battery_percent": 88}); err != nil {
		t.Fatal(err)
	}
	if err := installDeviceControl(&handlers, "client.wifi.saved.list", map[string]any{"networks": []any{map[string]any{"ssid": "home"}}}); err != nil {
		t.Fatal(err)
	}
	if err := installDeviceControl(&handlers, "client.device.sound.play", map[string]any{"error_code": -32602}); err != nil {
		t.Fatal(err)
	}
	if err := installDeviceControl(&handlers, "client.wifi.saved.forget", map[string]any{"error_code": 404, "error_message": "unknown"}); err != nil {
		t.Fatal(err)
	}
	if err := installDeviceControl(&handlers, "client.device.status.get", map[string]any{"battery_percent": "eighty"}); err == nil {
		t.Fatal("malformed scripted status accepted")
	}
	status, err := handlers.SetVolume(context.Background(), 35, true)
	if err != nil || *status.Volume != 35 || !*status.Muted || *status.BatteryPercent != 88 {
		t.Fatalf("scripted volume = %+v, %v", status, err)
	}
	networks, err := handlers.SavedWifi(context.Background())
	if err != nil || len(networks) != 1 || networks[0].Ssid != "home" {
		t.Fatalf("scripted saved list = %+v, %v", networks, err)
	}
	var rpcErr rpcapi.Error
	if err := handlers.PlaySound(context.Background(), "chime", nil); !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("scripted sound error = %v", err)
	}
	if err := handlers.ForgetWifi(context.Background(), "x"); !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.RPCErrorCodeNotFound || rpcErr.Message != "unknown" {
		t.Fatalf("scripted forget error = %v", err)
	}
	if handlers.Reboot != nil || handlers.Status != nil {
		t.Fatal("unscripted methods must stay unsupported")
	}

	client := &gizcli.Client{}
	counts := map[string]*inboundCounter{}
	steps := []Step{{ID: "volume", Client: "peer", ClientRPC: &ClientRPCOperation{Method: "client.device.volume.set"}}}
	if err := configureClientRPC(client, "peer", steps, &variables{values: map[string]value{}}, counts); err != nil {
		t.Fatal(err)
	}
	if counts["peer:client.device.volume.set"] == nil {
		t.Fatalf("counts = %v", counts)
	}
}
