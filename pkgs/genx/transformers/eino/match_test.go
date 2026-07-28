package eino

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	genxmatch "github.com/GizClaw/gizclaw-go/pkgs/genx/match"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

func TestMatchNodeValidatesTypedPortsAndRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "missing input", mutate: func(config *Config) {
			config.Graph.Nodes[0].Inputs = nil
		}, want: "input ports must be exactly"},
		{name: "extra input", mutate: func(config *Config) {
			config.Graph.Nodes[0].Inputs["extra"] = Binding{From: "input.text"}
		}, want: "input ports must be exactly"},
		{name: "wrong input type", mutate: func(config *Config) {
			config.Graph.Nodes[0].Inputs["text"] = Binding{From: "matches"}
		}, want: "requires string, got list"},
		{name: "missing output", mutate: func(config *Config) {
			config.Graph.Nodes[0].Outputs = nil
		}, want: "output ports must be exactly"},
		{name: "wrong output type", mutate: func(config *Config) {
			config.Graph.Nodes[0].Outputs["matches"] = "answer"
		}, want: "requires list, got string"},
		{name: "padded model", mutate: func(config *Config) {
			config.Graph.Nodes[0].Match.Model = " route "
		}, want: "trimmed model alias"},
		{name: "qualified model", mutate: func(config *Config) {
			config.Graph.Nodes[0].Match.Model = "model/route"
		}, want: "without '/'"},
		{name: "empty rules", mutate: func(config *Config) {
			config.Graph.Nodes[0].Match.Rules = nil
		}, want: "rules are required"},
		{name: "union conflict", mutate: func(config *Config) {
			config.Graph.Nodes[0].Passthrough = &PassthroughNode{}
		}, want: "exactly one node body"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := matchConfig(&componentMapResolver{chat: &fakeChatModel{}})
			test.mutate(&config)
			if _, err := New(t.Context(), config); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestMatchNodeExecutesInBothTriggerModes(t *testing.T) {
	for _, mode := range []NodeTriggerMode{NodeTriggerAnyPredecessor, NodeTriggerAllPredecessor} {
		t.Run(string(mode), func(t *testing.T) {
			chat := &fakeChatModel{chunks: []*schema.Message{
				{Role: schema.Assistant, Content: "play_music: title="},
				{Role: schema.Assistant, Content: "卡农\n"},
			}}
			config := matchConfig(&componentMapResolver{chat: chat})
			config.Graph.Compile.NodeTriggerMode = mode
			normalized, graph := compileMatchGraph(t, config)
			state, err := newRunState(
				normalized.fields,
				graphInput{Text: "我想听卡农"},
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("newRunState() error = %v", err)
			}
			if err := graph.execute(t.Context(), state); err != nil {
				t.Fatalf("execute() error = %v", err)
			}
			got, err := state.value("matches")
			if err != nil {
				t.Fatalf("State matches: %v", err)
			}
			want := []any{map[string]any{
				"rule": "play_music",
				"args": map[string]any{
					"title": map[string]any{
						"value":     "卡农",
						"var":       map[string]any{"label": "歌曲名", "type": "string"},
						"has_value": true,
					},
				},
				"raw_text": "",
			}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("matches = %#v, want %#v", got, want)
			}
			chat.mu.Lock()
			defer chat.mu.Unlock()
			if len(chat.inputs) != 1 || len(chat.inputs[0]) != 2 {
				t.Fatalf("model inputs = %#v", chat.inputs)
			}
			if chat.inputs[0][0].Role != schema.System ||
				!strings.Contains(chat.inputs[0][0].Content, "play_music") ||
				chat.inputs[0][1].Role != schema.User ||
				chat.inputs[0][1].Content != "我想听卡农" {
				t.Fatalf("model inputs = %#v", chat.inputs[0])
			}
		})
	}
}

func TestMatchNodeAcceptsEmptyTextAndPublishesAtomically(t *testing.T) {
	chat := &fakeChatModel{
		chunks:      []*schema.Message{{Role: schema.Assistant, Content: "empty\n"}},
		terminalErr: errors.New("provider disconnected"),
	}
	config := matchConfig(&componentMapResolver{chat: chat})
	config.Graph.Nodes[0].Match.Rules = []*genxmatch.Rule{{
		Name: "empty", Patterns: []genxmatch.Pattern{{Input: ""}},
	}}
	normalized, graph := compileMatchGraph(t, config)
	state, err := newRunState(normalized.fields, graphInput{Text: ""}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	if err := graph.execute(t.Context(), state); err == nil ||
		!strings.Contains(err.Error(), "provider disconnected") {
		t.Fatalf("execute() error = %v", err)
	}
	if _, err := state.value("matches"); err == nil {
		t.Fatal("Match published parsed prefix after stream failure")
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if len(chat.inputs) != 1 || chat.inputs[0][1].Content != "" {
		t.Fatalf("model inputs = %#v", chat.inputs)
	}
}

func TestMatchNodeResolvesOnceAndOwnsCallerRules(t *testing.T) {
	chat := &fakeChatModel{chunks: []*schema.Message{{
		Role: schema.Assistant, Content: "route: value=kept\n",
	}}}
	resolver := &matchCountingResolver{chat: chat}
	config := matchConfig(resolver)
	rule := config.Graph.Nodes[0].Match.Rules[0]
	normalized, graph := compileMatchGraph(t, config)
	rule.Vars["title"] = genxmatch.Var{Label: "mutated", Type: "float"}
	rule.Name = "mutated"
	if resolver.calls != 1 {
		t.Fatalf("ResolveChatModel calls = %d, want 1", resolver.calls)
	}
	state, err := newRunState(normalized.fields, graphInput{Text: "input"}, nil, nil)
	if err != nil {
		t.Fatalf("newRunState() error = %v", err)
	}
	if err := graph.execute(t.Context(), state); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if resolver.calls != 1 {
		t.Fatalf("ResolveChatModel calls after execute = %d, want 1", resolver.calls)
	}
	chat.mu.Lock()
	defer chat.mu.Unlock()
	if !strings.Contains(chat.inputs[0][0].Content, "play_music") ||
		strings.Contains(chat.inputs[0][0].Content, "mutated") {
		t.Fatalf("compiled prompt observed caller mutation: %q", chat.inputs[0][0].Content)
	}
}

func TestMatchNodeResolverFailureStopsConstruction(t *testing.T) {
	resolverErr := errors.New("resolver unavailable")
	config := matchConfig(&matchCountingResolver{err: resolverErr})
	if _, err := New(t.Context(), config); !errors.Is(err, resolverErr) {
		t.Fatalf("New() error = %v, want %v", err, resolverErr)
	}
}

func TestMatchNodeCompiledGraphIsConcurrent(t *testing.T) {
	chat := &echoMatchChatModel{}
	normalized, graph := compileMatchGraph(t, matchConfig(&componentMapResolver{chat: chat}))
	const count = 16
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			text := fmt.Sprintf("value-%d", index)
			state, err := newRunState(normalized.fields, graphInput{Text: text}, nil, nil)
			if err != nil {
				failures <- err
				return
			}
			if err := graph.execute(t.Context(), state); err != nil {
				failures <- err
				return
			}
			value, err := state.value("matches")
			if err != nil {
				failures <- err
				return
			}
			items := value.([]any)
			if items[0].(map[string]any)["raw_text"] != text {
				failures <- fmt.Errorf("raw_text = %#v, want %q", items, text)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestMatchNodeCompilesAndExecutesInCompositeGraphs(t *testing.T) {
	t.Run("Subgraph", func(t *testing.T) {
		child := matchConfig(&componentMapResolver{}).Graph
		config := Config{
			Agent:      AgentConfig{ID: "subgraph-match"},
			Components: &componentMapResolver{chat: &echoMatchChatModel{}},
			Limits:     Limits{MaxOutputBytes: 1 << 20},
			Graph: GraphDefinition{
				Name: "root",
				State: StateDefinition{Fields: []StateField{{
					Name: "answer", Type: StateString, Merge: MergeReplace,
				}}},
				Nodes: []NodeDefinition{{
					ID: "subgraph",
					Inputs: map[string]Binding{
						"text": {From: "input.text"},
					},
					Outputs:  map[string]string{"answer": "answer"},
					Subgraph: &SubgraphNode{Graph: child},
				}},
				Edges: []EdgeDefinition{
					{From: "start", To: "subgraph"}, {From: "subgraph", To: "end"},
				},
				Outputs: []OutputDefinition{{
					Node: "subgraph", Field: "answer", Name: "answer",
					MIMEType: "text/plain", Primary: true,
				}},
			},
		}
		normalized, graph := compileMatchGraph(t, config)
		state, err := newRunState(normalized.fields, graphInput{Text: "hello"}, nil, nil)
		if err != nil {
			t.Fatalf("newRunState() error = %v", err)
		}
		if err := graph.execute(t.Context(), state); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
		if answer, err := state.value("answer"); err != nil || answer != "matched" {
			t.Fatalf("answer = %#v, %v", answer, err)
		}
	})

	t.Run("Race", func(t *testing.T) {
		child := matchConfig(&componentMapResolver{}).Graph
		config := Config{
			Agent:      AgentConfig{ID: "race-match"},
			Components: &componentMapResolver{chat: &echoMatchChatModel{}},
			Limits:     Limits{MaxOutputBytes: 1 << 20},
			Graph: GraphDefinition{
				Name: "root",
				State: StateDefinition{Fields: []StateField{{
					Name: "answer", Type: StateString, Merge: MergeReplace,
				}}},
				Nodes: []NodeDefinition{{
					ID: "race",
					Inputs: map[string]Binding{
						"text": {From: "input.text"},
					},
					Outputs: map[string]string{"answer": "answer"},
					Race: &RaceNode{
						Branches: []RaceBranch{
							{ID: "a", Graph: child},
							{ID: "b", Graph: child},
						},
						Winner:         RaceWinnerDefinition{Mode: RaceFirstSuccess},
						MaxConcurrency: 2,
					},
				}},
				Edges: []EdgeDefinition{
					{From: "start", To: "race"}, {From: "race", To: "end"},
				},
				Outputs: []OutputDefinition{{
					Node: "race", Field: "answer", Name: "answer",
					MIMEType: "text/plain", Primary: true,
				}},
			},
		}
		normalized, graph := compileMatchGraph(t, config)
		state, err := newRunState(normalized.fields, graphInput{Text: "hello"}, nil, nil)
		if err != nil {
			t.Fatalf("newRunState() error = %v", err)
		}
		if err := graph.execute(t.Context(), state); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
		if answer, err := state.value("answer"); err != nil || answer != "matched" {
			t.Fatalf("answer = %#v, %v", answer, err)
		}
	})

	t.Run("Batch", func(t *testing.T) {
		child := matchConfig(&componentMapResolver{}).Graph
		child.State.Fields = append(child.State.Fields, StateField{
			Name: "item", Type: StateString, Merge: MergeReplace,
		})
		child.Nodes[0].Inputs["text"] = Binding{From: "item"}
		config := Config{
			Agent:      AgentConfig{ID: "batch-match"},
			Components: &componentMapResolver{chat: &echoMatchChatModel{}},
			Limits:     Limits{MaxOutputBytes: 1 << 20},
			Graph: GraphDefinition{
				Name: "root",
				State: StateDefinition{Fields: []StateField{
					{Name: "items", Type: StateList, Merge: MergeReplace},
					{Name: "answer", Type: StateString, Merge: MergeReplace},
				}},
				Nodes: []NodeDefinition{
					{
						ID:      "batch",
						Outputs: map[string]string{"items": "items"},
						Batch: &BatchNode{
							Items: Binding{From: "input.parts"}, Graph: child, MaxConcurrency: 2,
						},
					},
					{
						ID: "finish",
						Inputs: map[string]Binding{
							"items": {From: "items"},
						},
						Outputs: map[string]string{"answer": "answer"},
						Script: &ScriptNode{
							Language: ScriptStarlark,
							Source:   "def run(input):\n    return {\"answer\": \"batched\"}\n",
							Limits: ScriptLimits{
								MaxExecutionSteps: 1_000,
								Timeout:           time.Second,
								MaxInputBytes:     1 << 10,
								MaxOutputBytes:    1 << 10,
							},
						},
					},
				},
				Edges: []EdgeDefinition{
					{From: "start", To: "batch"},
					{From: "batch", To: "finish"},
					{From: "finish", To: "end"},
				},
				Outputs: []OutputDefinition{{
					Node: "finish", Field: "answer", Name: "answer",
					MIMEType: "text/plain", Primary: true,
				}},
			},
		}
		normalized, graph := compileMatchGraph(t, config)
		state, err := newRunState(
			normalized.fields,
			graphInput{Parts: []any{"one", "two"}},
			nil,
			nil,
		)
		if err != nil {
			t.Fatalf("newRunState() error = %v", err)
		}
		if err := graph.execute(t.Context(), state); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
		items, err := state.value("items")
		if err != nil {
			t.Fatalf("State items: %v", err)
		}
		if want := []any{"matched", "matched"}; !reflect.DeepEqual(items, want) {
			t.Fatalf("items = %#v, want %#v", items, want)
		}
	})
}

func matchConfig(resolver ComponentResolver) Config {
	return Config{
		Agent: AgentConfig{ID: "match-agent"},
		Graph: GraphDefinition{
			Name: "match",
			State: StateDefinition{Fields: []StateField{
				{Name: "matches", Type: StateList, Merge: MergeReplace},
				{Name: "answer", Type: StateString, Merge: MergeReplace},
			}},
			Nodes: []NodeDefinition{
				{
					ID: "route",
					Inputs: map[string]Binding{
						"text": {From: "input.text"},
					},
					Outputs: map[string]string{"matches": "matches"},
					Match: &MatchNode{
						Model: "router",
						Rules: []*genxmatch.Rule{{
							Name: "play_music",
							Vars: map[string]genxmatch.Var{
								"title": {Label: "歌曲名", Type: "string"},
							},
							Patterns: []genxmatch.Pattern{{Input: "我想听[title]"}},
						}},
					},
				},
				{
					ID: "finish",
					Inputs: map[string]Binding{
						"matches": {From: "matches"},
					},
					Outputs: map[string]string{"answer": "answer"},
					Script: &ScriptNode{
						Language: ScriptStarlark,
						Source:   "def run(input):\n    return {\"answer\": \"matched\"}\n",
						Limits: ScriptLimits{
							MaxExecutionSteps: 1_000,
							Timeout:           time.Second,
							MaxInputBytes:     1 << 10,
							MaxOutputBytes:    1 << 10,
						},
					},
				},
			},
			Edges: []EdgeDefinition{
				{From: "start", To: "route"},
				{From: "route", To: "finish"},
				{From: "finish", To: "end"},
			},
			Outputs: []OutputDefinition{{
				Node: "finish", Field: "answer", Name: "answer",
				MIMEType: "text/plain", Primary: true,
			}},
		},
		Components: resolver,
		Limits:     Limits{MaxOutputBytes: 1 << 20},
	}
}

func compileMatchGraph(t *testing.T, config Config) (*normalizedConfig, *compiledGraph) {
	t.Helper()
	normalized, err := normalizeConfig(config)
	if err != nil {
		t.Fatalf("normalizeConfig() error = %v", err)
	}
	graph, err := buildGraph(t.Context(), normalized, normalized.Graph, "Graph")
	if err != nil {
		t.Fatalf("buildGraph() error = %v", err)
	}
	return normalized, graph
}

type matchCountingResolver struct {
	chat  model.BaseChatModel
	err   error
	calls int
}

func (resolver *matchCountingResolver) ResolveChatModel(
	context.Context,
	string,
) (model.BaseChatModel, error) {
	resolver.calls++
	if resolver.err != nil {
		return nil, resolver.err
	}
	return resolver.chat, nil
}

func (*matchCountingResolver) ResolveRetriever(
	context.Context,
	string,
) (retriever.Retriever, error) {
	return nil, errors.New("not supported")
}

type echoMatchChatModel struct{}

func (*echoMatchChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := (&echoMatchChatModel{}).Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.Recv()
}

func (*echoMatchChatModel) Stream(
	_ context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	text := ""
	if len(input) > 1 && input[1] != nil {
		text = input[1].Content
	}
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage(text+"\n", nil),
	}), nil
}
