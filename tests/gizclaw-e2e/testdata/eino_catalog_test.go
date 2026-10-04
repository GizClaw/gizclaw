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
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
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

func TestNativeEinoScenarioGraphs(t *testing.T) {
	for _, pattern := range []string{"resources/04-workflows/*-eino-*.yaml", "resources/04-workflows/32-giztest-workflow-tester.yaml", "resources/04-workflows/48-mem0-extraction.yaml", "workspaces/eino-*.json"} {
		paths, err := filepath.Glob(pattern)
		if err != nil || len(paths) == 0 {
			t.Fatalf("%s: %v", pattern, err)
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				for _, spec := range fixtureEinos(t, path) {
					graph, err := einoconfig.MapGraph(spec.Graph)
					if err != nil {
						t.Fatal(err)
					}
					compiled, err := genxeino.New(t.Context(), genxeino.Config{Agent: genxeino.AgentConfig{ID: "fixture"}, Graph: graph, Components: fixtureComponents{}, Memory: fixtureMemoryConfig()})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = compiled.Close() })
					for _, node := range graph.Nodes {
						if node.Script != nil && (strings.Contains(node.Script.Source, "_scope") || strings.Contains(node.Script.Source, "_channels")) {
							t.Fatalf("%s retains mechanically translated business source", node.ID)
						}
					}
				}
			})
		}
	}
}

func runRules(t *testing.T, name, command string, game map[string]any) map[string]any {
	t.Helper()
	graph := fixtureGraph(t, "workspaces/eino-"+name+".json")
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == "rules" {
			graph.Nodes[index].Inputs["text"] = genxeino.Binding{From: "command"}
		}
	}
	graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: "command", Type: genxeino.StateString, Merge: genxeino.MergeReplace})
	return runFixtureScript(t, graph, "rules", map[string]any{"game": game, "command": command})
}

func TestNativeWerewolfRulesAndPrivacy(t *testing.T) {
	initial := runRules(t, "werewolf", "开始", map[string]any{})
	game := initial["game"].(map[string]any)
	roles := game["roles"].(map[string]any)
	if len(roles) != 8 || game["phase"] != "night" {
		t.Fatalf("initial game = %v", game)
	}
	wolves := 0
	for _, role := range roles {
		if role == "狼人" {
			wolves++
		}
	}
	if wolves != 2 || strings.Contains(initial["context"].(string), `"roles"`) {
		t.Fatal("invalid role allocation or private role map leaked into public context")
	}
	// A known role map makes rule assertions independent from game-start time.
	game["roles"] = map[string]any{"1": "狼人", "2": "狼人", "3": "预言家", "4": "女巫", "5": "猎人", "6": "平民", "7": "平民", "8": "平民"}
	inspected := runRules(t, "werewolf", "查验1号", game)
	if inspected["valid"] != true || !strings.Contains(inspected["private"].(string), "1号是狼人") || strings.Contains(inspected["context"].(string), "1号是狼人") {
		t.Fatalf("inspection/private projection = %v", inspected)
	}
	game = inspected["game"].(map[string]any)
	if runRules(t, "werewolf", "查验2号", game)["valid"] != false {
		t.Fatal("second inspection in one night accepted")
	}
	if runRules(t, "werewolf", "刀4号", game)["valid"] != false {
		t.Fatal("non-wolf kill accepted")
	}
	rejected := runRules(t, "werewolf", "查验3号", game)
	if rejected["valid"] != false {
		t.Fatal("self-target was accepted")
	}
	day := runRules(t, "werewolf", "继续天亮", game)
	game = day["game"].(map[string]any)
	if game["phase"] != "speech" || len(game["dead"].([]any)) != 1 {
		t.Fatalf("night transition = %v", game)
	}
	if runRules(t, "werewolf", "我的发言", game)["valid"] != false {
		t.Fatal("eliminated player was allowed to act")
	}
	game["dead"] = []any{int64(6)}
	game = runRules(t, "werewolf", "我的发言：先分析线索", game)["game"].(map[string]any)
	if game["phase"] != "vote" {
		t.Fatalf("speech transition = %v", game)
	}
	if runRules(t, "werewolf", "投票3号", game)["valid"] != false {
		t.Fatal("self-vote accepted")
	}
	voted := runRules(t, "werewolf", "投票1号", game)
	replayed := runRules(t, "werewolf", "投票1号", game)
	first, _ := json.Marshal(voted["game"])
	second, _ := json.Marshal(replayed["game"])
	if string(first) != string(second) {
		t.Fatal("same vote state produced different rule outcomes")
	}
	if voted["valid"] != true || len(voted["game"].(map[string]any)["log"].([]any)) != 1 {
		t.Fatalf("valid vote = %v", voted)
	}
	spectator := voted["game"].(map[string]any)
	spectator["dead"], spectator["phase"], spectator["winner"] = []any{int64(3)}, "vote", ""
	progressed := runRules(t, "werewolf", "继续旁观", spectator)
	if progressed["valid"] != true || progressed["game"].(map[string]any)["phase"] == "vote" {
		t.Fatal("spectator continuation stalled")
	}
	// Eliminating the last wolf is decided by state rules, not narration.
	game = voted["game"].(map[string]any)
	game["roles"] = map[string]any{"1": "狼人", "2": "平民", "3": "女巫", "4": "平民", "5": "猎人", "6": "平民", "7": "平民", "8": "预言家"}
	game["phase"], game["dead"], game["poison_used"] = "night", []any{}, false
	finished := runRules(t, "werewolf", "毒1号", game)["game"].(map[string]any)
	if finished["winner"] != "好人" || finished["phase"] != "ended" {
		t.Fatalf("victory = %v", finished)
	}
}

