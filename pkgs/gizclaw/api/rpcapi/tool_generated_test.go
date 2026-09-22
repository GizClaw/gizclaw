package rpcapi

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/protobuf/proto"
)

func TestClientToolPlaylistItemsEncode(t *testing.T) {
	request, err := ClientToolRequestMessage(rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET, map[string]any{
		"items": []any{map[string]any{"url": "https://example.com/track.mp3", "title": "Track"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded := new(rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest)
	if err := proto.Unmarshal(encoded, decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Url != "https://example.com/track.mp3" {
		t.Fatalf("playlist items: %+v", decoded.Items)
	}
}

func TestSafeToolPayloadRoundTripAndMethodRegistry(t *testing.T) {
	method, err := ProtoMethod(RPCMethodServerToolGet)
	if err != nil {
		t.Fatalf("ProtoMethod(server.tool.get) error = %v", err)
	}
	if got, err := MethodFromProto(method); err != nil || got != RPCMethodServerToolGet {
		t.Fatalf("MethodFromProto() = %q, %v", got, err)
	}
	tool := Tool{
		Name:        "play-music",
		I18n:        map[string]ResourceI18nText{"en": {DisplayName: "Play Music"}, "zh-CN": {DisplayName: "播放音乐"}},
		InputSchema: jsonschema.Schema{Type: "object", Required: []string{"query"}, Properties: map[string]*jsonschema.Schema{"query": {Type: "string"}}},
		InvokeName:  "search_music",
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
	if got.Value.Name != tool.Name || got.Value.InvokeName != tool.InvokeName || got.Value.InputSchema.Properties["query"].Type != "string" {
		t.Fatalf("Tool round trip = %#v", got)
	}

}
