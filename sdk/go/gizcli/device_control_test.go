package gizcli

import (
	"context"
	"errors"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func deviceControlDispatch(t *testing.T, device *Client, method any, encode func(*rpcapi.RPCPayload) error) *rpcapi.RPCResponse {
	t.Helper()
	var params *rpcapi.RPCPayload
	if encode != nil {
		params = &rpcapi.RPCPayload{}
		if err := encode(params); err != nil {
			t.Fatal(err)
		}
	}
	var requestMethod rpcapi.RPCMethod
	switch value := method.(type) {
	case rpcapi.RPCMethod:
		requestMethod = value
	case rpcpb.ClientTool:
		requestMethod = rpcapi.RPCMethodClientToolV0Invoke
		var err error
		params, err = rpcapi.EncodeClientToolRequest(value, params)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported test selector %T", method)
	}
	resp, err := (&rpcClient{peer: device}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "device", Method: requestMethod, Params: params})
	if err != nil {
		t.Fatalf("dispatch(%v): %v", method, err)
	}
	if tool, ok := method.(rpcpb.ClientTool); ok && resp.Error == nil {
		resp.Result, err = rpcapi.DecodeClientToolResponse(tool, resp.Result)
		if err != nil {
			t.Fatal(err)
		}
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
	var gotSound string
	var gotDuration, gotDelay, gotFindDuration *int64
	findCalls := 0
	var gotScanTimeout *int64
	var gotConnectSSID string
	var gotPassphrase *string
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		Status: func(context.Context) (rpcapi.PeerStatus, error) { return rpcapi.PeerStatus{Volume: new(50)}, nil },
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

	resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET, nil)
	status, err := resp.Result.AsClientDeviceStatusGetResponse()
	if err != nil || status.Value.Volume == nil || *status.Value.Volume != 50 {
		t.Fatalf("status = %+v, %v (resp %#v)", status, err, resp)
	}

	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "chime", DurationMs: new(int64(1500))})
	})
	if resp.Error != nil || gotSound != "chime" || gotDuration == nil || *gotDuration != 1500 {
		t.Fatalf("sound = %#v sound=%q duration=%v", resp, gotSound, gotDuration)
	}
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOUND_PLAY, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceSoundPlayRequest(rpcapi.ClientDeviceSoundPlayRequest{Sound: "unknown"})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("rejected sound = %#v", resp)
	}

	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFindRequest(rpcapi.ClientDeviceFindRequest{DurationMs: new(int64(8000))})
	})
	if resp.Error != nil || gotFindDuration == nil || *gotFindDuration != 8000 {
		t.Fatalf("find = %#v duration=%v", resp, gotFindDuration)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, nil); resp.Error != nil || gotFindDuration != nil {
		t.Fatalf("find without params = %#v duration=%v", resp, gotFindDuration)
	}
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFindRequest(rpcapi.ClientDeviceFindRequest{DurationMs: new(int64(-1))})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument || findCalls != 2 {
		t.Fatalf("negative find duration = %#v after %d calls", resp, findCalls)
	}

	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceRebootRequest(rpcapi.ClientDeviceRebootRequest{DelayMs: new(int64(2000))})
	})
	if resp.Error != nil || gotDelay == nil || *gotDelay != 2000 {
		t.Fatalf("reboot = %#v delay=%v", resp, gotDelay)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, nil); resp.Error != nil || gotDelay != nil {
		t.Fatalf("reboot without params = %#v delay=%v", resp, gotDelay)
	}

	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST, nil)
	list, err := resp.Result.AsClientWifiSavedListResponse()
	if err != nil || len(list.Networks) != 2 || list.Networks[1].Ssid != "office" {
		t.Fatalf("saved = %+v, %v", list, err)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "office"})
	}); resp.Error != nil {
		t.Fatalf("forget = %#v", resp)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "missing"})
	}); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("forget missing = %#v", resp)
	}
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiScanRequest(rpcapi.ClientWifiScanRequest{TimeoutMs: new(int64(8000))})
	})
	scan, err := resp.Result.AsClientWifiScanResponse()
	if err != nil || len(scan.Networks) != 1 || scan.Networks[0].Ssid != "office" || gotScanTimeout == nil || *gotScanTimeout != 8000 {
		t.Fatalf("scan = %+v, %v timeout=%v", scan, err, gotScanTimeout)
	}
	passphrase := "correct-horse"
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_CONNECT, func(p *rpcapi.RPCPayload) error {
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
	for _, method := range []rpcpb.ClientTool{
		rpcpb.ClientTool_CLIENT_TOOL_DEVICE_STATUS_GET, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST,
		rpcpb.ClientTool_CLIENT_TOOL_WIFI_SCAN,
	} {
		resp := deviceControlDispatch(t, &Client{}, method, nil)
		if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
			t.Fatalf("%s without handlers = %#v", method, resp)
		}
	}
	// Partial handlers: only the missing method is unsupported.
	device := &Client{}
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		SavedWifi: func(context.Context) ([]rpcapi.WifiSavedNetwork, error) { return nil, errors.New("radio off") },
		ForgetWifi: func(context.Context, string) error {
			return rpcapi.Error{Code: rpcapi.StatusCodeUnavailable, Message: "busy"}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("reboot without handler = %#v", resp)
	}
	resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_LIST, nil)
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInternal || resp.Error.Message == "radio off" {
		t.Fatalf("handler failure must be redacted INTERNAL_ERROR: %#v", resp)
	}
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: "home"})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnavailable || resp.Error.Message != "busy" {
		t.Fatalf("typed rpc error must pass through: %#v", resp)
	}
	for _, ssid := range []string{"", "123456789012345678901234567890123"} {
		resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientWifiSavedForgetRequest(rpcapi.ClientWifiSavedForgetRequest{Ssid: ssid})
		})
		if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
			t.Fatalf("ssid %q = %#v", ssid, resp)
		}
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_WIFI_SAVED_FORGET, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("forget without params = %#v", resp)
	}
	if resp, err := (&rpcClient{}).dispatch(context.Background(), &rpcapi.RPCRequest{Id: "x", Method: rpcapi.RPCMethodClientToolV0Invoke}); err != nil || resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
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
	resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Channel: &beta, Sha256: &digest})
	})
	if resp.Error != nil || gotChannel == nil || *gotChannel != beta || gotSha256 == nil || *gotSha256 != digest {
		t.Fatalf("update = %#v channel=%v sha256=%v", resp, gotChannel, gotSha256)
	}

	// Omitted params leave both choices to the device.
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, nil); resp.Error != nil || gotChannel != nil || gotSha256 != nil {
		t.Fatalf("update without params = %#v channel=%v sha256=%v", resp, gotChannel, gotSha256)
	}

	// An unspecified channel encodes on the wire but names no channel.
	unspecified := rpcapi.FirmwareChannelName("unspecified")
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Channel: &unspecified})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("unspecified channel = %#v", resp)
	}

	// A digest the device does not resolve is rejected, not failed.
	other := "b1c2d3e4f5061728394a5b6c7d8e9f0ab1c2d3e4f5061728394a5b6c7d8e9f0a"
	resp = deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, func(p *rpcapi.RPCPayload) error {
		return p.FromClientFirmwareUpdateRequest(rpcapi.ClientFirmwareUpdateRequest{Sha256: &other})
	})
	if resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("mismatched digest = %#v", resp)
	}

	// Firmware without the provider answers METHOD_NOT_FOUND.
	if resp := deviceControlDispatch(t, &Client{}, rpcpb.ClientTool_CLIENT_TOOL_FIRMWARE_UPDATE, nil); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("update without handler = %#v", resp)
	}
}

