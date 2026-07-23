package localserver_test

import (
	"io/fs"
	"maps"
	"path/filepath"
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
	if len(catalog.Resources) != 43 {
		t.Fatalf("resources = %d, want 43", len(catalog.Resources))
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
	for _, identity := range []string{"RuntimeProfile/default", "RuntimeProfile/showcase"} {
		if !identities[identity] {
			t.Fatalf("missing local Play registration dependency %s", identity)
		}
	}
	for kind, want := range map[string]int{
		"Credential": 6, "VolcTenant": 2, "MiniMaxTenant": 1,
		"DeepSeekTenant": 1, "DashScopeTenant": 1, "Model": 10,
		"Workflow": 10, "Voice": 1, "PetDef": 9, "RuntimeProfile": 2,
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
	var parsed struct {
		Spec struct {
			Workflows struct {
				System struct {
					FriendChatroom string `yaml:"friend_chatroom"`
					GroupChatroom  string `yaml:"group_chatroom"`
					Pet            string `yaml:"pet"`
				} `yaml:"system"`
				Collections map[string]map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"collections"`
			} `yaml:"workflows"`
			Resources struct {
				Models map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"models"`
				Voices map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"voices"`
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
	wantWorkflows := map[string]string{
		"translate-zh-en-auto": "ast-translate-zh-en-auto",
		"translate-zh-ja":      "ast-translate-zh-ja",
		"translate-zh-ko":      "ast-translate-zh-ko",
		"translate-zh-es":      "ast-translate-zh-es",
		"doubao-realtime":      "doubao-realtime-conversation",
		"general-assistant":    "flowcraft-chat-assistant",
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
	wantModels := map[string]string{
		"asr":         "volc-bigasr-sauc",
		"realtime":    "doubao-realtime-dialog",
		"translation": "volc-ast-translate",
		"chat":        "doubao-seed-2-0-lite",
		"extraction":  "deepseek-v4-flash",
		"embedding":   "qwen3.7-text-embedding",
		"pet-chat":    "doubao-seed-2-0-lite",
		"pet-extract": "deepseek-v4-flash",
		"pet-asr":     "volc-bigasr-sauc",
	}
	gotModels := make(map[string]string, len(parsed.Spec.Resources.Models))
	for alias, binding := range parsed.Spec.Resources.Models {
		gotModels[alias] = binding.ResourceID
	}
	if !maps.Equal(gotModels, wantModels) {
		t.Fatalf("RuntimeProfile/default Models = %#v, want semantic role aliases %#v", gotModels, wantModels)
	}
	wantVoices := map[string]string{
		"doubao-assistant": "volc-tenant:volc-main:zh_female_vv_jupiter_bigtts",
		"assistant-voice":  "volc-tenant:volc-main:zh_female_qingxinnvsheng_mars_bigtts",
		"cute-pet":         "volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts",
		"translator":       "volc-tenant:volc-main:zh_female_sophie_conversation_wvae_bigtts",
		"narrator":         "volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts",
		"game-master":      "volc-tenant:volc-main:zh_male_changtianyi_mars_bigtts",
		"detective":        "volc-tenant:volc-main:ICL_zh_male_lengjungaozhi_tob",
		"police-officer":   "volc-tenant:volc-main:ICL_zh_male_zhengzhiqingnian_tob",
		"sun-wukong":       "volc-tenant:volc-main:zh_male_sunwukong_mars_bigtts",
		"tang-sanzang":     "volc-tenant:volc-main:zh_male_tangseng_mars_bigtts",
		"zhu-bajie":        "volc-tenant:volc-main:zh_male_zhubajie_mars_bigtts",
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
	if got := parsed.Spec.Gameplay.Adoption.Pool; len(got) != 9 {
		t.Fatalf("RuntimeProfile/default adoption pool entries = %d, want 9", len(got))
	} else {
		for _, entry := range got {
			if strings.TrimSpace(entry.PetDef) == "" {
				t.Fatalf("RuntimeProfile/default adoption entry = %#v, want PetDef alias", entry)
			}
		}
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
			I18n map[string]any `yaml:"i18n"`
			Spec struct {
				Voice map[string]any `yaml:"voice"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal(data, &petDef); err != nil {
			t.Fatal(err)
		}
		if len(petDef.I18n) != 0 {
			t.Fatalf("%s stores local i18n instead of RuntimeProfile binding metadata", resource.Name)
		}
		if _, exists := petDef.Spec.Voice["voice_id"]; exists {
			t.Fatalf("%s stores voice_id in PetDef instead of the configured Pet Workflow", resource.Name)
		}
		prompt, _ := petDef.Spec.Voice["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			t.Fatalf("%s voice prompt is empty", resource.Name)
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

func TestBundledFlowcraftGeneratorsUseProductionTokenBudget(t *testing.T) {
	source, err := desktopresources.LocalServer()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := fs.Glob(source, "resources/04-workflows/*-flowcraft-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			raw, err := fs.ReadFile(source, path)
			if err != nil {
				t.Fatal(err)
			}
			var resource struct {
				Spec struct {
					Flowcraft struct {
						Agent struct {
							Graph struct {
								Nodes []struct {
									ID     string `yaml:"id"`
									Type   string `yaml:"type"`
									Config struct {
										MaxTokens int `yaml:"max_tokens"`
									} `yaml:"config"`
								} `yaml:"nodes"`
							} `yaml:"graph"`
						} `yaml:"agent"`
					} `yaml:"flowcraft"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(raw, &resource); err != nil {
				t.Fatal(err)
			}
			for _, node := range resource.Spec.Flowcraft.Agent.Graph.Nodes {
				if node.Type == "llm" && node.Config.MaxTokens != 2048 {
					t.Errorf("generator node %q max_tokens = %d, want 2048", node.ID, node.Config.MaxTokens)
				}
			}
		})
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
