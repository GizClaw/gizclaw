package rpcapi

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestAppPayloadsAndRegistry(t *testing.T) {
	tests := []struct {
		method   RPCMethod
		id       int32
		request  proto.Message
		response proto.Message
	}{
		{RPCMethodClientAppList, 121, &rpcpb.ClientAppListRequest{}, &rpcpb.ClientAppListResponse{Runtime: "runtime.lua.gizos", Apps: []*rpcpb.InstalledApp{{AppName: "desk_clock", Sha256: "digest"}}}},
		{RPCMethodClientAppInstall, 122, &rpcpb.ClientAppInstallRequest{AppName: "desk_clock", Url: "https://example.com/app.tar.zlib", Sha256: "digest", Size: 42}, &rpcpb.ClientAppInstallResponse{}},
		{RPCMethodClientAppUninstall, 123, &rpcpb.ClientAppUninstallRequest{AppName: "desk_clock"}, &rpcpb.ClientAppUninstallResponse{}},
		{RPCMethodClientAppInvoke, 124, &rpcpb.ClientAppInvokeRequest{AppName: "desk_clock", Method: "get_alarm", ArgsJson: `{"hour":8}`}, &rpcpb.ClientAppInvokeResponse{ResultJson: `{"ok":true}`}},
		{RPCMethodClientAppJobStart, 125, &rpcpb.ClientAppJobStartRequest{AppName: "desk_clock", Method: "show_clock", ArgsJson: `{}`}, &rpcpb.ClientAppJobStartResponse{JobId: 7}},
		{RPCMethodClientAppJobCancel, 126, &rpcpb.ClientAppJobCancelRequest{JobId: 7}, &rpcpb.ClientAppJobCancelResponse{}},
	}
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			method, err := ProtoMethod(tt.method)
			if err != nil || int32(method) != tt.id {
				t.Fatalf("method = %v, %v", method, err)
			}
			if got, err := MethodFromProto(method); err != nil || got != tt.method {
				t.Fatalf("reverse method = %v, %v", got, err)
			}
			for _, value := range []proto.Message{tt.request, tt.response} {
				name := string(value.ProtoReflect().Descriptor().Name())
				var payload RPCPayload
				if err := payload.encode(name, value); err != nil {
					t.Fatal(err)
				}
				got := value.ProtoReflect().New().Interface()
				if err := payload.decode(name, got); err != nil {
					t.Fatal(err)
				}
				if !proto.Equal(value, got) {
					t.Fatalf("%s round trip = %v, want %v", name, got, value)
				}
			}
		})
	}
	if RPCMethod("client.tool.invoke").Valid() {
		t.Fatal("removed client.tool.invoke remains valid")
	}
}
