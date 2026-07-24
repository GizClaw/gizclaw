package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestGraphExecutionParallelFanOutJoin(t *testing.T) {
	t.Parallel()
	generator := newBarrierGenerator(2)
	transformer, err := New(Config{
		ID: "parallel", Name: "Parallel", Models: generator,
		Graph: flowgraph.GraphDefinition{
			Name: "parallel", Entry: "start",
			Nodes: []flowgraph.NodeDefinition{
				{ID: "start", Type: "passthrough"},
				{ID: "left", Type: "llm", Config: map[string]any{"model": "left"}},
				{ID: "right", Type: "llm", Config: map[string]any{"model": "right"}},
				{ID: "join", Type: "passthrough"},
			},
			Edges: []flowgraph.EdgeDefinition{
				{From: "start", To: "left"}, {From: "start", To: "right"},
				{From: "left", To: "join"}, {From: "right", To: "join"},
				{From: "join", To: flowgraph.END},
			},
		},
		PublishNodes: []string{"left", "right"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("parallel"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	text := joinedText(drain(t, output))
	if !strings.Contains(text, "model/left") || !strings.Contains(text, "model/right") {
		t.Fatalf("parallel output = %q", text)
	}
	if generator.maxActiveCount() < 2 {
		t.Fatalf("max active model calls = %d, want proven overlap", generator.maxActiveCount())
	}
}

func TestGraphExecutionConcurrentRunIsolation(t *testing.T) {
	t.Parallel()
	transformer, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const count = 16
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			input := fmt.Sprintf("graph-run-%d", index)
			output, transformErr := transformer.Transform(t.Context(), textInput(input))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != "reply: "+input {
				failures <- fmt.Errorf("output %q does not belong to %q", got, input)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func TestGraphExecutionConcurrentToolkitIsolation(t *testing.T) {
	t.Parallel()
	var active atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	tool, err := genx.NewFuncTool[map[string]string](
		"echo",
		"echo one invocation value",
		genx.InvokeFunc[map[string]string](func(ctx context.Context, _ *genx.FuncCall, input map[string]string) (any, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				seen := maximum.Load()
				if current <= seen || maximum.CompareAndSwap(seen, current) {
					break
				}
			}
			if current >= 2 {
				releaseOnce.Do(func() { close(release) })
			}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return map[string]string{"value": input["value"]}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewFuncTool() error = %v", err)
	}
	toolkit, err := genx.NewToolkit(tool)
	if err != nil {
		t.Fatalf("NewToolkit() error = %v", err)
	}
	generator := &concurrentToolGenerator{}
	config := testConfig(generator)
	config.Toolkit = toolkit
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const count = 8
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			input := fmt.Sprintf("tool-run-%d", index)
			output, transformErr := transformer.Transform(t.Context(), textInput(input))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != input {
				failures <- fmt.Errorf("output %q does not belong to %q", got, input)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
	if maximum.Load() < 2 {
		t.Fatalf("maximum concurrent tool calls = %d", maximum.Load())
	}
	if generator.missingTools.Load() != 0 {
		t.Fatalf("model rounds without Toolkit = %d", generator.missingTools.Load())
	}
}

type concurrentToolGenerator struct {
	missingTools atomic.Int32
}

func (generator *concurrentToolGenerator) GenerateStream(
	_ context.Context,
	_ string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	toolCount := 0
	for range modelContext.Tools() {
		toolCount++
	}
	if toolCount != 1 {
		generator.missingTools.Add(1)
	}
	var userText string
	var result *genx.ToolResult
	for message := range modelContext.Messages() {
		switch payload := message.Payload.(type) {
		case genx.Contents:
			if message.Role != genx.RoleUser {
				continue
			}
			userText = ""
			for _, part := range payload {
				if text, ok := part.(genx.Text); ok {
					userText += string(text)
				}
			}
		case *genx.ToolResult:
			result = payload
		}
	}
	builder := genx.NewGrowableStreamBuilder(modelContext, 2)
	if result == nil {
		arguments, err := json.Marshal(map[string]string{"value": userText})
		if err != nil {
			return nil, err
		}
		if err := builder.Add(&genx.MessageChunk{
			Role: genx.RoleModel,
			ToolCall: &genx.ToolCall{
				ID:       "same-provider-id",
				FuncCall: &genx.FuncCall{Name: "echo", Arguments: string(arguments)},
			},
		}); err != nil {
			return nil, err
		}
	} else {
		var decoded map[string]string
		if err := json.Unmarshal([]byte(result.Result), &decoded); err != nil {
			return nil, err
		}
		if err := builder.Add(&genx.MessageChunk{
			Role: genx.RoleModel, Part: genx.Text(decoded["value"]),
		}); err != nil {
			return nil, err
		}
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (*concurrentToolGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("Invoke must not be used")
}

func TestGraphExecutionConditionalAndDefaultRouting(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		approved bool
		want     string
	}{
		{name: "condition", approved: true, want: "model/yes:route"},
		{name: "default", approved: false, want: "model/no:route"},
	} {
		t.Run(test.name, func(t *testing.T) {
			generator := &patternGenerator{}
			transformer, err := New(Config{
				ID: "conditional", Name: "Conditional", Models: generator,
				BoardInputs: func(context.Context) (map[string]any, error) {
					return map[string]any{"approved": test.approved}, nil
				},
				Graph: flowgraph.GraphDefinition{
					Name: "conditional", Entry: "start",
					Nodes: []flowgraph.NodeDefinition{
						{ID: "start", Type: "passthrough"},
						{ID: "yes", Type: "llm", Config: map[string]any{"model": "yes"}},
						{ID: "no", Type: "llm", Config: map[string]any{"model": "no"}},
					},
					Edges: []flowgraph.EdgeDefinition{
						{From: "start", To: "yes", Condition: "approved == true"},
						{From: "start", To: "no"},
						{From: "yes", To: flowgraph.END}, {From: "no", To: flowgraph.END},
					},
				},
				PublishNodes: []string{"yes", "no"},
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput("route"))
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
	generator := &patternGenerator{}
	transformer, err := New(Config{
		ID: "loop", Name: "Loop", Models: generator, MaxIterations: 12,
		Graph: flowgraph.GraphDefinition{
			Name: "loop", Entry: "seed",
			Nodes: []flowgraph.NodeDefinition{
				{
					ID: "seed", Type: "script",
					Config: map[string]any{"source": `board.setVar("counter", 0);`},
				},
				{
					ID: "increment", Type: "script",
					Config: map[string]any{"source": `board.setVar("counter", Number(board.getVar("counter") || 0) + 1);`},
				},
				{ID: "answer", Type: "llm", Config: map[string]any{"model": "done"}},
			},
			Edges: []flowgraph.EdgeDefinition{
				{From: "seed", To: "increment"},
				{From: "increment", To: "increment", Condition: "counter < 3"},
				{From: "increment", To: "answer", Condition: "counter >= 3"},
				{From: "answer", To: flowgraph.END},
			},
		},
		PublishNodes: []string{"answer"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("loop"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "model/done:loop" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionPublishAllowListAndRouteLifecycle(t *testing.T) {
	t.Parallel()
	transformer, err := New(Config{
		ID: "publish", Name: "Publish", Models: &patternGenerator{},
		Graph: flowgraph.GraphDefinition{
			Name: "publish", Entry: "hidden",
			Nodes: []flowgraph.NodeDefinition{
				{ID: "hidden", Type: "llm", Config: map[string]any{"model": "hidden"}},
				{ID: "visible", Type: "llm", Config: map[string]any{"model": "visible"}},
			},
			Edges: []flowgraph.EdgeDefinition{
				{From: "hidden", To: "visible"}, {From: "visible", To: flowgraph.END},
			},
		},
		PublishNodes: []string{"visible"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("publish"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "model/visible:publish" {
		t.Fatalf("published output = %q", got)
	}
	var streamID string
	var bos, eos int
	for _, chunk := range chunks {
		if chunk.Ctrl != nil {
			if streamID == "" {
				streamID = chunk.Ctrl.StreamID
			}
			if chunk.Ctrl.StreamID != streamID {
				t.Fatalf("response changed StreamID from %q to %q", streamID, chunk.Ctrl.StreamID)
			}
		}
		if chunk.IsBeginOfStream() {
			bos++
		}
		if chunk.IsEndOfStream() {
			eos++
			if chunk.Name != assistantLabel || chunk.Ctrl.Label != assistantLabel {
				t.Fatalf("EOS route = %q/%q", chunk.Name, chunk.Ctrl.Label)
			}
		} else if _, ok := chunk.Part.(genx.Text); ok && chunk.Name != "visible" {
			t.Fatalf("published data route = %q, want visible", chunk.Name)
		}
	}
	if streamID == "" || bos != 1 || eos != 1 {
		t.Fatalf("lifecycle id=%q BOS=%d EOS=%d", streamID, bos, eos)
	}
}

func TestGraphExecutionDownstreamCloseCancelsProducer(t *testing.T) {
	t.Parallel()
	generator := &cancelTrackingGenerator{cancelled: make(chan struct{})}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("first"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("Next() error = %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "partial" {
			break
		}
	}
	if err := output.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-generator.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("downstream Close did not cancel the producer")
	}
}

type barrierGenerator struct {
	target int

	mu        sync.Mutex
	started   int
	active    int
	maxActive int
	release   chan struct{}
}

func newBarrierGenerator(target int) *barrierGenerator {
	return &barrierGenerator{target: target, release: make(chan struct{})}
}

func (generator *barrierGenerator) GenerateStream(
	ctx context.Context,
	pattern string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	generator.mu.Lock()
	generator.started++
	generator.active++
	generator.maxActive = max(generator.maxActive, generator.active)
	if generator.started == generator.target {
		close(generator.release)
	}
	generator.mu.Unlock()
	select {
	case <-generator.release:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	generator.mu.Lock()
	generator.active--
	generator.mu.Unlock()
	return responseStream(modelContext, pattern+":"+lastUserText(modelContext)), nil
}

func (*barrierGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (generator *barrierGenerator) maxActiveCount() int {
	generator.mu.Lock()
	defer generator.mu.Unlock()
	return generator.maxActive
}

type patternGenerator struct{}

func (*patternGenerator) GenerateStream(
	_ context.Context,
	pattern string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	return responseStream(modelContext, pattern+":"+lastUserText(modelContext)), nil
}

func (*patternGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

type cancelTrackingGenerator struct {
	cancelled chan struct{}
}

func (generator *cancelTrackingGenerator) GenerateStream(
	ctx context.Context,
	_ string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial")})
		<-ctx.Done()
		close(generator.cancelled)
		_ = builder.Abort(context.Cause(ctx))
	}()
	return builder.Stream(), nil
}

func (*cancelTrackingGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}
