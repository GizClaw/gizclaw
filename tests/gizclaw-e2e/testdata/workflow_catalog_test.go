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
	"15-dashscope-realtime.yaml",
	"16-doubao-realtime-duplex.yaml",
	"17-eino-memory.yaml",
	"18-flowcraft-configured-memory.yaml",
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

func TestMemoryMigratedFlowcraftFixturesDecodeTypedGraph(t *testing.T) {
	for _, filename := range []string{
		"05-flowcraft-basic.yaml",
		"06-flowcraft-chat.yaml",
		"08-flowcraft-journey.yaml",
		"10-flowcraft-multi-role-storyteller.yaml",
		"11-flowcraft-murder-mystery.yaml",
		"12-flowcraft-poetry-adventure-li-bai.yaml",
		"13-flowcraft-werewolf.yaml",
	} {
		t.Run(filename, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("resources", "04-workflows", filename))
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
			workflow, err := resource.AsWorkflowResource()
			if err != nil {
				t.Fatal(err)
			}
			if workflow.Spec.Memory == nil || strings.TrimSpace(string(*workflow.Spec.Memory)) == "" {
				t.Fatal("memory alias is required")
			}
			if workflow.Spec.Flowcraft == nil {
				t.Fatal("flowcraft config is required")
			}
			if err := workflow.Spec.Flowcraft.Validate(); err != nil {
				t.Fatalf("Flowcraft config: %v", err)
			}
			hasObserve := false
			for _, node := range workflow.Spec.Flowcraft.Graph.Nodes {
				discriminator, err := node.Discriminator()
				if err != nil {
					t.Fatalf("Flowcraft node discriminator: %v", err)
				}
				hasObserve = hasObserve || discriminator == "memory_observe"
			}
			if !hasObserve {
				t.Fatal("explicit memory_observe node is required")
			}
		})
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
			if layout.Spec.Flowcraft.Extraction.Model == "" ||
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
						Graph struct {
							Nodes []flowcraftGeneratorNode `yaml:"nodes"`
						} `yaml:"graph"`
					} `yaml:"flowcraft"`
				} `yaml:"spec"`
			}
			if err := yaml.Unmarshal(raw, &resource); err != nil {
				t.Fatal(err)
			}
			assertFlowcraftGeneratorTokenBudget(t, resource.Spec.Flowcraft.Graph.Nodes)
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
						Graph struct {
							Nodes []flowcraftGeneratorNode `json:"nodes"`
						} `json:"graph"`
					} `json:"flowcraft"`
				} `json:"workflow"`
			}
			if err := json.Unmarshal(raw, &workspace); err != nil {
				t.Fatal(err)
			}
			assertFlowcraftGeneratorTokenBudget(t, workspace.Workflow.Flowcraft.Graph.Nodes)
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

func TestMurderMysterySolvedChatRefreshesAuditBeforeObservation(t *testing.T) {
	assertSolvedChatAuditEdge := func(t *testing.T, edges []workflowEdge) {
		t.Helper()
		for _, edge := range edges {
			if edge.From == "solved_chat" {
				if edge.To != "write_case_audit" {
					t.Fatalf("solved_chat edge targets %q, want write_case_audit", edge.To)
				}
				return
			}
		}
		t.Fatal("solved_chat edge is missing")
	}

	t.Run("resource", func(t *testing.T) {
		var resource struct {
			Spec struct {
				Flowcraft struct {
					Graph struct {
						Edges []workflowEdge `yaml:"edges"`
					} `yaml:"graph"`
				} `yaml:"flowcraft"`
			} `yaml:"spec"`
		}
		raw, err := os.ReadFile(filepath.Join("resources", "04-workflows", "11-flowcraft-murder-mystery.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(raw, &resource); err != nil {
			t.Fatal(err)
		}
		assertSolvedChatAuditEdge(t, resource.Spec.Flowcraft.Graph.Edges)
	})

	t.Run("workspace", func(t *testing.T) {
		var workspace struct {
			Workflow struct {
				Flowcraft struct {
					Graph struct {
						Edges []workflowEdge `json:"edges"`
					} `json:"graph"`
				} `json:"flowcraft"`
			} `json:"workflow"`
		}
		raw, err := os.ReadFile(filepath.Join("workspaces", "flowcraft-murder-mystery.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &workspace); err != nil {
			t.Fatal(err)
		}
		assertSolvedChatAuditEdge(t, workspace.Workflow.Flowcraft.Graph.Edges)
	})
}

func TestWerewolfLifecycleToolNodesAreRemoved(t *testing.T) {
	var resource struct {
		Spec struct {
			Flowcraft struct {
				Graph struct {
					Nodes []workflowNodePublication `yaml:"nodes"`
				} `yaml:"graph"`
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
	assertWerewolfLifecycleNodesRemoved(t, "resource", resource.Spec.Flowcraft.Graph.Nodes)

	var workspace struct {
		Workflow struct {
			Flowcraft struct {
				Graph struct {
					Nodes []workflowNodePublication `json:"nodes"`
				} `json:"graph"`
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
	assertWerewolfLifecycleNodesRemoved(t, "workspace", workspace.Workflow.Flowcraft.Graph.Nodes)
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
