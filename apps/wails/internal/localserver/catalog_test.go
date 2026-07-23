package localserver_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
	"github.com/goccy/go-yaml"
)

func TestBundledCatalogContainsOnlyDefaultRuntimeProfile(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	var resources []string
	err = fs.WalkDir(source, "resources", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && (path.Ext(name) == ".yaml" || path.Ext(name) == ".yml") {
			resources = append(resources, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0] != "resources/07-runtime-profiles/00-default.yaml" {
		t.Fatalf("bundled declarative resources = %v, want RuntimeProfile/default only", resources)
	}
	var assetCount int
	err = fs.WalkDir(source, "assets/pet-defs", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && path.Ext(name) == ".pixa" {
			assetCount++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if assetCount != 9 {
		t.Fatalf("bundled PetDef PIXA assets = %d, want 9", assetCount)
	}
	profile, err := fs.ReadFile(source, resources[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(profile)
	for _, forbidden := range []string{"volc-main"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("RuntimeProfile/default retains %q", forbidden)
		}
	}
	var parsed struct {
		Spec struct {
			Workflows struct {
				System struct {
					FriendChatroom string `yaml:"friend_chatroom"`
					GroupChatroom  string `yaml:"group_chatroom"`
					Pet            string `yaml:"pet"`
				} `yaml:"system"`
			} `yaml:"workflows"`
			Resources struct {
				PetDefs map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"pet_defs"`
			} `yaml:"resources"`
			Gameplay struct {
				Adoption struct {
					Pool []struct {
						PetDef string `yaml:"pet_def"`
					} `yaml:"pool"`
				} `yaml:"adoption"`
			} `yaml:"gameplay"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(profile, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Spec.Workflows.System.FriendChatroom != "chatroom" ||
		parsed.Spec.Workflows.System.GroupChatroom != "chatroom" ||
		parsed.Spec.Workflows.System.Pet != "pet-care" {
		t.Fatalf("RuntimeProfile/default system Workflows = %#v", parsed.Spec.Workflows.System)
	}
	if len(parsed.Spec.Resources.PetDefs) != 9 {
		t.Fatalf("RuntimeProfile/default PetDef bindings = %d, want 9", len(parsed.Spec.Resources.PetDefs))
	}
	if len(parsed.Spec.Gameplay.Adoption.Pool) != 9 {
		t.Fatalf("RuntimeProfile/default adoption pool = %d, want 9", len(parsed.Spec.Gameplay.Adoption.Pool))
	}
	for alias, binding := range parsed.Spec.Resources.PetDefs {
		if binding.ResourceID != "petdef-"+alias {
			t.Fatalf("RuntimeProfile/default PetDef %s = %s", alias, binding.ResourceID)
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
