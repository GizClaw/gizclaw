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
	game["pending_day"] = int64(0)
	game["roles"] = map[string]any{"1": "狼人", "2": "狼人", "3": "预言家", "4": "女巫", "5": "猎人", "6": "平民", "7": "平民", "8": "平民"}
	if runRules(t, "werewolf", "查验18号", game)["valid"] != false {
		t.Fatal("out-of-range target was treated as seat 8")
	}
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
	ballot := voted["game"].(map[string]any)["log"].([]any)[0].(map[string]any)["votes"].(map[string]any)
	for seat, planned := range game["next_votes"].(map[string]any) {
		if seat != "3" && ballot[seat] != planned {
			t.Fatalf("NPC %s ballot %v contradicted its stated intent %v", seat, ballot[seat], planned)
		}
	}
	spectator := voted["game"].(map[string]any)
	spectator["dead"], spectator["phase"], spectator["winner"] = []any{int64(3)}, "vote", ""
	progressed := runRules(t, "werewolf", "继续旁观", spectator)
	if progressed["valid"] != true || progressed["game"].(map[string]any)["phase"] == "vote" {
		t.Fatal("spectator continuation stalled")
	}
	// Eliminating the last wolf is decided by state rules, not narration.
	game = voted["game"].(map[string]any)
	game["pending_day"] = int64(0)
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
			if name == "werewolf" {
				official, ok := fields["official"].(string)
				if !ok || !strings.Contains(official, "【法官公示】") || !strings.Contains(fields["answer"].(string), "【法官公示】") || len(fields["actors"].([]any)) != 0 {
					t.Fatalf("native referee did not isolate the model output: %v", fields)
				}
			} else if fields["answer"] != "fixture reply" {
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

func TestNativeJourneyChapterOrder(t *testing.T) {
	for _, name := range []string{"journey", "multi-role-storyteller"} {
		game := runRules(t, name, "开始", map[string]any{})["game"].(map[string]any)
		for _, setting := range []string{"天宫", "启程", "试炼", "三国", "取经"} {
			if name == "multi-role-storyteller" {
				game = runRules(t, name, "我选择让同伴分工护送，并承诺履行本章约定", game)["game"].(map[string]any)
			}
			game = runRules(t, name, "继续", game)["game"].(map[string]any)
			if game["setting"] != setting {
				t.Fatalf("%s chapter setting = %v, want %s", name, game["setting"], setting)
			}
		}
	}
}

func TestNativeChapterQuestionsAndObjectionsDoNotAdvance(t *testing.T) {
	for _, name := range []string{"journey", "multi-role-storyteller"} {
		game := runRules(t, name, "开始", map[string]any{})["game"].(map[string]any)
		for _, text := range []string{"不要继续，先解释刚才的约定", "下一章是什么？", "请解释为什么需要继续", "继续？"} {
			if got := runRules(t, name, text, game)["game"].(map[string]any); got["chapter"] != int64(0) || got["progress"] != int64(0) {
				t.Fatalf("%s advanced on question or objection %q: %v", name, text, got)
			}
		}
	}
	game := runRules(t, "multi-role-storyteller", "开始", map[string]any{})["game"].(map[string]any)
	if runRules(t, "multi-role-storyteller", "继续", game)["game"].(map[string]any)["chapter"] != int64(0) {
		t.Fatal("chapter advanced without a confirmed agreement")
	}
	game = runRules(t, "multi-role-storyteller", "我选择让妖怪守山，取经后偿还旧债", game)["game"].(map[string]any)
	objected := runRules(t, "multi-role-storyteller", "不同意，先解释守山如何保护猴群", game)["game"].(map[string]any)
	if runRules(t, "multi-role-storyteller", "继续", objected)["game"].(map[string]any)["chapter"] != int64(0) {
		t.Fatal("chapter advanced while the agreement was disputed")
	}
	confirmed := runRules(t, "multi-role-storyteller", "我确认师徒共同护山，妖怪负责巡逻", objected)["game"].(map[string]any)
	if runRules(t, "multi-role-storyteller", "请继续", confirmed)["game"].(map[string]any)["chapter"] != int64(1) {
		t.Fatal("confirmed agreement did not permit continuation")
	}
}
func TestNativeMysteryRequiresBothMotiveClues(t *testing.T) {
	for _, partial := range []string{"遗嘱", "旧报纸"} {
		game := runRules(t, "murder-mystery", "调查门锁、壁炉、留声机和后廊", map[string]any{})["game"].(map[string]any)
		game = runRules(t, "murder-mystery", "查看"+partial, game)["game"].(map[string]any)
		result := runRules(t, "murder-mystery", "指认沈知秋", game)
		game = result["game"].(map[string]any)
		if game["solved"] != false {
			t.Fatalf("only %s completed the case", partial)
		}
		hidden := "生父"
		remaining := "旧报纸"
		if partial == "旧报纸" {
			hidden, remaining = "遗产", "遗嘱"
		}
		if strings.Contains(result["context"].(string), "裙摆") {
			t.Fatal("unexamined skirt clue leaked from footprint evidence")
		}
		if strings.Contains(result["context"].(string), hidden) {
			t.Fatalf("undiscovered clue leaked: %v", result["context"])
		}
		game = runRules(t, "murder-mystery", "查看"+remaining, game)["game"].(map[string]any)
		if runRules(t, "murder-mystery", "指认沈知秋", game)["game"].(map[string]any)["solved"] != true {
			t.Fatal("complete motive evidence did not permit solution")
		}
	}
}

func TestScriptQualityNativePlayerAndJudge(t *testing.T) {
	for _, path := range []string{"resources/04-workflows/50-script-quality-player.yaml", "resources/04-workflows/51-script-quality-judge.yaml"} {
		graph := fixtureGraph(t, path)
		fields := runFixtureGraph(t, graph, map[string]any{"turn": int64(0), "mode": ""})
		if fields["answer"] != "fixture reply" {
			t.Fatal("native quality graph did not execute")
		}
		transformer, err := genxeino.New(t.Context(), genxeino.Config{Agent: genxeino.AgentConfig{ID: "quality"}, Graph: graph, Components: fixtureComponents{}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transformer.Close() })
	}
}

func TestNativeWorkflowTesterRejectsPrematureVerdicts(t *testing.T) {
	graph := fixtureGraph(t, "resources/04-workflows/32-giztest-workflow-tester.yaml")
	for _, text := range []string{"PASS", "FAIL", "  pass  ", ""} {
		result := runFixtureScript(t, graph, "publish", map[string]any{"turn": int64(1), "answer": text})
		if result["published"] != "请就刚才的话题再补充一点好吗？" {
			t.Fatalf("probe emitted premature verdict: %v", result["published"])
		}
	}
	for _, text := range []string{"PASS", "FAIL"} {
		result := runFixtureScript(t, graph, "publish", map[string]any{"turn": int64(8), "answer": text})
		if result["published"] != text {
			t.Fatalf("verdict changed: %v", result["published"])
		}
	}
}

func TestNativeWerewolfDiscussionDoesNotExecuteActions(t *testing.T) {
	initial := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	initial["pending_day"] = int64(0)
	initial["roles"] = map[string]any{"1": "狼人", "2": "狼人", "3": "女巫", "4": "预言家", "5": "猎人", "6": "平民", "7": "平民", "8": "平民"}
	for _, text := range []string{"我不使用毒药，想问昨晚谁被刀了", "我是3号，请问5号的身份能公开吗？"} {
		result := runRules(t, "werewolf", text, initial)
		game := result["game"].(map[string]any)
		if result["valid"] != true || game["poison_used"] != false || len(game["dead"].([]any)) != 0 || game["phase"] != "night" {
			t.Fatalf("discussion executed a night action: %v", result)
		}
	}
	initial["phase"] = "speech"
	result := runRules(t, "werewolf", "上一轮6号投票给2号，我想听5号对此的看法", initial)
	game := result["game"].(map[string]any)
	if result["valid"] != true || game["phase"] != "vote" {
		t.Fatalf("ballot discussion blocked speech: %v", result)
	}
	queried := runRules(t, "werewolf", "我想问5号为什么投票给2号", game)
	if queried["valid"] != true || len(queried["game"].(map[string]any)["log"].([]any)) != 0 {
		t.Fatalf("question submitted a ballot: %v", queried)
	}
	if runRules(t, "werewolf", "投票3号", game)["valid"] != false {
		t.Fatal("explicit self-vote was accepted")
	}
	voted := runRules(t, "werewolf", "投票1号", game)
	if voted["valid"] != true || len(voted["game"].(map[string]any)["log"].([]any)) != 1 {
		t.Fatalf("explicit legal ballot did not execute: %v", voted)
	}
}

func TestNativeWerewolfNightInspectionWithPhaseQualifier(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["roles"] = map[string]any{"1": "狼人", "2": "狼人", "3": "预言家", "4": "女巫", "5": "猎人", "6": "平民", "7": "平民", "8": "平民"}
	for _, command := range []string{"我选择在夜晚查验8号苏禾的身份。", "我决定在本夜查验8号"} {
		result := runRules(t, "werewolf", command, game)
		if result["game"].(map[string]any)["inspected_day"] != int64(1) || !strings.Contains(result["private"].(string), "8号是好人") || strings.Contains(result["context"].(string), "8号是好人") {
			t.Fatalf("legal inspection was omitted or exposed: %v", result)
		}
	}
	for _, command := range []string{"我不选择在夜晚查验8号", "昨晚我选择在夜晚查验8号", "我想问昨晚查验8号了吗"} {
		if runRules(t, "werewolf", command, game)["game"].(map[string]any)["inspected_day"] != int64(0) {
			t.Fatalf("discussion executed an inspection: %s", command)
		}
	}
}

func TestNativeWerewolfHunterResolvesBeforeVictory(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["roles"] = map[string]any{"1": "狼人", "2": "平民", "3": "猎人", "4": "预言家", "5": "女巫", "6": "平民", "7": "平民", "8": "狼人"}
	game["dead"] = []any{int64(4), int64(5), int64(6), int64(7), int64(8)}
	game["pending_day"], game["pending_victim"] = int64(1), int64(3)
	attacked := runRules(t, "werewolf", "继续天亮", game)
	pending := attacked["game"].(map[string]any)
	if pending["winner"] != "" || pending["hunter_pending"] != true {
		t.Fatalf("victory bypassed the player's hunter choice: %v", pending)
	}
	shot := runRules(t, "werewolf", "开枪1号", pending)
	if shot["game"].(map[string]any)["winner"] != "好人" || !strings.Contains(shot["official"].(string), "猎人开枪带走1号") {
		t.Fatalf("legal hunter shot did not settle victory: %v", shot)
	}
	for _, command := range []string{"我现在开枪带走1号，继续旁观。", "我决定开枪1号，继续旁观。"} {
		combined := runRules(t, "werewolf", command, pending)
		if combined["game"].(map[string]any)["winner"] != "好人" || !strings.Contains(combined["official"].(string), "猎人开枪带走1号") {
			t.Fatalf("continuation discarded the preceding hunter shot: %v", combined)
		}
	}
	npc := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	npc["roles"] = map[string]any{"1": "狼人", "2": "猎人", "3": "女巫", "4": "预言家", "5": "平民", "6": "平民", "7": "平民", "8": "狼人"}
	npc["pending_day"], npc["pending_victim"] = int64(1), int64(2)
	attacked = runRules(t, "werewolf", "继续天亮", npc)
	if attacked["game"].(map[string]any)["hunter_used"] != true || !strings.Contains(attacked["official"].(string), "2号猎人开枪带走") {
		t.Fatalf("NPC hunter's eligible elimination did not trigger a shot: %v", attacked)
	}
	poisoned := runRules(t, "werewolf", "毒2号", npc)
	if poisoned["game"].(map[string]any)["hunter_used"] == true || strings.Contains(poisoned["official"].(string), "猎人开枪") {
		t.Fatalf("poisoned hunter fired: %v", poisoned)
	}
	ended := shot["game"].(map[string]any)
	farewell := runRules(t, "werewolf", "就先到这里，下次再玩。", ended)
	if !strings.Contains(farewell["official"].(string), "谢谢参与，再见") {
		t.Fatal("farewell repeated the victory announcement without closing")
	}
	question := runRules(t, "werewolf", "猎人出局时开枪规则是什么？", ended)
	if !strings.Contains(question["official"].(string), "被女巫毒杀不能开枪") {
		t.Fatal("hunter rule question was unanswered")
	}
}

func TestNativeJourneyFutureMentionDoesNotSkipChapters(t *testing.T) {
	for _, name := range []string{"journey", "multi-role-storyteller"} {
		game := runRules(t, name, "开始", map[string]any{})["game"].(map[string]any)
		game = runRules(t, name, "我希望最终取得真经，先聊聊你大闹天宫的经历", game)["game"].(map[string]any)
		if game["setting"] != "花果山" {
			t.Fatalf("%s future goal changed the current chapter: %v", name, game)
		}
		if name == "multi-role-storyteller" {
			game = runRules(t, name, "我确认由妖怪守山并在取经后偿债", game)["game"].(map[string]any)
		}
		game = runRules(t, name, "继续", game)["game"].(map[string]any)
		if game["setting"] != "天宫" {
			t.Fatalf("%s explicit continuation skipped a chapter: %v", name, game)
		}
	}
}

func TestScriptQualityPlayerTracksItsRemainingTurns(t *testing.T) {
	graph := fixtureGraph(t, "resources/04-workflows/50-script-quality-player.yaml")
	for _, before := range []int64{0, 18, 19} {
		fields := runFixtureGraph(t, graph, map[string]any{"turn": before, "mode": "actions"})
		if fields["turn"] != before+1 {
			t.Fatalf("player turn = %v, want %d", fields["turn"], before+1)
		}
	}
}

func TestNativeWerewolfPrivateTeammatesAndTally(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["pending_day"] = int64(0)
	game["roles"] = map[string]any{"1": "狼人", "2": "平民", "3": "狼人", "4": "预言家", "5": "猎人", "6": "女巫", "7": "平民", "8": "平民"}
	rejected := runRules(t, "werewolf", "刀1号", game)
	if rejected["valid"] != false || !strings.Contains(rejected["feedback"].(string), "同伴") || !strings.Contains(rejected["private"].(string), "1号") {
		t.Fatalf("teammate rejection did not explain a legal alternative: %v", rejected)
	}
	if strings.Contains(rejected["context"].(string), "同伴") {
		t.Fatal("private teammate information entered public Memory input")
	}
	game = runRules(t, "werewolf", "刀2号", game)["game"].(map[string]any)
	game = runRules(t, "werewolf", "我的发言：4号的理由值得质疑", game)["game"].(map[string]any)
	game = runRules(t, "werewolf", "投票4号", game)["game"].(map[string]any)
	ballot := game["log"].([]any)[0].(map[string]any)
	counts := ballot["counts"].(map[string]any)
	var total int64
	for _, count := range counts {
		total += count.(int64)
	}
	if total != int64(len(ballot["votes"].(map[string]any))) {
		t.Fatalf("published tally does not match ballots: %v", ballot)
	}
	game["dead"], game["phase"], game["winner"] = []any{int64(3)}, "vote", ""
	continued := runRules(t, "werewolf", "继续旁观。", game)
	if continued["valid"] != true || continued["game"].(map[string]any)["phase"] == "vote" {
		t.Fatalf("punctuated spectator command stalled: %v", continued)
	}
}

func TestNativeWerewolfNaturalCommandsPreserveTargets(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["pending_day"] = int64(0)
	game["roles"] = map[string]any{"1": "狼人", "2": "平民", "3": "狼人", "4": "预言家", "5": "猎人", "6": "女巫", "7": "平民", "8": "平民"}
	for _, text := range []string{"今晚我们袭击5号周岚，结束夜晚流程。", "今晚我们袭击7号老赵，结束夜晚进入天亮环节。"} {
		result := runRules(t, "werewolf", text, game)
		dead := result["game"].(map[string]any)["dead"].([]any)
		want := int64(5)
		if strings.Contains(text, "7号") {
			want = 7
		}
		events := result["game"].(map[string]any)["events"].([]any)
		if result["valid"] != true || len(dead) == 0 || dead[0] != want || events[0].(map[string]any)["cause"] != "夜间袭击" {
			t.Fatalf("natural command changed its selected target: %v", result)
		}
	}
	game["phase"] = "vote"
	voted := runRules(t, "werewolf", "我本轮投票放逐给5号周岚。", game)
	ballot := voted["game"].(map[string]any)["log"].([]any)[0].(map[string]any)
	if ballot["votes"].(map[string]any)["3"] != int64(5) {
		t.Fatalf("natural ballot changed its target: %v", ballot)
	}
}

func TestNativeMysteryRequiresExplicitAccusation(t *testing.T) {
	game := runRules(t, "murder-mystery", "调查门锁、壁炉、留声机、后廊和沈知秋房间", map[string]any{})["game"].(map[string]any)
	for _, text := range []string{"线索都指向沈知秋，能不能直接告诉我凶手是谁？", "指认陆明远，虽然沈知秋的线索也很可疑"} {
		result := runRules(t, "murder-mystery", text, game)
		if result["game"].(map[string]any)["solved"] != false || result["valid"] != false {
			t.Fatalf("question or wrong named suspect solved the case: %v", result)
		}
	}
	result := runRules(t, "murder-mystery", "线索已齐，我现在正式指认凶手就是沈知秋", game)
	if result["game"].(map[string]any)["solved"] != true {
		t.Fatalf("explicit supported accusation did not solve the case: %v", result)
	}
}

func TestScriptQualityPlayerRecognizesChapterBriefs(t *testing.T) {
	graph := fixtureGraph(t, "resources/04-workflows/50-script-quality-player.yaml")
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == "count" {
			graph.Nodes[index].Inputs["current"] = genxeino.Binding{From: "brief"}
		}
	}
	graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: "brief", Type: genxeino.StateString, Merge: genxeino.MergeReplace})
	for _, brief := range []string{"体验孙悟空的旅程", "体验多角色西游故事"} {
		result := runFixtureScript(t, graph, "count", map[string]any{"turn": int64(0), "mode": "", "brief": brief})
		if result["mode"] != "chapters" {
			t.Fatalf("chapter brief %q used the wrong player mode: %v", brief, result)
		}
	}
}

