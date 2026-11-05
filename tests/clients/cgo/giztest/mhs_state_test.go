package main

import (
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"google.golang.org/protobuf/proto"
)

func instanceFixture(t *testing.T) any {
	t.Helper()
	doc, err := giztest.LoadDocument("../../../gizclaw-e2e/giztest/server.device.mhs.management.instances.giztest.yaml", driver{})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range doc.Steps {
		if step.ClientRPC != nil && step.ClientRPC.Method == "client.mhs.v0.read" {
			return step.ClientRPC.Response
		}
	}
	t.Fatal("instance document has no read provider")
	return nil
}

func instanceProvider(t *testing.T, fixture any) *clientRPCProvider {
	t.Helper()
	provider := newClientRPCProvider()
	for _, method := range []string{"client.mhs.v0.read", "client.mhs.v0.write"} {
		if err := provider.install(method, fixture); err != nil {
			t.Fatal(err)
		}
	}
	return provider
}

func mhsAnswer(t *testing.T, provider *clientRPCProvider, method rpcpb.RpcMethod, request proto.Message, wantCode rpcpb.StatusCode) []byte {
	t.Helper()
	payload, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	data, code, message, err := provider.answer(method, 0, payload)
	if err != nil || code != int32(wantCode) {
		t.Fatalf("method=%v code=%d message=%q err=%v, want %v", method, code, message, err, wantCode)
	}
	return data
}

