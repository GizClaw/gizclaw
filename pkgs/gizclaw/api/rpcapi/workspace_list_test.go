package rpcapi

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestWorkspaceListSystemPresence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		system *bool
		wire   []byte
		json   string
	}{
		{"omitted", nil, nil, `{}`},
		{"ordinary", new(false), []byte{0x20, 0x00}, `{"system":false}`},
		{"system", new(true), []byte{0x20, 0x01}, `{"system":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := WorkspaceListRequest{System: tc.system}
			var payload RPCPayload
			if err := payload.FromWorkspaceListRequest(request); err != nil {
				t.Fatal(err)
			}
			wire, err := encodeRPCRequestPayload(RPCMethodServerWorkspaceList, &payload)
			if err != nil || !bytes.Equal(wire, tc.wire) {
				t.Fatalf("wire = %x, %v, want %x", wire, err, tc.wire)
			}
			var message rpcpb.WorkspaceListRequest
			if err := proto.Unmarshal(wire, &message); err != nil || !reflect.DeepEqual(message.System, tc.system) {
				t.Fatalf("protobuf system = %v, %v, want %v", message.System, err, tc.system)
			}
			decoded, err := payload.AsWorkspaceListRequest()
			if err != nil || !reflect.DeepEqual(decoded, request) {
				t.Fatalf("decoded request = %+v, %v, want %+v", decoded, err, request)
			}
			encoded, err := json.Marshal(decoded)
			if err != nil || string(encoded) != tc.json {
				t.Fatalf("JSON = %s, %v, want %s", encoded, err, tc.json)
			}
		})
	}
}
