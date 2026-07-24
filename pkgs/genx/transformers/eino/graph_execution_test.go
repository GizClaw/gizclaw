package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestGraphExecutionConditionalRouting(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"research", "simple"} {
		config := textConfig()
		config.Graph.State.Fields = []StateField{
			{Name: "intent", Type: StateString, Merge: MergeReplace},
			{Name: "selected", Type: StateString, Merge: MergeReplace},
			{Name: "answer", Type: StateString, Merge: MergeReplace},
		}
		config.Graph.Nodes = []NodeDefinition{
			{
				ID: "classify", Inputs: map[string]Binding{"value": {From: "input.text"}},
				Outputs:   map[string]string{"value": "intent"},
				Transform: &TransformNode{Operation: TransformSelect},
			},
			{
				ID: "research", Inputs: map[string]Binding{"value": {From: "input.text"}},
				Outputs:   map[string]string{"value": "selected"},
				Transform: &TransformNode{Operation: TransformSelect},
			},
			{
				ID: "simple", Inputs: map[string]Binding{"value": {From: "input.text"}},
				Outputs:   map[string]string{"value": "selected"},
				Transform: &TransformNode{Operation: TransformSelect},
			},
			{
				ID: "join", Inputs: map[string]Binding{"value": {From: "selected"}},
				Outputs:   map[string]string{"value": "answer"},
				Transform: &TransformNode{Operation: TransformSelect},
			},
		}
		config.Graph.Edges = []EdgeDefinition{
			{From: "start", To: "classify"}, {From: "research", To: "join"},
			{From: "simple", To: "join"}, {From: "join", To: "end"},
		}
		config.Graph.Branches = []BranchDefinition{{
			From: "classify", Mode: BranchFirstMatch,
			Routes: []BranchRoute{{
				When: Predicate{Field: "intent", Op: PredicateEqual, Value: "research"}, To: "research",
			}},
			Default: "simple",
		}}
		config.Graph.Outputs[0].Node = "join"
		transformer, err := New(t.Context(), config)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		output, err := transformer.Transform(t.Context(), textInput(input))
		if err != nil {
			t.Fatalf("Transform() error = %v", err)
		}
		if got := joinedText(drain(t, output)); got != input {
			t.Fatalf("output = %q", got)
		}
	}
}

