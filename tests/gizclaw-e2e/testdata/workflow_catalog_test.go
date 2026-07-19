package testdata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
)

type workflowNodePublication struct {
	ID      string `json:"id" yaml:"id"`
	Publish *bool  `json:"publish" yaml:"publish"`
}

var workflowFixtureFiles = []string{
	"00-ast-translate-tts.yaml",
	"01-ast-translate-zh-jp.yaml",
	"02-ast-translate.yaml",
	"03-chatroom.yaml",
	"04-doubao-realtime.yaml",
	"05-flowcraft-basic.yaml",
	"06-flowcraft-chat.yaml",
	"07-flowcraft-func-chat.yaml",
	"08-flowcraft-journey.yaml",
	"09-flowcraft-match-route.yaml",
	"10-flowcraft-multi-role-storyteller.yaml",
	"11-flowcraft-murder-mystery.yaml",
	"12-flowcraft-poetry-adventure-li-bai.yaml",
	"13-flowcraft-werewolf.yaml",
	"14-ast-translate-zh-en.yaml",
	"20-flowcraft-assistant.yaml",
	"21-flowcraft-support.yaml",
	"22-chatroom-direct.yaml",
	"23-pet-care.yaml",
	"30-family-circle-chatroom.yaml",
}

type workflowFixture struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	I18n any `yaml:"i18n"`
	Icon any `yaml:"icon"`
}

func TestWorkflowCatalogFixtures(t *testing.T) {
	workflowDir := filepath.Join("resources", "04-workflows")
	for _, filename := range workflowFixtureFiles {
		filename := filename
		t.Run(filename, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(workflowDir, filename))
			if err != nil {
				t.Fatal(err)
			}
			var fixture workflowFixture
			if err := yaml.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			if fixture.Kind != "Workflow" || fixture.Metadata.Name == "" {
				t.Fatalf("fixture identity = kind %q name %q", fixture.Kind, fixture.Metadata.Name)
			}
			if fixture.Icon != nil || fixture.I18n != nil {
				t.Fatalf("Workflow display metadata must be client-owned: icon=%#v i18n=%#v", fixture.Icon, fixture.I18n)
			}
		})
	}
}

func TestWerewolfLifecycleToolNodesAreInternal(t *testing.T) {
	var resource struct {
		Spec struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `yaml:"nodes"`
					} `yaml:"graph"`
				} `yaml:"agent"`
			} `yaml:"flowcraft"`
		} `yaml:"spec"`
	}
	resourceRaw, err := os.ReadFile(filepath.Join("resources", "04-workflows", "13-flowcraft-werewolf.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(resourceRaw, &resource); err != nil {
		t.Fatal(err)
	}
	assertWerewolfLifecycleNodesInternal(t, "resource", resource.Spec.Flowcraft.Agent.Graph.Nodes)

	var workspace struct {
		Workflow struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `json:"nodes"`
					} `json:"graph"`
				} `json:"agent"`
			} `json:"flowcraft"`
		} `json:"workflow"`
	}
	workspaceRaw, err := os.ReadFile(filepath.Join("workspaces", "flowcraft-werewolf.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(workspaceRaw, &workspace); err != nil {
		t.Fatal(err)
	}
	assertWerewolfLifecycleNodesInternal(t, "workspace", workspace.Workflow.Flowcraft.Agent.Graph.Nodes)
}

func assertWerewolfLifecycleNodesInternal(t *testing.T, source string, nodes []workflowNodePublication) {
	t.Helper()
	want := map[string]bool{"call_game_event": false, "call_game_over_event": false}
	for _, node := range nodes {
		if _, ok := want[node.ID]; !ok {
			continue
		}
		if node.Publish == nil || *node.Publish {
			t.Fatalf("%s node %s publish = %v, want explicit false", source, node.ID, node.Publish)
		}
		delete(want, node.ID)
	}
	if len(want) != 0 {
		t.Fatalf("%s missing lifecycle nodes: %v", source, want)
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
		"gameplay-assets":  "gameplay",
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
