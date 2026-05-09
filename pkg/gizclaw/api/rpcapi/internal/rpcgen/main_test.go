package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRPCWritesBundledSpec(t *testing.T) {
	root := newTestRepo(t)
	writeTestRPCSpec(t, root)
	t.Chdir(filepath.Join(root, "pkg", "gizclaw", "api", "rpcapi"))

	if err := resolveRPC(); err != nil {
		t.Fatalf("resolveRPC() error = %v", err)
	}

	resolved := readTestJSON(t, filepath.Join(root, outDirRel, resolvedRPCFile))
	schemas := resolved["components"].(map[string]any)["schemas"].(map[string]any)
	if _, ok := schemas["PingRequest"]; !ok {
		t.Fatal("resolved spec missing PingRequest")
	}
	req := schemas["RPCRequest"].(map[string]any)
	params := req["properties"].(map[string]any)["params"].(map[string]any)
	if got, want := params["$ref"], "#/components/schemas/PingRequest"; got != want {
		t.Fatalf("RPCRequest.params ref = %v, want %v", got, want)
	}
}

func TestRunResolveArg(t *testing.T) {
	root := newTestRepo(t)
	writeTestRPCSpec(t, root)
	t.Chdir(filepath.Join(root, "pkg", "gizclaw", "api", "rpcapi"))

	if err := run([]string{resolveArg}); err != nil {
		t.Fatalf("run(resolve) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, outDirRel, resolvedRPCFile)); err != nil {
		t.Fatalf("resolved file stat error = %v", err)
	}
}

func TestRunRejectsInvalidArgs(t *testing.T) {
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "missing -resolve") {
		t.Fatalf("run(nil) error = %v, want missing resolve", err)
	}
	if err := run([]string{"-bad"}); err == nil || !strings.Contains(err.Error(), `unknown argument "-bad"`) {
		t.Fatalf("run(bad) error = %v, want unknown argument", err)
	}
}

func TestProcessEntrySpecAliasesTopLevelRefs(t *testing.T) {
	root := newTestRepo(t)
	writeTestRPCSpec(t, root)

	g := newTestGenerator(root)
	if err := g.processEntrySpec(filepath.Join(root, entrySpecRel)); err != nil {
		t.Fatalf("processEntrySpec() error = %v", err)
	}

	bundle, err := g.bundleSpec()
	if err != nil {
		t.Fatalf("bundleSpec() error = %v", err)
	}
	var resolved map[string]any
	if err := json.Unmarshal(bundle, &resolved); err != nil {
		t.Fatalf("json.Unmarshal(bundle) error = %v", err)
	}
	schemas := resolved["components"].(map[string]any)["schemas"].(map[string]any)
	alias := schemas["PingAlias"].(map[string]any)
	if got, want := alias["$ref"], "#/components/schemas/PingRequest"; got != want {
		t.Fatalf("PingAlias ref = %v, want %v", got, want)
	}
}

func TestProcessEntrySpecRejectsConflictingSchemas(t *testing.T) {
	root := newTestRepo(t)
	writeJSONFile(t, filepath.Join(root, entrySpecRel), map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "RPC API",
			"version": "1.0.0",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": map[string]any{
				"Left": map[string]any{
					"$ref": "./rpc/left.json#/components/schemas/Thing",
				},
				"Right": map[string]any{
					"$ref": "./rpc/right.json#/components/schemas/Thing",
				},
			},
		},
	})
	writeJSONFile(t, filepath.Join(root, "api", "rpc", "left.json"), schemaDoc(map[string]any{
		"Thing": map[string]any{"type": "string"},
	}))
	writeJSONFile(t, filepath.Join(root, "api", "rpc", "right.json"), schemaDoc(map[string]any{
		"Thing": map[string]any{"type": "integer"},
	}))

	err := newTestGenerator(root).processEntrySpec(filepath.Join(root, entrySpecRel))
	if err == nil || !strings.Contains(err.Error(), `schema "Thing" is defined differently`) {
		t.Fatalf("processEntrySpec() error = %v, want conflicting schema error", err)
	}
}

func TestFindRepoRootFromNestedDirectory(t *testing.T) {
	root := newTestRepo(t)
	writeTestRPCSpec(t, root)
	nested := filepath.Join(root, "pkg", "gizclaw", "api", "rpcapi")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Chdir(nested)

	got, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot() error = %v", err)
	}
	if got != root {
		t.Fatalf("findRepoRoot() = %q, want %q", got, root)
	}
}