func TestNativeMysteryAndPoetryProgression(t *testing.T) {
	mystery := runRules(t, "murder-mystery", "指认沈知秋", map[string]any{})["game"].(map[string]any)
	if mystery["solved"] != false {
		t.Fatal("mystery completed without evidence")
	}
	mystery = runRules(t, "murder-mystery", "调查书房门锁、壁炉、留声机、后廊和沈知秋房间", mystery)["game"].(map[string]any)
	mystery = runRules(t, "murder-mystery", "指认沈知秋", mystery)["game"].(map[string]any)
	if mystery["solved"] != true {
		t.Fatal("supported evidence-backed solution did not finish")
	}
	poetry := runRules(t, "poetry-adventure-li-bai", "错误答案", map[string]any{})["game"].(map[string]any)
	if poetry["stage"] != int64(0) {
		t.Fatal("wrong answer advanced the checkpoint")
	}
	for _, answer := range []string{"床前明月光", "大江东去，浪淘尽", "小桥流水人家", "要留清白在人间"} {
		poetry = runRules(t, "poetry-adventure-li-bai", answer, poetry)["game"].(map[string]any)
	}
	if poetry["stage"] != int64(4) || len(poetry["cleared"].([]any)) != 4 || poetry["score"] != int64(400) {
		t.Fatalf("poetry completion = %v", poetry)
	}
	replay := runRules(t, "poetry-adventure-li-bai", "要留清白在人间", poetry)["game"].(map[string]any)
	if replay["score"] != int64(400) || len(replay["cleared"].([]any)) != 4 {
		t.Fatal("completed checkpoint awarded twice")
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
	return runFixtureGraph(t, graph, fields)
}

func runFixtureGraph(t *testing.T, graph genxeino.GraphDefinition, fields map[string]any) map[string]any {
	t.Helper()
	store := &fixtureStateStore{fields: fields}
	selected := []string{}
	for _, field := range graph.State.Fields {
		selected = append(selected, field.Name)
	}
	transformer, err := genxeino.New(t.Context(), genxeino.Config{Agent: genxeino.AgentConfig{ID: "fixture"}, Graph: graph, Components: fixtureComponents{}, Memory: fixtureMemoryConfig(), State: &genxeino.StatePersistenceConfig{Store: store, Scope: "fixture", Fields: selected}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transformer.Close() })
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
			t.Fatalf("%s runtime failure: %s", graph.Name, chunk.Ctrl.Error)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.fields
}

// fixtureComponents executes the real prompt/model binding without provider I/O.
type fixtureComponents struct{}

func (fixtureComponents) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	return fixtureModel{}, nil
}
func (fixtureComponents) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, errors.New("unexpected retriever")
}

type fixtureModel struct{}

func (fixtureModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("fixture reply", nil), nil
}
func (fixtureModel) Stream(ctx context.Context, messages []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := (fixtureModel{}).Generate(ctx, messages, options...)
	return schema.StreamReaderFromArray([]*schema.Message{message}), err
}
func TestNativeScenarioExecution(t *testing.T) {
	for _, name := range []string{"journey", "multi-role-storyteller", "murder-mystery", "poetry-adventure-li-bai", "werewolf", "chat", "planner-latency-comparison"} {
		t.Run(name, func(t *testing.T) {
			graph := fixtureGraph(t, "workspaces/eino-"+name+".json")
			fields := runFixtureGraph(t, graph, map[string]any{"game": map[string]any{}})
			if fields["answer"] != "fixture reply" {
				t.Fatalf("native pipeline did not complete: %v", fields)
			}
		})
	}
}

func fixtureMemoryConfig() *genxeino.MemoryConfig {
	return &genxeino.MemoryConfig{Store: fixtureMemory{}, Scope: memory.Scope{AppID: "fixture", UserID: "player"}}
}

type fixtureMemory struct{}

func (fixtureMemory) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{Operation: &memory.Operation{ID: "fixture", Status: memory.OperationSucceeded}}, nil
}
func (fixtureMemory) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}
func (fixtureMemory) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, memory.ErrUnsupported
}
func (fixtureMemory) Delete(context.Context, memory.DeleteRequest) error {
	return memory.ErrUnsupported
}
func (fixtureMemory) Wait(context.Context, memory.OperationRequest) (memory.ObserveResult, error) {
	return memory.ObserveResult{Operation: &memory.Operation{ID: "fixture", Status: memory.OperationSucceeded}}, nil
}

func (fixtureMemory) SupportsDirectFactObservation() bool { return true }

func TestNativeLatencyComparisonsRemainDistinct(t *testing.T) {
	for _, test := range []struct {
		name   string
		models int
	}{{"latency-comparison", 1}, {"planner-latency-comparison", 2}} {
		graph := fixtureGraph(t, "workspaces/eino-"+test.name+".json")
		models := 0
		for _, node := range graph.Nodes {
			if node.ChatModel == nil {
				continue
			}
			models++
			want := 128
			if node.ID == "planner" {
				want = 64
			}
			if node.ChatModel.MaxTokens == nil || *node.ChatModel.MaxTokens != want {
				t.Fatalf("%s/%s budget changed", test.name, node.ID)
			}
		}
		if models != test.models {
			t.Fatalf("%s has %d model calls, want %d", test.name, models, test.models)
		}
	}
}
