package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoRPCAPIPayloadMapFromPeerProto(t *testing.T) {
	dir := t.TempDir()
	peerProto := filepath.Join(dir, "peer.proto")
	goOut := filepath.Join(dir, "payload_proto_gen.go")
	if err := os.WriteFile(peerProto, []byte(`syntax = "proto3";
package gizclaw.rpc.v1;

enum RpcMethod {
  RPC_METHOD_UNSPECIFIED = 0;
  // rpc: all.ping request=PingRequest response=PingResponse
  RPC_METHOD_ALL_PING = 42;
  // rpc: server.info.put request=ServerPutInfoRequest response=ServerPutInfoResponse
  RPC_METHOD_SERVER_INFO_PUT = 43;
}
`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err := run([]string{
		"-proto", peerProto,
		"-go-rpcapi-out", goOut,
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(goOut)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"RPCMethodAllPing: \"PingRequest\"",
		"RPCMethodServerInfoPut: \"ServerPutInfoRequest\"",
		"RPCMethodAllPing: \"PingResponse\"",
		"RPCMethodServerInfoPut: \"ServerPutInfoResponse\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated Go payload map missing %q", want)
		}
	}
}