func TestRewriteRefsHandlesArrays(t *testing.T) {
	root := newTestRepo(t)
	writeTestRPCSpec(t, root)
	g := newTestGenerator(root)

	value := map[string]any{
		"anyOf": []any{
			map[string]any{
				"$ref": "./rpc/ping.json#/components/schemas/PingRequest",
			},
		},
	}
	rewritten, err := g.rewriteRefs(filepath.Join(root, entrySpecRel), value)
	if err != nil {
		t.Fatalf("rewriteRefs() error = %v", err)
	}
	anyOf := rewritten.(map[string]any)["anyOf"].([]any)
	item := anyOf[0].(map[string]any)
	if got, want := item["$ref"], "#/components/schemas/PingRequest"; got != want {
		t.Fatalf("rewritten array ref = %v, want %v", got, want)
	}
}

func TestRewriteRefRejectsUnsupportedRefs(t *testing.T) {
	root := newTestRepo(t)
	g := newTestGenerator(root)

	if _, err := g.rewriteRef(filepath.Join(root, entrySpecRel), "https://example.invalid/schema.json#/components/schemas/X"); err == nil {
		t.Fatal("rewriteRef(remote) should fail")
	}
	if _, err := g.rewriteRef(filepath.Join(root, entrySpecRel), "./rpc/ping.json"); err == nil {
		t.Fatal("rewriteRef(no fragment) should fail")
	}
	if _, err := g.rewriteRef(filepath.Join(root, entrySpecRel), "./rpc/ping.json#/components/parameters/X"); err == nil {
		t.Fatal("rewriteRef(non-schema fragment) should fail")
	}
}

func TestComponentSchemasRejectsMalformedDocs(t *testing.T) {
	if _, err := componentSchemas(map[string]any{}); err == nil {
		t.Fatal("componentSchemas(missing components) should fail")
	}
	if _, err := componentSchemas(map[string]any{
		"components": map[string]any{},
	}); err == nil {
		t.Fatal("componentSchemas(missing schemas) should fail")
	}
}

func TestReadJSONRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := readJSON(path); err == nil {
		t.Fatal("readJSON(invalid) should fail")
	}
}

func TestWriteIfChangedSkipsMatchingContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	data := []byte("{}\n")
	if err := writeIfChanged(path, data); err != nil {
		t.Fatalf("writeIfChanged(first) error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if err := writeIfChanged(path, data); err != nil {
		t.Fatalf("writeIfChanged(second) error = %v", err)
	}
	nextInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(second) error = %v", err)
	}
	if !nextInfo.ModTime().Equal(info.ModTime()) {
		t.Fatal("writeIfChanged rewrote matching content")
	}
}

func TestJSONEqualRejectsUnmarshalableValues(t *testing.T) {
	if jsonEqual(map[string]any{"bad": func() {}}, map[string]any{}) {
		t.Fatal("jsonEqual() should reject unmarshalable values")
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test.local/rpcgen\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, outDirRel), 0o755); err != nil {
		t.Fatalf("MkdirAll(outDirRel) error = %v", err)
	}
	return root
}

func newTestGenerator(root string) *generator {
	return &generator{
		root:         root,
		visitedFiles: map[string]bool{},
		schemas:      map[string]any{},
		schemaSource: map[string]string{},
		entryAliases: map[string]string{},
	}
}

func writeTestRPCSpec(t *testing.T, root string) {
	t.Helper()
	writeJSONFile(t, filepath.Join(root, entrySpecRel), map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "RPC API",
			"version": "1.0.0",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": map[string]any{
				"RPCRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"params": map[string]any{
							"$ref": "./rpc/ping.json#/components/schemas/PingRequest",
						},
					},
				},
				"PingAlias": map[string]any{
					"$ref": "./rpc/ping.json#/components/schemas/PingRequest",
				},
			},
		},
	})
	writeJSONFile(t, filepath.Join(root, "api", "rpc", "ping.json"), schemaDoc(map[string]any{
		"PingRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"client_send_time": map[string]any{
					"type":   "integer",
					"format": "int64",
				},
			},
		},
	}))
}

func schemaDoc(schemas map[string]any) map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Schema",
			"version": "1.0.0",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"schemas": schemas,
		},
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readTestJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", path, err)
	}
	return out
}
