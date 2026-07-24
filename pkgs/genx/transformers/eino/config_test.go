package eino

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
)

func TestNewValidatesGraphContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "agent", mutate: func(config *Config) { config.Agent.ID = "" }},
		{name: "negative tool call limit", mutate: func(config *Config) { config.MaxToolCalls = -1 }},
		{name: "tool call limit without Toolkit", mutate: func(config *Config) { config.MaxToolCalls = 1 }},
		{name: "node union", mutate: func(config *Config) { config.Graph.Nodes[0].Passthrough = &PassthroughNode{} }},
		{name: "unknown binding", mutate: func(config *Config) {
			config.Graph.Nodes[0].Inputs["input"] = Binding{From: "missing"}
		}},
		{name: "output MIME", mutate: func(config *Config) { config.Graph.Outputs[0].MIMEType = "application/json" }},
		{name: "primary", mutate: func(config *Config) { config.Graph.Outputs[0].Primary = false }},
		{name: "all predecessor cycle", mutate: func(config *Config) {
			config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
			config.Graph.Edges = append(config.Graph.Edges, EdgeDefinition{From: "answer", To: "answer"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := textConfig()
			test.mutate(&config)
			if _, err := New(context.Background(), config); err == nil {
				t.Fatal("New() succeeded, want validation error")
			}
		})
	}
}

func TestNewOwnsGraphConfiguration(t *testing.T) {
	t.Parallel()
	config := textConfig()
	transformer, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	config.Graph.Nodes[0].Transform.Order[0] = "changed"
	output, err := transformer.Transform(t.Context(), textInput("owned"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "owned" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphCopyPreservesPredicateIntegerType(t *testing.T) {
	t.Parallel()
	const value = int64(9_007_199_254_740_993)
	source := GraphDefinition{
		Branches: []BranchDefinition{{
			Routes: []BranchRoute{{
				When: Predicate{Field: "count", Op: PredicateEqual, Value: value},
			}},
		}},
	}
	cloned, err := cloneGraph(source)
	if err != nil {
		t.Fatalf("cloneGraph() error = %v", err)
	}
	clonedValue := cloned.Branches[0].Routes[0].When.Value
	got, ok := clonedValue.(int64)
	if !ok || got != value {
		t.Fatalf("predicate value = %#v (%T), want int64(%d)", clonedValue, clonedValue, value)
	}
}

func TestNewRejectsInvalidNodePorts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "unknown transform", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform.Operation = "unknown"
		}},
		{name: "concat order", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform.Order = []string{"missing"}
		}},
		{name: "select port", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform = &TransformNode{Operation: TransformSelect}
		}},
		{name: "passthrough port", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform = nil
			config.Graph.Nodes[0].Passthrough = &PassthroughNode{}
		}},
		{name: "decode limits", mutate: func(config *Config) {
			config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{
				Name: "object", Type: StateObject, Merge: MergeReplace,
			})
			config.Graph.Nodes[0] = NodeDefinition{
				ID: "answer", Inputs: map[string]Binding{"text": {From: "input.text"}},
				Outputs: map[string]string{"object": "object"},
				Transform: &TransformNode{
					Operation: TransformDecodeJSON,
				},
			}
			config.Graph.Outputs[0].Field = "object"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := textConfig()
			test.mutate(&config)
			if _, err := New(t.Context(), config); err == nil {
				t.Fatal("New() succeeded, want node-port validation error")
			}
		})
	}
}

func TestNewRejectsConcurrentStateWriters(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "left", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect},
		},
		{
			ID: "right", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "left"}, {From: "start", To: "right"},
		{From: "left", To: "end"}, {From: "right", To: "end"},
	}
	config.Graph.Outputs[0].Node = "left"
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "concurrently write") {
		t.Fatalf("New() error = %v, want concurrent-writer rejection", err)
	}
}

