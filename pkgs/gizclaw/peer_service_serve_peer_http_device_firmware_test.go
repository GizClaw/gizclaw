package gizclaw

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const (
	stableFirmwareSha256  = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	betaFirmwareSha256    = "b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a"
	unknownFirmwareSha256 = "c1d2e3f405162738495a6b7c8d9e0f1bc1d2e3f405162738495a6b7c8d9e0f1b"
)

// seedBoundFirmware stores one Firmware configuration and binds it to the
// fixture owner, so the read projection resolves it exactly like production.
func seedBoundFirmware(t *testing.T, f *deviceHTTPFixture, id string) {
	t.Helper()
	ctx := context.Background()
	response, err := f.firmware.CreateFirmware(ctx, adminhttp.CreateFirmwareRequestObject{Body: &adminhttp.FirmwareUpsert{
		Id:          id,
		Description: new("Devkit firmware channels"),
		Slots: apitypes.FirmwareSlots{
			Stable: apitypes.FirmwareSlot{
				Description: new("Devkit firmware 1.0.3"),
				Package:     &apitypes.FirmwarePackage{Url: "https://firmware.example.com/devkit/1.0.3.tar.zlib", Sha256: stableFirmwareSha256, Size: 4096},
			},
			Beta: apitypes.FirmwareSlot{
				Package: &apitypes.FirmwarePackage{Url: "https://firmware.example.com/devkit/1.1.0-beta.tar.zlib", Sha256: betaFirmwareSha256, Size: 8192},
			},
		},
	}})
	if err != nil {
		t.Fatalf("CreateFirmware() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateFirmware200JSONResponse); !ok {
		t.Fatalf("CreateFirmware() response = %T", response)
	}
	if _, err := f.peers.BindFirmware(ctx, f.owner, id); err != nil {
		t.Fatalf("BindFirmware() error = %v", err)
	}
}

func TestGetDeviceFirmwareReturnsEveryChannelWhileOffline(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedBoundFirmware(t, f, "devkit")

	// No connection is registered for the owner, so this also proves the read
	// never contacts the device.
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/firmware", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET firmware status = %d body=%s", response.Code, response.Body.String())
	}
	result := decodeJSON[peerhttp.DeviceFirmware](t, response)
	if result.Description == nil || *result.Description != "Devkit firmware channels" {
		t.Fatalf("description = %v", result.Description)
	}
	if result.Slots.Stable.Package == nil || result.Slots.Stable.Package.Sha256 != stableFirmwareSha256 || result.Slots.Stable.Package.Size != 4096 {
		t.Fatalf("stable slot = %+v", result.Slots.Stable)
	}
	if result.Slots.Stable.Description == nil || *result.Slots.Stable.Description != "Devkit firmware 1.0.3" {
		t.Fatalf("stable description = %v", result.Slots.Stable.Description)
	}
	if result.Slots.Beta.Package == nil || result.Slots.Beta.Package.Sha256 != betaFirmwareSha256 {
		t.Fatalf("beta slot = %+v", result.Slots.Beta)
	}
	// An unconfigured channel is reported as an empty slot, never as an error.
	if result.Slots.Develop.Package != nil || result.Slots.Develop.Description != nil {
		t.Fatalf("develop slot = %+v, want empty", result.Slots.Develop)
	}
}

