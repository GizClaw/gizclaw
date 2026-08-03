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

type flowcraftFixtureGraph struct {
	Entry         string                 `json:"entry" yaml:"entry"`
	Edges         []workflowEdge         `json:"edges" yaml:"edges"`
	Nodes         []flowcraftFixtureNode `json:"nodes" yaml:"nodes"`
	MaxIterations int                    `json:"max_iterations" yaml:"max_iterations"`
}

type flowcraftFixtureNode struct {
	ID     string `json:"id" yaml:"id"`
	Type   string `json:"type" yaml:"type"`
	Config struct {
		Source string `json:"source" yaml:"source"`
		Query  struct {
			TextFrom string `json:"text_from" yaml:"text_from"`
		} `json:"query" yaml:"query"`
		Observations []struct {
			TurnsFrom string `json:"turns_from" yaml:"turns_from"`
			TextFrom  string `json:"text_from" yaml:"text_from"`
			Facts     []struct {
				TextFrom string `json:"text_from" yaml:"text_from"`
			} `json:"facts" yaml:"facts"`
		} `json:"observations" yaml:"observations"`
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
	"19-flowcraft-realtime-chat.yaml",
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

func TestFlowcraftDirectFactFixturesDoNotMixModelExtraction(t *testing.T) {
	resources := []string{
		"05-flowcraft-basic.yaml",
		"06-flowcraft-chat.yaml",
		"08-flowcraft-journey.yaml",
		"10-flowcraft-multi-role-storyteller.yaml",
		"11-flowcraft-murder-mystery.yaml",
		"12-flowcraft-poetry-adventure-li-bai.yaml",
		"13-flowcraft-werewolf.yaml",
		"18-flowcraft-configured-memory.yaml",
	}
	for _, filename := range resources {
		t.Run("resource/"+filename, func(t *testing.T) {
			assertFlowcraftDirectFactsDoNotMixModelExtraction(t, loadResourceFlowcraftGraph(t, filename))
		})
	}

	workspaces := []string{
		"flowcraft-basic.json",
		"flowcraft-chat.json",
		"flowcraft-journey.json",
		"flowcraft-multi-role-storyteller.json",
		"flowcraft-murder-mystery.json",
		"flowcraft-poetry-adventure-li-bai.json",
		"flowcraft-werewolf.json",
		"flowcraft-configured-memory.json",
	}
	for _, filename := range workspaces {
		t.Run("workspace/"+filename, func(t *testing.T) {
			assertFlowcraftDirectFactsDoNotMixModelExtraction(t, loadWorkspaceFlowcraftGraph(t, filename))
		})
	}
}

func assertFlowcraftDirectFactsDoNotMixModelExtraction(t *testing.T, graph flowcraftFixtureGraph) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Type != "memory_observe" {
			continue
		}
		factCount := 0
		turnCount := 0
		for _, observation := range node.Config.Observations {
			factCount += len(observation.Facts)
			if observation.TurnsFrom != "" {
				turnCount++
			}
		}
		if factCount > 0 && turnCount > 0 {
			t.Fatalf("memory_observe node %q mixes model extraction with %d direct Facts", node.ID, factCount)
		}
	}
}

func TestFlowcraftJourneyIterationBudgetCoversEveryRoute(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		graph flowcraftFixtureGraph
	}{
		{name: "resource", graph: loadResourceFlowcraftGraph(t, "08-flowcraft-journey.yaml")},
		{name: "workspace", graph: loadWorkspaceFlowcraftGraph(t, "flowcraft-journey.json")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			longest := longestAcyclicFlowcraftRoute(t, fixture.graph)
			headroom := fixture.graph.MaxIterations - longest
			if headroom < 2 || headroom > 8 {
				t.Fatalf("max_iterations = %d for longest route %d, want bounded headroom in [2, 8]", fixture.graph.MaxIterations, longest)
			}
		})
	}
}

func TestFlowcraftWerewolfPreparesSelfStartRecallQuery(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		graph flowcraftFixtureGraph
	}{
		{name: "resource", graph: loadResourceFlowcraftGraph(t, "13-flowcraft-werewolf.yaml")},
		{name: "workspace", graph: loadWorkspaceFlowcraftGraph(t, "flowcraft-werewolf.json")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if fixture.graph.Entry != "prepare_memory_query" {
				t.Fatalf("entry = %q, want prepare_memory_query", fixture.graph.Entry)
			}
			if !hasFlowcraftFixtureEdge(fixture.graph.Edges, "prepare_memory_query", "recall_game_memory") {
				t.Fatal("prepare_memory_query does not route to recall_game_memory")
			}
			prepare := findFlowcraftFixtureNode(t, fixture.graph.Nodes, "prepare_memory_query")
			if prepare.Type != "script" || !strings.Contains(prepare.Config.Source, "input || \"狼人游戏状态与公开进度\"") {
				t.Fatalf("prepare_memory_query does not provide an input fallback: %#v", prepare.Config)
			}
			for _, nodeID := range []string{"recall_game_memory", "recall_public_memory"} {
				node := findFlowcraftFixtureNode(t, fixture.graph.Nodes, nodeID)
				if node.Type != "memory_recall" || node.Config.Query.TextFrom != "memory_query" {
					t.Fatalf("node %q query = %q, want memory_query", nodeID, node.Config.Query.TextFrom)
				}
			}
		})
	}
}