func readInstance(t *testing.T, provider *clientRPCProvider, id string, hwd rpcpb.ClientHwd) proto.Message {
	t.Helper()
	data := mhsAnswer(t, provider, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ, &rpcpb.ClientMhsV0ReadRequest{Id: id, Hwd: hwd}, rpcpb.StatusCode_STATUS_CODE_OK)
	response := new(rpcpb.ClientMhsV0ReadResponse)
	if err := proto.Unmarshal(data, response); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(response.GetWriteCapabilities().GetFields(), []string{"enabled"}) {
		t.Fatalf("%s write capabilities = %v", id, response.WriteCapabilities)
	}
	value, err := rpcapi.ClientHwdReadResponseMessage(hwd, response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func writeInstance(t *testing.T, provider *clientRPCProvider, id string, hwd rpcpb.ClientHwd, fields map[string]any, wantCode rpcpb.StatusCode) []byte {
	t.Helper()
	request, err := rpcapi.ClientHwdWriteRequestMessage(hwd, fields)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return mhsAnswer(t, provider, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_WRITE, &rpcpb.ClientMhsV0WriteRequest{Id: id, Hwd: hwd, Payload: payload}, wantCode)
}

func TestMhsInstancesWriteReadAndIsolation(t *testing.T) {
	fixture := instanceFixture(t)
	provider := instanceProvider(t, fixture)
	other := instanceProvider(t, fixture)
	modemMain := &rpcpb.ModemHwdReadResponse{Enabled: new(true), Registered: new(false)}
	modemBackup := &rpcpb.ModemHwdReadResponse{Enabled: new(false), Powered: new(false)}
	nfcLeft := &rpcpb.NfcHwdReadResponse{Enabled: new(true), CardPresent: new(false), LastRead: &rpcpb.NfcReadSnapshot{
		ReadAtUnixMs: 1600000000000, Uid: []byte{0, 1}, Result: "failed", ErrorCode: new("READ_FAILED"),
	}}
	nfcRight := &rpcpb.NfcHwdReadResponse{Enabled: new(false), CardPresent: new(false), LastRead: &rpcpb.NfcReadSnapshot{
		ReadAtUnixMs: 1600000000001, Uid: []byte{2, 3}, Result: "success",
	}}
	instances := []struct {
		id    string
		hwd   rpcpb.ClientHwd
		value proto.Message
	}{
		{"modem.main", rpcpb.ClientHwd_CLIENT_HWD_MODEM, modemMain},
		{"modem.backup", rpcpb.ClientHwd_CLIENT_HWD_MODEM, modemBackup},
		{"nfc.left", rpcpb.ClientHwd_CLIENT_HWD_NFC, nfcLeft},
		{"nfc.right", rpcpb.ClientHwd_CLIENT_HWD_NFC, nfcRight},
	}
	for _, instance := range instances {
		if got := readInstance(t, provider, instance.id, instance.hwd); !proto.Equal(got, instance.value) {
			t.Fatalf("initial %s = %v, want %v", instance.id, got, instance.value)
		}
	}
	for _, instance := range []struct {
		id      string
		hwd     rpcpb.ClientHwd
		applied proto.Message
	}{
		{"modem.main", rpcpb.ClientHwd_CLIENT_HWD_MODEM, &rpcpb.ModemHwdWriteResponse{Applied: &rpcpb.ModemHwdReadResponse{Enabled: new(false), Registered: new(false)}}},
		{"nfc.left", rpcpb.ClientHwd_CLIENT_HWD_NFC, &rpcpb.NfcHwdWriteResponse{Applied: &rpcpb.NfcHwdReadResponse{Enabled: new(false), CardPresent: new(false), LastRead: proto.CloneOf(nfcLeft.LastRead)}}},
	} {
		data := writeInstance(t, provider, instance.id, instance.hwd, map[string]any{"enabled": false}, rpcpb.StatusCode_STATUS_CODE_OK)
		response := new(rpcpb.ClientMhsV0WriteResponse)
		if err := proto.Unmarshal(data, response); err != nil {
			t.Fatal(err)
		}
		applied, err := rpcapi.ClientHwdWriteResponseMessage(instance.hwd, response.Payload)
		if err != nil || !proto.Equal(applied, instance.applied) {
			t.Fatalf("write %s = %v err=%v, want %v", instance.id, applied, err, instance.applied)
		}
	}
	for _, instance := range instances {
		if got := readInstance(t, other, instance.id, instance.hwd); !proto.Equal(got, instance.value) {
			t.Fatalf("other client %s = %v, want %v", instance.id, got, instance.value)
		}
	}
	modemMain.Enabled = new(false)
	nfcLeft.Enabled = new(false)
	for _, instance := range instances {
		if got := readInstance(t, provider, instance.id, instance.hwd); !proto.Equal(got, instance.value) {
			t.Fatalf("after write %s = %v, want %v", instance.id, got, instance.value)
		}
	}
}

func TestMhsInstancesConcurrent(t *testing.T) {
	provider := instanceProvider(t, instanceFixture(t))
	for _, instance := range []struct {
		id  string
		hwd rpcpb.ClientHwd
	}{
		{"modem.main", rpcpb.ClientHwd_CLIENT_HWD_MODEM},
		{"modem.backup", rpcpb.ClientHwd_CLIENT_HWD_MODEM},
		{"nfc.left", rpcpb.ClientHwd_CLIENT_HWD_NFC},
		{"nfc.right", rpcpb.ClientHwd_CLIENT_HWD_NFC},
	} {
		t.Run(instance.id, func(t *testing.T) {
			t.Parallel()
			for i := range 16 {
				enabled := i%2 == 0
				writeInstance(t, provider, instance.id, instance.hwd, map[string]any{"enabled": enabled}, rpcpb.StatusCode_STATUS_CODE_OK)
				value := readInstance(t, provider, instance.id, instance.hwd).ProtoReflect()
				field := value.Descriptor().Fields().ByName("enabled")
				if !value.Has(field) || value.Get(field).Bool() != enabled {
					t.Fatalf("%s lost written enabled=%v: %v", instance.id, enabled, value)
				}
			}
		})
	}
}

func TestMhsInstancesRejectRequestsWithoutMutation(t *testing.T) {
	provider := instanceProvider(t, map[string]any{"instances": map[string]any{
		"led.status": map[string]any{"hwd": "led", "value": map[string]any{"enabled": true, "brightness_percent": 50}, "write_fields": []string{"enabled"}},
	}})
	for _, request := range []*rpcpb.ClientMhsV0ReadRequest{
		{Id: "led.missing", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED},
		{Id: "led.status", Hwd: rpcpb.ClientHwd_CLIENT_HWD_NFC},
	} {
		mhsAnswer(t, provider, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_READ, request, rpcpb.StatusCode_STATUS_CODE_NOT_FOUND)
	}
	writeInstance(t, provider, "led.missing", rpcpb.ClientHwd_CLIENT_HWD_LED, map[string]any{"enabled": false}, rpcpb.StatusCode_STATUS_CODE_NOT_FOUND)
	writeInstance(t, provider, "led.status", rpcpb.ClientHwd_CLIENT_HWD_NFC, map[string]any{"enabled": false}, rpcpb.StatusCode_STATUS_CODE_NOT_FOUND)
	writeInstance(t, provider, "led.status", rpcpb.ClientHwd_CLIENT_HWD_LED, map[string]any{"enabled": false, "brightness_percent": uint32(20)}, rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED)
	for _, payload := range [][]byte{nil, {0xff}} {
		mhsAnswer(t, provider, rpcpb.RpcMethod_RPC_METHOD_CLIENT_MHS_V0_WRITE, &rpcpb.ClientMhsV0WriteRequest{Id: "led.status", Hwd: rpcpb.ClientHwd_CLIENT_HWD_LED, Payload: payload}, rpcpb.StatusCode_STATUS_CODE_INVALID_ARGUMENT)
	}
	want := &rpcpb.LedHwdReadResponse{Enabled: new(true), BrightnessPercent: proto.Uint32(50)}
	if got := readInstance(t, provider, "led.status", rpcpb.ClientHwd_CLIENT_HWD_LED); !proto.Equal(got, want) {
		t.Fatalf("rejected write changed state to %v, want %v", got, want)
	}
}

func TestMhsInstancesRejectInvalidFixtures(t *testing.T) {
	for _, fixture := range []any{
		map[string]any{"instances": []any{}},
		map[string]any{"instances": map[string]any{"bad id": map[string]any{"hwd": "nfc"}}},
		map[string]any{"instances": map[string]any{"nfc.left": map[string]any{"hwd": "unknown"}}},
		map[string]any{"instances": map[string]any{"nfc.left": map[string]any{"hwd": "nfc", "value": map[string]any{"unknown": true}}}},
	} {
		provider := newClientRPCProvider()
		if err := provider.install("client.mhs.v0.read", fixture); err == nil {
			t.Fatalf("accepted invalid fixture: %v", fixture)
		}
		if provider.installed("client.mhs.v0.read") {
			t.Fatal("invalid fixture installed a provider")
		}
	}
}
