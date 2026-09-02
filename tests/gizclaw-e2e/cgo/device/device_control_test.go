//go:build gizclaw_e2e

package device_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	cgointernal "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cgo/internal"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

// TestCSDKDeviceControlRoundTrip connects a C SDK device whose rpc_provider
// implements client.device.* and client.wifi.*, then drives the API-key Public
// HTTP control routes and verifies the Server-to-device reverse RPC path end
// to end: HTTP response, device-side state, and server.status.get agreement.
func TestCSDKDeviceControlRoundTrip(t *testing.T) {
	registrationToken := os.Getenv("GIZCLAW_TEST_REGISTRATION_TOKEN")
	if registrationToken == "" {
		t.Fatal("GIZCLAW_TEST_REGISTRATION_TOKEN is required")
	}
	h := clitest.NewSetupHarness(t, "cgo-device-control")
	const contextName = "cgo-device-control"
	h.CreateContext(contextName).MustSucceed(t)
	h.RegisterContext(contextName, "--sn", "cgo-device-control-sn").MustSucceed(t)
	keyPair := h.ContextKeyPair(contextName)

	device, err := cgointernal.NewClientWithCredentials(h.ClientEndpoint(), keyPair.Private.String())
	if err != nil {
		t.Fatalf("connect C SDK device: %v", err)
	}
	stopPolling := startDevicePolling(t, device)
	defer stopPolling()

	if _, err := device.Register(registrationToken); err != nil {
		t.Fatalf("server.register: %v", err)
	}
	var created rpcpb.APIKeyCreateResponse
	if err := device.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_API_KEY_CREATE, &rpcpb.APIKeyCreateRequest{DisplayName: "phone"}, &created); err != nil {
		t.Fatalf("server.api_key.create: %v", err)
	}
	apiKey := created.GetApiKey()
	if apiKey == "" {
		t.Fatalf("server.api_key.create returned no credential: %s", created.String())
	}
	call := func(method, path string, body string) *http.Response {
		t.Helper()
		return deviceHTTP(t, h, apiKey, method, path, body)
	}

	// Volume: HTTP response, device state, stored status, and RPC read agree.
	var control peerhttp.DeviceControlStatus
	decodeDeviceJSON(t, call(http.MethodPut, "/gizclaw/v1/device/volume", `{"level":35,"muted":true}`), http.StatusOK, &control)
	if control.Status.Volume == nil || *control.Status.Volume != 35 || control.Status.Muted == nil || !*control.Status.Muted || control.Status.BatteryPercent == nil || *control.Status.BatteryPercent != 88 {
		t.Fatalf("volume control status = %+v", control.Status)
	}
	state := mustDeviceState(t, device)
	if state.Volume != 35 || !state.Muted || state.VolumeCalls != 1 {
		t.Fatalf("device state after volume.set = %+v", state)
	}
	var status apitypes.PeerStatus
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/status", ""), http.StatusOK, &status)
	if status.Volume == nil || *status.Volume != 35 || status.Muted == nil || !*status.Muted {
		t.Fatalf("GET status after volume.set = %+v", status)
	}
	var rpcStatus rpcpb.ServerGetStatusResponse
	if err := device.CallRPC(rpcpb.RpcMethod_RPC_METHOD_SERVER_STATUS_GET, &rpcpb.ServerGetStatusRequest{}, &rpcStatus); err != nil {
		t.Fatalf("server.status.get: %v", err)
	}
	if rpcStatus.GetValue().GetVolume() != 35 || !rpcStatus.GetValue().GetMuted() {
		t.Fatalf("server.status.get after volume.set = %s", rpcStatus.String())
	}
	var rejected apitypes.ErrorResponse
	decodeDeviceJSON(t, call(http.MethodPut, "/gizclaw/v1/device/volume", `{"level":101,"muted":false}`), http.StatusBadRequest, &rejected)
	if rejected.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("out-of-range volume = %+v", rejected)
	}

	// Sound: device-validated identifier.
	closeBody(call(http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"chime","duration_ms":1500}`), http.StatusNoContent, t)
	state = mustDeviceState(t, device)
	if state.SoundCalls != 1 || state.LastSound != "chime" || state.LastDurationMs != 1500 {
		t.Fatalf("device state after sound.play = %+v", state)
	}
	decodeDeviceJSON(t, call(http.MethodPost, "/gizclaw/v1/device/actions/play-sound", `{"sound":"unknown"}`), http.StatusBadRequest, &rejected)
	if rejected.Error.Code != "DEVICE_REJECTED" {
		t.Fatalf("unknown sound = %+v", rejected)
	}

	// Wi-Fi: status, saved list, forget, and not-found.
	var wifi peerhttp.DeviceWifiStatus
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/wifi", ""), http.StatusOK, &wifi)
	if !wifi.Connected || wifi.Ssid == nil || *wifi.Ssid != "home" || wifi.RssiDbm == nil || *wifi.RssiDbm != -61 || wifi.Ip == nil || *wifi.Ip != "192.0.2.20" {
		t.Fatalf("wifi status = %+v", wifi)
	}
	var saved peerhttp.DeviceWifiSavedList
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/wifi/saved", ""), http.StatusOK, &saved)
	if len(saved.Networks) != 2 || saved.Networks[0].Ssid != "home" || saved.Networks[1].Ssid != "office" {
		t.Fatalf("saved networks = %+v", saved)
	}
	closeBody(call(http.MethodDelete, "/gizclaw/v1/device/wifi/saved/office", ""), http.StatusNoContent, t)
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/wifi/saved", ""), http.StatusOK, &saved)
	if len(saved.Networks) != 1 || saved.Networks[0].Ssid != "home" {
		t.Fatalf("saved networks after forget = %+v", saved)
	}
	decodeDeviceJSON(t, call(http.MethodDelete, "/gizclaw/v1/device/wifi/saved/office", ""), http.StatusNotFound, &rejected)
	if rejected.Error.Code != "WIFI_NETWORK_NOT_FOUND" {
		t.Fatalf("forget missing network = %+v", rejected)
	}
	state = mustDeviceState(t, device)
	if state.ForgetCalls != 2 || state.SavedListCalls != 2 || state.WifiStatusCalls != 1 || len(state.SavedNetworks) != 1 {
		t.Fatalf("device wifi state = %+v", state)
	}

	// Reboot: acknowledged, then the same connection answers DEVICE_OFFLINE.
	closeBody(call(http.MethodPost, "/gizclaw/v1/device/actions/reboot", `{"delay_ms":2000}`), http.StatusNoContent, t)
	state = mustDeviceState(t, device)
	if state.RebootCalls != 1 || state.LastDelayMs != 2000 {
		t.Fatalf("device state after reboot = %+v", state)
	}
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/wifi", ""), http.StatusConflict, &rejected)
	if rejected.Error.Code != "DEVICE_OFFLINE" {
		t.Fatalf("control after reboot = %+v", rejected)
	}
	if mustDeviceState(t, device).WifiStatusCalls != 1 {
		t.Fatal("control after reboot reached the rebooting device")
	}

	// Reads keep working after the device goes away; controls stay offline.
	stopPolling()
	device.Close()
	decodeDeviceJSON(t, call(http.MethodGet, "/gizclaw/v1/device/status", ""), http.StatusOK, &status)
	if status.Volume == nil || *status.Volume != 35 {
		t.Fatalf("GET status after disconnect = %+v", status)
	}
	decodeDeviceJSON(t, call(http.MethodPut, "/gizclaw/v1/device/volume", `{"level":10,"muted":false}`), http.StatusConflict, &rejected)
	if rejected.Error.Code != "DEVICE_OFFLINE" {
		t.Fatalf("control after disconnect = %+v", rejected)
	}
}

// startDevicePolling keeps the C client polling so Server-initiated RPCs are
// dispatched to its provider while the test issues HTTP requests.
func startDevicePolling(t *testing.T, device *cgointernal.Client) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			if err := device.Poll(10 * time.Millisecond); err != nil && ctx.Err() == nil {
				return
			}
		}
	}()
	var stopped bool
	return func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		<-done
	}
}

func mustDeviceState(t *testing.T, device *cgointernal.Client) cgointernal.DeviceState {
	t.Helper()
	state, err := device.DeviceState()
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func deviceHTTP(t *testing.T, h *clitest.Harness, apiKey, method, path, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, h.PublicHTTPURL()+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeDeviceJSON(t *testing.T, response *http.Response, wantStatus int, out any) {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d body=%s", response.Request.URL.Path, response.StatusCode, wantStatus, data)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %T: %v body=%s", out, err, data)
	}
}

func closeBody(response *http.Response, wantStatus int, t *testing.T) {
	t.Helper()
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d body=%s", response.Request.URL.Path, response.StatusCode, wantStatus, data)
	}
}
