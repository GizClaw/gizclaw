//go:build gizclaw_e2e

package connect_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

// TestDeviceAPIUserStory exercises the API-key-bound device and contact
// extension with a key created over real Peer RPC. The connected gizcli
// client does not implement client.device.* providers, so control commands
// verify the DEVICE_UNSUPPORTED contract; the C SDK device in
// tests/gizclaw-e2e/cgo/device covers the volume round trip.
func TestDeviceAPIUserStory(t *testing.T) {
	h := clitest.NewSetupHarness(t, "902-device-api")
	h.CreateContext("device-api-device").MustSucceed(t)
	h.RegisterContext("device-api-device", "--sn", "connect-device-api-sn").MustSucceed(t)
	client := h.ConnectClientFromContext("device-api-device")
	defer func() { _ = client.Close() }()
	registrationToken := os.Getenv("GIZCLAW_TEST_REGISTRATION_TOKEN")
	if registrationToken == "" {
		t.Fatal("GIZCLAW_TEST_REGISTRATION_TOKEN is required")
	}
	if _, err := client.Register(t.Context(), "connect.device-api.register", registrationToken); err != nil {
		t.Fatalf("register device: %v", err)
	}
	created, err := client.CreateAPIKey(t.Context(), "device-api-key", rpcapi.APIKeyCreateRequest{DisplayName: "phone", ManageAPIKeys: false})
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	response := apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/device", nil)
	var device apitypes.DeviceInfo
	decodeE2EJSON(t, response, http.StatusOK, &device)
	if device.Identifiers == nil || device.Identifiers.Sn == nil || *device.Identifiers.Sn != "connect-device-api-sn" {
		t.Fatalf("device = %+v, want registered SN", device)
	}

	response = apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/device/runtime", nil)
	var runtime apitypes.Runtime
	decodeE2EJSON(t, response, http.StatusOK, &runtime)
	if !runtime.Online {
		t.Fatalf("runtime = %+v, want online while the Peer connection is open", runtime)
	}

	response = apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/device/status", nil)
	var status apitypes.PeerStatus
	decodeE2EJSON(t, response, http.StatusOK, &status)
	rpcStatus, err := client.GetServerStatus(t.Context(), "connect.device-api.status")
	if err != nil {
		t.Fatalf("server.status.get: %v", err)
	}
	if (status.Volume == nil) != (rpcStatus.Volume == nil) || (status.Volume != nil && *status.Volume != *rpcStatus.Volume) {
		t.Fatalf("HTTP status volume %v differs from RPC status volume %v", status.Volume, rpcStatus.Volume)
	}

	// The gizcli Peer does not provide client.device.* methods: the Server
	// must map METHOD_NOT_FOUND to 501 DEVICE_UNSUPPORTED while online.
	response = apiKeyRequest(t, h, created.APIKey, http.MethodPut, "/gizclaw/v1/device/volume", []byte(`{"level":35,"muted":false}`))
	var controlError apitypes.ErrorResponse
	decodeE2EJSON(t, response, http.StatusNotImplemented, &controlError)
	if controlError.Error.Code != "DEVICE_UNSUPPORTED" {
		t.Fatalf("volume error = %+v", controlError)
	}

	response = apiKeyRequest(t, h, created.APIKey, http.MethodPost, "/gizclaw/v1/contacts", []byte(`{"name":"mom","display_name":"Mom","phone_number":"+8613800000001"}`))
	var contact peerhttp.Contact
	decodeE2EJSON(t, response, http.StatusCreated, &contact)
	if contact.Name != "mom" {
		t.Fatalf("created contact = %+v", contact)
	}
	response = apiKeyRequest(t, h, created.APIKey, http.MethodPut, "/gizclaw/v1/contacts/mom", []byte(`{"display_name":"Mother"}`))
	decodeE2EJSON(t, response, http.StatusOK, &contact)
	if contact.DisplayName == nil || *contact.DisplayName != "Mother" {
		t.Fatalf("updated contact = %+v", contact)
	}
	rpcContact, err := client.GetContact(t.Context(), "connect.device-api.contact", rpcapi.ContactGetRequest{Name: "mom"})
	if err != nil {
		t.Fatalf("server.contact.get: %v", err)
	}
	if rpcContact.DisplayName == nil || *rpcContact.DisplayName != "Mother" {
		t.Fatalf("RPC contact = %+v, want the HTTP update", rpcContact)
	}
	response = apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/contacts", nil)
	var page peerhttp.ContactList
	decodeE2EJSON(t, response, http.StatusOK, &page)
	if len(page.Items) != 1 || page.Items[0].Name != "mom" {
		t.Fatalf("contact list = %+v", page)
	}
	response = apiKeyRequest(t, h, created.APIKey, http.MethodDelete, "/gizclaw/v1/contacts/mom", nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete contact status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
	response = apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/contacts/mom", nil)
	decodeE2EJSON(t, response, http.StatusNotFound, &controlError)

	// Closing the Peer connection turns control commands into DEVICE_OFFLINE
	// while reads keep working.
	_ = client.Close()
	response = apiKeyRequest(t, h, created.APIKey, http.MethodPost, "/gizclaw/v1/device/actions/play-sound", []byte(`{"sound":"chime"}`))
	decodeE2EJSON(t, response, http.StatusConflict, &controlError)
	if controlError.Error.Code != "DEVICE_OFFLINE" {
		t.Fatalf("offline error = %+v", controlError)
	}
	response = apiKeyRequest(t, h, created.APIKey, http.MethodGet, "/gizclaw/v1/device/status", nil)
	decodeE2EJSON(t, response, http.StatusOK, &status)
}

func decodeE2EJSON(t *testing.T, response *http.Response, wantStatus int, out any) {
	t.Helper()
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d body=%s", response.StatusCode, wantStatus, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %T: %v body=%s", out, err, body)
	}
}