// The settings patch is validated before any handler runs, and the capability
// list is derived from the handlers that are actually installed.
func TestDeviceToolHandlersAndCapabilityList(t *testing.T) {
	handlers := DeviceControlHandlers{Reboot: func(context.Context, *int64) error { return nil }, FactoryReset: func(context.Context, bool) error { return nil }}
	want := []rpcpb.ClientTool{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_REBOOT, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET}
	if got := handlers.supportedTools(); !reflect.DeepEqual(got, want) {
		t.Fatalf("tools: %v want %v", got, want)
	}
	if got := (*DeviceControlHandlers)(nil).supportedTools(); len(got) != 0 {
		t.Fatalf("empty tools: %v", got)
	}
}

func TestRPCClientSocialPingHandler(t *testing.T) {
	device := &Client{}
	ping := func(p *rpcapi.RPCPayload) error {
		return p.FromClientSocialPingRequest(rpcapi.ClientSocialPingRequest{FromPeerPublicKey: "friend", FriendGroupName: new("team")})
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING, ping); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("ping without handler = %#v", resp)
	}
	var got rpcapi.ClientSocialPingRequest
	if err := device.HandleSocialPing(func(_ context.Context, request rpcapi.ClientSocialPingRequest) error {
		got = request
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING, ping); resp.Error != nil || got.FromPeerPublicKey != "friend" || got.FriendGroupName == nil || *got.FriendGroupName != "team" {
		t.Fatalf("ping = %#v got=%+v", resp, got)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING, func(p *rpcapi.RPCPayload) error {
		return p.FromClientSocialPingRequest(rpcapi.ClientSocialPingRequest{})
	}); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatalf("ping without sender = %#v", resp)
	}
	if err := device.HandleSocialPing(func(context.Context, rpcapi.ClientSocialPingRequest) error { return errors.New("busy") }); err != nil {
		t.Fatal(err)
	}
	if resp := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_SOCIAL_PING, ping); resp.Error == nil || resp.Error.Code != rpcapi.StatusCodeInternal {
		t.Fatalf("failing handler = %#v", resp)
	}
}