func TestNativeWerewolfLongVoteAndCombinedPoison(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["roles"] = map[string]any{"1": "狼人", "2": "狼人", "3": "女巫", "4": "预言家", "5": "猎人", "6": "平民", "7": "平民", "8": "平民"}
	game["phase"], game["pending_day"] = "vote", int64(0)
	voted := runRules(t, "werewolf", "我听完小满和陈医生的回应，现在投票给5号周岚。", game)
	ballot := voted["game"].(map[string]any)["log"].([]any)[0].(map[string]any)
	if ballot["votes"].(map[string]any)["3"] != int64(5) || voted["game"].(map[string]any)["phase"] == "vote" {
		t.Fatalf("long vote failed to advance exactly once: %v", voted)
	}
	game["phase"], game["dead"], game["log"], game["events"] = "night", []any{}, []any{}, []any{}
	game["pending_day"], game["poison_used"] = int64(0), false
	poisoned := runRules(t, "werewolf", "本轮平票后场上剩余狼人需要追刀，我选择使用毒药毒杀7号老赵，毒杀完成后继续天亮。", game)
	result := poisoned["game"].(map[string]any)
	if poisoned["valid"] != true || result["poison_used"] != true || result["phase"] == "night" {
		t.Fatalf("combined skill and night completion failed: %v", poisoned)
	}
	events := result["events"].([]any)
	if len(events) != 2 || events[0].(map[string]any)["seat"] != int64(7) || events[0].(map[string]any)["cause"] != "女巫毒药" || events[1].(map[string]any)["cause"] != "夜间袭击" {
		t.Fatalf("death causes were lost or merged: %v", events)
	}
	if !strings.Contains(poisoned["official"].(string), "7号因女巫毒药出局") {
		t.Fatal("the referee did not publish the actual skill result")
	}
}

