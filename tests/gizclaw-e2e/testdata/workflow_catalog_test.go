package testdata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
)

type workflowNodePublication struct {
	ID      string `json:"id" yaml:"id"`
	Publish *bool  `json:"publish" yaml:"publish"`
}

type workflowEdge struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

type workflowFixture struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		ID string `yaml:"id"`
	} `yaml:"metadata"`
	I18n any `yaml:"i18n"`
	Icon any `yaml:"icon"`
	Spec struct {
		Items []workflowFixture `yaml:"items"`
	} `yaml:"spec"`
}

func TestServerWorkspaceFixtureHasNoImplicitOrUnconsumedStoreEntries(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("server-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	storage, ok := config["storage"].(map[string]any)
	if !ok {
		t.Fatalf("storage = %#v", config["storage"])
	}
	stores, ok := config["stores"].(map[string]any)
	if !ok {
		t.Fatalf("stores = %#v", config["stores"])
	}
	services, ok := config["services"].(map[string]any)
	if !ok {
		t.Fatalf("services = %#v", config["services"])
	}
	for _, legacy := range []string{"agent_host", "system_log", "peers", "credentials", "firmwares", "minimax", "workspaces", "workflows", "history"} {
		if _, exists := config[legacy]; exists {
			t.Fatalf("legacy top-level binding %q remains", legacy)
		}
	}

	referencedStores := map[string]struct{}{}
	collectFixtureReferences(services, stores, referencedStores)
	referencedStorage := map[string]struct{}{}
	for name, value := range stores {
		if _, exists := referencedStores[name]; !exists {
			t.Fatalf("stores.%s has no explicit service consumer", name)
		}
		store, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("stores.%s = %#v", name, value)
		}
		if connector, ok := store["storage"].(string); ok && connector != "" {
			referencedStorage[connector] = struct{}{}
		}
	}
	for name := range storage {
		if _, exists := referencedStorage[name]; !exists {
			t.Fatalf("storage.%s has no logical Store consumer", name)
		}
	}
	workspace := services["workspace"].(map[string]any)
	if workspace["assets_store"] != "workspace-assets" {
		t.Fatalf("owner asset binding = workspace:%v", workspace["assets_store"])
	}
	if workspace["history_store"] != "workspace-history" {
		t.Fatalf("workspace history binding = %v, want workspace-history", workspace["history_store"])
	}
	if workspace["history_assets_store"] != "workspace-history-assets" {
		t.Fatalf("workspace history asset binding = %v", workspace["history_assets_store"])
	}
	history := stores["workspace-history"].(map[string]any)
	if history["kind"] != "log.mutable" || history["storage"] != "business-db" || history["table"] != "workspace_history" || history["ttl"] != "720h" {
		t.Fatalf("workspace history Store = %#v", history)
	}
	historyAssets := stores["workspace-history-assets"].(map[string]any)
	if historyAssets["kind"] != "objectstore" || historyAssets["ttl"] != history["ttl"] {
		t.Fatalf("workspace history asset Store = %#v", historyAssets)
	}
}

func TestEdgeWorkspaceGatewayLimitsAreRuntimeParameters(t *testing.T) {
	variables := []string{
		"GIZCLAW_E2E_GATEWAY_MAX_SESSIONS",
		"GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS",
		"GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM",
		"GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION",
		"GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM",
		"GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES",
	}
	for _, name := range []string{"config.yaml.template", "config.gateway-relay.yaml.template"} {
		raw, err := os.ReadFile(filepath.Join("edge-workspace", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, variable := range variables {
			if !strings.Contains(string(raw), "${"+variable+"}") {
				t.Errorf("%s does not use %s", name, variable)
			}
		}
	}
}

func collectFixtureReferences(value any, stores map[string]any, references map[string]struct{}) {
	switch current := value.(type) {
	case map[string]any:
		for _, child := range current {
			collectFixtureReferences(child, stores, references)
		}
	case []any:
		for _, child := range current {
			collectFixtureReferences(child, stores, references)
		}
	case string:
		if _, exists := stores[current]; exists {
			references[current] = struct{}{}
		}
	}
}

func TestWorkflowCatalogFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("resources", "04-workflows", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no Workflow fixtures found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var fixture workflowFixture
			if err := yaml.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			workflows := []workflowFixture{fixture}
			if fixture.Kind == "ResourceList" {
				if len(fixture.Spec.Items) == 0 {
					t.Fatal("ResourceList has no items")
				}
				workflows = fixture.Spec.Items
			}
			for _, workflow := range workflows {
				assertWorkflowCatalogFixture(t, workflow)
			}
		})
	}
}

func assertWorkflowCatalogFixture(t *testing.T, fixture workflowFixture) {
	t.Helper()
	if fixture.Kind != "Workflow" || fixture.Metadata.ID == "" {
		t.Fatalf("fixture identity = kind %q id %q", fixture.Kind, fixture.Metadata.ID)
	}
	if fixture.Icon != nil || fixture.I18n != nil {
		t.Fatalf("Workflow %q display metadata must be client-owned: icon=%#v i18n=%#v", fixture.Metadata.ID, fixture.Icon, fixture.I18n)
	}
}

func TestMemoryLayoutCatalogFixturesDecodeAllProviders(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("resources", "04-memory-layouts", "*.yaml"))
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
			layout, err := resource.AsMemoryLayoutResource()
			if err != nil {
				t.Fatal(err)
			}
			if layout.Spec.Mem0SelfHosted == nil ||
				layout.Spec.Mem0SelfHosted.CustomInstructions == nil ||
				layout.Spec.Mem0.CustomInstructions == nil ||
				strings.TrimSpace(*layout.Spec.Mem0.CustomInstructions) == "" ||
				len(layout.Spec.VolcMem0.Strategies) == 0 {
				t.Fatalf("incomplete provider blocks: %#v", layout.Spec)
			}
		})
	}
}

func TestSocialFixtures(t *testing.T) {
	for _, filename := range []string{"00-family-circle.yaml", "10-contacts.yaml"} {
		t.Run(filename, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("fixtures", "social", filename))
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Kind string `yaml:"kind"`
				Spec struct {
					Items []struct {
						Kind string `yaml:"kind"`
					} `yaml:"items"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			if fixture.Kind != "ResourceList" || len(fixture.Spec.Items) == 0 {
				t.Fatalf("social fixture = kind %q items %d", fixture.Kind, len(fixture.Spec.Items))
			}
			for i, item := range fixture.Spec.Items {
				if item.Kind == "" {
					t.Fatalf("social fixture item %d has no kind", i)
				}
			}
		})
	}
}

func TestE2EServerConfigProvidesOwnerAssetStores(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("server-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Stores map[string]struct {
			Kind    string `yaml:"kind"`
			Storage string `yaml:"storage"`
			Prefix  string `yaml:"prefix"`
		} `yaml:"stores"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"workspace-assets": "workspaces",
	}
	for name, prefix := range wants {
		store, ok := config.Stores[name]
		if !ok {
			t.Fatalf("missing owner asset store %q", name)
		}
		if store.Kind != "objectstore" || store.Storage != "local-files" || store.Prefix != prefix {
			t.Fatalf("owner asset store %q = %#v", name, store)
		}
	}
}
