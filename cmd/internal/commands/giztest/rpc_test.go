package giztestcmd

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestLookupMethodUsesProtoMetadata(t *testing.T) {
	info, err := lookupMethod("all.ping")
	if err != nil {
		t.Fatal(err)
	}
	if info.request != "PingRequest" || info.response != "PingResponse" {
		t.Fatalf("info = %#v", info)
	}
	if _, err := lookupMethod("missing.method"); err == nil {
		t.Fatal("unknown method accepted")
	}
}

func TestUnaryRPCResponseAcceptsLargeCatalogEnvelope(t *testing.T) {
	for _, size := range []int{10, rpcapi.MaxFrameSize + 20} {
		want := &rpcpb.RpcResponse{Id: "catalog", Body: &rpcpb.RpcResponse_Payload{Payload: bytes.Repeat([]byte{42}, size)}}
		data, err := proto.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		frameType := rpcapi.FrameTypeBinary
		if len(data) > rpcapi.MaxFrameSize {
			frameType = rpcapi.FrameTypeText
		}
		for len(data) > 0 {
			n := min(len(data), rpcapi.MaxFrameSize)
			if err := rpcapi.WriteFrame(&encoded, rpcapi.Frame{Type: frameType, Payload: data[:n]}); err != nil {
				t.Fatal(err)
			}
			data = data[n:]
		}
		if err := rpcapi.WriteEOS(&encoded); err != nil {
			t.Fatal(err)
		}
		encoded.WriteString("next stream")
		got, err := readUnaryRPCResponse(&encoded)
		if err != nil || !proto.Equal(got, want) || encoded.String() != "next stream" {
			t.Fatalf("size %d: id=%q payload=%d error=%v remaining=%d", size, got.GetId(), len(got.GetPayload()), err, encoded.Len())
		}
	}
}

func TestUnaryRPCResponseRejectsMalformedAndUnboundedEnvelopes(t *testing.T) {
	frame := rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: []byte{10, 1, 'x'}}
	rows := map[string][]rpcapi.Frame{
		"missing EOS":          {frame},
		"mixed encoding":       {frame, {Type: rpcapi.FrameTypeBinary, Payload: []byte{0}}},
		"invalid protobuf":     {{Type: rpcapi.FrameTypeText, Payload: []byte{255}}, {Type: rpcapi.FrameTypeEOS}},
		"EOS without response": {{Type: rpcapi.FrameTypeEOS}},
	}
	for range 17 {
		rows["oversized"] = append(rows["oversized"], rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: make([]byte, rpcapi.MaxFrameSize)})
	}
	for name, frames := range rows {
		t.Run(name, func(t *testing.T) {
			var encoded bytes.Buffer
			for _, frame := range frames {
				if err := rpcapi.WriteFrame(&encoded, frame); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readUnaryRPCResponse(&encoded); err == nil {
				t.Fatal("accepted invalid envelope")
			}
		})
	}
}