func TestNewAcceptsOnlyProvenExclusiveStateWriters(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.State.Fields = []StateField{
		{Name: "gate", Type: StateString, Merge: MergeReplace},
		{Name: "selected", Type: StateString, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "gate", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "gate"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "left", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "selected"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "right", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "selected"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "join", Inputs: map[string]Binding{"value": {From: "selected"}},
			Outputs: map[string]string{"value": "answer"}, Passthrough: &PassthroughNode{},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "gate"}, {From: "left", To: "join"},
		{From: "right", To: "join"}, {From: "join", To: "end"},
	}
	config.Graph.Branches = []BranchDefinition{{
		From: "gate", Mode: BranchFirstMatch,
		Routes: []BranchRoute{{
			When: Predicate{Field: "gate", Op: PredicateEqual, Value: "left"}, To: "left",
		}},
		Default: "right",
	}}
	config.Graph.Outputs[0].Node = "join"
	if _, err := New(t.Context(), config); err != nil {
		t.Fatalf("New() rejected exclusive writers: %v", err)
	}

	config.Graph.Edges = append(config.Graph.Edges, EdgeDefinition{From: "start", To: "left"})
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "concurrently write") {
		t.Fatalf("New() error = %v, want concurrent-writer rejection", err)
	}
}

func TestNewRejectsInvalidRoutingAndOptionalConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "cannot reach end", mutate: func(config *Config) {
			config.Graph.Edges = config.Graph.Edges[:1]
		}},
		{name: "unknown fan-in", mutate: func(config *Config) {
			config.Graph.Compile.FanIn = map[string]FanInConfig{"missing": {}}
		}},
		{name: "impossible fan-in", mutate: func(config *Config) {
			config.Graph.Compile.FanIn = map[string]FanInConfig{"answer": {}}
		}},
		{name: "unbounded cycle", mutate: func(config *Config) {
			config.Graph.Edges = append(config.Graph.Edges, EdgeDefinition{From: "answer", To: "answer"})
		}},
		{name: "State store", mutate: func(config *Config) {
			config.State = &StatePersistenceConfig{Scope: "scope", Fields: []string{"answer"}}
		}},
		{name: "History limit", mutate: func(config *Config) {
			config.History = &HistoryConfig{Limit: 0}
		}},
		{name: "Memory store", mutate: func(config *Config) {
			config.Memory = &MemoryConfig{Scope: "scope"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := textConfig()
			test.mutate(&config)
			if _, err := New(t.Context(), config); err == nil {
				t.Fatal("New() succeeded, want validation error")
			}
		})
	}
}

