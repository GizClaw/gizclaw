package main

import (
	"slices"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestMhsProviderValuesAndDiscovery(t *testing.T) {
	provider := newClientRPCProvider()
	info, err := lookupMethod("client.mhs.v0.read")
	if err != nil {
		t.Fatal(err)
	}
	_, code, _, err := provider.answer(info.id, nil)
	if err != nil || code != int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED) {
		t.Fatalf("uninstalled code=%d err=%v", code, err)
	}
	response := map[string]any{"states": []any{
		map[string]any{"device_id": "led.main", "state": "enabled", "value": map[string]any{"bool_value": false}},
		map[string]any{"device_id": "display.main", "state": "brightness", "value": map[string]any{"int_value": 40}},
	}}
	if err := provider.install("client.mhs.v0.read", response); err != nil {
		t.Fatal(err)
	}
	data, code, _, err := provider.answer(info.id, nil)
	if err != nil || code != 0 {
		t.Fatalf("read code=%d err=%v", code, err)
	}
	read := new(rpcpb.ClientMhsV0ReadResponse)
	if err := proto.Unmarshal(data, read); err != nil {
		t.Fatal(err)
	}
	if len(read.States) != 2 || read.States[0].Value.Value == nil || read.States[0].Value.GetBoolValue() || read.States[1].Value.GetIntValue() != 40 {
		t.Fatalf("read=%v", read)
	}
	methodsInfo, err := lookupMethod(rpcMethodsGet)
	if err != nil {
		t.Fatal(err)
	}
	data, code, _, err = provider.answer(methodsInfo.id, nil)
	if err != nil || code != 0 {
		t.Fatalf("discovery code=%d err=%v", code, err)
	}
	methods := new(rpcpb.ClientRpcMethodsGetResponse)
	if err := proto.Unmarshal(data, methods); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(methods.Methods, "client.mhs.v0.read") || slices.Contains(methods.Methods, "client.mhs.v0.write") {
		t.Fatalf("methods=%v", methods.Methods)
	}
	if err := provider.install("client.mhs.v0.write", response); err != nil {
		t.Fatal(err)
	}
	writeInfo, err := lookupMethod("client.mhs.v0.write")
	if err != nil {
		t.Fatal(err)
	}
	data, code, _, err = provider.answer(writeInfo.id, nil)
	written := new(rpcpb.ClientMhsV0WriteResponse)
	if err != nil || code != 0 {
		t.Fatalf("write code=%d err=%v", code, err)
	}
	if err := proto.Unmarshal(data, written); err != nil || len(written.States) != 2 || written.States[1].Value.GetIntValue() != 40 {
		t.Fatalf("write=%v err=%v", written, err)
	}
}
