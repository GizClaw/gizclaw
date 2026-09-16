package rpcapi

import (
	"reflect"
	"testing"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestDeviceControlMethodRegistry(t *testing.T) {
	want := map[RPCMethod]int32{
		RPCMethodClientDeviceStatusGet: 100, RPCMethodClientDeviceVolumeSet: 101, RPCMethodClientDeviceSoundPlay: 102,
		RPCMethodClientDeviceReboot: 103, RPCMethodClientWifiStatusGet: 104, RPCMethodClientWifiSavedList: 105,
		RPCMethodClientWifiSavedForget: 106,
		RPCMethodClientWifiScan:        108,
		RPCMethodClientWifiConnect:     109,
	}
	for method, id := range want {
		if !method.Valid() {
			t.Fatalf("%s is not a valid method", method)
		}
		if got := int32(rpcMethodToProto[method]); got != id {
			t.Fatalf("%s id = %d, want %d", method, got, id)
		}
	}
}

func TestDeviceControlPayloadRoundTrip(t *testing.T) {
	reportedAt := time.Unix(1_700_000_000, 0).UTC()
	status := PeerStatus{Volume: new(35), Muted: new(true), BatteryPercent: new(80), ReportedAt: &reportedAt}

	var payload RPCPayload
	if err := payload.FromClientDeviceVolumeSetRequest(ClientDeviceVolumeSetRequest{Level: 35, Muted: true}); err != nil {
		t.Fatal(err)
	}
	var wire rpcpb.ClientDeviceVolumeSetRequest
	if err := proto.Unmarshal(payload.payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.GetLevel() != 35 || !wire.GetMuted() {
		t.Fatalf("wire volume request = %+v", &wire)
	}
	request, err := payload.AsClientDeviceVolumeSetRequest()
	if err != nil || request.Level != 35 || !request.Muted {
		t.Fatalf("volume request round trip = %+v, %v", request, err)
	}

	if err := payload.FromClientDeviceVolumeSetResponse(ClientDeviceVolumeSetResponse{Value: status}); err != nil {
		t.Fatal(err)
	}
	volumeResponse, err := payload.AsClientDeviceVolumeSetResponse()
	if err != nil || *volumeResponse.Value.Volume != 35 || !*volumeResponse.Value.Muted || *volumeResponse.Value.BatteryPercent != 80 || !volumeResponse.Value.ReportedAt.Equal(reportedAt) {
		t.Fatalf("volume response round trip = %+v, %v", volumeResponse, err)
	}

	if err := payload.FromClientDeviceStatusGetResponse(ClientDeviceStatusGetResponse{Value: status}); err != nil {
		t.Fatal(err)
	}
	statusResponse, err := payload.AsClientDeviceStatusGetResponse()
	if err != nil || *statusResponse.Value.Volume != 35 {
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

	wifi := WifiStatus{Connected: true, Ssid: new("home"), RssiDbm: new(int64(-55)), Ip: new("192.0.2.10"), Bssid: new("aa:bb:cc:dd:ee:ff")}
	if err := payload.FromClientWifiStatusGetResponse(ClientWifiStatusGetResponse{Value: wifi}); err != nil {
		t.Fatal(err)
	}
	wifiResponse, err := payload.AsClientWifiStatusGetResponse()
	if err != nil || !wifiResponse.Value.Connected || *wifiResponse.Value.Ssid != "home" || *wifiResponse.Value.RssiDbm != -55 || *wifiResponse.Value.Ip != "192.0.2.10" || *wifiResponse.Value.Bssid != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("wifi status round trip = %+v, %v", wifiResponse, err)
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
		{"wifi request", func() error { return payload.FromClientWifiStatusGetRequest(ClientWifiStatusGetRequest{}) }, func() error { _, err := payload.AsClientWifiStatusGetRequest(); return err }},
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

func TestDeviceControlMethodPayloadNames(t *testing.T) {
	want := map[RPCMethod][2]string{
		RPCMethodClientDeviceStatusGet: {"ClientDeviceStatusGetRequest", "ClientDeviceStatusGetResponse"},
		RPCMethodClientDeviceVolumeSet: {"ClientDeviceVolumeSetRequest", "ClientDeviceVolumeSetResponse"},
		RPCMethodClientDeviceSoundPlay: {"ClientDeviceSoundPlayRequest", "ClientDeviceSoundPlayResponse"},
		RPCMethodClientDeviceReboot:    {"ClientDeviceRebootRequest", "ClientDeviceRebootResponse"},
		RPCMethodClientWifiStatusGet:   {"ClientWifiStatusGetRequest", "ClientWifiStatusGetResponse"},
		RPCMethodClientWifiSavedList:   {"ClientWifiSavedListRequest", "ClientWifiSavedListResponse"},
		RPCMethodClientWifiSavedForget: {"ClientWifiSavedForgetRequest", "ClientWifiSavedForgetResponse"},
		RPCMethodClientWifiScan:        {"ClientWifiScanRequest", "ClientWifiScanResponse"},
		RPCMethodClientWifiConnect:     {"ClientWifiConnectRequest", "ClientWifiConnectResponse"},
	}
	for method, names := range want {
		if got := rpcRequestPayloadMessages[method]; got != names[0] {
			t.Fatalf("%s request message = %q, want %q", method, got, names[0])
		}
		if got := rpcResponsePayloadMessages[method]; got != names[1] {
			t.Fatalf("%s response message = %q, want %q", method, got, names[1])
		}
		// Every registered message must be resolvable by the dynamic codec.
		for _, name := range names {
			if _, err := newRPCPayloadMessage(name); err != nil {
				t.Fatalf("message %s: %v", name, err)
			}
		}
	}
	// A payload encoded for one message is rejected when decoded as another.
	var payload RPCPayload
	if err := payload.FromClientDeviceVolumeSetRequest(ClientDeviceVolumeSetRequest{Level: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := payload.AsClientWifiSavedForgetRequest(); err == nil {
		t.Fatal("decoding a volume request as a forget request must fail")
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

// The four device-configuration methods round-trip through the dynamic payload
// codec, which is what proves their proto messages and registry names agree.
func TestDeviceSettingsAndCapabilityPayloadRoundTrip(t *testing.T) {
	mode := DeviceInteractionModePushToTalk
	feedback := DeviceKeyFeedbackSoundAndVibrate
	settings := DeviceSettings{
		CellularEnabled:        new(true),
		ScreenOffTimeoutMs:     new(int64(30000)),
		ScreenBrightness:       new(int64(60)),
		LedBrightness:          new(int64(20)),
		Locale:                 new("zh-CN"),
		DefaultInteractionMode: &mode,
		KeyFeedback:            &feedback,
	}

	var setReq RPCPayload
	if err := setReq.FromClientDeviceSettingsSetRequest(ClientDeviceSettingsSetRequest{Value: settings}); err != nil {
		t.Fatalf("FromClientDeviceSettingsSetRequest() error = %v", err)
	}
	gotSet, err := setReq.AsClientDeviceSettingsSetRequest()
	if err != nil {
		t.Fatalf("AsClientDeviceSettingsSetRequest() error = %v", err)
	}
	if gotSet.Value.Locale == nil || *gotSet.Value.Locale != "zh-CN" {
		t.Fatalf("Locale = %#v, want zh-CN", gotSet.Value.Locale)
	}
	if gotSet.Value.CellularEnabled == nil || !*gotSet.Value.CellularEnabled {
		t.Fatalf("CellularEnabled = %#v, want true", gotSet.Value.CellularEnabled)
	}
	if gotSet.Value.DefaultInteractionMode == nil || *gotSet.Value.DefaultInteractionMode != mode {
		t.Fatalf("DefaultInteractionMode = %#v, want %v", gotSet.Value.DefaultInteractionMode, mode)
	}
	if gotSet.Value.KeyFeedback == nil || *gotSet.Value.KeyFeedback != feedback {
		t.Fatalf("KeyFeedback = %#v, want %v", gotSet.Value.KeyFeedback, feedback)
	}

	// An absent member stays absent, which is how a set request says "leave
	// this option alone" and a response says "this device has no such option".
	var getResp RPCPayload
	if err := getResp.FromClientDeviceSettingsGetResponse(ClientDeviceSettingsGetResponse{
		Value: DeviceSettings{ScreenBrightness: new(int64(10))},
	}); err != nil {
		t.Fatalf("FromClientDeviceSettingsGetResponse() error = %v", err)
	}
	gotGet, err := getResp.AsClientDeviceSettingsGetResponse()
	if err != nil {
		t.Fatalf("AsClientDeviceSettingsGetResponse() error = %v", err)
	}
	if gotGet.Value.Locale != nil || gotGet.Value.CellularEnabled != nil {
		t.Fatalf("absent members decoded as present: %+v", gotGet.Value)
	}

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
	want := []string{string(RPCMethodClientDeviceReboot), string(RPCMethodClientDeviceSettingsGet)}
	if err := methods.FromClientRPCMethodsGetResponse(ClientRPCMethodsGetResponse{Methods: want}); err != nil {
		t.Fatalf("FromClientRPCMethodsGetResponse() error = %v", err)
	}
	gotMethods, err := methods.AsClientRPCMethodsGetResponse()
	if err != nil {
		t.Fatalf("AsClientRPCMethodsGetResponse() error = %v", err)
	}
	if !reflect.DeepEqual(gotMethods.Methods, want) {
		t.Fatalf("Methods = %#v, want %#v", gotMethods.Methods, want)
	}
}
