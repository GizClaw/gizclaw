package localserver_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
)

func TestBundledCatalogIsCompleteAndNeutral(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := localserver.LoadCatalog(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Resources) != 44 {
		t.Fatalf("resources = %d, want 44", len(catalog.Resources))
	}
	if len(catalog.WorkflowIcons) != 10 || len(catalog.PetDefPIXAs) != 9 || len(catalog.VoiceSyncs) != 1 {
		t.Fatalf("assets = workflows:%d pets:%d voice-sync:%d", len(catalog.WorkflowIcons), len(catalog.PetDefPIXAs), len(catalog.VoiceSyncs))
	}
	if len(catalog.Requirements) != 11 {
		t.Fatalf("environment requirements = %d, want 11", len(catalog.Requirements))
	}
	kinds := map[string]int{}
	identities := map[string]bool{}
	for _, resource := range catalog.Resources {
		kinds[resource.Kind]++
		identities[resource.Kind+"/"+resource.Name] = true
		if resource.Kind == "Workspace" {
			t.Fatalf("bundled client-created resource: %+v", resource)
		}
	}
	for _, identity := range []string{
		"Firmware/desktop-local",
		"RuntimeProfile/desktop-local",
	} {
		if !identities[identity] {
			t.Fatalf("missing local Play registration dependency %s", identity)
		}
	}
	for kind, want := range map[string]int{
		"Credential": 7, "VolcTenant": 2, "MiniMaxTenant": 1,
		"OpenAITenant": 2, "DashScopeTenant": 1, "Model": 10,
		"Workflow": 10, "PetDef": 9, "Firmware": 1, "RuntimeProfile": 1,
	} {
		if kinds[kind] != want {
			t.Fatalf("%s resources = %d, want %d", kind, kinds[kind], want)
		}
	}
	for _, removed := range []string{"ACLRole", "ACLView", "ACLPolicyBinding", "GameRuleset"} {
		if kinds[removed] != 0 {
			t.Fatalf("legacy %s resources = %d, want 0", removed, kinds[removed])
		}
	}
	profile, err := fs.ReadFile(catalog.FS, "resources/07-gameplay/20-default-gameplay.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(profile), "pet-care") {
		t.Fatalf("built-in pet-care Workflow must not be exposed by RuntimeProfile:\n%s", profile)
	}
	for _, requirement := range catalog.Requirements {
		if requirement.Name == "input" {
			t.Fatal("Flowcraft runtime placeholder was exposed as Desktop environment")
		}
		if requirement.Name == "GIZCLAW_MINIMAX_CN_VOICE_BASE_URL" || requirement.Name == "GIZCLAW_MINIMAX_GLOBAL_VOICE_BASE_URL" {
			t.Fatalf("fixed MiniMax endpoint was exposed as Desktop environment %s", requirement.Name)
		}
	}
	if identities["Credential/minimax-global-credential"] {
		t.Fatal("bundled unused MiniMax Global credential")
	}
	miniMaxTenant, err := fs.ReadFile(catalog.FS, "resources/01-tenants/02-minimax-cn.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(miniMaxTenant), "base_url: https://api.minimaxi.com") {
		t.Fatal("MiniMax CN tenant does not use the fixed CN endpoint")
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
		"resources/05-workspaces/00-invalid.yaml": {Data: []byte("kind: Workspace\nmetadata:\n  name: invalid\n")},
	})
	if err == nil {
		t.Fatal("LoadCatalog() error = nil")
	}
}
