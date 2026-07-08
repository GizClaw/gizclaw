package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunGeneratesFiles(t *testing.T) {
	root := t.TempDir()
	writeTestProtoFiles(t, root)
	var stderr bytes.Buffer
	code := run([]string{
		"-proto", filepath.Join(root, "peer.proto"),
		"-payload-proto", filepath.Join(root, "payload.proto"),
		"-out", filepath.Join(root, "out"),
		"-package", "gzc",
	}, &stderr)
	if code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "out", "gzc_rpc_generated.h")); err != nil {
		t.Fatalf("generated header missing: %v", err)
	}
}

func writeTestProtoFiles(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, "peer.proto"), `syntax = "proto3";
package gizclaw.rpc.v1;

enum RpcMethod {
  RPC_METHOD_UNSPECIFIED = 0;
  // rpc: all.ping request=PingRequest response=PingResponse
  RPC_METHOD_ALL_PING = 42;
}
`)
	writeTestFile(t, filepath.Join(root, "payload.proto"), `syntax = "proto3";
package gizclaw.rpc.v1;

message PingRequest {
  int64 client_send_time = 1;
}

message PingResponse {
  int64 server_time = 1;
}
`)
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