func TestGraphExecutionNativeParallelJoin(t *testing.T) {
	t.Parallel()
	barrier := newParallelBarrier(2)
	resolver := &lambdaMapResolver{lambdas: map[string]*compose.Lambda{
		"left": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if err := barrier.wait(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"value": input["value"].(string) + "-L"}, nil
		}),
		"right": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if err := barrier.wait(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"value": input["value"].(string) + "-R"}, nil
		}),
	}}
	config := textConfig()
	config.Lambdas = resolver
	config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
	config.Graph.State.Fields = []StateField{
		{Name: "left", Type: StateString, Merge: MergeReplace},
		{Name: "right", Type: StateString, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		lambdaNode("left", "left", "left"),
		lambdaNode("right", "right", "right"),
		{
			ID: "join",
			Inputs: map[string]Binding{
				"left": {From: "left"}, "right": {From: "right"},
			},
			Outputs: map[string]string{"text": "answer"},
			Transform: &TransformNode{
				Operation: TransformConcatText, Order: []string{"left", "right"}, Separator: "|",
			},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "left"}, {From: "start", To: "right"},
		{From: "left", To: "join"}, {From: "right", To: "join"}, {From: "join", To: "end"},
	}
	config.Graph.Outputs[0].Node = "join"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("x"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "x-L|x-R" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionMultiDestinationBranchAndFanIn(t *testing.T) {
	t.Parallel()
	barrier := newParallelBarrier(2)
	resolver := &lambdaMapResolver{lambdas: map[string]*compose.Lambda{
		"left": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if err := barrier.wait(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"value": input["value"].(string) + "-L"}, nil
		}),
		"right": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if err := barrier.wait(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"value": input["value"].(string) + "-R"}, nil
		}),
	}}
	config := textConfig()
	config.Lambdas = resolver
	config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
	config.Graph.Compile.FanIn = map[string]FanInConfig{
		"join": {StreamMergeWithSourceEOF: true},
	}
	config.Graph.State.Fields = []StateField{
		{Name: "gate", Type: StateString, Merge: MergeReplace},
		{Name: "left", Type: StateString, Merge: MergeReplace},
		{Name: "right", Type: StateString, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "gate", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "gate"}, Transform: &TransformNode{Operation: TransformSelect},
		},
		lambdaNode("left", "left", "left"),
		lambdaNode("right", "right", "right"),
		{
			ID: "join",
			Inputs: map[string]Binding{
				"left": {From: "left"}, "right": {From: "right"},
			},
			Outputs: map[string]string{"text": "answer"},
			Transform: &TransformNode{
				Operation: TransformConcatText, Order: []string{"left", "right"}, Separator: "|",
			},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "gate"},
		{From: "left", To: "join"}, {From: "right", To: "join"}, {From: "join", To: "end"},
	}
	config.Graph.Branches = []BranchDefinition{{
		From: "gate", Mode: BranchAllMatch,
		Routes: []BranchRoute{
			{When: Predicate{Field: "gate", Op: PredicateExists}, To: "left"},
			{When: Predicate{Field: "gate", Op: PredicateExists}, To: "right"},
		},
		Default: "left",
	}}
	config.Graph.Outputs[0].Node = "join"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("fanout"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "fanout-L|fanout-R" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionAnyPredecessorSkipsUnselectedBranch(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "left", want: "left"},
		{input: "other", want: "right"},
	} {
		t.Run(test.input, func(t *testing.T) {
			config := textConfig()
			config.Graph.Compile.NodeTriggerMode = NodeTriggerAnyPredecessor
			config.Graph.State.Fields = []StateField{
				{Name: "intent", Type: StateString, Merge: MergeReplace},
				{Name: "selected", Type: StateString, Merge: MergeReplace},
				{Name: "answer", Type: StateString, Merge: MergeReplace},
			}
			config.Graph.Nodes = []NodeDefinition{
				{
					ID: "classify", Inputs: map[string]Binding{"value": {From: "input.text"}},
					Outputs: map[string]string{"value": "intent"}, Transform: &TransformNode{Operation: TransformSelect},
				},
				{
					ID: "left", Outputs: map[string]string{"text": "selected"},
					Script: constantTextScript("left"),
				},
				{
					ID: "right", Outputs: map[string]string{"text": "selected"},
					Script: constantTextScript("right"),
				},
				{
					ID: "join", Inputs: map[string]Binding{"value": {From: "selected"}},
					Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect},
				},
			}
			config.Graph.Edges = []EdgeDefinition{
				{From: "start", To: "classify"},
				{From: "left", To: "join"}, {From: "right", To: "join"}, {From: "join", To: "end"},
			}
			config.Graph.Branches = []BranchDefinition{{
				From: "classify", Mode: BranchFirstMatch,
				Routes: []BranchRoute{{
					When: Predicate{Field: "intent", Op: PredicateEqual, Value: "left"}, To: "left",
				}},
				Default: "right",
			}}
			config.Graph.Outputs[0].Node = "join"
			transformer, err := New(t.Context(), config)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput(test.input))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if got := joinedText(drain(t, output)); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGraphExecutionBoundedLoop(t *testing.T) {
	t.Parallel()
	script := func(source string) *ScriptNode {
		return &ScriptNode{
			Language: ScriptStarlark, Source: source,
			Limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
		}
	}
	config := textConfig()
	config.Graph.Compile.MaxRunSteps = 20
	config.Graph.State.Fields = []StateField{
		{Name: "counter", Type: StateInteger, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "seed", Outputs: map[string]string{"counter": "counter"},
			Script: script("def run(input):\n  return {\"counter\": 0}\n"),
		},
		{
			ID: "increment", Inputs: map[string]Binding{"counter": {From: "counter"}},
			Outputs: map[string]string{"counter": "counter"},
			Script:  script("def run(input):\n  return {\"counter\": input[\"counter\"] + 1}\n"),
		},
		{
			ID: "finish", Inputs: map[string]Binding{"counter": {From: "counter"}},
			Outputs: map[string]string{"text": "answer"},
			Script:  script("def run(input):\n  return {\"text\": \"%d\" % input[\"counter\"]}\n"),
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "seed"}, {From: "seed", To: "increment"}, {From: "finish", To: "end"},
	}
	config.Graph.Branches = []BranchDefinition{{
		From: "increment", Mode: BranchFirstMatch,
		Routes: []BranchRoute{{
			When: Predicate{Field: "counter", Op: PredicateLess, Value: 3}, To: "increment",
		}},
		Default: "finish",
	}}
	config.Graph.Outputs[0].Node = "finish"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("ignored"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "3" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionStreamsBeforeComponentCompletion(t *testing.T) {
	t.Parallel()
	component := newBlockingChatModel()
	transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: component}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("stream"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var first genx.Text
	for first == "" {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		first, _ = chunk.Part.(genx.Text)
	}
	if first != "first" {
		t.Fatalf("first incremental chunk = %q", first)
	}
	select {
	case <-component.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("model did not remain blocked after first incremental chunk")
	}
	close(component.release)
	if got := joinedText(drain(t, output)); got != "second" {
		t.Fatalf("remaining output = %q", got)
	}
}

func TestGraphExecutionDownstreamCloseCancelsComponent(t *testing.T) {
	t.Parallel()
	component := newBlockingChatModel()
	transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: component}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("cancel"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "first" {
			break
		}
	}
	if err := output.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-component.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("downstream Close did not cancel the model")
	}
}

func TestGraphExecutionDeclaredOutputRoutes(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Graph.State.Fields = []StateField{
		{Name: "primary", Type: StateString, Merge: MergeReplace},
		{Name: "detail", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{{
		ID: "publish", Inputs: map[string]Binding{"text": {From: "input.text"}},
		Outputs: map[string]string{"primary": "primary", "detail": "detail"},
		Script: &ScriptNode{
			Language: ScriptStarlark,
			Source:   "def run(input):\n  return {\"primary\": input[\"text\"], \"detail\": \"detail:\" + input[\"text\"]}\n",
			Limits: ScriptLimits{
				MaxExecutionSteps: 1_000, Timeout: time.Second,
				MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
			},
		},
	}}
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "publish"}, {From: "publish", To: "end"}}
	config.Graph.Outputs = []OutputDefinition{
		{Node: "publish", Field: "primary", Name: "assistant", MIMEType: "text/plain", Primary: true},
		{Node: "publish", Field: "detail", Name: "detail", MIMEType: "text/markdown"},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var previousIDs map[string]string
	for _, input := range []string{"one", "two"} {
		output, err := transformer.Transform(t.Context(), textInput(input))
		if err != nil {
			t.Fatalf("Transform() error = %v", err)
		}
		chunks := drain(t, output)
		routeText := make(map[string]string)
		routeIDs := make(map[string]string)
		lastEOS := ""
		for _, chunk := range chunks {
			if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
				routeText[chunk.Name] += string(text)
			}
			if chunk.Ctrl != nil {
				if existing := routeIDs[chunk.Name]; existing != "" && existing != chunk.Ctrl.StreamID {
					t.Fatalf("route %q changed StreamID", chunk.Name)
				}
				routeIDs[chunk.Name] = chunk.Ctrl.StreamID
				if chunk.IsEndOfStream() {
					lastEOS = chunk.Name
					if chunk.Ctrl.Label != chunk.Name {
						t.Fatalf("route %q label = %q", chunk.Name, chunk.Ctrl.Label)
					}
				}
			}
		}
		if routeText["assistant"] != input || routeText["detail"] != "detail:"+input {
			t.Fatalf("route text = %#v", routeText)
		}
		if lastEOS != "assistant" {
			t.Fatalf("last EOS route = %q, want primary assistant", lastEOS)
		}
		if previousIDs != nil {
			for route, streamID := range routeIDs {
				if previousIDs[route] == streamID {
					t.Fatalf("route %q reused StreamID %q", route, streamID)
				}
			}
		}
		previousIDs = routeIDs
	}
}

func TestGraphExecutionRaceCancelsLosingBranch(t *testing.T) {
	t.Parallel()
	slowStarted := make(chan struct{})
	slowCancelled := make(chan struct{})
	var slowOnce sync.Once
	resolver := &lambdaMapResolver{lambdas: map[string]*compose.Lambda{
		"fast": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			select {
			case <-slowStarted:
			case <-ctx.Done():
				return nil, context.Cause(ctx)
			}
			return map[string]any{"value": input["value"]}, nil
		}),
		"slow": compose.InvokableLambda(func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			slowOnce.Do(func() { close(slowStarted) })
			<-ctx.Done()
			close(slowCancelled)
			return nil, context.Cause(ctx)
		}),
	}}
	config := textConfig()
	config.Lambdas = resolver
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"text": {From: "input.text"}},
		Outputs: map[string]string{"answer": "answer"},
		Race: &RaceNode{
			Branches: []RaceBranch{
				{ID: "fast", Graph: lambdaChildGraph("fast")},
				{ID: "slow", Graph: lambdaChildGraph("slow")},
			},
			Winner: RaceWinnerDefinition{Mode: RaceFirstSuccess}, MaxConcurrency: 2,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("winner"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "winner" {
		t.Fatalf("output = %q", got)
	}
	select {
	case <-slowCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Race loser was not cancelled")
	}
}

func TestGraphExecutionRaceFirstOutputCancelsLoserImmediately(t *testing.T) {
	t.Parallel()
	loserStarted := make(chan struct{})
	loserCancelled := make(chan struct{})
	winnerRelease := make(chan struct{})
	resolver := &namedComponentResolver{chat: map[string]model.BaseChatModel{
		"winner": &firstOutputWinnerModel{
			loserStarted: loserStarted,
			release:      winnerRelease,
		},
		"loser": &firstOutputLoserModel{
			started: loserStarted, cancelled: loserCancelled,
		},
	}}
	child := func(name string) GraphDefinition {
		return GraphDefinition{
			Name: name,
			State: StateDefinition{Fields: []StateField{{
				Name: "answer", Type: StateString, Merge: MergeReplace,
			}}},
			Nodes: []NodeDefinition{{
				ID: "model", Inputs: map[string]Binding{"messages": {From: "input.messages"}},
				Outputs:   map[string]string{"text": "answer"},
				ChatModel: &ChatModelNode{Model: name},
			}},
			Edges: []EdgeDefinition{{From: "start", To: "model"}, {From: "model", To: "end"}},
			Outputs: []OutputDefinition{{
				Node: "model", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true,
			}},
		}
	}
	config := textConfig()
	config.Components = resolver
	config.Graph.Nodes[0] = NodeDefinition{
		ID: "answer", Inputs: map[string]Binding{"messages": {From: "input.messages"}},
		Outputs: map[string]string{"answer": "answer"},
		Race: &RaceNode{
			Branches: []RaceBranch{
				{ID: "winner", Graph: child("winner")},
				{ID: "loser", Graph: child("loser")},
			},
			Winner: RaceWinnerDefinition{Mode: RaceFirstOutput}, MaxConcurrency: 2,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("race"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-loserCancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Race loser remained active after winner's first output")
	}
	close(winnerRelease)
	if got := joinedText(drain(t, output)); got != "first" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionRaceFirstOutputPreservesEmissionOrder(t *testing.T) {
	t.Parallel()
	started := make(chan int, 2)
	started <- 1
	started <- 0
	allDone := make(chan struct{})
	close(allDone)

	winner, err := firstRaceOutput(t.Context(), started, allDone)
	if err != nil {
		t.Fatalf("firstRaceOutput() error = %v", err)
	}
	if winner != 1 {
		t.Fatalf("firstRaceOutput() winner = %d, want first emitted branch 1", winner)
	}
}

func TestGraphExecutionBatchBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	t.Parallel()
	tracker := newBatchTracker(2)
	resolver := &lambdaMapResolver{lambdas: map[string]*compose.Lambda{
		"batch": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
			if err := tracker.enter(ctx); err != nil {
				return nil, err
			}
			defer tracker.leave()
			return map[string]any{"value": input["value"]}, nil
		}),
	}}
	child := GraphDefinition{
		Name: "batch-item",
		State: StateDefinition{Fields: []StateField{
			{Name: "item", Type: StateString, Merge: MergeReplace},
			{Name: "answer", Type: StateString, Merge: MergeReplace},
		}},
		Nodes: []NodeDefinition{{
			ID: "answer", Inputs: map[string]Binding{"value": {From: "item"}},
			Outputs: map[string]string{"value": "answer"}, Lambda: &LambdaRefNode{Lambda: "batch"},
		}},
		Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
		Outputs: []OutputDefinition{{
			Node: "answer", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true,
		}},
	}
	config := textConfig()
	config.Lambdas = resolver
	config.Graph.State.Fields = []StateField{
		{Name: "items", Type: StateList, Merge: MergeReplace},
		{Name: "results", Type: StateList, Merge: MergeReplace},
		{Name: "answer", Type: StateString, Merge: MergeReplace},
	}
	config.Graph.Nodes = []NodeDefinition{
		{
			ID: "items", Outputs: map[string]string{"items": "items"},
			Script: &ScriptNode{
				Language: ScriptStarlark,
				Source:   "def run(input):\n  return {\"items\": [\"a\", \"b\", \"c\", \"d\"]}\n",
				Limits: ScriptLimits{
					MaxExecutionSteps: 1_000, Timeout: time.Second,
					MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
				},
			},
		},
		{
			ID: "batch", Outputs: map[string]string{"items": "results"},
			Batch: &BatchNode{Items: Binding{From: "items"}, Graph: child, MaxConcurrency: 2},
		},
		{
			ID: "answer", Inputs: map[string]Binding{"items": {From: "results"}},
			Outputs: map[string]string{"text": "answer"},
			Script: &ScriptNode{
				Language: ScriptStarlark,
				Source:   "def run(input):\n  return {\"text\": \"|\".join(input[\"items\"])}\n",
				Limits: ScriptLimits{
					MaxExecutionSteps: 1_000, Timeout: time.Second,
					MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
				},
			},
		},
	}
	config.Graph.Edges = []EdgeDefinition{
		{From: "start", To: "items"}, {From: "items", To: "batch"},
		{From: "batch", To: "answer"}, {From: "answer", To: "end"},
	}
	config.Graph.Outputs[0].Node = "answer"
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("ignored"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-tracker.full:
	case <-time.After(5 * time.Second):
		t.Fatal("Batch did not start two children")
	}
	close(tracker.release)
	if got := joinedText(drain(t, output)); got != "a|b|c|d" {
		t.Fatalf("output = %q", got)
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.maxActive != 2 {
		t.Fatalf("Batch max concurrency = %d, want 2", tracker.maxActive)
	}
}

func lambdaChildGraph(name string) GraphDefinition {
	return GraphDefinition{
		Name: name,
		State: StateDefinition{Fields: []StateField{{
			Name: "answer", Type: StateString, Merge: MergeReplace,
		}}},
		Nodes: []NodeDefinition{{
			ID: "answer", Inputs: map[string]Binding{"value": {From: "input.text"}},
			Outputs: map[string]string{"value": "answer"}, Lambda: &LambdaRefNode{Lambda: name},
		}},
		Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
		Outputs: []OutputDefinition{{
			Node: "answer", Field: "answer", Name: "answer", MIMEType: "text/plain", Primary: true,
		}},
	}
}

func constantTextScript(text string) *ScriptNode {
	return &ScriptNode{
		Language: ScriptStarlark,
		Source:   fmt.Sprintf("def run(input):\n  return {\"text\": %q}\n", text),
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000, Timeout: time.Second,
			MaxInputBytes: 1 << 10, MaxOutputBytes: 1 << 10,
		},
	}
}

func lambdaNode(id, name, field string) NodeDefinition {
	return NodeDefinition{
		ID: id, Inputs: map[string]Binding{"value": {From: "input.text"}},
		Outputs: map[string]string{"value": field}, Lambda: &LambdaRefNode{Lambda: name},
	}
}

type lambdaMapResolver struct {
	lambdas map[string]*compose.Lambda
}

type namedComponentResolver struct {
	chat map[string]model.BaseChatModel
}

func (resolver *namedComponentResolver) ResolveChatModel(
	_ context.Context,
	name string,
) (model.BaseChatModel, error) {
	component := resolver.chat[name]
	if component == nil {
		return nil, fmt.Errorf("missing %s", name)
	}
	return component, nil
}

func (*namedComponentResolver) ResolveRetriever(
	context.Context,
	string,
) (retriever.Retriever, error) {
	return nil, errors.New("not supported")
}

func (resolver *lambdaMapResolver) ResolveLambda(_ context.Context, name string) (ResolvedLambda, error) {
	lambda := resolver.lambdas[name]
	if lambda == nil {
		return ResolvedLambda{}, fmt.Errorf("missing %s", name)
	}
	return ResolvedLambda{
		Lambda:  lambda,
		Inputs:  map[string]StateType{"value": StateString},
		Outputs: map[string]StateType{"value": StateString},
	}, nil
}

type parallelBarrier struct {
	target  int
	mu      sync.Mutex
	started int
	release chan struct{}
}

type batchTracker struct {
	target int

	mu        sync.Mutex
	active    int
	maxActive int
	full      chan struct{}
	release   chan struct{}
	once      sync.Once
}

func newBatchTracker(target int) *batchTracker {
	return &batchTracker{
		target: target, full: make(chan struct{}), release: make(chan struct{}),
	}
}

func (tracker *batchTracker) enter(ctx context.Context) error {
	tracker.mu.Lock()
	tracker.active++
	tracker.maxActive = max(tracker.maxActive, tracker.active)
	if tracker.active == tracker.target {
		tracker.once.Do(func() { close(tracker.full) })
	}
	tracker.mu.Unlock()
	select {
	case <-tracker.release:
		return nil
	case <-ctx.Done():
		tracker.leave()
		return context.Cause(ctx)
	}
}

func (tracker *batchTracker) leave() {
	tracker.mu.Lock()
	tracker.active--
	tracker.mu.Unlock()
}

func newParallelBarrier(target int) *parallelBarrier {
	return &parallelBarrier{target: target, release: make(chan struct{})}
}

func (barrier *parallelBarrier) wait(ctx context.Context) error {
	barrier.mu.Lock()
	barrier.started++
	if barrier.started == barrier.target {
		close(barrier.release)
	}
	barrier.mu.Unlock()
	select {
	case <-barrier.release:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

type blockingChatModel struct {
	waiting   chan struct{}
	release   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

type firstOutputWinnerModel struct {
	loserStarted <-chan struct{}
	release      <-chan struct{}
}

func (*firstOutputWinnerModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New("not supported")
}

func (winner *firstOutputWinnerModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](0)
	go func() {
		defer writer.Close()
		select {
		case <-winner.loserStarted:
		case <-ctx.Done():
			return
		}
		if writer.Send(schema.AssistantMessage("first", nil), nil) {
			return
		}
		select {
		case <-winner.release:
		case <-ctx.Done():
		}
	}()
	return reader, nil
}

type firstOutputLoserModel struct {
	started   chan<- struct{}
	cancelled chan<- struct{}
}

func (*firstOutputLoserModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, errors.New("not supported")
}

func (loser *firstOutputLoserModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](0)
	go func() {
		defer writer.Close()
		close(loser.started)
		<-ctx.Done()
		close(loser.cancelled)
	}()
	return reader, nil
}

func newBlockingChatModel() *blockingChatModel {
	return &blockingChatModel{
		waiting: make(chan struct{}), release: make(chan struct{}), cancelled: make(chan struct{}),
	}
}

func (chat *blockingChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	var chunks []*schema.Message
	for {
		chunk, recvErr := reader.Recv()
		if errors.Is(recvErr, io.EOF) {
			return schema.ConcatMessages(chunks)
		}
		if recvErr != nil {
			return nil, recvErr
		}
		chunks = append(chunks, chunk)
	}
}

func (chat *blockingChatModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](0)
	go func() {
		defer writer.Close()
		if writer.Send(schema.AssistantMessage("first", nil), nil) {
			return
		}
		chat.once.Do(func() { close(chat.waiting) })
		select {
		case <-chat.release:
			writer.Send(schema.AssistantMessage("second", nil), nil)
		case <-ctx.Done():
			close(chat.cancelled)
		}
	}()
	return reader, nil
}
