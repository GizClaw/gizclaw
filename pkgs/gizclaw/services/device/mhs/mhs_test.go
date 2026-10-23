package mhs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/api"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func testManifest() apitypes.MhsV0Manifest {
	return apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{
		{Id: "led.left", Hwd: "led"},
		{Id: "led.right", Hwd: "led"},
		{Id: "battery.main", Hwd: "battery"},
		{Id: "display.main", Hwd: "display"},
	}}
}

func TestValidateManifest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*apitypes.MhsV0Manifest)
	}{
		{"nil devices", func(m *apitypes.MhsV0Manifest) { m.Devices = nil }},
		{"duplicate id", func(m *apitypes.MhsV0Manifest) { m.Devices[1].Id = m.Devices[0].Id }},
		{"invalid id", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Id = "LED.Left" }},
		{"long id", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Id = strings.Repeat("x", 65) }},
		{"unknown HWD", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Hwd = "device" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := testManifest()
			tc.mutate(&manifest)
			if err := ValidateManifest(manifest); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	if err := ValidateManifest(testManifest()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{}}); err != nil {
		t.Fatal(err)
	}
}

func TestHwdOpenAPIAndProtobufRegistryAgree(t *testing.T) {
	data, err := api.Files.ReadFile("http/shared/mhs_v0.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
				Discriminator struct {
					Mapping map[string]string `json:"mapping"`
				} `json:"discriminator"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	hwds := contract.Components.Schemas["MhsV0Device"].Properties["hwd"].Enum
	writes := contract.Components.Schemas["MhsV0WriteRequest"].Discriminator.Mapping
	if len(hwds) != rpcpb.ClientHwd(0).Descriptor().Values().Len()-1 || len(writes) != 5 {
		t.Fatalf("HWD counts differ: manifest=%d proto=%d writable=%d", len(hwds), rpcpb.ClientHwd(0).Descriptor().Values().Len()-1, len(writes))
	}
	for _, name := range hwds {
		hwd, err := rpcapi.ClientHwdByName(name)
		if err != nil {
			t.Fatal(err)
		}
		meta, err := rpcapi.ClientHwdMetadata(hwd)
		if err != nil {
			t.Fatal(err)
		}
		_, inWrites := writes[name]
		if (meta.WriteRequest != "") != inWrites {
			t.Fatalf("%s writability differs across contracts", name)
		}
	}
}

func TestReadInstanceAndType(t *testing.T) {
	for _, id := range []string{"led.left", "led.right"} {
		request, err := ReadRequest(testManifest(), apitypes.MhsV0ReadRequest{Id: id, Hwd: "led"})
		if err != nil || request.Id != id || request.Hwd != rpcpb.ClientHwd_CLIENT_HWD_LED {
			t.Fatalf("read %s: %v, %v", id, request, err)
		}
	}
	for _, request := range []apitypes.MhsV0ReadRequest{
		{Id: "led.left", Hwd: "battery"},
		{Id: "led.missing", Hwd: "led"},
		{Id: "led.left", Hwd: "device"},
	} {
		if _, err := ReadRequest(testManifest(), request); err == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
}

func displayWrite(t *testing.T, value apitypes.DisplayHwdWriteRequest) apitypes.MhsV0WriteRequest {
	t.Helper()
	var request apitypes.MhsV0WriteRequest
	if err := request.FromMhsV0DisplayWriteRequest(apitypes.MhsV0DisplayWriteRequest{Id: "display.main", Hwd: "display", Value: value}); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestTypedWriteAndReadOnlyBoundary(t *testing.T) {
	request, err := WriteRequest(testManifest(), displayWrite(t, apitypes.DisplayHwdWriteRequest{BrightnessPercent: proto.Int64(0), Enabled: new(false)}))
	if err != nil {
		t.Fatal(err)
	}
	var payload rpcpb.DisplayHwdWriteRequest
	if err := proto.Unmarshal(request.Payload, &payload); err != nil || payload.BrightnessPercent == nil || *payload.BrightnessPercent != 0 || payload.Enabled == nil || *payload.Enabled {
		t.Fatalf("lost zero or false presence: %+v, %v", &payload, err)
	}
	for _, value := range []apitypes.DisplayHwdWriteRequest{{}, {BrightnessPercent: proto.Int64(101)}} {
		if _, err := WriteRequest(testManifest(), displayWrite(t, value)); err == nil {
			t.Fatalf("accepted invalid write %+v", value)
		}
	}
	var readOnly apitypes.MhsV0WriteRequest
	if err := json.Unmarshal([]byte(`{"id":"battery.main","hwd":"battery","value":{"percent":50}}`), &readOnly); err == nil {
		if _, err := WriteRequest(testManifest(), readOnly); err == nil {
			t.Fatal("accepted battery write")
		}
	}
}

func TestReadAndWriteResponse(t *testing.T) {
	read := &rpcpb.ClientMhsV0ReadRequest{Id: "display.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_DISPLAY}
	readPayload, err := proto.Marshal(&rpcpb.DisplayHwdReadResponse{BrightnessPercent: proto.Uint32(0), Enabled: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ReadResponse(read, &rpcpb.ClientMhsV0ReadResponse{Payload: readPayload})
	if err != nil {
		t.Fatal(err)
	}
	value, err := result.AsMhsV0DisplayReadResult()
	if err != nil || value.Value.BrightnessPercent == nil || *value.Value.BrightnessPercent != 0 || value.Value.Enabled == nil || *value.Value.Enabled {
		t.Fatalf("read result: %+v, %v", value, err)
	}
	if _, err := ReadResponse(read, &rpcpb.ClientMhsV0ReadResponse{}); err == nil {
		t.Fatal("accepted missing read payload")
	}
	writePayload, err := proto.Marshal(&rpcpb.DisplayHwdWriteResponse{Applied: &rpcpb.DisplayHwdReadResponse{BrightnessPercent: proto.Uint32(85)}})
	if err != nil {
		t.Fatal(err)
	}
	write, err := WriteResponse(&rpcpb.ClientMhsV0WriteRequest{Id: read.Id, Hwd: read.Hwd}, &rpcpb.ClientMhsV0WriteResponse{Payload: writePayload})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := write.AsMhsV0DisplayReadResult()
	if err != nil || applied.Value.BrightnessPercent == nil || *applied.Value.BrightnessPercent != 85 {
		t.Fatalf("write result: %+v, %v", applied, err)
	}
}

func TestModemAndNFCProtocolProjection(t *testing.T) {
	manifest := apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "nfc.left", Hwd: "nfc"}, {Id: "nfc.right", Hwd: "nfc"}, {Id: "modem.main", Hwd: "modem"}}}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"nfc.left", "nfc.right"} {
		request, err := ReadRequest(manifest, apitypes.MhsV0ReadRequest{Id: id, Hwd: "nfc"})
		if err != nil || request.Id != id {
			t.Fatal("instance identity lost")
		}
	}
	modem := &rpcpb.ClientMhsV0ReadRequest{Id: "modem.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_MODEM}
	payload, _ := proto.Marshal(&rpcpb.ModemHwdReadResponse{Enabled: new(false), Registered: new(false)})
	result, err := ReadResponse(modem, &rpcpb.ClientMhsV0ReadResponse{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	value, err := result.AsMhsV0ModemReadResult()
	if err != nil || value.Value.Enabled == nil || *value.Value.Enabled || value.Value.Powered != nil {
		t.Fatal("lost modem unknown vs false")
	}
	snapshot := &rpcpb.NfcReadSnapshot{ReadAtUnixMs: 1000, RawData: []byte{0, 255}, Result: "failed", ErrorCode: new("IO")}
	payload, _ = proto.Marshal(&rpcpb.NfcHwdReadResponse{CardPresent: new(false), LastRead: snapshot})
	nfc := &rpcpb.ClientMhsV0ReadRequest{Id: "nfc.left", Hwd: rpcpb.ClientHwd_CLIENT_HWD_NFC}
	result, err = ReadResponse(nfc, &rpcpb.ClientMhsV0ReadResponse{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	nfcValue, err := result.AsMhsV0NfcReadResult()
	if err != nil || nfcValue.Value.LastRead == nil || nfcValue.Value.CardPresent == nil || *nfcValue.Value.CardPresent || nfcValue.Value.LastRead.Result != "failed" {
		t.Fatal("NFC cached failure projection lost")
	}
	write := &rpcpb.ClientMhsV0WriteRequest{Id: "modem.main", Hwd: modem.Hwd}
	payload, _ = proto.Marshal(&rpcpb.ModemHwdWriteResponse{Applied: &rpcpb.ModemHwdReadResponse{Powered: new(false)}})
	if _, err = WriteResponse(write, &rpcpb.ClientMhsV0WriteResponse{Payload: payload}); err == nil {
		t.Fatal("missing applied switch accepted")
	}
}
