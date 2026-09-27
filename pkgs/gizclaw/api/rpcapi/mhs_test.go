package rpcapi

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestClientHwdRegistryAndPayloads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		writable bool
	}{
		{"wifi", false}, {"ble", false}, {"modem", false}, {"battery", false},
		{"mic", false}, {"display", true}, {"led", true}, {"speaker", true},
	} {
		hwd, err := ClientHwdByName(tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		meta, err := ClientHwdMetadata(hwd)
		if err != nil || meta.Name != tc.name || meta.ReadResponse == "" || (meta.WriteRequest != "") != tc.writable {
			t.Fatalf("%s: invalid metadata %+v, %v", tc.name, meta, err)
		}
		if _, err := ClientHwdReadResponseMessage(hwd, nil); err != nil {
			t.Fatalf("%s read codec: %v", tc.name, err)
		}
		if _, err := ClientHwdWriteRequestFromBytes(hwd, nil); (err == nil) != tc.writable {
			t.Fatalf("%s writable=%v err=%v", tc.name, tc.writable, err)
		}
	}
	if _, err := ClientHwdByName("device"); err == nil {
		t.Fatal("accepted non-HWD device")
	}
	if _, err := ClientHwdMetadata(0); err == nil {
		t.Fatal("accepted unspecified HWD")
	}
}

func TestClientHwdRPCEnvelope(t *testing.T) {
	request := &rpcpb.ClientMhsV0WriteRequest{
		Id: "speaker.main", Hwd: rpcpb.ClientHwd_CLIENT_HWD_SPEAKER,
	}
	request.Payload, _ = proto.Marshal(&rpcpb.SpeakerHwdWriteRequest{VolumePercent: proto.Uint32(0), Muted: new(false)})
	var payload RPCPayload
	if err := payload.FromClientMhsV0WriteRequest(request); err != nil {
		t.Fatal(err)
	}
	wire, err := encodeRPCRequestPayload(RPCMethodClientMhsV0Write, &payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRPCRequestPayload(RPCMethodClientMhsV0Write, wire)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decoded.AsClientMhsV0WriteRequest()
	if err != nil || !proto.Equal(got, request) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	message, err := ClientHwdWriteRequestFromBytes(got.Hwd, got.Payload)
	if err != nil {
		t.Fatal(err)
	}
	value := message.(*rpcpb.SpeakerHwdWriteRequest)
	if value.VolumePercent == nil || *value.VolumePercent != 0 || value.Muted == nil || *value.Muted {
		t.Fatalf("lost explicit defaults: %+v", value)
	}
	for method, id := range map[RPCMethod]int32{RPCMethodClientMhsV0Read: 133, RPCMethodClientMhsV0Write: 134} {
		got, err := ProtoMethod(method)
		if err != nil || int32(got) != id {
			t.Fatalf("%v = %v, %v", method, got, err)
		}
	}
}
