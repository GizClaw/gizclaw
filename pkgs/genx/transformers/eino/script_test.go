package eino

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestScriptRoundTripsTypedValuesThroughProductionGraph(t *testing.T) {
	t.Parallel()
	store := &recordingStateStore{snapshot: StateSnapshot{
		Version: "typed-1",
		Fields: map[string]any{
			"source_blob":     []byte("binary"),
			"source_messages": []*schema.Message{schema.UserMessage("hello")},
			"source_documents": []*schema.Document{{
				ID: "doc", Content: "content", MetaData: map[string]any{"rank": int64(1)},
			}},
		},
	}}
	config := textConfig()
	config.Graph.State.Fields = []StateField{
		{Name: "source_blob", Type: StateBlob, Merge: MergeReplace},
		{Name: "source_messages", Type: StateMessages, Merge: MergeReplace},
		{Name: "source_documents", Type: StateDocuments, Merge: MergeReplace},
		{Name: "blob", Type: StateBlob, Merge: MergeReplace},
		{Name: "messages", Type: StateMessages, Merge: MergeReplace},
		{Name: "documents", Type: StateDocuments, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{{
		ID: "answer",
		Inputs: map[string]Binding{
			"blob": {From: "source_blob"}, "messages": {From: "source_messages"},
			"documents": {From: "source_documents"},
		},
		Outputs: map[string]string{
			"blob": "blob", "messages": "messages", "documents": "documents", "text": "answer",
		},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source: "def run(input):\n" +
				"  return {\"blob\": input[\"blob\"], \"messages\": input[\"messages\"], " +
				"\"documents\": input[\"documents\"], \"text\": \"typed\"}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 1 << 12, MaxOutputBytes: 1 << 12,
			},
		},
	}}
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}}
	config.Graph.Outputs[0].Node = "answer"
	config.State = &StatePersistenceConfig{
		Store: store, Scope: "typed",
		Fields: []string{
			"source_blob", "source_messages", "source_documents",
			"blob", "messages", "documents", "answer",
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("ignored"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "typed" {
		t.Fatalf("output = %q", got)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if got, ok := store.compareFields["blob"].([]byte); !ok || string(got) != "binary" {
		t.Fatalf("Script blob = %#v", store.compareFields["blob"])
	}
	messages, ok := store.compareFields["messages"].([]*schema.Message)
	if !ok || len(messages) != 1 || messages[0].Role != schema.User || messages[0].Content != "hello" {
		t.Fatalf("Script messages = %#v", store.compareFields["messages"])
	}
	documents, ok := store.compareFields["documents"].([]*schema.Document)
	if !ok || len(documents) != 1 || documents[0].ID != "doc" ||
		documents[0].MetaData["rank"] != int64(1) {
		t.Fatalf("Script documents = %#v", store.compareFields["documents"])
	}
}

func TestScriptEnforcesStepTimeoutAndByteLimits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		limits ScriptLimits
		input  map[string]any
		want   string
	}{
		{
			name:   "steps",
			source: "def run(input):\n  total = 0\n  for i in range(100000):\n    total += i\n  return {\"text\": str(total)}\n",
			limits: ScriptLimits{
				MaxExecutionSteps: 100, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
			input: map[string]any{}, want: "step",
		},
		{
			name:   "timeout",
			source: "def run(input):\n  total = 0\n  for i in range(100000000):\n    total += i\n  return {\"text\": str(total)}\n",
			limits: ScriptLimits{
				MaxExecutionSteps: 1_000_000_000, Timeout: time.Millisecond,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
			input: map[string]any{}, want: "deadline",
		},
		{
			name:   "input bytes",
			source: "def run(input):\n  return {\"text\": input[\"text\"]}\n",
			limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 8, MaxOutputBytes: 1 << 10,
			},
			input: map[string]any{"text": "too large"}, want: "input",
		},
		{
			name:   "output bytes",
			source: "def run(input):\n  return {\"text\": \"too large\"}\n",
			limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 8,
			},
			input: map[string]any{}, want: "output",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := compileScript(t.Context(), ScriptNode{
				Language: ScriptStarlark, Source: test.source, Limits: test.limits,
			})
			if err != nil {
				t.Fatalf("compileScript() error = %v", err)
			}
			_, err = script.run(t.Context(), test.input, map[string]StateType{"text": StateString})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("run() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestScriptHonorsCancellation(t *testing.T) {
	t.Parallel()
	script, err := compileScript(t.Context(), ScriptNode{
		Language: ScriptStarlark,
		Source: "def run(input):\n  total = 0\n  for i in range(100000000):\n" +
			"    total += i\n  return {\"text\": str(total)}\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000_000_000, Timeout: time.Minute,
			MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
		},
	})
	if err != nil {
		t.Fatalf("compileScript() error = %v", err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(context.Canceled)
	_, err = script.run(ctx, map[string]any{}, map[string]StateType{"text": StateString})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") {
		t.Fatalf("run() error = %v, want cancellation", err)
	}
}

func TestScriptInitializesMutableGlobalsPerRun(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"value": {From: "input.text"}},
		Outputs: map[string]string{"text": "answer"},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source: "values = []\n" +
				"def run(input):\n" +
				"  values.append(input[\"value\"])\n" +
				"  return {\"text\": \"|\".join(values)}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: 10_000, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const count = 32
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			input := fmt.Sprintf("turn-%d", index)
			output, transformErr := transformer.Transform(t.Context(), textInput(input))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != input {
				failures <- fmt.Errorf("Script global leaked: output %q, want %q", got, input)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestScriptInitializationHonorsCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"value": {From: "input.text"}},
		Outputs: map[string]string{"text": "answer"},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source: "def initialize():\n" +
				"  value = 0\n" +
				"  for i in range(1000000000):\n" +
				"    value += i\n" +
				"initialize()\n" +
				"def run(input):\n" +
				"  return {\"text\": input[\"value\"]}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: ^uint64(0), Timeout: time.Nanosecond,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
		},
	}
	if _, err := New(t.Context(), config); err == nil ||
		(!strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "cancel")) {
		t.Fatalf("New() initialization error = %v, want timeout cancellation", err)
	}

	config.Graph.Nodes[0].Script.Limits.Timeout = time.Second
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := New(ctx, config); err == nil || !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("New() canceled initialization error = %v", err)
	}
}
