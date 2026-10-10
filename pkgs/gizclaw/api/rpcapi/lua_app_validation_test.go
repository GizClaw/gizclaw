package rpcapi

import (
	"encoding/base64"
	"strings"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestLuaAppRequestBounds(t *testing.T) {
	for _, request := range []proto.Message{
		&rpcpb.ClientLuaAppListRequest{},
		&rpcpb.ClientLuaAppInstallRequest{Url: "http://apps.test/app"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64,eJwDAAAAAAE="},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/octet-stream;base64,eJwDAAAAAAE="},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/tetris.lua-app.tar.zlib?token=opaque"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "tetris", Params: map[string]string{"mode": "single", "text": "玩一局", "empty": ""}},
	} {
		if err := ValidateClientToolRequest(request); err != nil {
			t.Fatal(err)
		}
	}
	for _, request := range []proto.Message{
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64,AB=="},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64,AA=A"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64,"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64,AA==\n"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:text/plain;base64,AA=="},
		&rpcpb.ClientLuaAppInstallRequest{Url: "data:application/zlib;base64," + strings.Repeat("A", LuaAppDataURLMaxBytes)},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://user:pass@apps.test/app"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/app#"},
		&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/app", Sha256: new("bad")},
		&rpcpb.ClientLuaAppRunRequest{AppId: "../game"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "Game"},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"": "empty key"}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"text": "a\x00b"}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"text": strings.Repeat("字", 342)}},
		&rpcpb.ClientLuaAppRunRequest{AppId: "game", Params: map[string]string{"a": strings.Repeat("a", 1024), "b": strings.Repeat("a", 1024), "c": strings.Repeat("a", 1024), "d": strings.Repeat("a", 1024)}},
	} {
		if ValidateClientToolRequest(request) == nil {
			t.Fatalf("accepted invalid %T", request)
		}
	}
}

func TestLuaAppCatalogAndCodec(t *testing.T) {
	app := &rpcpb.LuaAppInfo{AppId: "tetris", Version: "0.1.0", DisplayName: new("俄罗斯方块")}
	if err := ValidateLuaAppResponse(&rpcpb.ClientLuaAppListResponse{Apps: []*rpcpb.LuaAppInfo{app}}); err != nil {
		t.Fatal(err)
	}
	if ValidateLuaAppResponse(&rpcpb.ClientLuaAppListResponse{Apps: []*rpcpb.LuaAppInfo{app, app}}) == nil {
		t.Fatal("accepted duplicate IDs")
	}
	if ValidateLuaAppResponse(&rpcpb.ClientLuaAppInstallResponse{}) == nil {
		t.Fatal("accepted missing installed app")
	}
	request, err := ClientToolRequestMessage(rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, map[string]any{"app_id": "tetris", "params": map[string]any{"mode": "single", "level": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ClientToolRequestFromBytes(rpcpb.ClientTool_CLIENT_TOOL_LUA_APP_RUN, data)
	if err != nil || !proto.Equal(request, decoded) {
		t.Fatalf("decoded=%v err=%v", decoded, err)
	}
}

func TestLuaAppInstallFragmentedEnvelopeSize(t *testing.T) {
	// Larger than the measured largest product archive (164484 bytes), and
	// crosses three 65535-byte frame boundaries after Base64 encoding.
	archive := make([]byte, 164484)
	for i := range archive {
		archive[i] = byte(i)
	}
	source := "data:application/zlib;base64," + base64.StdEncoding.EncodeToString(archive)
	request := &rpcpb.ClientLuaAppInstallRequest{Url: source}
	if err := ValidateLuaAppRequest(request); err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= 3*65535 || len(encoded) >= LuaAppDataURLMaxBytes {
		t.Fatalf("unexpected wire size: %d", len(encoded))
	}
	var roundtrip rpcpb.ClientLuaAppInstallRequest
	if err := proto.Unmarshal(encoded, &roundtrip); err != nil || roundtrip.Url != source {
		t.Fatalf("round trip: %v", err)
	}
}

func TestLuaAppNilPayloadsAreRejected(t *testing.T) {
	var request *rpcpb.ClientLuaAppInstallStreamRequest
	var response *rpcpb.ClientLuaAppInstallResponse
	if ValidateLuaAppRequest(request) == nil || ValidateLuaAppResponse(response) == nil {
		t.Fatal("nil installer payload accepted")
	}
}

func TestLuaAppBinaryArchiveSizeLimit(t *testing.T) {
	for _, size := range []uint32{1, 524288, 524289} {
		request := &rpcpb.ClientLuaAppInstallStreamRequest{ContentLength: size, Sha256: strings.Repeat("a", 64)}
		err := ValidateLuaAppRequest(request)
		if (err == nil) != (size <= 524288) {
			t.Fatalf("length %d: %v", size, err)
		}
	}
	// The limit applies to bytes pushed directly, not HTTP(S) download URLs.
	if err := ValidateLuaAppRequest(&rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/large.lua-app.tar.zlib"}); err != nil {
		t.Fatal(err)
	}
}
