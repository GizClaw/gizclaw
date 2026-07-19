package rpcapi

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestToolPayloadRoundTripAndMethodRegistry(t *testing.T) {
	method, err := ProtoMethod(RPCMethodServerToolCreate)
	if err != nil {
		t.Fatalf("ProtoMethod(server.tool.create) error = %v", err)
	}
	if got, err := MethodFromProto(method); err != nil || got != RPCMethodServerToolCreate {
		t.Fatalf("MethodFromProto() = %q, %v", got, err)
	}
	name := "play_music"
	peer := "peer-a"
	owner := "owner-a"
	enabled := true
	tool := Tool{
		Id:             "peer.peer-a.music.play",
		Name:           &name,
		Source:         ToolSourceDevice,
		Enabled:        &enabled,
		OwnerPeer:      &peer,
		OwnerPublicKey: &owner,
		InputSchema: jsonschema.Schema{
			Type:       "object",
			Required:   []string{"query"},
			Properties: map[string]*jsonschema.Schema{"query": {Type: "string"}},
		},
		Executor: ToolExecutor{Kind: ToolExecutorKindDeviceRpc, Method: &name, PeerId: &peer},
	}
	var payload RPCPayload
	if err := payload.FromToolCreateRequest(tool); err != nil {
		t.Fatalf("FromToolCreateRequest() error = %v", err)
	}
	got, err := payload.AsToolCreateRequest()
	if err != nil {
		t.Fatalf("AsToolCreateRequest() error = %v", err)
	}
	if got.Id != tool.Id || got.OwnerPublicKey == nil || *got.OwnerPublicKey != owner || got.InputSchema.Type != "object" || got.InputSchema.Properties["query"].Type != "string" {
		t.Fatalf("Tool round trip = %#v", got)
	}

	invoke := ToolInvokeResponse{DataJson: `{"ok":true}`}
	if err := payload.FromToolInvokeResponse(invoke); err != nil {
		t.Fatalf("FromToolInvokeResponse() error = %v", err)
	}
	decoded, err := payload.AsToolInvokeResponse()
	if err != nil || string(decoded.DataJson) != `{"ok":true}` {
		t.Fatalf("AsToolInvokeResponse() = %s, %v", decoded.DataJson, err)
	}
}
