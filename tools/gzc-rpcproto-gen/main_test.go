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
