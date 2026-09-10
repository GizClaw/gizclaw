package gizcli

import (
	"context"
	"errors"
	"reflect"
	"slices"
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
	var gotDuration, gotDelay, gotFindDuration *int64
	findCalls := 0
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
		Find: func(_ context.Context, duration *int64) error {
			findCalls++
			gotFindDuration = duration
			return nil
		},
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

	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceFind, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFindRequest(rpcapi.ClientDeviceFindRequest{DurationMs: new(int64(8000))})
	})
	if resp.Error != nil || gotFindDuration == nil || *gotFindDuration != 8000 {
		t.Fatalf("find = %#v duration=%v", resp, gotFindDuration)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceFind, nil); resp.Error != nil || gotFindDuration != nil {
		t.Fatalf("find without params = %#v duration=%v", resp, gotFindDuration)
	}
	resp = deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceFind, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFindRequest(rpcapi.ClientDeviceFindRequest{DurationMs: new(int64(-1))})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument || findCalls != 2 {
		t.Fatalf("negative find duration = %#v after %d calls", resp, findCalls)
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
	if len(observed) != 14 {
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

// The settings patch is validated before any handler runs, and the capability
// list is derived from the handlers that are actually installed.
func TestDeviceSettingsHandlersAndCapabilityList(t *testing.T) {
	handlers := DeviceControlHandlers{
		Reboot: func(context.Context, *int64) error { return nil },
		GetSettings: func(context.Context) (rpcapi.DeviceSettings, error) {
			return rpcapi.DeviceSettings{ScreenBrightness: new(int64(30))}, nil
		},
		FactoryReset: func(context.Context, bool) error { return nil },
	}
	want := []string{
		string(rpcapi.RPCMethodClientDeviceReboot),
		string(rpcapi.RPCMethodClientDeviceSettingsGet),
		string(rpcapi.RPCMethodClientDeviceFactoryReset),
		string(rpcapi.RPCMethodClientRPCMethodsGet),
	}
	if got := handlers.supportedDeviceMethods(); !reflect.DeepEqual(got, want) {
		t.Fatalf("supportedDeviceMethods() = %#v, want %#v", got, want)
	}
	// A device with no handlers still answers, listing only the method that
	// produced the answer.
	var empty DeviceControlHandlers
	if got := empty.supportedDeviceMethods(); !reflect.DeepEqual(got, []string{string(rpcapi.RPCMethodClientRPCMethodsGet)}) {
		t.Fatalf("empty supportedDeviceMethods() = %#v", got)
	}

	mode := rpcapi.DeviceInteractionModeRealtime
	unknown := rpcapi.DeviceInteractionMode("telepathy")
	for name, patch := range map[string]rpcapi.DeviceSettings{
		"ok":           {ScreenBrightness: new(int64(0)), LedBrightness: new(int64(100)), Locale: new("zh-CN"), DefaultInteractionMode: &mode},
		"empty patch":  {},
		"zero timeout": {ScreenOffTimeoutMs: new(int64(0))},
	} {
		if !validDeviceSettingsPatch(patch) {
			t.Fatalf("validDeviceSettingsPatch(%s) = false, want true", name)
		}
	}
	for name, patch := range map[string]rpcapi.DeviceSettings{
		"brightness over 100": {ScreenBrightness: new(int64(101))},
		"negative led":        {LedBrightness: new(int64(-1))},
		"negative timeout":    {ScreenOffTimeoutMs: new(int64(-1))},
		"empty locale":        {Locale: new("")},
		"unknown enum":        {DefaultInteractionMode: &unknown},
	} {
		if validDeviceSettingsPatch(patch) {
			t.Fatalf("validDeviceSettingsPatch(%s) = true, want false", name)
		}
	}
}

func TestRPCClientSocialPingHandler(t *testing.T) {
	device := &Client{}
	ping := func(p *rpcapi.RPCPayload) error {
		return p.FromClientSocialPingRequest(rpcapi.ClientSocialPingRequest{FromPeerPublicKey: "friend", FriendGroupName: new("team")})
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientSocialPing, ping); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("ping without handler = %#v", resp)
	}
	var got rpcapi.ClientSocialPingRequest
	if err := device.HandleSocialPing(func(_ context.Context, request rpcapi.ClientSocialPingRequest) error {
		got = request
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientSocialPing, ping); resp.Error != nil || got.FromPeerPublicKey != "friend" || got.FriendGroupName == nil || *got.FriendGroupName != "team" {
		t.Fatalf("ping = %#v got=%+v", resp, got)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientSocialPing, func(p *rpcapi.RPCPayload) error {
		return p.FromClientSocialPingRequest(rpcapi.ClientSocialPingRequest{})
	}); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("ping without sender = %#v", resp)
	}
	if err := device.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return errors.New("busy") }); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientSocialPing, ping); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInternal {
		t.Fatalf("failing handler = %#v", resp)
	}
}