func TestNativeWerewolfPublisherRejectsModelRulings(t *testing.T) {
	graph := fixtureGraph(t, "workspaces/eino-werewolf.json")
	public := `{"phase":"vote","alive":[1,3,4,5],"dead":[2,6,7,8],"winner":""}`
	actors := []any{map[string]any{"seat": int64(1), "name": "林知", "ended": false}, map[string]any{"seat": int64(4), "name": "小满", "ended": false}, map[string]any{"seat": int64(2), "name": "阿澈", "ended": false}, map[string]any{"seat": int64(5), "name": "周岚", "ended": false}}
	replies := []any{"天亮了，进入第2夜。", "5号获得6票。", "我赞同你的怀疑。", "我不认同你对4号的判断，需要更多理由。"}
	result := runFixtureScript(t, graph, "publish", map[string]any{"game": map[string]any{}, "audit_due": false, "official": "【法官公示】可信投票阶段。", "context": public, "actors": actors, "replies": replies})
	answer := result["answer"].(string)
	if !strings.Contains(answer, "【周岚】") || strings.Contains(answer, "第2夜") || strings.Contains(answer, "6票") || strings.Contains(answer, "【阿澈】") {
		t.Fatalf("model speech overrode the referee or revived a dead seat: %s", answer)
	}
}

