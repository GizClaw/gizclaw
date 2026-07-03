package rpcgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorEmitsRepresentativeBindings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api", "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(root, "api", "rpc", "all.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "PingRequest":{"type":"object","required":["client_send_time"],"properties":{"client_send_time":{"type":"integer","format":"int64"},"tag":{"type":"string"}}},
    "PingResponse":{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"},"server_time":{"type":"integer","format":"int64"}}}
  }}
}`)
	out := filepath.Join(root, "out")
	err := Run(Config{SchemaPath: filepath.Join(root, "api", "rpc.json"), IncludeDirs: []string{filepath.Join(root, "api")}, OutDir: out, Package: "gzc", Format: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	types := readFile(t, filepath.Join(out, "gzc_rpc_types.h"))
	for _, want := range []string{
		"typedef struct {",
		"int64_t client_send_time;",
		"bool has_tag;",
		"gzc_ping_request_t",
		"gzc_ping_response_t",
	} {
		if !strings.Contains(types, want) {
			t.Fatalf("types missing %q:\n%s", want, types)
		}
	}
	methods := readFile(t, filepath.Join(out, "gzc_rpc_methods.h"))
	if !strings.Contains(methods, "#define GZC_RPC_METHOD_ALL_PING \"all.ping\"") {
		t.Fatalf("methods missing all.ping constant:\n%s", methods)
	}
}

func TestGeneratorResolvesRefsFromIncludeRoots(t *testing.T) {
	root := t.TempDir()
	schemaDir := filepath.Join(root, "schema")
	includeDir := filepath.Join(root, "shared")
	writeFile(t, filepath.Join(schemaDir, "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"rpc/all.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"rpc/all.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(includeDir, "rpc", "all.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "PingRequest":{"type":"object","properties":{}},
    "PingResponse":{"type":"object","properties":{}}
  }}
}`)
	out := filepath.Join(root, "out")
	err := Run(Config{SchemaPath: filepath.Join(schemaDir, "rpc.json"), IncludeDirs: []string{includeDir}, OutDir: out, Package: "gzc", Format: true})
	if err != nil {
		t.Fatalf("Run() with include root error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "gzc_rpc_methods.h")); err != nil {
		t.Fatalf("methods header missing: %v", err)
	}
}

func TestGeneratorCheckReportsDrift(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api", "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"./rpc/all.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(root, "api", "rpc", "all.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "PingRequest":{"type":"object","properties":{}},
    "PingResponse":{"type":"object","properties":{}}
  }}
}`)
	out := filepath.Join(root, "out")
	if err := Run(Config{SchemaPath: filepath.Join(root, "api", "rpc.json"), OutDir: out, Package: "gzc", Format: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	writeFile(t, filepath.Join(out, "gzc_rpc_methods.h"), "stale\n")
	if err := Run(Config{SchemaPath: filepath.Join(root, "api", "rpc.json"), OutDir: out, Package: "gzc", Check: true}); err == nil {
		t.Fatal("Run(check) should report stale generated output")
	}
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
