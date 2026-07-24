package eino

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestNodeValidationRejectsEveryMalformedNodeKind(t *testing.T) {
	t.Parallel()
	fields := map[string]StateType{
		"text": StateString, "other": StateString, "boolean": StateBoolean,
		"messages": StateMessages, "object": StateObject, "list": StateList,
		"documents": StateDocuments, "blob": StateBlob,
	}
	validChild := childTextGraph("child", &TransformNode{
		Operation: TransformSelect,
	})
	validChild.Nodes[0].Inputs = map[string]Binding{"value": {From: "input.text"}}
	validChild.Nodes[0].Outputs = map[string]string{"value": "answer"}
	temperature := float32(math.NaN())
	zero := 0
	tests := []struct {
		name string
		node NodeDefinition
	}{
		{
			name: "prompt format",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"messages": "messages"},
				Prompt: &PromptNode{Format: "bad", Messages: []PromptMessage{{Role: PromptUser, Template: "x"}}},
			},
		},
		{
			name: "prompt empty",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"messages": "messages"},
				Prompt: &PromptNode{Format: PromptFString},
			},
		},
		{
			name: "prompt output",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"text": "text"},
				Prompt: &PromptNode{Format: PromptFString, Messages: []PromptMessage{{Role: PromptUser, Template: "x"}}},
			},
		},
		{
			name: "prompt message shape",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"messages": "messages"},
				Prompt: &PromptNode{Format: PromptFString, Messages: []PromptMessage{{}}},
			},
		},
		{
			name: "prompt missing placeholder",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"messages": "messages"},
				Prompt: &PromptNode{Format: PromptFString, Messages: []PromptMessage{{Placeholder: "missing"}}},
			},
		},
		{
			name: "prompt placeholder type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "text"}},
				Outputs: map[string]string{"messages": "messages"},
				Prompt:  &PromptNode{Format: PromptFString, Messages: []PromptMessage{{Placeholder: "text"}}},
			},
		},
		{
			name: "chat model name",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				Outputs: map[string]string{"text": "text"}, ChatModel: &ChatModelNode{},
			},
		},
		{
			name: "chat temperature",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				Outputs:   map[string]string{"text": "text"},
				ChatModel: &ChatModelNode{Model: "chat", Temperature: &temperature},
			},
		},
		{
			name: "chat tokens",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				Outputs:   map[string]string{"text": "text"},
				ChatModel: &ChatModelNode{Model: "chat", MaxTokens: &zero},
			},
		},
		{
			name: "chat input set",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "text"}},
				Outputs: map[string]string{"text": "text"}, ChatModel: &ChatModelNode{Model: "chat"},
			},
		},
		{
			name: "chat input type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "text"}},
				Outputs: map[string]string{"text": "text"}, ChatModel: &ChatModelNode{Model: "chat"},
			},
		},
		{
			name: "chat no output",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				ChatModel: &ChatModelNode{Model: "chat"},
			},
		},
		{
			name: "chat unknown output",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				Outputs: map[string]string{"unknown": "text"}, ChatModel: &ChatModelNode{Model: "chat"},
			},
		},
		{
			name: "chat messages output type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"messages": {From: "messages"}},
				Outputs: map[string]string{"messages": "text"}, ChatModel: &ChatModelNode{Model: "chat"},
			},
		},
		{
			name: "transform empty operation",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"value": {From: "text"}},
				Outputs: map[string]string{"value": "text"}, Transform: &TransformNode{},
			},
		},
		{
			name: "select output type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"value": {From: "text"}},
				Outputs:   map[string]string{"value": "boolean"},
				Transform: &TransformNode{Operation: TransformSelect},
			},
		},
		{
			name: "concat duplicate order",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "text"}},
				Outputs: map[string]string{"text": "text"},
				Transform: &TransformNode{
					Operation: TransformConcatText, Order: []string{"text", "text"},
				},
			},
		},
		{
			name: "concat input type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"boolean": {From: "boolean"}},
				Outputs: map[string]string{"text": "text"},
				Transform: &TransformNode{
					Operation: TransformConcatText, Order: []string{"boolean"},
				},
			},
		},
		{
			name: "decode input type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "boolean"}},
				Outputs: map[string]string{"object": "object"},
				Transform: &TransformNode{
					Operation: TransformDecodeJSON, MaxInputBytes: 10, MaxOutputBytes: 10,
				},
			},
		},
		{
			name: "decode output type",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "text"}},
				Outputs: map[string]string{"object": "text"},
				Transform: &TransformNode{
					Operation: TransformDecodeJSON, MaxInputBytes: 10, MaxOutputBytes: 10,
				},
			},
		},
		{
			name: "build messages fields",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"text": {From: "text"}},
				Outputs: map[string]string{"messages": "messages"},
				Transform: &TransformNode{
					Operation: TransformBuildMessages,
					Messages:  []TransformMessage{{Role: PromptUser, Input: "text", Text: "both"}},
				},
			},
		},
		{
			name: "build messages input",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"boolean": {From: "boolean"}},
				Outputs: map[string]string{"messages": "messages"},
				Transform: &TransformNode{
					Operation: TransformBuildMessages,
					Messages:  []TransformMessage{{Role: PromptUser, Input: "boolean"}},
				},
			},
		},
		{
			name: "script configuration",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"text": "text"},
				Script: &ScriptNode{Language: ScriptStarlark},
			},
		},
		{
			name: "lambda name",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"text": "text"}, Lambda: &LambdaRefNode{},
			},
		},
		{
			name: "race concurrency",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{Winner: RaceWinnerDefinition{Mode: RaceFirstSuccess}},
			},
		},
		{
			name: "race first success predicate",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{
					Branches: []RaceBranch{{ID: "child", Graph: validChild}},
					Winner: RaceWinnerDefinition{
						Mode: RaceFirstSuccess,
						When: &Predicate{Field: "answer", Op: PredicateExists},
					},
					MaxConcurrency: 1,
				},
			},
		},
		{
			name: "race predicate missing",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{
					Branches:       []RaceBranch{{ID: "child", Graph: validChild}},
					Winner:         RaceWinnerDefinition{Mode: RacePredicate},
					MaxConcurrency: 1,
				},
			},
		},
		{
			name: "race mode",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{
					Branches:       []RaceBranch{{ID: "child", Graph: validChild}},
					Winner:         RaceWinnerDefinition{Mode: "unknown"},
					MaxConcurrency: 1,
				},
			},
		},
		{
			name: "race branch ID",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{
					Branches:       []RaceBranch{{Graph: validChild}},
					Winner:         RaceWinnerDefinition{Mode: RaceFirstSuccess},
					MaxConcurrency: 1,
				},
			},
		},
		{
			name: "race duplicate branch",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Race: &RaceNode{
					Branches: []RaceBranch{
						{ID: "child", Graph: validChild},
						{ID: "child", Graph: validChild},
					},
					Winner:         RaceWinnerDefinition{Mode: RaceFirstSuccess},
					MaxConcurrency: 2,
				},
			},
		},
		{
			name: "batch concurrency",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"items": "list"},
				Batch: &BatchNode{
					Items: Binding{From: "list"}, Graph: validChild,
				},
			},
		},
		{
			name: "batch items",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"items": "list"},
				Batch: &BatchNode{
					Items: Binding{From: "missing"}, Graph: validChild, MaxConcurrency: 1,
				},
			},
		},
		{
			name: "passthrough cardinality",
			node: NodeDefinition{
				ID: "node", Inputs: map[string]Binding{"one": {From: "text"}, "two": {From: "other"}},
				Outputs: map[string]string{"value": "text"}, Passthrough: &PassthroughNode{},
			},
		},
		{
			name: "retriever configuration",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"documents": "documents"},
				Retriever: &RetrieverNode{Query: Binding{From: "text"}},
			},
		},
		{
			name: "retriever query",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"documents": "documents"},
				Retriever: &RetrieverNode{
					Retriever: "store", Query: Binding{From: "missing"}, TopK: 1,
				},
			},
		},
		{
			name: "subgraph invalid",
			node: NodeDefinition{
				ID: "node", Outputs: map[string]string{"answer": "text"},
				Subgraph: &SubgraphNode{Graph: GraphDefinition{}},
			},
		},
	}
	validator := &normalizedConfig{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validator.validateNode(test.node, fields, "Graph", 0)
			if err == nil {
				err = validateNodePorts(test.node, fields, "Graph")
			}
			if err == nil {
				t.Fatalf("malformed %s node passed validation", test.name)
			}
		})
	}
}

