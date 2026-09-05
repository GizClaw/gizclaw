package gizcli

import (
	"context"
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func deviceControlDispatch(t *testing.T, device *Client, method rpcapi.RPCMethod, encode func(*rpcapi.RPCPayload) error) *rpcapi.RPCResponse {
	t.Helper()
	var params *rpcapi.RPCPayload
	if encode != nil {
		params = &rpcapi.RPCPayload{}
		if err := encode(params); err != nil {
			t.Fatal(err)
		}
	}
	resp, err := (&rpcClient{peer: device}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "device", Method: method, Params: params})
	if err != nil {
		t.Fatalf("dispatch(%s) error = %v", method, err)
	}
	return resp
}

func TestRPCClientDeviceControlHandlers(t *testing.T) {
	device := &Client{}
	var observed []rpcapi.RPCMethod
	if err := device.ObserveClientRPC(func(method rpcapi.RPCMethod) { observed = append(observed, method) }); err != nil {
		t.Fatal(err)
	}
	saved := []rpcapi.WifiSavedNetwork{{Ssid: "home"}, {Ssid: "office"}}
	var gotLevel int64
	var gotMuted bool
	var gotSound string
	var gotDuration, gotDelay *int64
	var gotScanTimeout *int64
	var gotConnectSSID string
	var gotPassphrase *string
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		Status: func(context.Context) (rpcapi.PeerStatus, error) { return rpcapi.PeerStatus{Volume: new(50)}, nil },
		SetVolume: func(_ context.Context, level int64, muted bool) (rpcapi.PeerStatus, error) {
			gotLevel, gotMuted = level, muted
			value := int(level)
			return rpcapi.PeerStatus{Volume: &value, Muted: &muted}, nil
		},
		PlaySound: func(_ context.Context, sound string, duration *int64) error {
			gotSound, gotDuration = sound, duration
			if sound != "chime" {
				return ErrDeviceRejected
			}
			return nil
		},
		Reboot: func(_ context.Context, delay *int64) error { gotDelay = delay; return nil },
		WifiStatus: func(context.Context) (rpcapi.WifiStatus, error) {
			return rpcapi.WifiStatus{Connected: true, Ssid: new("home")}, nil
		},
		SavedWifi: func(context.Context) ([]rpcapi.WifiSavedNetwork, error) { return saved, nil },
		ForgetWifi: func(_ context.Context, ssid string) error {
			if ssid != "office" {
				return ErrDeviceResourceNotFound
			}
			return nil
		},
		ScanWifi: func(_ context.Context, timeoutMs *int64) ([]rpcapi.WifiScanResult, error) {
			gotScanTimeout = timeoutMs
			return []rpcapi.WifiScanResult{{Ssid: "office", RssiDbm: new(int64(-42))}}, nil
		},
		ConnectWifi: func(_ context.Context, ssid string, passphrase *string) error {
			gotConnectSSID, gotPassphrase = ssid, passphrase
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceStatusGet, nil)
	status, err := resp.Result.AsClientDeviceStatusGetResponse()
	if err != nil || status.Value.Volume == nil || *status.Value.Volume != 50 {
		t.Fatalf("status = %+v, %v (resp %#v)", status, err, resp)
	}

	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceVolumeSet, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceVolumeSetRequest(rpcapi.ClientDeviceVolumeSetRequest{Level: 35, Muted: true})
	})
	volume, err := resp.Result.AsClientDeviceVolumeSetResponse()
	if err != nil || *volume.Value.Volume != 35 || !*volume.Value.Muted || gotLevel != 35 || !gotMuted {
		t.Fatalf("volume = %+v, %v", volume, err)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceVolumeSet, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceVolumeSetRequest(rpcapi.ClientDeviceVolumeSetRequest{Level: 101})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("out-of-range volume = %#v", resp)
	}

	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceSoundPlay, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "chime", DurationMs: new(int64(1500))})
	})
	if resp.Error != nil || gotSound != "chime" || gotDuration == nil || *gotDuration != 1500 {
		t.Fatalf("sound = %#v sound=%q duration=%v", resp, gotSound, gotDuration)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceSoundPlay, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "unknown"})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("rejected sound = %#v", resp)
	}

	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceReboot, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceRebootRequest(rpcapi.ClientDeviceRebootRequest{DelayMs: new(int64(2000))})
	})
	if resp.Error != nil || gotDelay == nil || *gotDelay != 2000 {
		t.Fatalf("reboot = %#v delay=%v", resp, gotDelay)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceReboot, nil); resp.Error != nil || gotDelay != nil {
		t.Fatalf("reboot without params = %#v delay=%v", resp, gotDelay)
	}

	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiStatusGet, nil)
	wifi, err := resp.Result.AsClientWifiStatusGetResponse()
	if err != nil || !wifi.Value.Connected || *wifi.Value.Ssid != "home" {
		t.Fatalf("wifi = %+v, %v", wifi, err)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedList, nil)
	list, err := resp.Result.AsClientWifiSavedListResponse()
	if err != nil || len(list.Networks) != 2 || list.Networks[1].Ssid != "office" {
		t.Fatalf("saved = %+v, %v", list, err)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedForget, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "office"})
	}); resp.Error != nil {
		t.Fatalf("forget = %#v", resp)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedForget, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "missing"})
	}); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("forget missing = %#v", resp)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiScan, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiScanRequest(rpcapi.ClientWifiScanRequest{TimeoutMs: new(int64(8000))})
	})
	scan, err := resp.Result.AsClientWifiScanResponse()
	if err != nil || len(scan.Networks) != 1 || scan.Networks[0].Ssid != "office" || gotScanTimeout == nil || *gotScanTimeout != 8000 {
		t.Fatalf("scan = %+v, %v timeout=%v", scan, err, gotScanTimeout)
	}
	passphrase := "correct-horse"
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiConnect, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiConnectRequest(rpcapi.ClientWifiConnectRequest{Ssid: "office", Passphrase: &passphrase})
	})
	if resp.Error != nil || gotConnectSSID != "office" || gotPassphrase == nil || *gotPassphrase != passphrase {
		t.Fatalf("connect = %#v ssid=%q passphrase=%v", resp, gotConnectSSID, gotPassphrase)
	}
	if len(observed) != 12 {
		t.Fatalf("observed %d valid dispatches: %v", len(observed), observed)
	}
}

