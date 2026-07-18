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
	if len(catalog.Resources) != 70 {
		t.Fatalf("resources = %d, want 70", len(catalog.Resources))
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
		"ACLPolicyBinding/default-client-model-minimax-cn-m3",
		"ACLPolicyBinding/default-client-credential-minimax-cn-openai-credential",
		"ACLPolicyBinding/default-client-voice-volc-sunwukong",
		"ACLPolicyBinding/default-client-gameruleset-default-gameplay",
	} {
		if !identities[identity] {
			t.Fatalf("missing Pet runtime dependency closure %s", identity)
		}
	}
	for kind, want := range map[string]int{
		"Credential": 7, "VolcTenant": 2, "MiniMaxTenant": 1,
		"OpenAITenant": 2, "DashScopeTenant": 1, "Model": 10,
		"Workflow": 10, "PetDef": 9, "GameRuleset": 1,
		"ACLRole": 3, "ACLView": 1, "ACLPolicyBinding": 23,
	} {
		if kinds[kind] != want {
			t.Fatalf("%s resources = %d, want %d", kind, kinds[kind], want)
		}
	}
	credentialRole, err := fs.ReadFile(catalog.FS, "resources/90-acl/02-credential-user-role.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(credentialRole), "    - use") || strings.Contains(string(credentialRole), "    - read") {
		t.Fatalf("credential-user role must grant use without read:\n%s", credentialRole)
	}
	for _, name := range []string{
		"30-00-credential-volc-main-credential.yaml",
		"30-01-credential-deepseek-main-credential.yaml",
		"30-02-credential-minimax-cn-openai-credential.yaml",
	} {
		binding, err := fs.ReadFile(catalog.FS, "resources/90-acl/"+name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(binding), "role: credential-user") {
			t.Fatalf("credential binding %s is peer-readable:\n%s", name, binding)
		}
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