func TestNativeWerewolfPrivateHistoryIsNotNPCMemory(t *testing.T) {
	graph := fixtureGraph(t, "workspaces/eino-werewolf.json")
	node := fixtureNode(t, graph, "public-history")
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == node.ID {
			graph.Nodes[i].Inputs["history"] = genxeino.Binding{From: "source_history"}
		}
	}
	graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: "source_history", Type: genxeino.StateMessages, Merge: genxeino.MergeReplace})
	history := []*schema.Message{schema.AssistantMessage("私密提示（仅你可见）：你是狼人，同伴为2号。\n【林知】我认为目前信息不足。", nil)}
	result := runFixtureScript(t, graph, "public-history", map[string]any{"source_history": history})
	messages := result["public_history"].([]*schema.Message)
	if len(messages) != 1 || strings.Contains(messages[0].Content, "狼人") || strings.Contains(messages[0].Content, "同伴") || !strings.Contains(messages[0].Content, "【林知】") {
		t.Fatalf("private referee hints entered NPC history: %v", messages)
	}
	history = []*schema.Message{
		schema.AssistantMessage("【法官公示】当前阶段：夜晚。私密提示：同伴为2号。", nil),
		schema.UserMessage("我选择袭击4号"),
		schema.AssistantMessage("【法官公示】当前阶段：白天发言。\n【林知】请说清你的怀疑。", nil),
		schema.UserMessage("我怀疑1号，因为他没有证据就抗推4号。"),
	}
	result = runFixtureScript(t, graph, "public-history", map[string]any{"source_history": history})
	messages = result["public_history"].([]*schema.Message)
	if len(messages) != 2 || messages[1].Role != schema.User || !strings.Contains(messages[1].Content, "没有证据") || strings.Contains(messages[1].Content, "袭击") {
		t.Fatalf("public reasoning was lost or private night action leaked: %v", messages)
	}
}