func TestGraphValidationRejectsAdversarialTopologyAndMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "blank input port",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Inputs[""] = config.Graph.Nodes[0].Inputs["input"]
			},
			want: "blank input port",
		},
		{
			name: "blank output port",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Outputs[""] = "answer"
			},
			want: "blank output port",
		},
		{
			name: "undeclared output field",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Outputs["text"] = "missing"
			},
			want: "undeclared field",
		},
		{
			name: "duplicate node output field",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Outputs["duplicate"] = "answer"
			},
			want: "more than once",
		},
		{
			name: "lambda resolver",
			mutate: func(config *Config) {
				config.Graph.Nodes[0].Transform = nil
				config.Graph.Nodes[0].Lambda = &LambdaRefNode{Lambda: "named"}
			},
			want: "Lambdas is required",
		},
		{
			name: "no nodes",
			mutate: func(config *Config) {
				config.Graph.Nodes = nil
			},
			want: "requires Nodes",
		},
		{
			name: "branch target",
			mutate: func(config *Config) {
				config.Graph.Edges = []EdgeDefinition{{From: "start", To: "answer"}}
				config.Graph.Branches = []BranchDefinition{{
					From: "answer", Mode: BranchFirstMatch,
					Routes: []BranchRoute{{
						When: Predicate{Field: "answer", Op: PredicateExists},
						To:   "missing",
					}},
					Default: "end",
				}}
			},
			want: "unknown target",
		},
		{
			name: "cannot reach end",
			mutate: func(config *Config) {
				config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
					ID: "trap", Inputs: map[string]Binding{"value": {From: "input.text"}},
					Outputs: map[string]string{"value": "answer"}, Passthrough: &PassthroughNode{},
				})
				config.Graph.Edges = []EdgeDefinition{
					{From: "start", To: "answer"},
					{From: "answer", To: "end"},
					{From: "start", To: "trap"},
				}
			},
			want: "cannot reach end",
		},
		{
			name: "all predecessor cycle",
			mutate: func(config *Config) {
				config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
				config.Graph.Compile.MaxRunSteps = 10
				config.Graph.Edges = []EdgeDefinition{
					{From: "start", To: "answer"},
					{From: "answer", To: "answer"},
					{From: "answer", To: "end"},
				}
			},
			want: "self-cycle",
		},
		{
			name: "output name",
			mutate: func(config *Config) {
				config.Graph.Outputs[0].Name = " answer "
			},
			want: "requires Name",
		},
		{
			name: "duplicate output source",
			mutate: func(config *Config) {
				duplicate := config.Graph.Outputs[0]
				duplicate.Name = "second"
				duplicate.Primary = false
				config.Graph.Outputs = append(config.Graph.Outputs, duplicate)
			},
			want: "more than once",
		},
		{
			name: "nesting depth",
			mutate: func(config *Config) {
				graph := childTextGraph("leaf", &TransformNode{Operation: TransformSelect})
				graph.Nodes[0].Inputs = map[string]Binding{"value": {From: "input.text"}}
				graph.Nodes[0].Outputs = map[string]string{"value": "answer"}
				for range 18 {
					graph = GraphDefinition{
						Name:  "nested",
						State: StateDefinition{Fields: []StateField{{Name: "answer", Type: StateString, Merge: MergeReplace}}},
						Nodes: []NodeDefinition{{
							ID: "answer", Inputs: map[string]Binding{"text": {From: "input.text"}},
							Outputs:  map[string]string{"answer": "answer"},
							Subgraph: &SubgraphNode{Graph: graph},
						}},
						Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
						Outputs: []OutputDefinition{{
							Node: "answer", Field: "answer", Name: "answer",
							MIMEType: "text/plain", Primary: true,
						}},
					}
				}
				config.Graph = graph
			},
			want: "nesting depth",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := textConfig()
			test.mutate(&config)
			_, err := New(context.Background(), config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPredicateValidationRejectsEveryMalformedForm(t *testing.T) {
	t.Parallel()
	fields := map[string]StateType{
		"text": StateString, "integer": StateInteger, "object": StateObject,
	}
	tests := []Predicate{
		{},
		{All: []Predicate{}},
		{Any: []Predicate{}},
		{All: []Predicate{{}}, Any: []Predicate{{}}},
		{Not: &Predicate{}},
		{Field: "missing", Op: PredicateExists},
		{Field: "text", Op: PredicateEqual, Value: 1},
		{Field: "integer", Op: PredicateContains, Value: 1},
		{Field: "text", Op: PredicateLess, Value: 1},
		{Field: "text", Op: "unknown"},
	}
	for _, predicate := range tests {
		if err := validatePredicate(predicate, fields); err == nil {
			t.Fatalf("validatePredicate(%#v) succeeded", predicate)
		}
	}
	for _, predicate := range []Predicate{
		{Field: "text", Op: PredicateExists},
		{Field: "text", Op: PredicateNotExists},
		{Field: "text", Op: PredicateNotEqual, Value: "other"},
		{Field: "object", Op: PredicateContains, Value: "key"},
		{Field: "integer", Op: PredicateGreaterEqual, Value: 1},
		{Any: []Predicate{{Field: "text", Op: PredicateExists}}},
		{All: []Predicate{{Field: "text", Op: PredicateExists}}},
		{Not: &Predicate{Field: "text", Op: PredicateExists}},
	} {
		if err := validatePredicate(predicate, fields); err != nil {
			t.Fatalf("validatePredicate(%#v) error = %v", predicate, err)
		}
	}
}

func TestNodePortValidationAdversarialMatrix(t *testing.T) {
	t.Parallel()
	fields := map[string]StateType{
		"text": StateString, "other": StateString, "boolean": StateBoolean,
		"messages": StateMessages, "object": StateObject, "list": StateList,
		"documents": StateDocuments,
	}
	valid := []NodeDefinition{
		{
			ID: "prompt", Inputs: map[string]Binding{"history": {From: "messages"}},
			Outputs: map[string]string{"messages": "messages"},
			Prompt: &PromptNode{Messages: []PromptMessage{
				{Role: PromptUser, Template: "text"},
				{Placeholder: "history", Optional: true},
			}},
		},
		{
			ID: "chat", Inputs: map[string]Binding{"messages": {From: "messages"}},
			Outputs:   map[string]string{"text": "text", "messages": "messages"},
			ChatModel: &ChatModelNode{},
		},
		{
			ID: "select", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs:   map[string]string{"value": "other"},
			Transform: &TransformNode{Operation: TransformSelect},
		},
		{
			ID: "concat", Inputs: map[string]Binding{
				"left": {From: "text"}, "right": {From: "other"},
			},
			Outputs: map[string]string{"text": "text"},
			Transform: &TransformNode{
				Operation: TransformConcatText, Order: []string{"left", "right"},
			},
		},
		{
			ID: "decode", Inputs: map[string]Binding{"text": {From: "text"}},
			Outputs: map[string]string{"object": "object"},
			Transform: &TransformNode{
				Operation: TransformDecodeJSON, MaxInputBytes: 10, MaxOutputBytes: 10,
			},
		},
		{
			ID: "messages", Inputs: map[string]Binding{"text": {From: "text"}},
			Outputs: map[string]string{"messages": "messages"},
			Transform: &TransformNode{
				Operation: TransformBuildMessages,
				Messages: []TransformMessage{
					{Role: PromptSystem, Text: "system"},
					{Role: PromptUser, Input: "text"},
					{Role: PromptAssistant, Input: "text"},
				},
			},
		},
		{
			ID: "script", Outputs: map[string]string{"text": "text"},
			Script: validScriptForValidation(),
		},
		{
			ID: "lambda", Inputs: map[string]Binding{"text": {From: "text"}},
			Outputs: map[string]string{"text": "text"}, Lambda: &LambdaRefNode{},
		},
		{
			ID: "batch", Outputs: map[string]string{"items": "list"},
			Batch: &BatchNode{Items: Binding{From: "list"}},
		},
		{
			ID: "passthrough", Inputs: map[string]Binding{"value": {From: "boolean"}},
			Outputs: map[string]string{"value": "boolean"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "retriever", Outputs: map[string]string{"documents": "documents"},
			Retriever: &RetrieverNode{Query: Binding{From: "text"}},
		},
	}
	for _, node := range valid {
		if err := validateNodePorts(node, fields, "Graph"); err != nil {
			t.Fatalf("validateNodePorts(%q) error = %v", node.ID, err)
		}
	}

	invalid := []NodeDefinition{
		{
			ID: "prompt-output-type", Outputs: map[string]string{"messages": "text"},
			Prompt: &PromptNode{Messages: []PromptMessage{{Role: PromptUser, Template: "x"}}},
		},
		{
			ID: "prompt-shape", Outputs: map[string]string{"messages": "messages"},
			Prompt: &PromptNode{Messages: []PromptMessage{{}}},
		},
		{
			ID: "prompt-placeholder-missing", Outputs: map[string]string{"messages": "messages"},
			Prompt: &PromptNode{Messages: []PromptMessage{{Placeholder: "history"}}},
		},
		{
			ID: "chat-too-many-outputs", Inputs: map[string]Binding{"messages": {From: "messages"}},
			Outputs: map[string]string{
				"text": "text", "messages": "messages", "extra": "text",
			},
			ChatModel: &ChatModelNode{},
		},
		{
			ID: "select-outputs", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs:   map[string]string{"wrong": "text"},
			Transform: &TransformNode{Operation: TransformSelect},
		},
		{
			ID: "concat-output", Inputs: map[string]Binding{"text": {From: "text"}},
			Outputs:   map[string]string{"text": "boolean"},
			Transform: &TransformNode{Operation: TransformConcatText, Order: []string{"text"}},
		},
		{
			ID: "decode-limits", Inputs: map[string]Binding{"text": {From: "text"}},
			Outputs:   map[string]string{"object": "object"},
			Transform: &TransformNode{Operation: TransformDecodeJSON},
		},
		{
			ID: "messages-role", Outputs: map[string]string{"messages": "messages"},
			Transform: &TransformNode{
				Operation: TransformBuildMessages,
				Messages:  []TransformMessage{{Role: "tool", Text: "x"}},
			},
		},
		{
			ID: "messages-input-set", Inputs: map[string]Binding{"extra": {From: "text"}},
			Outputs: map[string]string{"messages": "messages"},
			Transform: &TransformNode{
				Operation: TransformBuildMessages,
				Messages:  []TransformMessage{{Role: PromptUser, Text: "x"}},
			},
		},
		{ID: "script-empty", Script: validScriptForValidation()},
		{ID: "lambda-empty", Lambda: &LambdaRefNode{}},
		{
			ID: "batch-inputs", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs: map[string]string{"items": "list"},
			Batch:   &BatchNode{Items: Binding{From: "list"}},
		},
		{
			ID: "batch-type", Outputs: map[string]string{"items": "list"},
			Batch: &BatchNode{Items: Binding{From: "text"}},
		},
		{
			ID: "batch-output-type", Outputs: map[string]string{"items": "text"},
			Batch: &BatchNode{Items: Binding{From: "list"}},
		},
		{
			ID: "passthrough-output", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs: map[string]string{"wrong": "text"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "passthrough-type", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs: map[string]string{"value": "boolean"}, Passthrough: &PassthroughNode{},
		},
		{
			ID: "retriever-input", Inputs: map[string]Binding{"value": {From: "text"}},
			Outputs:   map[string]string{"documents": "documents"},
			Retriever: &RetrieverNode{Query: Binding{From: "text"}},
		},
		{
			ID: "retriever-query-type", Outputs: map[string]string{"documents": "documents"},
			Retriever: &RetrieverNode{Query: Binding{From: "boolean"}},
		},
		{
			ID: "retriever-output", Outputs: map[string]string{"documents": "text"},
			Retriever: &RetrieverNode{Query: Binding{From: "text"}},
		},
	}
	for _, node := range invalid {
		if err := validateNodePorts(node, fields, "Graph"); err == nil {
			t.Fatalf("validateNodePorts(%q) succeeded", node.ID)
		}
	}
}

func TestConfigCopyAndOutputHelpersAdversarialBoundaries(t *testing.T) {
	t.Parallel()
	if _, err := cloneGraph(GraphDefinition{Branches: []BranchDefinition{{
		Routes: []BranchRoute{{When: Predicate{
			Field: "value", Op: PredicateEqual, Value: make(chan int),
		}}},
	}}}); err == nil {
		t.Fatal("cloneGraph(unmarshalable predicate) succeeded")
	}
	if err := restorePredicateValues(
		GraphDefinition{Nodes: []NodeDefinition{{ID: "one"}}},
		&GraphDefinition{},
	); err == nil {
		t.Fatal("restorePredicateValues(shape mismatch) succeeded")
	}
	if err := restorePredicateValue(
		[]BranchRoute{{}},
		nil,
	); err == nil {
		t.Fatal("restorePredicateValue(shape mismatch) succeeded")
	}
	source := &Predicate{
		All: []Predicate{{Field: "integer", Op: PredicateEqual, Value: int64(1)}},
		Any: []Predicate{{Field: "text", Op: PredicateEqual, Value: "value"}},
		Not: &Predicate{Field: "boolean", Op: PredicateEqual, Value: true},
	}
	target := &Predicate{
		All: []Predicate{{}},
		Any: []Predicate{{}},
		Not: &Predicate{},
	}
	if err := copyPredicateValue(source, target); err != nil {
		t.Fatalf("copyPredicateValue() error = %v", err)
	}
	if target.All[0].Value != int64(1) || target.Any[0].Value != "value" ||
		target.Not.Value != true {
		t.Fatalf("copied predicate = %#v", target)
	}
	if err := copyPredicateValue(
		&Predicate{Value: make(chan int)},
		&Predicate{},
	); err == nil {
		t.Fatal("copyPredicateValue(unsupported value) succeeded")
	}
	if err := copyPredicateValue(
		&Predicate{All: []Predicate{{}}},
		&Predicate{},
	); err == nil {
		t.Fatal("copyPredicateValue(shape mismatch) succeeded")
	}

	nested := GraphDefinition{Nodes: []NodeDefinition{
		{Subgraph: &SubgraphNode{Graph: GraphDefinition{}}},
		{Batch: &BatchNode{Graph: GraphDefinition{}}},
		{Race: &RaceNode{
			Winner: RaceWinnerDefinition{When: &Predicate{
				Field: "value", Op: PredicateEqual, Value: int64(7),
			}},
			Branches: []RaceBranch{{Graph: GraphDefinition{}}},
		}},
	}}
	if _, err := cloneGraph(nested); err != nil {
		t.Fatalf("cloneGraph(nested) error = %v", err)
	}

	for _, test := range []struct {
		stateType StateType
		mimeType  string
		wantErr   bool
	}{
		{stateType: StateString, mimeType: "text/plain"},
		{stateType: StateBlob, mimeType: "application/octet-stream"},
		{stateType: StateString, mimeType: "bad mime", wantErr: true},
		{stateType: StateString, mimeType: "application/json", wantErr: true},
		{stateType: StateBlob, mimeType: "text/plain", wantErr: true},
		{stateType: StateList, mimeType: "application/json", wantErr: true},
	} {
		err := validateOutputMIME(test.stateType, test.mimeType)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateOutputMIME(%q, %q) error = %v", test.stateType, test.mimeType, err)
		}
	}
	if nodeProduces(NodeDefinition{Outputs: map[string]string{"value": "answer"}}, "missing") {
		t.Fatal("nodeProduces(missing) = true")
	}
	for _, test := range []struct {
		value     any
		stateType StateType
		want      bool
	}{
		{value: float64(2), stateType: StateInteger, want: true},
		{value: float64(2.5), stateType: StateInteger},
		{value: "2", stateType: StateInteger},
		{value: int16(2), stateType: StateNumber, want: true},
		{value: "2", stateType: StateNumber},
		{value: nil, stateType: StateMessages},
		{value: []*schema.Message{}, stateType: StateMessages, want: true},
	} {
		if got := valueMatchesStateType(test.value, test.stateType); got != test.want {
			t.Fatalf("valueMatchesStateType(%#v, %q) = %v, want %v", test.value, test.stateType, got, test.want)
		}
	}
}

func validScriptForValidation() *ScriptNode {
	return &ScriptNode{
		Language: ScriptStarlark,
		Source:   "def run(input):\n  return {}\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 100,
			Timeout:           time.Second,
			MaxInputBytes:     100,
			MaxOutputBytes:    100,
		},
	}
}