func TestGetDeviceFirmwareNotBound(t *testing.T) {
	f := newDeviceHTTPFixture(t)

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/firmware", "")
	if response.Code != http.StatusNotFound || errorCode(t, response) != publicHTTPFirmwareNotFound {
		t.Fatalf("unbound GET firmware status = %d body=%s", response.Code, response.Body.String())
	}

	// A binding whose configuration was deleted answers the same way.
	seedBoundFirmware(t, f, "devkit")
	if _, err := f.firmware.DeleteFirmware(context.Background(), adminhttp.DeleteFirmwareRequestObject{Id: "devkit"}); err != nil {
		t.Fatalf("DeleteFirmware() error = %v", err)
	}
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/firmware", "")
	if response.Code != http.StatusNotFound || errorCode(t, response) != publicHTTPFirmwareNotFound {
		t.Fatalf("dangling GET firmware status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdateDeviceFirmwareForwardsToDevice(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var lastRequest rpcapi.ClientFirmwareUpdateRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method != rpcapi.RPCMethodClientFirmwareUpdate {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unsupported"}.RPCResponse(), nil
		}
		params, err := req.Params.AsClientFirmwareUpdateRequest()
		if err != nil {
			return nil, err
		}
		lastRequest = params
		// The device owns the comparison: it refuses a digest that does not
		// match the package its own configuration resolves.
		if params.Sha256 != nil && *params.Sha256 == unknownFirmwareSha256 {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInvalidArgument, Message: "sha256 does not match the resolved package"}.RPCResponse(), nil
		}
		return newRPCResultResponse(req.Id, rpcapi.ClientFirmwareUpdateResponse{}, (*rpcapi.RPCPayload).FromClientFirmwareUpdateResponse)
	})
	f.manager.SetPeerUp(f.owner, device)

	beta := rpcapi.FirmwareChannelNameBeta
	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", `{"channel":"beta","sha256":"`+betaFirmwareSha256+`"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("POST firmware-update status = %d body=%s", response.Code, response.Body.String())
	}
	if lastRequest.Channel == nil || *lastRequest.Channel != beta {
		t.Fatalf("device received channel %v", lastRequest.Channel)
	}
	if lastRequest.Sha256 == nil || *lastRequest.Sha256 != betaFirmwareSha256 {
		t.Fatalf("device received sha256 %v", lastRequest.Sha256)
	}

	// The device restarts into the new image, so later commands answer
	// DEVICE_OFFLINE on the same connection exactly like reboot.
	response = f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/reboot", "")
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("post-update reboot status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdateDeviceFirmwareWithoutBody(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var lastRequest rpcapi.ClientFirmwareUpdateRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		params, err := req.Params.AsClientFirmwareUpdateRequest()
		if err != nil {
			return nil, err
		}
		lastRequest = params
		return newRPCResultResponse(req.Id, rpcapi.ClientFirmwareUpdateResponse{}, (*rpcapi.RPCPayload).FromClientFirmwareUpdateResponse)
	})
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("POST firmware-update status = %d body=%s", response.Code, response.Body.String())
	}
	if lastRequest.Channel != nil || lastRequest.Sha256 != nil {
		t.Fatalf("device received %+v, want the device's own channel", lastRequest)
	}
}

func TestUpdateDeviceFirmwareRejectsInvalidBody(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return newRPCResultResponse(req.Id, rpcapi.ClientFirmwareUpdateResponse{}, (*rpcapi.RPCPayload).FromClientFirmwareUpdateResponse)
	})
	f.manager.SetPeerUp(f.owner, device)

	for _, body := range []string{`{"channel":"nightly"}`, `{"sha256":"abc"}`, `{"sha256":"` + strings.ToUpper(stableFirmwareSha256) + `"}`} {
		response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("POST firmware-update %s status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if device.calls.Load() != 0 {
		t.Fatalf("validation failures reached the device: calls = %d", device.calls.Load())
	}
}

func TestUpdateDeviceFirmwareDeviceErrors(t *testing.T) {
	f := newDeviceHTTPFixture(t)

	// Offline before any device registers.
	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", "")
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("offline POST firmware-update status = %d body=%s", response.Code, response.Body.String())
	}

	// Firmware that predates client.firmware.update answers UNIMPLEMENTED, and
	// callers must read that as unsupported rather than as a failed update.
	unsupported := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unsupported"}.RPCResponse(), nil
	})
	f.manager.SetPeerUp(f.owner, unsupported)
	response = f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", "")
	if response.Code != http.StatusNotImplemented || errorCode(t, response) != deviceUnsupportedCode {
		t.Fatalf("unsupported POST firmware-update status = %d body=%s", response.Code, response.Body.String())
	}

	// A device that refuses the declared digest surfaces as a 400.
	rejecting := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeInvalidArgument, Message: "sha256 does not match the resolved package"}.RPCResponse(), nil
	})
	f.manager.SetPeerUp(f.owner, rejecting)
	response = f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/firmware-update", `{"sha256":"`+unknownFirmwareSha256+`"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched POST firmware-update status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceStatusCarriesReportedFirmwareDigest(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method != rpcapi.RPCMethodClientDeviceVolumeSet {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unsupported"}.RPCResponse(), nil
		}
		return deviceStatusResult(req.Id, rpcapi.PeerStatus{Volume: new(20), FirmwareSha256: new(stableFirmwareSha256)})
	})
	f.manager.SetPeerUp(f.owner, device)

	if response := f.do(t, http.MethodPut, "/gizclaw/v1/device/volume", `{"level":20,"muted":false}`); response.Code != http.StatusOK {
		t.Fatalf("PUT volume status = %d body=%s", response.Code, response.Body.String())
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/status", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", response.Code, response.Body.String())
	}
	status := decodeJSON[apitypes.PeerStatus](t, response)
	if status.FirmwareSha256 == nil || *status.FirmwareSha256 != stableFirmwareSha256 {
		t.Fatalf("stored firmware_sha256 = %v", status.FirmwareSha256)
	}
}
