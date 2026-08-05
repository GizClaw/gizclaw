package localserver_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
)

func TestCatalogEnvironmentUsesSavedThenProcessThenDefault(t *testing.T) {
	catalog := &localserver.Catalog{Requirements: []localserver.EnvironmentRequirement{
		{Name: "SAVED"},
		{Name: "PROCESS"},
		{Name: "DEFAULT", Default: new("fallback")},
		{Name: "MISSING"},
	}}
	process := map[string]string{"SAVED": "process-saved", "PROCESS": "process"}
	resolved, missing := catalog.ResolveEnvironment(map[string]string{"SAVED": "desktop"}, func(name string) (string, bool) {
		value, ok := process[name]
		return value, ok
	})
	if resolved["SAVED"] != "desktop" || resolved["PROCESS"] != "process" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if len(missing) != 1 || missing[0] != "MISSING" {
		t.Fatalf("missing = %v", missing)
	}
}

func TestCatalogRejectsWorkspaceResource(t *testing.T) {
	_, err := localserver.LoadCatalog(fstest.MapFS{
		"resources/05-workspaces/00-invalid.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Workspace\nmetadata:\n  id: invalid\n")},
	})
	if err == nil {
		t.Fatal("LoadCatalog() error = nil")
	}
}

func TestCatalogRejectsMetadataIDWithSurroundingWhitespace(t *testing.T) {
	_, err := localserver.LoadCatalog(fstest.MapFS{
		"resources/01-credentials/00-invalid.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Credential\nmetadata:\n  id: ' credential-a '\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("LoadCatalog() error = %v, want exact metadata.id rejection", err)
	}
}

func TestCatalogRejectsLegacyAssetsWithDefaultRuntimeProfile(t *testing.T) {
	profile := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  id: default\n")
	for name, data := range map[string][]byte{
		"assets/pets/a.pixa": []byte("asset"),
		"petdef-pixa.txt":    []byte("pet-a assets/pets/a.pixa\n"),
		"voice-sync.txt":     []byte("volc tenant-a\n"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := localserver.LoadCatalog(fstest.MapFS{
				"resources/07-runtime-profiles/00-default.yaml": {Data: profile},
				name: {Data: data},
			})
			if err == nil || !strings.Contains(err.Error(), "legacy") {
				t.Fatalf("LoadCatalog() error = %v, want legacy asset rejection", err)
			}
		})
	}
}