func TestNewRejectsAdditionalGraphContractViolations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "graph whitespace", mutate: func(config *Config) { config.Graph.Name = " invalid " }},
		{name: "duplicate state", mutate: func(config *Config) {
			config.Graph.State.Fields = append(config.Graph.State.Fields, config.Graph.State.Fields[0])
		}},
		{name: "unsupported state", mutate: func(config *Config) {
			config.Graph.State.Fields[0].Type = "unsupported"
		}},
		{name: "incompatible merge", mutate: func(config *Config) {
			config.Graph.State.Fields[0].Merge = MergeObject
		}},
		{name: "blank node", mutate: func(config *Config) { config.Graph.Nodes[0].ID = " " }},
		{name: "duplicate node", mutate: func(config *Config) {
			config.Graph.Nodes = append(config.Graph.Nodes, config.Graph.Nodes[0])
		}},
		{name: "unknown edge", mutate: func(config *Config) {
			config.Graph.Edges[0].To = "missing"
		}},
		{name: "unknown branch source", mutate: func(config *Config) {
			config.Graph.Edges = nil
			config.Graph.Branches = []BranchDefinition{{
				From: "missing", Mode: BranchFirstMatch,
				Routes:  []BranchRoute{{When: Predicate{Field: "answer", Op: PredicateExists}, To: "answer"}},
				Default: "answer",
			}}
		}},
		{name: "unsupported branch", mutate: func(config *Config) {
			config.Graph.Edges = []EdgeDefinition{{From: "answer", To: "end"}}
			config.Graph.Branches = []BranchDefinition{{
				From: "answer", Mode: "unsupported",
				Routes:  []BranchRoute{{When: Predicate{Field: "answer", Op: PredicateExists}, To: "end"}},
				Default: "end",
			}}
		}},
		{name: "empty branch routes", mutate: func(config *Config) {
			config.Graph.Edges = []EdgeDefinition{{From: "start", To: "answer"}}
			config.Graph.Branches = []BranchDefinition{{
				From: "answer", Mode: BranchFirstMatch, Default: "end",
			}}
		}},
		{name: "bad predicate", mutate: func(config *Config) {
			config.Graph.Edges = []EdgeDefinition{{From: "start", To: "answer"}}
			config.Graph.Branches = []BranchDefinition{{
				From: "answer", Mode: BranchFirstMatch,
				Routes:  []BranchRoute{{When: Predicate{Field: "missing", Op: PredicateExists}, To: "end"}},
				Default: "end",
			}}
		}},
		{name: "unreachable node", mutate: func(config *Config) {
			config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
				ID: "extra", Inputs: map[string]Binding{"value": {From: "input.text"}},
				Outputs: map[string]string{"value": "answer"}, Passthrough: &PassthroughNode{},
			})
		}},
		{name: "primary output bypass", mutate: func(config *Config) {
			config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{
				Name: "extra", Type: StateString, Merge: MergeReplace,
			})
			config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
				ID: "extra", Inputs: map[string]Binding{"value": {From: "input.text"}},
				Outputs: map[string]string{"value": "extra"}, Passthrough: &PassthroughNode{},
			})
			config.Graph.Edges = append(config.Graph.Edges,
				EdgeDefinition{From: "start", To: "extra"},
				EdgeDefinition{From: "extra", To: "end"},
			)
		}},
		{name: "duplicate output name", mutate: func(config *Config) {
			config.Graph.Outputs = append(config.Graph.Outputs, config.Graph.Outputs[0])
		}},
		{name: "output source mismatch", mutate: func(config *Config) {
			config.Graph.Outputs[0].Field = "missing"
		}},
		{name: "negative max steps", mutate: func(config *Config) {
			config.Graph.Compile.MaxRunSteps = -1
		}},
		{name: "unsupported trigger", mutate: func(config *Config) {
			config.Graph.Compile.NodeTriggerMode = "unsupported"
		}},
		{name: "script limits", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform = nil
			config.Graph.Nodes[0].Script = &ScriptNode{Language: ScriptStarlark, Source: "def run(input): return input"}
		}},
		{name: "missing component resolver", mutate: func(config *Config) {
			*config = chatConfig(nil)
		}},
		{name: "blank lambda", mutate: func(config *Config) {
			config.Graph.Nodes[0].Transform = nil
			config.Graph.Nodes[0].Lambda = &LambdaRefNode{}
		}},
		{name: "duplicate persistent field", mutate: func(config *Config) {
			config.State = &StatePersistenceConfig{
				Store: &recordingStateStore{}, Scope: "scope", Fields: []string{"answer", "answer"},
			}
		}},
		{name: "unknown persistent field", mutate: func(config *Config) {
			config.State = &StatePersistenceConfig{
				Store: &recordingStateStore{}, Scope: "scope", Fields: []string{"missing"},
			}
		}},
		{name: "history over limit", mutate: func(config *Config) {
			config.History = &HistoryConfig{Limit: 1_000_001}
		}},
		{name: "invalid recall", mutate: func(config *Config) {
			config.Memory = &MemoryConfig{
				Store: &recordingMemoryStore{}, Scope: "scope",
				Recall: []RecallDefinition{{QueryFrom: "input.text", Output: "answer"}},
			}
		}},
		{name: "wait without enabled", mutate: func(config *Config) {
			config.Memory = &MemoryConfig{
				Store: &recordingMemoryStore{}, Scope: "scope",
				Observe: ObservePolicy{WaitForCompletion: true},
			}
		}},
		{name: "wait without waiter", mutate: func(config *Config) {
			config.Memory = &MemoryConfig{
				Store: &recordingMemoryStore{}, Scope: "scope",
				Observe: ObservePolicy{Enabled: true, WaitForCompletion: true},
			}
		}},
		{name: "invalid observe fact", mutate: func(config *Config) {
			config.Memory = &MemoryConfig{
				Store: &recordingMemoryStore{}, Scope: "scope",
				Observe: ObservePolicy{
					Enabled: true, Facts: []ObserveDefinition{{TextFrom: "missing"}},
				},
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := textConfig()
			test.mutate(&config)
			if _, err := New(t.Context(), config); err == nil {
				t.Fatal("New() succeeded, want contract validation error")
			}
		})
	}
}

