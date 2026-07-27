package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRewriteRefsRewritesExternalDiscriminatorMappings(t *testing.T) {
	root := t.TempDir()
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(apiDir, "memory_layout.json")
	if err := os.WriteFile(target, []byte(`{
  "components": {
    "schemas": {
      "MemoryLayoutResource": {
        "type": "object"
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &generator{
		root:         root,
		visitedFiles: map[string]bool{},
		schemas:      map[string]any{},
		schemaSource: map[string]string{},
		entryAliases: map[string]string{},
	}
	rewritten, err := g.rewriteRefs(filepath.Join(apiDir, "resource.json"), map[string]any{
		"discriminator": map[string]any{
			"propertyName": "kind",
			"mapping": map[string]any{
				"MemoryLayout": "./memory_layout.json#/components/schemas/MemoryLayoutResource",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	discriminator := rewritten.(map[string]any)["discriminator"].(map[string]any)
	mapping := discriminator["mapping"].(map[string]any)
	if got := mapping["MemoryLayout"]; got != "#/components/schemas/MemoryLayoutResource" {
		t.Fatalf("mapping = %q", got)
	}
	if _, exists := g.schemas["MemoryLayoutResource"]; !exists {
		t.Fatal("mapped schema was not bundled")
	}
}