func TestNativeWerewolfCompleteFlowAcrossAllRoleRotations(t *testing.T) {
	deck := []string{"狼人", "狼人", "预言家", "女巫", "猎人", "平民", "平民", "平民"}
	for rotation := range 8 {
		t.Run(deck[(2+rotation)%8], func(t *testing.T) {
			game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
			roles := map[string]any{}
			for index := range 8 {
				roles[string(rune('1'+index))] = deck[(index+rotation)%8]
			}
			game["roles"], game["pending_day"] = roles, int64(0)
			for turn := range 40 {
				phase := game["phase"].(string)
				if phase == "ended" {
					break
				}
				alive := []int64{}
				dead := map[int64]bool{}
				for _, seat := range game["dead"].([]any) {
					dead[seat.(int64)] = true
				}
				for seat := int64(1); seat <= 8; seat++ {
					if !dead[seat] {
						alive = append(alive, seat)
					}
				}
				command := "继续天亮"
				if dead[3] {
					command = "继续旁观。"
				} else if phase == "speech" {
					command = "我的发言：我怀疑较少回应问题的玩家"
				} else if phase == "vote" {
					for _, seat := range alive {
						if seat != 3 {
							command = "投票" + string(rune('0'+seat)) + "号"
							break
						}
					}
				}
				result := runRules(t, "werewolf", command, game)
				next := result["game"].(map[string]any)
				if result["valid"] != true || len(next["dead"].([]any)) < len(dead) {
					t.Fatalf("turn %d failed or revived a seat: %v", turn, result)
				}
				if phase == "speech" && next["phase"] != "vote" {
					t.Fatalf("speech did not advance to vote: %v", next)
				}
				if phase == "vote" {
					ballot := next["log"].([]any)[len(next["log"].([]any))-1].(map[string]any)
					votes := ballot["votes"].(map[string]any)
					if len(votes) != len(alive) || next["phase"] == "vote" {
						t.Fatalf("ballot size or phase is wrong: %v", ballot)
					}
					for voter, target := range votes {
						seat := int64(voter[0] - '0')
						if dead[seat] || dead[target.(int64)] || seat == target.(int64) {
							t.Fatalf("dead or self-vote: %v", ballot)
						}
					}
				}
				game = next
			}
			if game["phase"] != "ended" || game["winner"] == "" {
				t.Fatalf("role rotation stalled before a winner: %v", game)
			}
		})
	}
}

