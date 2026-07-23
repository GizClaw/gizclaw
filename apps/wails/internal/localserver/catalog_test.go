package localserver_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
)

func TestBundledCatalogContainsOnlyDefaultRuntimeProfile(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := localserver.LoadCatalog(source)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Resources; len(got) != 1 || got[0].Kind != "RuntimeProfile" || got[0].Name != "default" {
		t.Fatalf("resources = %#v, want RuntimeProfile/default only", got)
	}
	if len(catalog.PetDefPIXAs) != 0 || len(catalog.VoiceSyncs) != 0 || len(catalog.Requirements) != 0 {
		t.Fatalf("embedded assets or requirements remain: %#v %#v %#v", catalog.PetDefPIXAs, catalog.VoiceSyncs, catalog.Requirements)
	}
	profile, err := fs.ReadFile(catalog.FS, "resources/07-runtime-profiles/00-default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(profile)
	for _, forbidden := range []string{"volc-main", "pet_defs:", "adoption:", "pet-care", "chatroom"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("RuntimeProfile/default retains %q", forbidden)
		}
	}
	for alias, resourceID := range map[string]string{
		"journey-narrator":            "volc-tenant:volc-cn-beijing:zh_female_shaoergushi_mars_bigtts",
		"journey-origin-narrator":     "volc-tenant:volc-cn-beijing:zh_female_shaoergushi_mars_bigtts",
		"journey-heaven-narrator":     "volc-tenant:volc-cn-beijing:zh_male_sunwukong_mars_bigtts",
		"journey-pilgrimage-narrator": "volc-tenant:volc-cn-beijing:zh_male_tangseng_mars_bigtts",
		"journey-trials-narrator":     "volc-tenant:volc-cn-beijing:zh_male_changtianyi_mars_bigtts",
		"journey-kingdoms-narrator":   "volc-tenant:volc-cn-beijing:zh_female_qingxinnvsheng_mars_bigtts",
		"journey-arrival-narrator":    "volc-tenant:volc-cn-beijing:zh_female_shaoergushi_mars_bigtts",
	} {
		binding := alias + ":\n        resource_id: " + resourceID
		if !strings.Contains(text, binding) {
			t.Fatalf("RuntimeProfile/default mapping %s = %s is missing", alias, resourceID)
		}
	}
}

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
		"resources/05-workspaces/00-invalid.yaml": {Data: []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: Workspace\nmetadata:\n  name: invalid\n")},
	})
	if err == nil {
		t.Fatal("LoadCatalog() error = nil")
	}
}

func TestCatalogRejectsLegacyAssetsWithDefaultRuntimeProfile(t *testing.T) {
	profile := []byte("apiVersion: gizclaw.admin/v1alpha1\nkind: RuntimeProfile\nmetadata:\n  name: default\n")
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
