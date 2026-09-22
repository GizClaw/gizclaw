//go:build gizclaw_e2e

package multiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// TestDeviceRoutesFollowTheKeyOwnerHome verifies the API-key device extension
// across the multi-Server deployment: the home Server and every Edge serve the
// owner's device routes and forward control commands to the connected device
// (an Edge resolves the key to its owner Server), while the foreign Server
// rejects the key directly without touching its own state store.
func TestDeviceRoutesFollowTheKeyOwnerHome(t *testing.T) {
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeAEndpoint := requiredEnv(t, "GIZCLAW_E2E_EDGE_A")
	edgeBEndpoint := requiredEnv(t, "GIZCLAW_E2E_EDGE_B")
	stateB := requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	peer, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	device := connectAndServe(t, peer, serverA, serverA.PublicKey, "device-routes-home")
	defer device.Close()
	registerSocialPeer(t, ctx, device, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	var finds int
	volume, muted, battery := 35, true, 88
	if err := device.HandleDeviceControl(gizcli.DeviceControlHandlers{
		Status: func(context.Context) (rpcapi.PeerStatus, error) {
			return rpcapi.PeerStatus{Volume: &volume, Muted: &muted, BatteryPercent: &battery}, nil
		},
		Find: func(context.Context, *int64) error { finds++; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	created, err := device.CreateAPIKey(ctx, "device-routes-key", rpcapi.APIKeyCreateRequest{DisplayName: "phone"})
	if err != nil {
		t.Fatalf("create API key on Server A: %v", err)
	}
	apiKey := created.APIKey

	// Public client APIs are Edge-only: the Servers' own listeners refuse
	// them before any key or device lookup.
	for name, base := range map[string]string{
		"server-a": deviceRouteBase(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A")),
		"server-b": deviceRouteBase(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B")),
	} {
		var denied apitypes.ErrorResponse
		deviceRouteJSON(t, base, apiKey, http.MethodGet, "/gizclaw/v1/device/runtime", "", http.StatusForbidden, &denied)
		if denied.Error.Code != "PRIVATE_INGRESS_DENIED" {
			t.Fatalf("direct %s ingress = %+v", name, denied)
		}
	}

	homeBases := map[string]string{
		"edge-a": deviceRouteBase(t, edgeAEndpoint),
		"edge-b": deviceRouteBase(t, edgeBEndpoint),
	}
	for name, base := range homeBases {
		t.Run(name, func(t *testing.T) {
			var runtime apitypes.Runtime
			deviceRouteJSON(t, base, apiKey, http.MethodGet, "/gizclaw/v1/device/runtime", "", http.StatusOK, &runtime)
			if !runtime.Online {
				t.Fatalf("runtime through %s = %+v, want online", name, runtime)
			}
			var control map[string]any
			deviceRouteJSON(t, base, apiKey, http.MethodPost, "/gizclaw/v1/device/tool/v0/invoke", `{"tool":"device.status.get","args":{}}`, http.StatusOK, &control)
			result, ok := control["result"].(map[string]any)
			if !ok || result["volume"] != float64(35) {
				t.Fatalf("live status through %s = %+v", name, control)
			}
			deviceRouteJSON(t, base, apiKey, http.MethodPost, "/gizclaw/v1/device/tool/v0/invoke", `{"tool":"device.find","args":{}}`, http.StatusOK, &control)
			if finds == 0 {
				t.Fatalf("find did not reach device through %s", name)
			}
			var status apitypes.PeerStatus
			deviceRouteJSON(t, base, apiKey, http.MethodGet, "/gizclaw/v1/device/status", "", http.StatusOK, &status)
			if status.Volume == nil || *status.Volume != 35 {
				t.Fatalf("status through %s = %+v", name, status)
			}
			var rejected apitypes.ErrorResponse
			deviceRouteJSON(t, base, apiKey, http.MethodPost, "/gizclaw/v1/device/tool/v0/invoke", `{"tool":"sound.play","args":{"sound":"chime"}}`, http.StatusNotImplemented, &rejected)
			if rejected.Error.Code != "DEVICE_UNSUPPORTED" {
				t.Fatalf("unimplemented provider through %s = %+v", name, rejected)
			}
		})
	}

	// The key is owned by Server A: the foreign Server rejects it before any
	// device or state lookup, and Server B keeps no PeerRun state even though
	// Edge B routed the same key to Server A above.
	beforeB := sqlDatabaseSnapshot(t, stateB)
	assertPeerRunAbsent(t, stateB, peer.Public)
	assertSnapshotEqual(t, "Server B state after Edge-routed device requests", beforeB, sqlDatabaseSnapshot(t, stateB))
	if finds != len(homeBases) {
		t.Fatalf("find calls = %d, want %d", finds, len(homeBases))
	}
	if serverB.PublicKey.Equal(serverA.PublicKey) {
		t.Fatal("the two Servers use the same identity")
	}

	// Disconnecting the device turns control into DEVICE_OFFLINE on the home
	// path while reads keep serving the stored status.
	if err := device.Close(); err != nil {
		t.Fatalf("close device: %v", err)
	}
	var offline apitypes.ErrorResponse
	deadline := time.Now().Add(15 * time.Second)
	for {
		response := deviceRouteRequest(t, homeBases["edge-a"], apiKey, http.MethodPost, "/gizclaw/v1/device/tool/v0/invoke", `{"tool":"device.find","args":{}}`)
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode == http.StatusConflict {
			_ = json.Unmarshal(body, &offline)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("control after disconnect status = %d body=%s", response.StatusCode, body)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if offline.Error.Code != "DEVICE_OFFLINE" {
		t.Fatalf("control after disconnect = %+v", offline)
	}
	var status apitypes.PeerStatus
	deviceRouteJSON(t, homeBases["edge-a"], apiKey, http.MethodGet, "/gizclaw/v1/device/status", "", http.StatusOK, &status)
	if status.Volume == nil || *status.Volume != 35 {
		t.Fatalf("status after disconnect = %+v", status)
	}
}

func deviceRouteBase(t *testing.T, endpoint string) string {
	t.Helper()
	endpoint = strings.TrimSpace(endpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		t.Fatalf("invalid endpoint %q", endpoint)
	}
	parsed.Path, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/")
}

func deviceRouteRequest(t *testing.T, base, apiKey, method, path, body string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s%s: %v", method, base, path, err)
	}
	return response
}

func deviceRouteJSON(t *testing.T, base, apiKey, method, path, body string, wantStatus int, out any) {
	t.Helper()
	response := deviceRouteRequest(t, base, apiKey, method, path, body)
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s%s status = %d, want %d body=%s", method, base, path, response.StatusCode, wantStatus, raw)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("decode %T from %s: %v", out, raw, err)
		}
	}
}
