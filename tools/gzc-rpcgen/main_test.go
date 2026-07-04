package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIncludeFlags(t *testing.T) {
	var flags includeFlags
	if err := flags.Set("api"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if got := flags.String(); got != "[api]" {
		t.Fatalf("String() = %q, want [api]", got)
	}
}

func TestRunGeneratesFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "api", "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeTestFile(t, filepath.Join(root, "api", "rpc", "all.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "PingRequest":{"type":"object","required":["client_send_time"],"properties":{"client_send_time":{"type":"integer","format":"int64"}}},
    "PingResponse":{"type":"object","required":["server_time"],"properties":{"server_time":{"type":"integer","format":"int64"}}}
  }}
}`)
	var stderr bytes.Buffer
	code := run([]string{
		"-schema", filepath.Join(root, "api", "rpc.json"),
		"-include", filepath.Join(root, "api"),
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

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
