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
		"int64 client_send_time = 1;",
		"message PingResponse {",
		"int64 server_time = 1;",
		"message WorkspaceHistoryAudioGetResponse {",
		"map<string, int64> badge_exp_delta",
		"oneof value {",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated proto missing %q", want)
		}
	}
}
