package rpcgen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratorLoadsModelFromProto(t *testing.T) {
	root := t.TempDir()
	writeProtoFixture(t, root)
	out := filepath.Join(root, "out")
	err := Run(Config{
		ProtoPath:        filepath.Join(root, "peer.proto"),
		PayloadProtoPath: filepath.Join(root, "payload.proto"),
		OutDir:           out,
		Package:          "gzc",
		Format:           true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	methods := readFile(t, filepath.Join(out, "gzc_rpc_encode.c"))
	if !bytes.Contains([]byte(methods), []byte(`{GZC_RPC_METHOD_ALL_PING, 42u, "PingRequest", "PingResponse"`)) {
		t.Fatalf("generated method table does not contain proto method id:\n%s", methods)
	}
	types := readFile(t, filepath.Join(out, "gzc_rpc_types.h"))
	if !bytes.Contains([]byte(types), []byte("bool has_kind;")) || !bytes.Contains([]byte(types), []byte("int32_t kind;")) {
		t.Fatalf("generated types do not expose optional enum as int32_t:\n%s", types)
	}
	encode := readFile(t, filepath.Join(out, "gzc_rpc_encode.c"))
	if !bytes.Contains([]byte(encode), []byte("gzc_rpc_proto_append_varint(platform, out_payload, 2, (uint64_t)(uint32_t)value->kind)")) {
		t.Fatalf("generated encode does not write optional enum as varint:\n%s", encode)
	}
}

func TestGeneratorCheckReportsDrift(t *testing.T) {
	root := t.TempDir()
	writeProtoFixture(t, root)
	out := filepath.Join(root, "out")
	cfg := Config{
		ProtoPath:        filepath.Join(root, "peer.proto"),
		PayloadProtoPath: filepath.Join(root, "payload.proto"),
		OutDir:           out,
		Package:          "gzc",
		Format:           true,
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	writeFile(t, filepath.Join(out, "gzc_rpc_methods.h"), "stale\n")
	cfg.Check = true
	if err := Run(cfg); err == nil {
		t.Fatal("Run(check) should report stale generated output")
	}
}

func writeProtoFixture(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "peer.proto"), `syntax = "proto3";
package gizclaw.rpc.v1;

enum RpcMethod {
  RPC_METHOD_UNSPECIFIED = 0;
  // rpc: all.ping request=PingRequest response=PingResponse
  RPC_METHOD_ALL_PING = 42;
}
`)
	writeFile(t, filepath.Join(root, "payload.proto"), `syntax = "proto3";
package gizclaw.rpc.v1;

enum PingKind {
  PING_KIND_UNSPECIFIED = 0;
  PING_KIND_FAST = 1;
}

message PingRequest {
  int64 client_send_time = 1;
  optional PingKind kind = 2;
}

message PingResponse {
  int64 server_time = 1;
}
`)
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
