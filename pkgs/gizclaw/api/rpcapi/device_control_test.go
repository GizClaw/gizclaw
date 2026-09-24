package rpcapi

import (
	"reflect"
	"strings"
	"testing"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestDeviceControlMethodRegistry(t *testing.T) {
	want := map[RPCMethod]int32{RPCMethodClientMhsV0Read: 133, RPCMethodClientMhsV0Write: 134, RPCMethodClientToolV0Invoke: 135, RPCMethodClientToolV0List: 136, RPCMethodClientRPCMethodsList: 137}
	for method, id := range want {
		if !method.Valid() || int32(rpcMethodToProto[method]) != id {
			t.Fatalf("%s does not map to %d", method, id)
		}
	}
	for _, id := range []int32{3, 4, 82, 100, 101, 102, 103, 104, 105, 106, 108, 109, 111, 113, 114, 115, 116, 117, 118, 119, 126, 127, 128, 129, 130, 131, 132} {
		if _, err := MethodFromProto(rpcpb.RpcMethod(id)); err == nil {
			t.Fatalf("retired method %d accepted", id)
		}
	}
}

func TestDeviceControlPayloadRoundTrip(t *testing.T) {
	reportedAt := time.Unix(1_700_000_000, 0).UTC()
	status := PeerStatus{Volume: new(35), Muted: new(true), BatteryPercent: new(80), ReportedAt: &reportedAt}

	var payload RPCPayload
	if err := payload.FromClientDeviceStatusGetResponse(ClientDeviceStatusGetResponse{Value: status}); err != nil {
		t.Fatal(err)
	}
	statusResponse, err := payload.AsClientDeviceStatusGetResponse()
	if err != nil || *statusResponse.Value.Volume != 35 || !*statusResponse.Value.Muted || *statusResponse.Value.BatteryPercent != 80 || !statusResponse.Value.ReportedAt.Equal(reportedAt) {
		t.Fatalf("status response round trip = %+v, %v", statusResponse, err)
	}

	if err := payload.FromClientDeviceSoundPlayRequest(ClientDeviceSoundPlayRequest{Sound: "chime", DurationMs: new(int64(1500))}); err != nil {
		t.Fatal(err)
	}
	sound, err := payload.AsClientDeviceSoundPlayRequest()
	if err != nil || sound.Sound != "chime" || sound.DurationMs == nil || *sound.DurationMs != 1500 {
		t.Fatalf("sound request round trip = %+v, %v", sound, err)
	}
	if err := payload.FromClientDeviceSoundPlayRequest(ClientDeviceSoundPlayRequest{Sound: "chime"}); err != nil {
		t.Fatal(err)
	}
	if sound, err := payload.AsClientDeviceSoundPlayRequest(); err != nil || sound.DurationMs != nil {
		t.Fatalf("sound request without duration = %+v, %v", sound, err)
	}

	if err := payload.FromClientDeviceRebootRequest(ClientDeviceRebootRequest{DelayMs: new(int64(2000))}); err != nil {
		t.Fatal(err)
	}
	reboot, err := payload.AsClientDeviceRebootRequest()
	if err != nil || reboot.DelayMs == nil || *reboot.DelayMs != 2000 {
		t.Fatalf("reboot request round trip = %+v, %v", reboot, err)
	}

	if err := payload.FromClientWifiSavedListResponse(ClientWifiSavedListResponse{Networks: []WifiSavedNetwork{{Ssid: "home"}, {Ssid: "office"}}}); err != nil {
		t.Fatal(err)
	}
	saved, err := payload.AsClientWifiSavedListResponse()
	if err != nil || len(saved.Networks) != 2 || saved.Networks[1].Ssid != "office" {
		t.Fatalf("saved list round trip = %+v, %v", saved, err)
	}

	if err := payload.FromClientWifiSavedForgetRequest(ClientWifiSavedForgetRequest{Ssid: "office"}); err != nil {
		t.Fatal(err)
	}
	forget, err := payload.AsClientWifiSavedForgetRequest()
	if err != nil || forget.Ssid != "office" {
		t.Fatalf("forget request round trip = %+v, %v", forget, err)
	}

	if err := payload.FromClientWifiScanRequest(ClientWifiScanRequest{TimeoutMs: new(int64(8000))}); err != nil {
		t.Fatal(err)
	}
	scanRequest, err := payload.AsClientWifiScanRequest()
	if err != nil || scanRequest.TimeoutMs == nil || *scanRequest.TimeoutMs != 8000 {
		t.Fatalf("scan request round trip = %+v, %v", scanRequest, err)
	}
	scanResult := WifiScanResult{Ssid: "home", Bssid: new("aa:bb:cc:dd:ee:ff"), RssiDbm: new(int64(-48)), FrequencyMhz: new(int64(5180)), Security: new("wpa3")}
	if err := payload.FromClientWifiScanResponse(ClientWifiScanResponse{Networks: []WifiScanResult{scanResult}}); err != nil {
		t.Fatal(err)
	}
	scanResponse, err := payload.AsClientWifiScanResponse()
	if err != nil || len(scanResponse.Networks) != 1 || scanResponse.Networks[0].Ssid != "home" || *scanResponse.Networks[0].FrequencyMhz != 5180 {
		t.Fatalf("scan response round trip = %+v, %v", scanResponse, err)
	}

	passphrase := "correct-horse"
	if err := payload.FromClientWifiConnectRequest(ClientWifiConnectRequest{Ssid: "home", Passphrase: &passphrase}); err != nil {
		t.Fatal(err)
	}
	connect, err := payload.AsClientWifiConnectRequest()
	if err != nil || connect.Ssid != "home" || connect.Passphrase == nil || *connect.Passphrase != passphrase {
		t.Fatalf("connect request round trip = %+v, %v", connect, err)
	}

	empty := []struct {
		name   string
		encode func() error
		decode func() error
	}{
		{"status request", func() error { return payload.FromClientDeviceStatusGetRequest(ClientDeviceStatusGetRequest{}) }, func() error { _, err := payload.AsClientDeviceStatusGetRequest(); return err }},
		{"sound response", func() error { return payload.FromClientDeviceSoundPlayResponse(ClientDeviceSoundPlayResponse{}) }, func() error { _, err := payload.AsClientDeviceSoundPlayResponse(); return err }},
		{"reboot response", func() error { return payload.FromClientDeviceRebootResponse(ClientDeviceRebootResponse{}) }, func() error { _, err := payload.AsClientDeviceRebootResponse(); return err }},
		{"saved request", func() error { return payload.FromClientWifiSavedListRequest(ClientWifiSavedListRequest{}) }, func() error { _, err := payload.AsClientWifiSavedListRequest(); return err }},
		{"forget response", func() error { return payload.FromClientWifiSavedForgetResponse(ClientWifiSavedForgetResponse{}) }, func() error { _, err := payload.AsClientWifiSavedForgetResponse(); return err }},
		{"connect response", func() error { return payload.FromClientWifiConnectResponse(ClientWifiConnectResponse{}) }, func() error { _, err := payload.AsClientWifiConnectResponse(); return err }},
	}
	for _, tc := range empty {
		if err := tc.encode(); err != nil {
			t.Fatalf("%s encode: %v", tc.name, err)
		}
		if err := tc.decode(); err != nil {
			t.Fatalf("%s decode: %v", tc.name, err)
		}
	}
}

func TestDeviceControlToolPayloadNames(t *testing.T) {
	values := rpcpb.ClientTool(0).Descriptor().Values()
	for i := 1; i < values.Len(); i++ {
		tool := rpcpb.ClientTool(values.Get(i).Number())
		metadata, err := ClientToolMetadata(tool)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{metadata.Request, metadata.Response} {
			if _, err := newRPCPayloadMessage(name); err != nil {
				t.Fatal(err)
			}
		}
		request, err := ClientToolRequestFromBytes(tool, nil)
		if err != nil {
			t.Fatal(err)
		}
		var inner RPCPayload
		inner = *newRPCPayload(metadata.Request, nil, false)
		wrapped, err := EncodeClientToolRequest(tool, &inner)
		if err != nil {
			t.Fatal(err)
		}
		invocation, err := wrapped.AsClientToolV0InvokeRequest()
		if err != nil || invocation.Tool != tool {
			t.Fatalf("tool %v envelope: %v %v", tool, invocation, err)
		}
		decoded, err := ClientToolRequestFromBytes(invocation.Tool, invocation.Payload)
		if err != nil || !proto.Equal(request, decoded) {
			t.Fatalf("tool %v request: %v", tool, err)
		}
		response, err := ClientToolResponseMessage(tool, nil)
		if err != nil {
			t.Fatal(err)
		}
		inner = *newRPCPayload(metadata.Response, nil, true)
		result, err := EncodeClientToolResponse(tool, &inner)
		if err != nil {
			t.Fatal(err)
		}
		unwrapped, err := DecodeClientToolResponse(tool, result)
		if err != nil {
			t.Fatal(err)
		}
		data, err := unwrapped.bytesForMessage(metadata.Response)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ClientToolResponseMessage(tool, data)
		if err != nil || !proto.Equal(response, got) {
			t.Fatalf("tool %v response: %v", tool, err)
		}
	}
	var inner RPCPayload
	if err := inner.FromClientDeviceSoundPlayRequest(ClientDeviceSoundPlayRequest{Sound: "chime"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inner.AsClientWifiSavedForgetRequest(); err == nil {
		t.Fatal("mismatched payload type accepted")
	}
	if _, err := ClientToolRequestFromBytes(rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FIND, []byte{8, 128}); err == nil {
		t.Fatal("malformed tool payload accepted")
	}
}

func TestOTAStatusRPCRoundTrip(t *testing.T) {
	want := &rpcpb.PeerOtaStatus{State: "downloading", UpdateId: "one", ObservedAt: "2026-09-06T00:00:00Z", DownloadPercent: new(0.0), TargetVersion: new("2.0")}
	var payload RPCPayload
	if err := payload.FromServerGetStatusResponse(PeerStatus{Ota: want}); err != nil {
		t.Fatal(err)
	}
	got, err := payload.AsServerGetStatusResponse()
	if err != nil || !proto.Equal(got.Ota, want) {
		t.Fatalf("OTA status RPC: %+v, %v", got, err)
	}
}

func TestDeviceCapabilityPayloadRoundTrip(t *testing.T) {
	var reset RPCPayload
	if err := reset.FromClientDeviceFactoryResetRequest(ClientDeviceFactoryResetRequest{KeepNetwork: new(true)}); err != nil {
		t.Fatalf("FromClientDeviceFactoryResetRequest() error = %v", err)
	}
	gotReset, err := reset.AsClientDeviceFactoryResetRequest()
	if err != nil {
		t.Fatalf("AsClientDeviceFactoryResetRequest() error = %v", err)
	}
	if gotReset.KeepNetwork == nil || !*gotReset.KeepNetwork {
		t.Fatalf("KeepNetwork = %#v, want true", gotReset.KeepNetwork)
	}

	var methods RPCPayload
	want := []rpcpb.RpcMethod{rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ, rpcpb.RpcMethod_RPC_METHOD_CLIENT_TOOL_V0_INVOKE}
	if err := methods.FromClientRpcMethodsListResponse(&rpcpb.ClientRpcMethodsListResponse{Methods: want}); err != nil {
		t.Fatal(err)
	}
	got, err := methods.AsClientRpcMethodsListResponse()
	if err != nil || !reflect.DeepEqual(got.Methods, want) {
		t.Fatalf("methods: %v %v", got, err)
	}
}

func TestClientRunWorkspaceSetRequestValid(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  ClientRunWorkspaceSetRequest
		want bool
	}{
		{"named", ClientRunWorkspaceSetRequest{WorkspaceName: "chat"}, true},
		{"kickoff", ClientRunWorkspaceSetRequest{WorkspaceName: "chat", Kickoff: new(true)}, true},
		{"empty", ClientRunWorkspaceSetRequest{}, false},
		{"too long", ClientRunWorkspaceSetRequest{WorkspaceName: strings.Repeat("w", 257)}, false},
	} {
		if got := tc.req.Valid(); got != tc.want {
			t.Fatalf("%s: Valid() = %v, want %v", tc.name, got, tc.want)
		}
	}
	var payload RPCPayload
	if err := payload.FromClientRunWorkspaceSetRequest(ClientRunWorkspaceSetRequest{WorkspaceName: "bedtime", Kickoff: new(true)}); err != nil {
		t.Fatal(err)
	}
	got, err := payload.AsClientRunWorkspaceSetRequest()
	if err != nil || got.WorkspaceName != "bedtime" || got.Kickoff == nil || !*got.Kickoff {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

// Product-defined settings retain explicit false, zero, string and integer values.
func TestMhsSettingsAndWifiPayloadRoundTrip(t *testing.T) {
	values := []*rpcpb.MhsStateValue{}
	for key, value := range map[string]*rpcpb.MhsValue{
		"volume":                {Value: &rpcpb.MhsValue_IntValue{IntValue: 35}},
		"muted":                 {Value: &rpcpb.MhsValue_BoolValue{BoolValue: true}},
		"cellular.enabled":      {Value: &rpcpb.MhsValue_BoolValue{BoolValue: true}},
		"screen.off-timeout-ms": {Value: &rpcpb.MhsValue_IntValue{IntValue: 30000}},
		"screen.brightness":     {Value: &rpcpb.MhsValue_IntValue{IntValue: 60}},
		"led.brightness":        {Value: &rpcpb.MhsValue_IntValue{IntValue: 20}},
		"locale":                {Value: &rpcpb.MhsValue_StringValue{StringValue: "zh-CN"}},
		"interaction.mode":      {Value: &rpcpb.MhsValue_StringValue{StringValue: "push-to-talk"}},
		"key.feedback":          {Value: &rpcpb.MhsValue_StringValue{StringValue: "sound-and-vibrate"}},
		"alert.mode":            {Value: &rpcpb.MhsValue_StringValue{StringValue: "vibrate"}},
		"sleep.timeout-ms":      {Value: &rpcpb.MhsValue_IntValue{IntValue: 0}},
		"nfc.enabled":           {Value: &rpcpb.MhsValue_BoolValue{BoolValue: false}},
		"wifi.connected":        {Value: &rpcpb.MhsValue_BoolValue{BoolValue: true}},
		"wifi.ssid":             {Value: &rpcpb.MhsValue_StringValue{StringValue: "home"}},
		"wifi.rssi-dbm":         {Value: &rpcpb.MhsValue_IntValue{IntValue: -55}},
		"wifi.ip":               {Value: &rpcpb.MhsValue_StringValue{StringValue: "192.0.2.10"}},
		"wifi.bssid":            {Value: &rpcpb.MhsValue_StringValue{StringValue: "aa:bb:cc:dd:ee:ff"}},
	} {
		values = append(values, &rpcpb.MhsStateValue{DeviceId: "device.main", State: key, Value: value})
	}
	request := &rpcpb.ClientMhsV0WriteRequest{States: values}
	var payload RPCPayload
	if err := payload.FromClientMhsV0WriteRequest(request); err != nil {
		t.Fatal(err)
	}
	got, err := payload.AsClientMhsV0WriteRequest()
	if err != nil || !proto.Equal(request, got) {
		t.Fatalf("MHS values: %v %v", got, err)
	}
}