// The settings, factory-reset and capability methods are reached through the
// real inbound dispatch, not only through their helpers: an unrouted method
// answers UNIMPLEMENTED even when its handler is installed.
func TestRPCClientDeviceSettingsDispatch(t *testing.T) {
	device := &Client{}
	var applied rpcapi.DeviceSettings
	var resetKeepNetwork *bool
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		Find: func(context.Context, *int64) error { return nil },
		GetSettings: func(context.Context) (rpcapi.DeviceSettings, error) {
			return rpcapi.DeviceSettings{ScreenBrightness: new(int64(30))}, nil
		},
		SetSettings: func(_ context.Context, patch rpcapi.DeviceSettings) (rpcapi.DeviceSettings, error) {
			applied = patch
			return patch, nil
		},
		FactoryReset: func(_ context.Context, keepNetwork bool) error {
			resetKeepNetwork = &keepNetwork
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	get := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceSettingsGet, nil)
	if get.Error != nil {
		t.Fatalf("settings.get = %#v", get.Error)
	}
	got, err := get.Result.AsClientDeviceSettingsGetResponse()
	if err != nil || got.Value.ScreenBrightness == nil || *got.Value.ScreenBrightness != 30 {
		t.Fatalf("settings.get value = %+v err=%v", got, err)
	}

	set := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceSettingsSet, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSettingsSetRequest(rpcapi.ClientDeviceSettingsSetRequest{Value: rpcapi.DeviceSettings{LedBrightness: new(int64(40))}})
	})
	if set.Error != nil || applied.LedBrightness == nil || *applied.LedBrightness != 40 {
		t.Fatalf("settings.set = %#v applied=%+v", set.Error, applied)
	}
	// An out-of-range member is rejected before the handler sees any of it.
	applied = rpcapi.DeviceSettings{}
	bad := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceSettingsSet, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSettingsSetRequest(rpcapi.ClientDeviceSettingsSetRequest{Value: rpcapi.DeviceSettings{ScreenBrightness: new(int64(140))}})
	})
	if bad.Error == nil || bad.Error.Code != rpcapi.StatusCodeInvalidArgument || applied.ScreenBrightness != nil {
		t.Fatalf("out-of-range settings.set = %#v applied=%+v", bad.Error, applied)
	}

	reset := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceFactoryReset, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFactoryResetRequest(rpcapi.ClientDeviceFactoryResetRequest{KeepNetwork: new(true)})
	})
	if reset.Error != nil || resetKeepNetwork == nil || !*resetKeepNetwork {
		t.Fatalf("factory_reset = %#v keepNetwork=%v", reset.Error, resetKeepNetwork)
	}

	if err := device.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return nil }); err != nil {
		t.Fatal(err)
	}
	list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsGet, nil)
	if list.Error != nil {
		t.Fatalf("rpc.methods.get = %#v", list.Error)
	}
	methods, err := list.Result.AsClientRPCMethodsGetResponse()
	if err != nil {
		t.Fatal(err)
	}
	// Every advertised method must itself be answered, not UNIMPLEMENTED.
	for _, method := range methods.Methods {
		if resp := deviceControlDispatch(t, device, rpcapi.RPCMethod(method), nil); resp.Error != nil && resp.Error.Code == rpcapi.StatusCodeUnimplemented {
			t.Fatalf("advertised method %s answered UNIMPLEMENTED", method)
		}
	}
	for _, want := range []rpcapi.RPCMethod{
		rpcapi.RPCMethodClientInfoGet,
		rpcapi.RPCMethodClientIdentifiersGet,
		rpcapi.RPCMethodClientDeviceFind,
		rpcapi.RPCMethodClientDeviceSettingsGet,
		rpcapi.RPCMethodClientDeviceSettingsSet,
		rpcapi.RPCMethodClientDeviceFactoryReset,
		rpcapi.RPCMethodClientSocialPing,
		rpcapi.RPCMethodClientRPCMethodsGet,
	} {
		if !slices.Contains(methods.Methods, string(want)) {
			t.Fatalf("rpc.methods.get = %v, missing %s", methods.Methods, want)
		}
	}
	for _, absent := range []rpcapi.RPCMethod{rpcapi.RPCMethodClientDeviceReboot, rpcapi.RPCMethodClientWifiScan} {
		if slices.Contains(methods.Methods, string(absent)) {
			t.Fatalf("rpc.methods.get = %v, advertises uninstalled %s", methods.Methods, absent)
		}
	}
}

// A device that installed no device control handlers still reports what it
// implements: the methods the Client answers itself, and the capability method.
func TestRPCClientCapabilityListWithoutDeviceHandlers(t *testing.T) {
	device := &Client{}
	resp := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsGet, nil)
	if resp.Error != nil {
		t.Fatalf("rpc.methods.get without handlers = %#v", resp.Error)
	}
	got, err := resp.Result.AsClientRPCMethodsGetResponse()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		string(rpcapi.RPCMethodClientInfoGet),
		string(rpcapi.RPCMethodClientIdentifiersGet),
		string(rpcapi.RPCMethodClientRPCMethodsGet),
	}
	if !reflect.DeepEqual(got.Methods, want) {
		t.Fatalf("rpc.methods.get without handlers = %v, want %v", got.Methods, want)
	}
}