func TestNativeWerewolfRescueAndNextNightTarget(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["roles"] = map[string]any{"1": "狼人", "2": "平民", "3": "女巫", "4": "预言家", "5": "猎人", "6": "狼人", "7": "平民", "8": "平民"}
	game["pending_day"] = int64(0)
	rescued := runRules(t, "werewolf", "我选择使用解药救下今晚受袭的2号玩家。", game)
	game = rescued["game"].(map[string]any)
	if rescued["valid"] != true || game["potion_used"] != true || game["phase"] != "speech" || len(game["dead"].([]any)) != 0 || !strings.Contains(rescued["official"].(string), "2号获救") {
		t.Fatalf("explicit rescue did not execute: %v", rescued)
	}
	game = runRules(t, "werewolf", "我的发言：8号的理由需要佐证", game)["game"].(map[string]any)
	voted := runRules(t, "werewolf", "我本轮决定投票给8号。", game)
	game = voted["game"].(map[string]any)
	if game["phase"] != "night" || game["pending_day"] != game["day"] || game["pending_victim"] == int64(0) {
		t.Fatalf("next night target was not prepared on entry: %v", voted)
	}
	for _, seat := range game["dead"].([]any) {
		if seat == game["pending_victim"] {
			t.Fatal("next night target points at an eliminated seat")
		}
	}
	if !strings.Contains(voted["private"].(string), "解药已用") || strings.Contains(voted["private"].(string), "可以说‘救人’") {
		t.Fatalf("used potion is offered again: %v", voted["private"])
	}
}

