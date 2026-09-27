package main

import (
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
	_, code, _, err := provider.answer(info.id, 0, nil)
	if err != nil || code != int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED) {
		t.Fatalf("uninstalled code=%d err=%v", code, err)
	}
	if err := provider.install("client.mhs.v0.read", map[string]any{"payload": "CAAQMg=="}); err != nil {
		t.Fatal(err)
	}
	data, code, _, err := provider.answer(info.id, 0, nil)
	if err != nil || code != 0 {
		t.Fatalf("read code=%d err=%v", code, err)
	}
	read := new(rpcpb.ClientMhsV0ReadResponse)
	if err := proto.Unmarshal(data, read); err != nil {
		t.Fatal(err)
	}
	var value rpcpb.LedHwdReadResponse
	if err := proto.Unmarshal(read.Payload, &value); err != nil || value.Enabled == nil || *value.Enabled || value.BrightnessPercent == nil || *value.BrightnessPercent != 50 {
		t.Fatalf("read=%+v, %v", &value, err)
	}
	_, mhs := provider.installedMasks()
	if mhs != 1 {
		t.Fatalf("MHS discovery mask = %b, want read only", mhs)
	}
	if err := provider.install("client.mhs.v0.write", map[string]any{"payload": "CgQIABAU"}); err != nil {
		t.Fatal(err)
	}
	writeInfo, err := lookupMethod("client.mhs.v0.write")
	if err != nil {
		t.Fatal(err)
	}
	data, code, _, err = provider.answer(writeInfo.id, 0, nil)
	written := new(rpcpb.ClientMhsV0WriteResponse)
	if err != nil || code != 0 {
		t.Fatalf("write code=%d err=%v", code, err)
	}
	if err := proto.Unmarshal(data, written); err != nil {
		t.Fatal(err)
	}
	var applied rpcpb.LedHwdWriteResponse
	if err := proto.Unmarshal(written.Payload, &applied); err != nil || applied.Applied == nil || applied.Applied.Enabled == nil || *applied.Applied.Enabled || applied.Applied.BrightnessPercent == nil || *applied.Applied.BrightnessPercent != 20 {
		t.Fatalf("write=%+v err=%v", &applied, err)
	}
	_, mhs = provider.installedMasks()
	if mhs != 3 {
		t.Fatalf("MHS discovery mask = %b, want read and write", mhs)
	}
}
