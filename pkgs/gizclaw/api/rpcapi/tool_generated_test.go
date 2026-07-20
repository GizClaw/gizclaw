package rpcapi

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestSafeToolPayloadRoundTripAndMethodRegistry(t *testing.T) {
	method, err := ProtoMethod(RPCMethodServerToolGet)
	if err != nil {
		t.Fatalf("ProtoMethod(server.tool.get) error = %v", err)
	}
	if got, err := MethodFromProto(method); err != nil || got != RPCMethodServerToolGet {
		t.Fatalf("MethodFromProto() = %q, %v", got, err)
	}
	tool := Tool{
		Alias:       "play-music",
		I18n:        map[string]AliasI18nText{"en": {DisplayName: "Play Music"}, "zh-CN": {DisplayName: "播放音乐"}},
		InputSchema: jsonschema.Schema{Type: "object", Required: []string{"query"}, Properties: map[string]*jsonschema.Schema{"query": {Type: "string"}}},
	}
	response := ToolGetResponse{Value: tool, RuntimeProfileName: "default", RuntimeProfileRevision: "revision"}
	var payload RPCPayload
	if err := payload.FromToolGetResponse(response); err != nil {
		t.Fatalf("FromToolGetResponse() error = %v", err)
	}
	got, err := payload.AsToolGetResponse()
	if err != nil {
		t.Fatalf("AsToolGetResponse() error = %v", err)
	}
	if got.Value.Alias != tool.Alias || got.Value.InputSchema.Properties["query"].Type != "string" {
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
