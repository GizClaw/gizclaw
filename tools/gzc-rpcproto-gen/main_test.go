package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRPCPayloadProto(t *testing.T) {
	out := filepath.Join(t.TempDir(), "payload.proto")
	err := run([]string{
		"-schema", "../../pkgs/gizclaw/api/rpcapi/rpc_resolved.json",
		"-out", out,
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"message PingRequest {",
		"int64 client_send_time = ",
		"message PingResponse {",
		"int64 server_time = ",
		"message WorkspaceHistoryAudioGetResponse {",
		"map<string, int64> badge_exp_delta",
		"oneof value {",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated proto missing %q", want)
		}
	}
}

func TestGenerateRPCPayloadProtoPreservesExistingNumbers(t *testing.T) {
	out := filepath.Join(t.TempDir(), "payload.proto")
	initial := []byte(`syntax = "proto3";
package gizclaw.rpc.v1;
message PingRequest {
  int64 client_send_time = 77;
}
enum WorkspaceInputMode {
  WORKSPACE_INPUT_MODE_UNSPECIFIED = 0;
  WORKSPACE_INPUT_MODE_REALTIME = 44;
}
`)
	if err := os.WriteFile(out, initial, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	err := run([]string{
		"-schema", "../../pkgs/gizclaw/api/rpcapi/rpc_resolved.json",
		"-out", out,
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"int64 client_send_time = 77;",
		"WORKSPACE_INPUT_MODE_REALTIME = 44;",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated proto did not preserve %q", want)
		}
	}
}

func TestGenerateGoRPCAPIPayloadMap(t *testing.T) {
	dir := t.TempDir()
	protoOut := filepath.Join(dir, "payload.proto")
	goOut := filepath.Join(dir, "payload_proto_gen.go")
	err := run([]string{
		"-schema", "../../pkgs/gizclaw/api/rpcapi/rpc_resolved.json",
		"-out", protoOut,
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
		"package rpcapi",
		"var rpcRequestPayloadMessages = map[RPCMethod]string{",
		"RPCMethodAllPing: \"PingRequest\"",
		"var rpcResponsePayloadMessages = map[RPCMethod]string{",
		"RPCMethodAllPing: \"PingResponse\"",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated Go payload map missing %q", want)
		}
	}
}

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