func TestRPCClientDeviceControlUnsupportedAndFailures(t *testing.T) {
	// No handlers installed: every device method is METHOD_NOT_FOUND.
	for _, method := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodClientDeviceStatusGet, rpcapi.RPCMethodClientWifiStatusGet, rpcapi.RPCMethodClientWifiSavedList,
		rpcapi.RPCMethodClientWifiScan, rpcapi.RPCMethodClientWifiConnect,
	} {
		resp := deviceControlDispatch(t, &Client{}, method, nil)
		if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
			t.Fatalf("%s without handlers = %#v", method, resp)
		}
	}
	// Partial handlers: only the missing method is unsupported.
	device := &Client{}
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		WifiStatus: func(context.Context) (rpcapi.WifiStatus, error) { return rpcapi.WifiStatus{}, errors.New("radio off") },
		ForgetWifi: func(context.Context, string) error {
			return rpcapi.Error{Code: rpcapi.StatusCodeUnavailable, Message: "busy"}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceReboot, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("reboot without handler = %#v", resp)
	}
	resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiStatusGet, nil)
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInternal || resp.Error.Message == "radio off" {
		t.Fatalf("handler failure must be redacted INTERNAL_ERROR: %#v", resp)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedForget, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "home"})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnavailable || resp.Error.Message != "busy" {
		t.Fatalf("typed rpc error must pass through: %#v", resp)
	}
	for _, ssid := range []string{"", "123456789012345678901234567890123"} {
		resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedForget, func(p *rpcapi.RPCPayload) error {
			return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: ssid})
		})
		if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
			t.Fatalf("ssid %q = %#v", ssid, resp)
		}
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientWifiSavedForget, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("forget without params = %#v", resp)
	}
	if resp, err := (&rpcClient{}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "x", Method: rpcapi.RPCMethodClientDeviceStatusGet}); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInternal {
		t.Fatalf("dispatch without peer = %#v, %v", resp, err)
	}
}

func TestRPCClientFirmwareUpdateProvider(t *testing.T) {
	digest := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	var gotChannel *rpcapi.FirmwareChannelName
	var gotSha256 *string
	device := &Client{}
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		UpdateFirmware: func(_ context.Context, channel *rpcapi.FirmwareChannelName, sha256 *string) error {
			gotChannel, gotSha256 = channel, sha256
			if sha256 != nil && *sha256 != digest {
				return ErrDeviceRejected
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	beta := rpcapi.FirmwareChannelNameBeta
	resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientFirmwareUpdate, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Channel: &beta, Sha256: &digest})
	})
	if resp.Error != nil || gotChannel == nil || *gotChannel != beta || gotSha256 == nil || *gotSha256 != digest {
		t.Fatalf("update = %#v channel=%v sha256=%v", resp, gotChannel, gotSha256)
	}

	// Omitted params leave both choices to the device.
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientFirmwareUpdate, nil); resp.Error != nil || gotChannel != nil || gotSha256 != nil {
		t.Fatalf("update without params = %#v channel=%v sha256=%v", resp, gotChannel, gotSha256)
	}

	// An unspecified channel encodes on the wire but names no channel.
	unspecified := rpcapi.FirmwareChannelName("unspecified")
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientFirmwareUpdate, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Channel: &unspecified})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("unspecified channel = %#v", resp)
	}

	// A digest the device does not resolve is rejected, not failed.
	other := "b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a"
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientFirmwareUpdate, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Sha256: &other})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("mismatched digest = %#v", resp)
	}

	// Firmware without the provider answers METHOD_NOT_FOUND.
	if resp := deviceControlDispatch(t, &Client{}, rpcapi.RPCMethodClientFirmwareUpdate, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("update without handler = %#v", resp)
	}
}
