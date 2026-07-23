package testdata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

type workflowNodePublication struct {
	ID      string `json:"id" yaml:"id"`
	Publish *bool  `json:"publish" yaml:"publish"`
}

type flowcraftGeneratorNode struct {
	ID     string `json:"id" yaml:"id"`
	Type   string `json:"type" yaml:"type"`
	Config struct {
		MaxTokens int `json:"max_tokens" yaml:"max_tokens"`
	} `json:"config" yaml:"config"`
}

var workflowFixtureFiles = []string{
	"00-ast-translate-tts.yaml",
	"01-ast-translate-zh-jp.yaml",
	"02-ast-translate.yaml",
	"03-chatroom.yaml",
	"04-doubao-realtime.yaml",
	"05-flowcraft-basic.yaml",
	"06-flowcraft-chat.yaml",
	"08-flowcraft-journey.yaml",
	"10-flowcraft-multi-role-storyteller.yaml",
	"11-flowcraft-murder-mystery.yaml",
	"12-flowcraft-poetry-adventure-li-bai.yaml",
	"13-flowcraft-werewolf.yaml",
	"14-ast-translate-zh-en.yaml",
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

func TestFlowcraftGeneratorsUseProductionTokenBudget(t *testing.T) {
	resourcePaths, err := filepath.Glob(filepath.Join("resources", "04-workflows", "*-flowcraft-*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range resourcePaths {
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var resource struct {
				Spec struct {
					Flowcraft struct {
						Agent struct {
							Graph struct {
								Nodes []flowcraftGeneratorNode `yaml:"nodes"`
							} `yaml:"graph"`
						} `yaml:"agent"`
					} `yaml:"flowcraft"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(raw, &resource); err != nil {
				t.Fatal(err)
			}
			assertFlowcraftGeneratorTokenBudget(t, resource.Spec.Flowcraft.Agent.Graph.Nodes)
		})
	}

	workspacePaths, err := filepath.Glob(filepath.Join("workspaces", "flowcraft-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range workspacePaths {
		t.Run(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var workspace struct {
				Workflow struct {
					Flowcraft struct {
						Agent struct {
							Graph struct {
								Nodes []flowcraftGeneratorNode `json:"nodes"`
							} `json:"graph"`
						} `json:"agent"`
					} `json:"flowcraft"`
				} `json:"workflow"`
			}
			if err := json.Unmarshal(raw, &workspace); err != nil {
				t.Fatal(err)
			}
			assertFlowcraftGeneratorTokenBudget(t, workspace.Workflow.Flowcraft.Agent.Graph.Nodes)
		})
	}
}

func assertFlowcraftGeneratorTokenBudget(t *testing.T, nodes []flowcraftGeneratorNode) {
	t.Helper()
	for _, node := range nodes {
		if node.Type == "llm" && node.Config.MaxTokens != 2048 {
			t.Errorf("generator node %q max_tokens = %d, want 2048", node.ID, node.Config.MaxTokens)
		}
	}
}

func TestWerewolfLifecycleToolNodesAreRemoved(t *testing.T) {
	var resource struct {
		Spec struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `yaml:"nodes"`
					} `yaml:"graph"`
				} `yaml:"agent"`
				Memory struct {
					Extract struct {
						Enabled bool   `yaml:"enabled"`
						Model   string `yaml:"model"`
					} `yaml:"extract"`
				} `yaml:"memory"`
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
	assertWerewolfLifecycleNodesRemoved(t, "resource", resource.Spec.Flowcraft.Agent.Graph.Nodes)
	if !resource.Spec.Flowcraft.Memory.Extract.Enabled || resource.Spec.Flowcraft.Memory.Extract.Model != "llm" {
		t.Fatalf("resource extraction = enabled %v model %q, want enabled with runtime alias llm", resource.Spec.Flowcraft.Memory.Extract.Enabled, resource.Spec.Flowcraft.Memory.Extract.Model)
	}

	var workspace struct {
		Workflow struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `json:"nodes"`
					} `json:"graph"`
				} `json:"agent"`
				Memory struct {
					Extract struct {
						Enabled bool   `json:"enabled"`
						Model   string `json:"model"`
					} `json:"extract"`
				} `json:"memory"`
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
	assertWerewolfLifecycleNodesRemoved(t, "workspace", workspace.Workflow.Flowcraft.Agent.Graph.Nodes)
	if !workspace.Workflow.Flowcraft.Memory.Extract.Enabled || workspace.Workflow.Flowcraft.Memory.Extract.Model != "llm" {
		t.Fatalf("workspace extraction = enabled %v model %q, want enabled with runtime alias llm", workspace.Workflow.Flowcraft.Memory.Extract.Enabled, workspace.Workflow.Flowcraft.Memory.Extract.Model)
	}
}

func assertWerewolfLifecycleNodesRemoved(t *testing.T, source string, nodes []workflowNodePublication) {
	t.Helper()
	for _, node := range nodes {
		if node.ID == "call_game_event" || node.ID == "call_game_over_event" {
			t.Fatalf("%s retains unsupported ToolCall node %q", source, node.ID)
		}
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
