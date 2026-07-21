package localserver_test

import (
	"io/fs"
	"maps"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GizClaw/gizclaw-go/apps/wails/internal/localserver"
	desktopresources "github.com/GizClaw/gizclaw-go/apps/wails/resources"
	"github.com/goccy/go-yaml"
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
	if len(catalog.PetDefPIXAs) != 9 || len(catalog.VoiceSyncs) != 2 {
		t.Fatalf("assets = pets:%d voice-sync:%d", len(catalog.PetDefPIXAs), len(catalog.VoiceSyncs))
	}
	if got := catalog.VoiceSyncs; got[0] != (localserver.VoiceSync{Provider: "minimax", Tenant: "minimax-cn"}) || got[1] != (localserver.VoiceSync{Provider: "volc", Tenant: "volc-main"}) {
		t.Fatalf("voice syncs = %#v", got)
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
	for _, identity := range []string{"RuntimeProfile/default"} {
		if !identities[identity] {
			t.Fatalf("missing local Play registration dependency %s", identity)
		}
	}
	for kind, want := range map[string]int{
		"Credential": 7, "VolcTenant": 2, "MiniMaxTenant": 1,
		"OpenAITenant": 2, "DashScopeTenant": 1, "Model": 10,
		"Workflow": 10, "Voice": 1, "PetDef": 9, "RuntimeProfile": 1,
	} {
		if kinds[kind] != want {
			t.Fatalf("%s resources = %d, want %d", kind, kinds[kind], want)
		}
	}
	for _, removed := range []string{"ACLRole", "ACLView", "ACLPolicyBinding", "GameRuleset", "Firmware"} {
		if kinds[removed] != 0 {
			t.Fatalf("legacy %s resources = %d, want 0", removed, kinds[removed])
		}
	}
	profile, err := fs.ReadFile(catalog.FS, "resources/07-runtime-profiles/00-default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(profile), "pet-care") {
		t.Fatalf("built-in pet-care Workflow must not be exposed by RuntimeProfile:\n%s", profile)
	}
	var parsed struct {
		Spec struct {
			Workflows struct {
				Collections map[string]map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"collections"`
			} `yaml:"workflows"`
			Resources struct {
				Voices map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"voices"`
			} `yaml:"resources"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(profile, &parsed); err != nil {
		t.Fatal(err)
	}
	wantWorkflows := map[string]string{
		"translate-zh-en-auto": "ast-translate-zh-en-auto",
		"translate-zh-ja":      "ast-translate-zh-ja",
		"translate-zh-ko":      "ast-translate-zh-ko",
		"translate-zh-es":      "ast-translate-zh-es",
		"doubao-realtime":      "doubao-realtime-conversation",
		"chat":                 "flowcraft-chat-assistant",
		"journey":              "flowcraft-journey-guide",
		"murder-mystery":       "flowcraft-murder-mystery",
	}
	gotWorkflows := map[string]string{}
	for _, workflows := range parsed.Spec.Workflows.Collections {
		for alias, binding := range workflows {
			gotWorkflows[alias] = binding.ResourceID
		}
	}
	if !maps.Equal(gotWorkflows, wantWorkflows) {
		t.Fatalf("RuntimeProfile/default Workflows = %#v, want %#v", gotWorkflows, wantWorkflows)
	}
	wantVoices := map[string]string{
		"doubao-assistant":  "volc-tenant:volc-main:zh_female_vv_jupiter_bigtts",
		"general-assistant": "volc-tenant:volc-main:zh_female_qingxinnvsheng_mars_bigtts",
		"cute-pet":          "volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts",
		"translator":        "volc-tenant:volc-main:zh_female_sophie_conversation_wvae_bigtts",
		"narrator":          "volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts",
		"game-master":       "volc-tenant:volc-main:zh_male_changtianyi_mars_bigtts",
		"detective":         "volc-tenant:volc-main:ICL_zh_male_lengjungaozhi_tob",
		"police-officer":    "volc-tenant:volc-main:ICL_zh_male_zhengzhiqingnian_tob",
		"sun-wukong":        "volc-tenant:volc-main:zh_male_sunwukong_mars_bigtts",
		"tang-sanzang":      "volc-tenant:volc-main:zh_male_tangseng_mars_bigtts",
		"zhu-bajie":         "volc-tenant:volc-main:zh_male_zhubajie_mars_bigtts",
	}
	gotVoices := make(map[string]string, len(parsed.Spec.Resources.Voices))
	for alias, binding := range parsed.Spec.Resources.Voices {
		gotVoices[alias] = binding.ResourceID
	}
	if !maps.Equal(gotVoices, wantVoices) {
		t.Fatalf("RuntimeProfile/default Voices = %#v, want %#v", gotVoices, wantVoices)
	}
	resourceIDs := map[string]struct{}{}
	for _, resourceID := range gotVoices {
		resourceIDs[resourceID] = struct{}{}
	}
	if len(resourceIDs) != len(gotVoices) {
		t.Fatalf("RuntimeProfile/default Voices reuse resource IDs: %#v", gotVoices)
	}
	for _, resource := range catalog.Resources {
		if resource.Kind != "PetDef" {
			continue
		}
		data, err := fs.ReadFile(catalog.FS, resource.Path)
		if err != nil {
			t.Fatal(err)
		}
		var petDef struct {
			Spec struct {
				Voice struct {
					VoiceID string `yaml:"voice_id"`
				} `yaml:"voice"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal(data, &petDef); err != nil {
			t.Fatal(err)
		}
		if petDef.Spec.Voice.VoiceID != "cute-pet" {
			t.Fatalf("%s voice_id = %q, want cute-pet", resource.Name, petDef.Spec.Voice.VoiceID)
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