func TestNativeWerewolfSpectatorNeverReceivesPlayerInstructions(t *testing.T) {
	game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
	game["phase"], game["dead"], game["winner"] = "speech", []any{int64(3)}, ""
	continued := runRules(t, "werewolf", "继续旁观", game)
	if continued["valid"] != true || continued["game"].(map[string]any)["phase"] != "vote" || strings.Contains(continued["official"].(string), "你的发言已记录") {
		t.Fatalf("spectator was treated as a speaker: %v", continued)
	}
	queried := runRules(t, "werewolf", "刚才的票型与理由是否一致？", continued["game"].(map[string]any))
	if queried["valid"] != true || queried["game"].(map[string]any)["phase"] != "vote" {
		t.Fatalf("spectator question was rejected or advanced the game: %v", queried)
	}
}

func TestNativeWerewolfActorContextsKeepRoleKnowledgeSeparate(t *testing.T) {
	graph := fixtureGraph(t, "workspaces/eino-werewolf.json")
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == "roles" {
			graph.Nodes[i].Inputs["text"] = genxeino.Binding{From: "command"}
		}
	}
	graph.State.Fields = append(graph.State.Fields, genxeino.StateField{Name: "command", Type: genxeino.StateString, Merge: genxeino.MergeReplace})
	game := map[string]any{"roles": map[string]any{"1": "狼人", "2": "预言家", "3": "女巫", "4": "平民", "5": "猎人", "6": "平民", "7": "狼人", "8": "平民"}, "log": []any{}, "speeches": map[string]any{"1": "上次我的观点"}, "next_votes": map[string]any{"1": int64(8), "2": int64(4)}}
	result := runFixtureScript(t, graph, "roles", map[string]any{"game": game, "command": "请林知和阿澈回应我的理由", "public_input": "请林知和阿澈回应我的理由", "context": `{"phase":"vote","alive":[1,2,3,4,5,6,7,8],"dead":[],"winner":""}`, "public_history": []*schema.Message{schema.AssistantMessage("【小满】我还需要更多线索。", nil)}})
	actors := result["actors"].([]any)
	if len(actors) != 2 {
		t.Fatalf("expected two requested responders, got %v", actors)
	}
	first, second := actors[0].(map[string]any), actors[1].(map[string]any)
	if first["role"] != "狼人" || second["role"] != "预言家" || first["own_last"] != "上次我的观点" {
		t.Fatalf("actor identity or own continuity was lost: %v", actors)
	}
	if !strings.Contains(first["knowledge"].(string), "7号") || strings.Contains(second["knowledge"].(string), "7号") || strings.Contains(first["public"].(string), "roles") || strings.Contains(second["public"].(string), "roles") {
		t.Fatalf("role table or teammate knowledge crossed actor scopes: %v", actors)
	}
	if !strings.Contains(first["knowledge"].(string), "暂定投8号") || !strings.Contains(second["knowledge"].(string), "暂定投4号") || strings.Contains(first["knowledge"].(string), "暂定投4号") || strings.Contains(first["public"].(string), "next_votes") {
		t.Fatalf("own ballot intent was lost or other intents leaked: %v", actors)
	}
	question := "我想问问8号平民，你出局前有没有察觉到1号或者2号的发言有狼人嫌疑？"
	result = runFixtureScript(t, graph, "roles", map[string]any{"game": game, "command": question, "public_input": question, "context": `{"phase":"ended","alive":[1,2,3,4,5,6,7],"dead":[8],"winner":"狼人"}`, "public_history": []*schema.Message{}})
	actors = result["actors"].([]any)
	if len(actors) != 1 || actors[0].(map[string]any)["name"] != "苏禾" || !strings.Contains(actors[0].(map[string]any)["knowledge"].(string), "你本人已出局") {
		t.Fatalf("suspects replaced the requested eliminated responder: %v", actors)
	}
	question = "我想问问2号预言家当时如何判断出1号狼人，而不是7号？"
	result = runFixtureScript(t, graph, "roles", map[string]any{"game": game, "command": question, "public_input": question, "context": `{"phase":"ended","alive":[1,2,3,4,5,6,7],"dead":[8],"winner":"狼人"}`, "public_history": []*schema.Message{}})
	actors = result["actors"].([]any)
	if len(actors) != 1 || actors[0].(map[string]any)["name"] != "阿澈" {
		t.Fatalf("embedded question targets replaced the explicit responder: %v", actors)
	}
}

