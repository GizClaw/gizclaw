package eino

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func conditionalOutputConfig() Config {
	config := textConfig()
	config.Graph.Compile.PrimaryOutputMode = PrimaryFirstOutput
	config.Graph.State.Fields = []StateField{
		{Name: "selected", Type: StateString, Merge: MergeReplace},
		{Name: "left", Type: StateString, Merge: MergeReplace},
		{Name: "right", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{ID: "select", Inputs: map[string]Binding{"value": {From: "input.text"}}, Outputs: map[string]string{"value": "selected"}, Passthrough: &PassthroughNode{}},
		{ID: "left", Inputs: map[string]Binding{"value": {From: "input.text"}}, Outputs: map[string]string{"value": "left"}, Passthrough: &PassthroughNode{}},
		{ID: "right", Inputs: map[string]Binding{"value": {From: "input.text"}}, Outputs: map[string]string{"value": "right"}, Passthrough: &PassthroughNode{}},
	}
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "select"}, {From: "left", To: "end"}, {From: "right", To: "end"}}
	config.Graph.Branches = []BranchDefinition{{From: "select", Mode: BranchFirstMatch, Routes: []BranchRoute{{When: Predicate{Field: "selected", Op: PredicateEqual, Value: "left"}, To: "left"}}, Default: "right"}}
	config.Graph.Outputs = []OutputDefinition{
		{Node: "left", Field: "left", Name: "left", MIMEType: "text/plain", Primary: true},
		{Node: "right", Field: "right", Name: "right", MIMEType: "text/plain"},
	}
	return config
}

func TestFirstOutputConditionalRoutesAndHistory(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"left", "right"} {
		t.Run(input, func(t *testing.T) {
			transformer, err := New(t.Context(), conditionalOutputConfig())
			if err != nil {
				t.Fatal(err)
			}
			output, err := transformer.Transform(t.Context(), textInput(input))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			chunks := drain(t, output)
			if got := joinedText(chunks); got != input {
				t.Fatalf("reply = %q, want %q", got, input)
			}
			for _, chunk := range chunks {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					t.Fatalf("terminal error = %q", chunk.Ctrl.Error)
				}
				if chunk.Name != input || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
					t.Fatalf("unselected route or incorrect assistant label: %#v", chunk)
				}
			}
			last := chunks[len(chunks)-1]
			if !last.IsEndOfStream() || last.Name != input {
				t.Fatalf("last boundary = %#v, want selected route EOS", last)
			}
			transformer.history.mu.Lock()
			defer transformer.history.mu.Unlock()
			messages := transformer.history.live
			if len(messages) != 2 || messages[0].Content != input || messages[1].Content != input {
				t.Fatalf("delivered History = %#v", messages)
			}
		})
	}
}

func TestFirstOutputRequiresActualPublication(t *testing.T) {
	t.Parallel()
	config := conditionalOutputConfig()
	config.Graph.Branches[0].Routes = append(config.Graph.Branches[0].Routes, BranchRoute{When: Predicate{Field: "selected", Op: PredicateEqual, Value: "skip"}, To: "end"})
	config.State = &StatePersistenceConfig{
		Store: &recordingStateStore{snapshot: StateSnapshot{Fields: map[string]any{"left": "stale", "right": "stale"}}},
		Scope: "no-output", Fields: []string{"left", "right"},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput("skip"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	chunks := drain(t, output)
	if len(chunks) == 0 || chunks[len(chunks)-1].Ctrl == nil || chunks[len(chunks)-1].Ctrl.Error == "" {
		t.Fatal("persisted stale values must not satisfy this turn's publication requirement")
	}
}

func TestFirstOutputEmptyEmissionIsProduced(t *testing.T) {
	t.Parallel()
	config := conditionalOutputConfig()
	config.Graph.Nodes[2].Outputs = map[string]string{"text": "right"}
	config.Graph.Nodes[2].Passthrough = nil
	config.Graph.Nodes[2].Inputs = nil
	config.Graph.Nodes[2].Script = &ScriptNode{Language: ScriptStarlark, Source: "def run(input):\n    return {\"text\": \"\"}\n", Limits: ScriptLimits{MaxExecutionSteps: 1000, Timeout: time.Second, MaxInputBytes: 4096, MaxOutputBytes: 4096}}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput("right"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for _, chunk := range drain(t, output) {
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("empty declared output failed: %q", chunk.Ctrl.Error)
		}
	}
}

func TestFirstOutputConcurrentInvocations(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), conditionalOutputConfig())
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for index := range 24 {
		wait.Go(func() {
			input := fmt.Sprintf("right-%d", index)
			output, err := transformer.Transform(t.Context(), textInput(input))
			if err != nil {
				t.Error(err)
				return
			}
			defer output.Close()
			chunks := drain(t, output)
			if got := joinedText(chunks); got != input {
				t.Errorf("reply = %q, want %q", got, input)
			}
			for _, chunk := range chunks {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					t.Errorf("terminal error = %q", chunk.Ctrl.Error)
				}
			}
		})
	}
	wait.Wait()
}

func TestFirstOutputRecordsAllDeliveredRoutes(t *testing.T) {
	t.Parallel()
	config := conditionalOutputConfig()
	config.Graph.Branches = nil
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "select"}, {From: "select", To: "left"}, {From: "left", To: "right"}, {From: "right", To: "end"}}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput("line"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "lineline" {
		t.Fatalf("reply = %q", got)
	}
	transformer.history.mu.Lock()
	defer transformer.history.mu.Unlock()
	if len(transformer.history.live) != 2 || transformer.history.live[1].Content != "lineline" {
		t.Fatalf("History did not retain both published lines: %#v", transformer.history.live)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
			t.Fatalf("terminal error = %q", chunk.Ctrl.Error)
		}
		if chunk.Role != genx.RoleModel {
			t.Fatalf("unexpected output role = %q", chunk.Role)
		}
	}
}

func TestFirstOutputInterruptBeforePublication(t *testing.T) {
	t.Parallel()
	for range 32 {
		started := make(chan struct{})
		cancelled := make(chan struct{})
		config := chatConfig(&componentMapResolver{chat: &firstOutputLoserModel{started: started, cancelled: cancelled}})
		config.Graph.Compile.PrimaryOutputMode = PrimaryFirstOutput
		transformer, err := New(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		input := newInputBuilder()
		output, err := transformer.Transform(t.Context(), input.Stream())
		if err != nil {
			t.Fatal(err)
		}
		addTextTurn(t, input, "waiting")
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("model did not start")
		}
		// An empty replacement still interrupts the old run, but does not start
		// another model call. Synchronize on the actual component lifecycle.
		if err := input.Add(genx.NewBeginOfStream(genx.NewStreamID())); err != nil {
			t.Fatal(err)
		}
		if err := input.Done(genx.Usage{}); err != nil {
			t.Fatal(err)
		}
		chunks := drain(t, output)
		if err := output.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("component cancellation was lost")
		}
		if len(chunks) != 2 || !chunks[0].IsBeginOfStream() || !chunks[1].IsEndOfStream() || chunks[1].Ctrl.Error != "interrupted" {
			t.Fatalf("interrupted pre-publication boundary = %#v", chunks)
		}
	}
}
