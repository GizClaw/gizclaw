package rpcgen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratorMatchesGoldenFiles(t *testing.T) {
	fixture := filepath.Join("testdata", "golden")
	out := filepath.Join(t.TempDir(), "out")
	err := Run(Config{
		SchemaPath:  filepath.Join(fixture, "api", "rpc.json"),
		IncludeDirs: []string{filepath.Join(fixture, "api")},
		OutDir:      out,
		Package:     "gzc",
		Format:      true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertGoldenDir(t, filepath.Join(fixture, "want"), out)
}

func TestGeneratorResolvesRefsFromIncludeRoots(t *testing.T) {
	root := t.TempDir()
	schemaDir := filepath.Join(root, "schema")
	includeDir := filepath.Join(root, "shared")
	writeFile(t, filepath.Join(schemaDir, "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"rpc/common.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"rpc/common.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(includeDir, "rpc", "common.json"), `{
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

func TestGeneratorReadsMethodIDsFromProto(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api", "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"./rpc/common.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"./rpc/common.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(root, "api", "rpc", "common.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "PingRequest":{"type":"object","properties":{}},
    "PingResponse":{"type":"object","properties":{}}
  }}
}`)
	writeFile(t, filepath.Join(root, "api", "rpc", "peer.proto"), `syntax = "proto3";
package gizclaw.rpc.v1;

enum RpcMethod {
  RPC_METHOD_UNSPECIFIED = 0;
  // rpc: all.ping request=PingRequest response=PingResponse
  RPC_METHOD_ALL_PING = 42;
}
`)
	out := filepath.Join(root, "out")
	err := Run(Config{
		SchemaPath: filepath.Join(root, "api", "rpc.json"),
		ProtoPath:  filepath.Join(root, "api", "rpc", "peer.proto"),
		OutDir:     out,
		Package:    "gzc",
		Format:     true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := readFile(t, filepath.Join(out, "gzc_rpc_encode.c"))
	if !bytes.Contains([]byte(got), []byte(`{GZC_RPC_METHOD_ALL_PING, 42u, "PingRequest", "PingResponse"`)) {
		t.Fatalf("generated method table does not contain proto method id:\n%s", got)
	}
}

func TestGeneratorCheckReportsDrift(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api", "rpc.json"), `{
  "openapi":"3.0.3",
  "components":{"schemas":{
    "RPCMethod":{"type":"string","enum":["all.ping"]},
    "RPCRequest":{"type":"object","properties":{"params":{"oneOf":[{"$ref":"./rpc/common.json#/components/schemas/PingRequest"}]}}},
    "RPCResponse":{"type":"object","properties":{"result":{"oneOf":[{"$ref":"./rpc/common.json#/components/schemas/PingResponse"}]}}}
  }}
}`)
	writeFile(t, filepath.Join(root, "api", "rpc", "common.json"), `{
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

func assertGoldenDir(t *testing.T, wantDir, gotDir string) {
	t.Helper()
	wantFiles := listRelativeFiles(t, wantDir)
	gotFiles := listRelativeFiles(t, gotDir)
	if len(wantFiles) != len(gotFiles) {
		t.Fatalf("generated file count = %d, want %d\ngot=%v\nwant=%v", len(gotFiles), len(wantFiles), gotFiles, wantFiles)
	}
	for i := range wantFiles {
		if gotFiles[i] != wantFiles[i] {
			t.Fatalf("generated file[%d] = %s, want %s\ngot=%v\nwant=%v", i, gotFiles[i], wantFiles[i], gotFiles, wantFiles)
		}
		want, err := os.ReadFile(filepath.Join(wantDir, wantFiles[i]))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(gotDir, gotFiles[i]))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("generated %s does not match golden\n--- got ---\n%s\n--- want ---\n%s", gotFiles[i], got, want)
		}
	}
}

func listRelativeFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