func TestNewValidatesNamedLambdaSchema(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"value": {From: "input.text"}},
		Outputs: map[string]string{"value": "answer"}, Lambda: &LambdaRefNode{Lambda: "named"},
	}
	config.Lambdas = staticLambdaResolver{resolved: ResolvedLambda{
		Lambda: compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
			return input, nil
		}),
		Inputs: map[string]StateType{"value": StateBoolean}, Outputs: map[string]StateType{"value": StateString},
	}}
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "requires boolean") {
		t.Fatalf("New() error = %v, want Lambda schema mismatch", err)
	}
}

func TestNewOwnsOptionalConfiguration(t *testing.T) {
	t.Parallel()
	store := &recordingStateStore{snapshot: StateSnapshot{Fields: map[string]any{}}}
	stateConfig := &StatePersistenceConfig{
		Store: store, Scope: "original", Fields: []string{"answer"},
	}
	config := textConfig()
	config.State = stateConfig
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	stateConfig.Scope = "mutated"
	stateConfig.Fields[0] = "missing"
	output, err := transformer.Transform(t.Context(), textInput("owned"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	drain(t, output)
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.loadScope != "original" || store.compareScope != "original" {
		t.Fatalf("owned State scopes load=%q compare=%q", store.loadScope, store.compareScope)
	}
	if !maps.Equal(store.compareFields, map[string]any{"answer": "owned"}) {
		t.Fatalf("owned State fields = %#v", store.compareFields)
	}
}

func TestNewCompilesScriptAndResolvesComponentsOnce(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0].Transform = nil
	config.Graph.Nodes[0].Script = &ScriptNode{
		Language: ScriptStarlark, Entrypoint: "missing",
		Source: "def run(input):\n  return {\"text\": \"unused\"}\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000, Timeout: time.Second,
			MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
		},
	}
	if _, err := New(t.Context(), config); err == nil || !strings.Contains(err.Error(), "entrypoint") {
		t.Fatalf("New() error = %v, want entrypoint validation", err)
	}

	resolver := &countingComponentResolver{}
	config = chatConfig(resolver)
	transformer, err := New(t.Context(), config)
	if err == nil || transformer != nil {
		t.Fatalf("New() = %#v, %v, want nil component error", transformer, err)
	}
	if resolver.chatCalls != 1 {
		t.Fatalf("ResolveChatModel calls = %d, want 1", resolver.chatCalls)
	}
}

func TestScriptIsSandboxedAndBounded(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer",
		Inputs: map[string]Binding{
			"value": {From: "input.text"},
		},
		Outputs: map[string]string{"text": "answer"},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source:   "def run(input):\n  return {\"text\": input[\"value\"] + \"!\"}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: 1000, Timeout: 1000000000,
				MaxInputBytes: 1024, MaxOutputBytes: 1024,
			},
		},
	}
	transformer, err := New(context.Background(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("script"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "script!" {
		t.Fatalf("output = %q", got)
	}

	config.Graph.Nodes[0].Script.Source = "def run(input):\n  return {\"text\": open(\"/tmp/secret\")}\n"
	if _, err := New(context.Background(), config); err == nil || !strings.Contains(err.Error(), "compile Script") {
		t.Fatalf("New() error = %v, want sandbox compile error", err)
	}
}

type staticLambdaResolver struct {
	resolved ResolvedLambda
	err      error
}

func (resolver staticLambdaResolver) ResolveLambda(context.Context, string) (ResolvedLambda, error) {
	return resolver.resolved, resolver.err
}

type countingComponentResolver struct {
	chatCalls int
}

func (resolver *countingComponentResolver) ResolveChatModel(context.Context, string) (model.BaseChatModel, error) {
	resolver.chatCalls++
	return nil, errors.New("unavailable")
}

func (*countingComponentResolver) ResolveRetriever(context.Context, string) (retriever.Retriever, error) {
	return nil, errors.New("unavailable")
}
