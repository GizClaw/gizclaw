package admincmd

import (
	"encoding/json"
	"testing"
)

func FuzzDecodeResourceData(f *testing.F) {
	for _, seed := range []struct {
		path string
		data []byte
	}{
		{
			path: "credential.json",
			data: []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"minimax-main"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`),
		},
		{
			path: "resources.yaml",
			data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: ResourceList\nmetadata:\n  name: bundle\nspec:\n  items:\n    - apiVersion: gizclaw.admin/v1alpha1\n      kind: Credential\n      metadata:\n        name: nested-credential\n      spec:\n        provider: openai\n        body:\n          api_key: ${GIZCLAW_FUZZ_SECRET:-secret}\n"),
		},
		{path: "bad.json", data: []byte(`{"kind":`)},
		{path: "bad.yaml", data: []byte("kind: [")},
		{path: "resource.txt", data: []byte(`{"kind":"Credential"}`)},
	} {
		f.Add(seed.path, seed.data)
	}

	f.Fuzz(func(t *testing.T, path string, data []byte) {
		if len(path) > 128 || len(data) > 8192 {
			return
		}
		resource, err := decodeResourceData(path, data)
		if err != nil {
			return
		}
		if _, err := json.Marshal(resource); err != nil {
			t.Fatalf("json.Marshal(decoded resource) error = %v", err)
		}
		if err := validateResourceKind(resource); err != nil {
			t.Fatalf("decoded resource failed kind validation: %v", err)
		}
	})
}