func TestRPCClientMhsSettingsDispatchAndDiscovery(t *testing.T) {
	device := &Client{}
	applied := 0
	reset := false
	if err := device.HandleDeviceControl(DeviceControlHandlers{
		ReadMhsStates: func(_ context.Context, request *rpcpb.ClientMhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadResponse, error) {
			return &rpcpb.ClientMhsV0ReadResponse{States: []*rpcpb.MhsStateValue{{DeviceId: request.States[0].DeviceId, State: request.States[0].State, Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: 30}}}}}, nil
		},
		WriteMhsStates: func(_ context.Context, request *rpcpb.ClientMhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteResponse, error) {
			if request.States[0].Value.GetIntValue() > 100 {
				return nil, ErrDeviceRejected
			}
			applied++
			return &rpcpb.ClientMhsV0WriteResponse{States: request.States}, nil
		},
		FactoryReset: func(_ context.Context, keep bool) error { reset = keep; return nil },
		Find:         func(context.Context, *int64) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	read := deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Read, func(p *rpcapi.RPCPayload) error {
		return p.FromClientMhsV0ReadRequest(&rpcpb.ClientMhsV0ReadRequest{States: []*rpcpb.MhsStateRef{{DeviceId: "display.main", State: "brightness"}}})
	})
	got, err := read.Result.AsClientMhsV0ReadResponse()
	if err != nil || got.States[0].Value.GetIntValue() != 30 {
		t.Fatalf("read %v %v", got, err)
	}
	for _, level := range []int64{0, 40, 140} {
		response := deviceControlDispatch(t, device, rpcapi.RPCMethodClientMhsV0Write, func(p *rpcapi.RPCPayload) error {
			return p.FromClientMhsV0WriteRequest(&rpcpb.ClientMhsV0WriteRequest{States: []*rpcpb.MhsStateValue{{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: level}}}}})
		})
		if level > 100 {
			if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
				t.Fatal("invalid brightness accepted")
			}
		} else if response.Error != nil {
			t.Fatal(response.Error)
		}
	}
	if applied != 2 {
		t.Fatalf("applied %d", applied)
	}
	response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FACTORY_RESET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFactoryResetRequest(rpcapi.ClientDeviceFactoryResetRequest{KeepNetwork: new(true)})
	})
	if response.Error != nil || !reset {
		t.Fatal("reset lost keep_network")
	}
	list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	tools, err := list.Result.AsClientToolV0ListResponse()
	want := []rpcpb.ClientTool{1, 2, 5, 6}
	if err != nil || !reflect.DeepEqual(tools.Tools, want) {
		t.Fatalf("tools %v %v", tools, err)
	}
	methods := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsList, nil)
	families, err := methods.Result.AsClientRpcMethodsListResponse()
	if err != nil || !reflect.DeepEqual(families.Methods, []rpcpb.RpcMethod{1, 2, 133, 134, 135, 136, 137}) {
		t.Fatalf("methods %v %v", families, err)
	}
}

func TestRPCClientCapabilityListWithoutDeviceHandlers(t *testing.T) {
	device := &Client{}
	list := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	tools, err := list.Result.AsClientToolV0ListResponse()
	if err != nil || !reflect.DeepEqual(tools.Tools, []rpcpb.ClientTool{1, 2}) {
		t.Fatalf("tools %v %v", tools, err)
	}
	methods := deviceControlDispatch(t, device, rpcapi.RPCMethodClientRPCMethodsList, nil)
	families, err := methods.Result.AsClientRpcMethodsListResponse()
	if err != nil || !reflect.DeepEqual(families.Methods, []rpcpb.RpcMethod{1, 2, 135, 136, 137}) {
		t.Fatalf("methods %v %v", families, err)
	}
}
