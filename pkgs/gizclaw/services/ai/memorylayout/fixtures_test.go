package memorylayout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
)

func TestE2EMemoryLayoutFixturesPassServiceValidation(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(
		"..", "..", "..", "..", "..",
		"tests", "gizclaw-e2e", "testdata", "resources", "04-memory-layouts", "*.yaml",
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 7 {
		t.Fatalf("MemoryLayout fixture count = %d, want at least 7", len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			jsonRaw, err := yaml.YAMLToJSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			var resource apitypes.Resource
			if err := json.Unmarshal(jsonRaw, &resource); err != nil {
				t.Fatal(err)
			}
			typed, err := resource.AsMemoryLayoutResource()
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = validate(apitypes.MemoryLayout{
				Id:   typed.Metadata.Id,
				Spec: typed.Spec,
			}, typed.Metadata.Id)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