func TestFlowcraftWerewolfConversationObservationHasSelfStartFallback(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		graph flowcraftFixtureGraph
	}{
		{name: "resource", graph: loadResourceFlowcraftGraph(t, "13-flowcraft-werewolf.yaml")},
		{name: "workspace", graph: loadWorkspaceFlowcraftGraph(t, "flowcraft-werewolf.json")},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			load := findFlowcraftFixtureNode(t, fixture.graph.Nodes, "load_game_state")
			if !strings.Contains(load.Config.Source, `board.setVar("werewolf_game_state_text"`) {
				t.Fatal("load_game_state does not prepare werewolf_game_state_text")
			}
			observe := findFlowcraftFixtureNode(t, fixture.graph.Nodes, "observe_game_conversation")
			if len(observe.Config.Observations) != 2 {
				t.Fatalf("observe_game_conversation observations = %#v", observe.Config.Observations)
			}
			var hasTurns, hasState bool
			for _, source := range observe.Config.Observations {
				hasTurns = hasTurns || source.TurnsFrom == "conversation"
				hasState = hasState || source.TextFrom == "werewolf_game_state_text"
			}
			if !hasTurns || !hasState {
				t.Fatalf("observe_game_conversation sources = %#v", observe.Config.Observations)
			}
		})
	}
}

func hasFlowcraftFixtureEdge(edges []workflowEdge, from, to string) bool {
	for _, edge := range edges {
		if edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

func loadResourceFlowcraftGraph(t *testing.T, filename string) flowcraftFixtureGraph {
	t.Helper()
	var resource struct {
		Spec struct {
			Flowcraft struct {
				Graph         flowcraftFixtureGraph `yaml:"graph"`
				MaxIterations int                   `yaml:"max_iterations"`
			} `yaml:"flowcraft"`
		} `yaml:"spec"`
	}
	raw, err := os.ReadFile(filepath.Join("resources", "04-workflows", filename))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &resource); err != nil {
		t.Fatal(err)
	}
	graph := resource.Spec.Flowcraft.Graph
	graph.MaxIterations = resource.Spec.Flowcraft.MaxIterations
	return graph
}

func loadWorkspaceFlowcraftGraph(t *testing.T, filename string) flowcraftFixtureGraph {
	t.Helper()
	var workspace struct {
		Workflow struct {
			Flowcraft struct {
				Graph         flowcraftFixtureGraph `json:"graph"`
				MaxIterations int                   `json:"max_iterations"`
			} `json:"flowcraft"`
		} `json:"workflow"`
	}
	raw, err := os.ReadFile(filepath.Join("workspaces", filename))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &workspace); err != nil {
		t.Fatal(err)
	}
	graph := workspace.Workflow.Flowcraft.Graph
	graph.MaxIterations = workspace.Workflow.Flowcraft.MaxIterations
	return graph
}

func findFlowcraftFixtureNode(t *testing.T, nodes []flowcraftFixtureNode, id string) flowcraftFixtureNode {
	t.Helper()
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("Flowcraft node %q is missing", id)
	return flowcraftFixtureNode{}
}

func longestAcyclicFlowcraftRoute(t *testing.T, graph flowcraftFixtureGraph) int {
	t.Helper()
	adjacency := make(map[string][]string, len(graph.Edges))
	for _, edge := range graph.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	visiting := make(map[string]bool)
	memo := make(map[string]int)
	var visit func(string) int
	visit = func(node string) int {
		if node == "__end__" {
			return 0
		}
		if length, ok := memo[node]; ok {
			return length
		}
		if visiting[node] {
			t.Fatalf("Flowcraft graph contains a cycle through %q", node)
		}
		visiting[node] = true
		longest := -1
		for _, next := range adjacency[node] {
			if length := visit(next); length > longest {
				longest = length
			}
		}
		visiting[node] = false
		if longest < 0 {
			t.Fatalf("Flowcraft route from %q does not reach __end__", node)
		}
		memo[node] = longest + 1
		return memo[node]
	}
	return visit(graph.Entry)
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