func TestNativeWerewolfPublishesOneCompleteAssistantTurn(t *testing.T) {
	for _, command := range []string{"我怀疑1号，请林知和阿澈回应", "投票3号"} {
		t.Run(command, func(t *testing.T) {
			valid := command != "投票3号"
			graph := fixtureGraph(t, "workspaces/eino-werewolf.json")
			game := runRules(t, "werewolf", "开始", map[string]any{})["game"].(map[string]any)
			game["phase"] = "speech"
			store := &fixtureStateStore{fields: map[string]any{"game": game}}
			transformer, err := genxeino.New(t.Context(), genxeino.Config{Agent: genxeino.AgentConfig{ID: "complete-turn"}, Graph: graph, Components: fixtureComponents{}, Memory: fixtureMemoryConfig(), State: &genxeino.StatePersistenceConfig{Store: store, Scope: "fixture", Fields: []string{"game"}}})
			if err != nil {
				t.Fatal(err)
			}
			defer transformer.Close()
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(command), Ctrl: &genx.StreamCtrl{StreamID: "complete-input", BeginOfStream: true, EndOfStream: true}}); err != nil {
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
			var text, streamID string
			begins, ends := 0, 0
			for {
				chunk, err := output.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if chunk.Ctrl != nil {
					if chunk.Ctrl.Error != "" {
						t.Fatal(chunk.Ctrl.Error)
					}
					if streamID == "" {
						streamID = chunk.Ctrl.StreamID
					}
					if chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != "assistant" {
						t.Fatal("one reply was split across output routes")
					}
				}
				if chunk.IsBeginOfStream() {
					begins++
				}
				if chunk.IsEndOfStream() {
					ends++
				}
				if part, ok := chunk.Part.(genx.Text); ok {
					text += string(part)
				}
			}
			if begins != 1 || ends != 1 || !strings.Contains(text, "【法官公示】") || (valid && (!strings.Contains(text, "【林知】") || !strings.Contains(text, "【阿澈】"))) || (!valid && strings.Contains(text, "【林知】")) {
				t.Fatalf("incomplete assistant turn BOS=%d EOS=%d text=%s", begins, ends, text)
			}

		})
	}
}
