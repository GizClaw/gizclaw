package testdata_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
	"github.com/goccy/go-yaml"
)

func fixtureEinos(t *testing.T, path string) []apitypes.EinoWorkflowSpec {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(path) == ".yaml" {
		raw, err = yaml.YAMLToJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
	}
	var document struct {
		Kind     string          `json:"kind"`
		Spec     json.RawMessage `json:"spec"`
		Workflow json.RawMessage `json:"workflow"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	fragments := []json.RawMessage{document.Spec}
	if filepath.Ext(path) == ".json" {
		// Workspace scenarios have runner-owned name/parameters metadata around
		// the public Eino payload; that envelope is not a WorkflowSpec.
		var workflow map[string]json.RawMessage
		if err := json.Unmarshal(document.Workflow, &workflow); err != nil {
			t.Fatal(err)
		}
		var public apitypes.EinoWorkflowSpec
		if err := json.Unmarshal(workflow["eino"], &public); err != nil {
			t.Fatal(err)
		}
		if err := einoconfig.Validate(public); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		return []apitypes.EinoWorkflowSpec{public}
	}
	if document.Kind == "ResourceList" {
		var list apitypes.ResourceListSpec
		if err := json.Unmarshal(document.Spec, &list); err != nil {
			t.Fatal(err)
		}
		fragments = nil
		for _, resource := range list.Items {
			workflow, err := resource.AsWorkflowResource()
			if err != nil {
				t.Fatal(err)
			}
			fragment, err := json.Marshal(workflow.Spec)
			if err != nil {
				t.Fatal(err)
			}
			fragments = append(fragments, fragment)
		}
	}
	var result []apitypes.EinoWorkflowSpec
	for _, fragment := range fragments {
		var spec apitypes.WorkflowSpec
		if err := json.Unmarshal(fragment, &spec); err != nil {
			t.Fatal(err)
		}
		if spec.Driver != apitypes.WorkflowDriverEino {
			continue
		}
		if spec.Eino == nil {
			t.Fatalf("%s does not contain an Eino payload", path)
		}
		if err := einoconfig.Validate(*spec.Eino); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		result = append(result, *spec.Eino)
	}
	if len(result) == 0 {
		t.Fatalf("%s does not contain an Eino workflow", path)
	}
	return result
}

func fixtureEino(t *testing.T, path string) apitypes.EinoWorkflowSpec {
	t.Helper()
	specs := fixtureEinos(t, path)
	if len(specs) != 1 {
		t.Fatalf("%s contains %d workflows; expected one", path, len(specs))
	}
	return specs[0]
}

func fixtureGraph(t *testing.T, path string) genxeino.GraphDefinition {
	t.Helper()
	graph, err := einoconfig.MapGraph(fixtureEino(t, path).Graph)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func TestEinoMemoryFixturesDecodeTypedGraph(t *testing.T) {
	for _, name := range []string{"basic", "chat", "journey", "multi-role-storyteller", "murder-mystery", "poetry-adventure-li-bai", "werewolf", "configured-memory"} {
		t.Run(name, func(t *testing.T) {
			graph := fixtureGraph(t, filepath.Join("workspaces", "eino-"+name+".json"))
			observations := 0
			for _, node := range graph.Nodes {
				if node.MemoryObserve == nil {
					continue
				}
				observations++
				observe := node.MemoryObserve
				if len(observe.Facts) != 0 && (observe.TextFrom != "" || observe.TurnsFrom != "") {
					t.Fatalf("%s mixes direct Facts and model extraction", node.ID)
				}
			}
			if observations == 0 {
				t.Fatal("explicit memory observation is missing")
			}
		})
	}
}

func TestEinoWorkspaceAndResourceGraphsValidate(t *testing.T) {
	for _, pattern := range []string{"resources/04-workflows/*-eino-*.yaml", "workspaces/eino-*.json"} {
		paths, err := filepath.Glob(pattern)
		if err != nil || len(paths) == 0 {
			t.Fatalf("%s: %v", pattern, err)
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) { fixtureEinos(t, path) })
		}
	}
}

func TestEinoJourneyIterationBudgetCoversEveryRoute(t *testing.T) {
	for _, path := range []string{"resources/04-workflows/08-eino-journey.yaml", "workspaces/eino-journey.json"} {
		t.Run(path, func(t *testing.T) {
			graph := fixtureGraph(t, path)
			adjacency := map[string][]string{}
			for _, edge := range graph.Edges {
				adjacency[edge.From] = append(adjacency[edge.From], edge.To)
			}
			for _, branch := range graph.Branches {
				adjacency[branch.From] = append(adjacency[branch.From], branch.Default)
				for _, route := range branch.Routes {
					adjacency[branch.From] = append(adjacency[branch.From], route.To)
				}
			}
			visiting := map[string]bool{}
			var longest func(string) int
			longest = func(node string) int {
				if node == "end" {
					return 0
				}
				if visiting[node] {
					t.Fatalf("unexpected cycle through %s", node)
				}
				visiting[node] = true
				length := 0
				for _, next := range adjacency[node] {
					length = max(length, longest(next)+1)
				}
				delete(visiting, node)
				return length
			}
			if steps := longest("start"); graph.Compile.MaxRunSteps < steps {
				t.Fatalf("MaxRunSteps=%d cannot cover %d-step route", graph.Compile.MaxRunSteps, steps)
			}
		})
	}
}

func TestEinoGeneratorsRetainTokenBudgets(t *testing.T) {
	for _, name := range []string{"basic", "chat", "journey", "multi-role-storyteller", "murder-mystery", "poetry-adventure-li-bai", "werewolf", "configured-memory", "planner-latency-comparison"} {
		graph := fixtureGraph(t, filepath.Join("workspaces", "eino-"+name+".json"))
		for _, node := range graph.Nodes {
			if node.ChatModel == nil {
				continue
			}
			want := 2048
			if name == "planner-latency-comparison" {
				want = map[string]int{"planner-model": 64, "answer-model": 128}[node.ID]
			}
			if node.ChatModel.MaxTokens == nil || *node.ChatModel.MaxTokens != want || want == 0 {
				t.Errorf("%s/%s max_tokens=%d, want %d", name, node.ID, node.ChatModel.MaxTokens, want)
			}
		}
	}
}

func TestEinoMurderMysterySolvedChatRefreshesAuditBeforeObservation(t *testing.T) {
	for _, path := range []string{"resources/04-workflows/11-eino-murder-mystery.yaml", "workspaces/eino-murder-mystery.json"} {
		graph := fixtureGraph(t, path)
		found := false
		for _, edge := range graph.Edges {
			if edge.From == "solved_chat-capture" {
				found = true
				if edge.To != "write_case_audit" {
					t.Fatalf("%s: solved chat routes to %s", path, edge.To)
				}
			}
		}
		if !found {
			t.Fatalf("%s: solved chat audit edge is missing", path)
		}
	}
}

func fixtureNode(t *testing.T, graph genxeino.GraphDefinition, id string) genxeino.NodeDefinition {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing node %s", id)
	return genxeino.NodeDefinition{}
}

type fixtureStateStore struct {
	mu     sync.Mutex
	fields map[string]any
}

func (store *fixtureStateStore) Load(context.Context, string) (genxeino.StateSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return genxeino.StateSnapshot{Fields: store.fields}, nil
}

func (store *fixtureStateStore) CompareAndSwap(_ context.Context, _, _ string, fields map[string]any) (genxeino.StateSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.fields = fields
	return genxeino.StateSnapshot{Version: "saved", Fields: fields}, nil
}

// Execute the committed Script with the production sandbox, state conversion,
// delivery observation and persistence paths rather than matching source text.
func runFixtureScript(t *testing.T, graph genxeino.GraphDefinition, id string, fields map[string]any) map[string]any {
	t.Helper()
	node := fixtureNode(t, graph, id)
	if node.Script == nil {
		t.Fatalf("%s is not a Script", id)
	}
	graph.Nodes = []genxeino.NodeDefinition{node, {
		ID: "complete", Outputs: map[string]string{"text": "completion"},
		Script: &genxeino.ScriptNode{Language: genxeino.ScriptStarlark, Source: "def run(input):\n    return {\"text\": \"checked\"}\n", Limits: genxeino.ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 4096, MaxOutputBytes: 4096}},
	}}
	graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: "completion", Type: genxeino.StateString, Merge: genxeino.MergeReplace})
	graph.Edges = []genxeino.EdgeDefinition{{From: "start", To: id}, {From: id, To: "complete"}, {From: "complete", To: "end"}}
	graph.Branches = nil
	graph.Outputs = []genxeino.OutputDefinition{{Node: "complete", Field: "completion", Name: "assistant", MIMEType: "text/plain", Primary: true}}
	store := &fixtureStateStore{fields: fields}
	selected := []string{}
	for _, field := range graph.State.Fields {
		selected = append(selected, field.Name)
	}
	transformer, err := genxeino.New(t.Context(), genxeino.Config{Agent: genxeino.AgentConfig{ID: "fixture"}, Graph: graph, State: &genxeino.StatePersistenceConfig{Store: store, Scope: "fixture", Fields: selected}})
	if err != nil {
		t.Fatal(err)
	}
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("execute"), Ctrl: &genx.StreamCtrl{StreamID: "fixture-input", BeginOfStream: true, EndOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("%s runtime failure: %s", id, chunk.Ctrl.Error)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fields
}

func TestEinoWerewolfSelfStartAndHiddenMoveIsolation(t *testing.T) {
	for _, path := range []string{"resources/04-workflows/13-eino-werewolf.yaml", "workspaces/eino-werewolf.json"} {
		t.Run(path, func(t *testing.T) {
			graph := fixtureGraph(t, path)
			for _, input := range []string{"", "我要查验1号"} {
				fields := map[string]any{"values": map[string]any{"input": input}, "channels": map[string]any{"main": []any{map[string]any{"role": "user", "content": input}}}}
				prepared := runFixtureScript(t, graph, "prepare_memory_query", fields)
				values := prepared["values"].(map[string]any)
				want := input
				if want == "" {
					want = "狼人游戏状态与公开进度"
				}
				if values["memory_query"] != want {
					t.Fatalf("memory query = %#v, want %q", values["memory_query"], want)
				}
				loaded := runFixtureScript(t, graph, "load_game_state", prepared)
				values = loaded["values"].(map[string]any)
				if text, ok := values["werewolf_game_state_text"].(string); !ok || strings.TrimSpace(text) == "" {
					t.Fatal("self-start did not prepare state text")
				}
				prompted := runFixtureScript(t, graph, "format_player_move-prompt", loaded)
				raw, err := json.Marshal(prompted["format_player_move-messages"])
				if err != nil {
					t.Fatal(err)
				}
				if input != "" && !strings.Contains(string(raw), input) {
					t.Fatalf("classifier lost latest user input: %s", raw)
				}
			}
			for _, node := range graph.Nodes {
				if node.ID == "call_game_event" || node.ID == "call_game_over_event" {
					t.Fatalf("unsupported lifecycle ToolCall %s", node.ID)
				}
			}
			turns := fixtureNode(t, graph, "observe_game_conversation-1").MemoryObserve
			state := fixtureNode(t, graph, "observe_game_conversation-0").MemoryObserve
			if turns == nil || turns.TurnsFrom == "" || state == nil || state.TextFrom == "" {
				t.Fatal("conversation and self-start state extraction must remain separate observations")
			}
		})
	}
}
