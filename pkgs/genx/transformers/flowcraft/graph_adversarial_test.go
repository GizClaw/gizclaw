package flowcraft

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestGraphExecutionSequentialEveryNodeKind(t *testing.T) {
	t.Parallel()
	transformer, err := New(Config{
		ID: "all-node-kinds", Name: "All node kinds", Models: &patternGenerator{},
		Graph: flowgraph.GraphDefinition{
			Name: "all-node-kinds", Entry: "seed",
			Nodes: []flowgraph.NodeDefinition{
				{
					ID: "seed", Type: "script",
					Config: map[string]any{"source": `board.setVar("visited", ["script"]);`},
				},
				{ID: "middle", Type: "passthrough"},
				{ID: "answer", Type: "llm", Config: map[string]any{"model": "answer"}},
			},
			Edges: []flowgraph.EdgeDefinition{
				{From: "seed", To: "middle"},
				{From: "middle", To: "answer"},
				{From: "answer", To: flowgraph.END},
			},
		},
		PublishNodes: []string{"answer"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("sequential"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "model/answer:sequential" {
		t.Fatalf("output = %q", got)
	}
}

func TestGraphExecutionReportsBoundedLoopExhaustion(t *testing.T) {
	t.Parallel()
	transformer, err := New(Config{
		ID: "loop-limit", Name: "Loop limit", Models: &patternGenerator{}, MaxIterations: 3,
		Graph: flowgraph.GraphDefinition{
			Name: "loop-limit", Entry: "loop",
			Nodes: []flowgraph.NodeDefinition{{
				ID: "loop", Type: "script",
				Config: map[string]any{"source": `board.setVar("counter", Number(board.getVar("counter") || 0) + 1);`},
			}},
			Edges: []flowgraph.EdgeDefinition{{From: "loop", To: "loop"}},
		},
		PublishNodes: []string{"loop"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("never exits"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if errorText := terminalError(chunks); errorText == "" ||
		!strings.Contains(strings.ToLower(errorText), "iteration") {
		t.Fatalf("terminal error = %q, chunks = %#v", errorText, chunks)
	}
}

func TestGraphExecutionPropagatesModelOpenAndProducerFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		generator genx.Generator
		wantText  string
		wantErr   string
	}{
		{
			name:      "open failure",
			generator: &failingGraphGenerator{openErr: errors.New("model refused to open")},
			wantErr:   "model refused to open",
		},
		{
			name:      "producer failure after prefix",
			generator: &failingGraphGenerator{prefix: "accepted-prefix", streamErr: errors.New("provider disconnected")},
			wantErr:   "provider disconnected",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transformer, err := New(testConfig(test.generator))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			output, err := transformer.Transform(t.Context(), textInput("failure"))
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			chunks := drain(t, output)
			if got := joinedText(chunks); got != test.wantText {
				t.Fatalf("output = %q, want %q", got, test.wantText)
			}
			if got := terminalError(chunks); !strings.Contains(got, test.wantErr) {
				t.Fatalf("terminal error = %q, want containing %q", got, test.wantErr)
			}
		})
	}
}

func TestGraphExecutionPreservesChunkOrderUnderBackpressure(t *testing.T) {
	t.Parallel()
	generator := &backpressureGraphGenerator{
		firstAdded: make(chan struct{}),
		release:    make(chan struct{}),
		finished:   make(chan struct{}),
	}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("slow consumer"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-generator.firstAdded:
	case <-time.After(5 * time.Second):
		t.Fatal("producer did not emit first chunk")
	}
	// Deliberately let the producer finish while the public consumer does not
	// pull. The public stream must retain every ordered delta.
	close(generator.release)
	select {
	case <-generator.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("producer did not finish while downstream was paused")
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "alpha-beta-gamma" {
		t.Fatalf("ordered output = %q", got)
	}
}

func TestGraphExecutionCloseWithErrorCancelsProducer(t *testing.T) {
	t.Parallel()
	generator := &cancelTrackingGenerator{cancelled: make(chan struct{})}
	transformer, err := New(testConfig(generator))
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
		if text, ok := chunk.Part.(genx.Text); ok && text == "partial" {
			break
		}
	}
	closer, ok := output.(interface{ CloseWithError(error) error })
	if !ok {
		t.Fatalf("output type %T does not support CloseWithError", output)
	}
	if err := closer.CloseWithError(errors.New("consumer rejected response")); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	select {
	case <-generator.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseWithError did not cancel graph producer")
	}
}

func TestGraphExecutionUsesFreshLifecycleForEveryVisibleRoute(t *testing.T) {
	t.Parallel()
	transformer, err := New(Config{
		ID: "visible-routes", Name: "Visible routes", Models: newBarrierGenerator(2),
		Graph: flowgraph.GraphDefinition{
			Name: "visible-routes", Entry: "start",
			Nodes: []flowgraph.NodeDefinition{
				{ID: "start", Type: "passthrough"},
				{ID: "left", Type: "llm", Config: map[string]any{"model": "left"}},
				{ID: "right", Type: "llm", Config: map[string]any{"model": "right"}},
			},
			Edges: []flowgraph.EdgeDefinition{
				{From: "start", To: "left"}, {From: "start", To: "right"},
				{From: "left", To: flowgraph.END}, {From: "right", To: flowgraph.END},
			},
		},
		PublishNodes: []string{"left", "right"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var previousID string
	for run := range 2 {
		output, err := transformer.Transform(t.Context(), textInput("routes"))
		if err != nil {
			t.Fatalf("Transform(%d) error = %v", run, err)
		}
		chunks := drain(t, output)
		names := map[string]bool{}
		streamID := ""
		bos, eos := 0, 0
		for _, chunk := range chunks {
			if chunk.Ctrl != nil {
				if streamID == "" {
					streamID = chunk.Ctrl.StreamID
				}
				if chunk.Ctrl.StreamID != streamID {
					t.Fatalf("run %d mixed StreamIDs %q and %q", run, streamID, chunk.Ctrl.StreamID)
				}
			}
			if chunk.IsBeginOfStream() {
				bos++
			}
			if chunk.IsEndOfStream() {
				eos++
			} else if _, ok := chunk.Part.(genx.Text); ok {
				names[chunk.Name] = true
				if mimeType, ok := chunk.MIMEType(); !ok || mimeType != "text/plain" {
					t.Fatalf("route %q MIME = %q, present=%v", chunk.Name, mimeType, ok)
				}
			}
		}
		if streamID == "" || streamID == previousID || bos != 1 || eos != 1 ||
			!names["left"] || !names["right"] {
			t.Fatalf("run %d lifecycle id=%q previous=%q BOS=%d EOS=%d names=%v", run, streamID, previousID, bos, eos, names)
		}
		previousID = streamID
	}
}

type failingGraphGenerator struct {
	openErr   error
	prefix    string
	streamErr error
}

func (generator *failingGraphGenerator) GenerateStream(
	_ context.Context,
	_ string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	if generator.openErr != nil {
		return nil, generator.openErr
	}
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		if generator.prefix != "" {
			_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(generator.prefix)})
		}
		_ = builder.Abort(generator.streamErr)
	}()
	return builder.Stream(), nil
}

func (*failingGraphGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

type backpressureGraphGenerator struct {
	firstAdded chan struct{}
	release    chan struct{}
	finished   chan struct{}
	once       sync.Once
}

func (generator *backpressureGraphGenerator) GenerateStream(
	_ context.Context,
	_ string,
	modelContext genx.ModelContext,
) (genx.Stream, error) {
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		defer close(generator.finished)
		for index, text := range []string{"alpha-", "beta-", "gamma"} {
			if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text)}); err != nil {
				_ = builder.Abort(err)
				return
			}
			if index == 0 {
				generator.once.Do(func() { close(generator.firstAdded) })
				<-generator.release
			}
		}
		_ = builder.Done(genx.Usage{})
	}()
	return builder.Stream(), nil
}

func (*backpressureGraphGenerator) Invoke(
	context.Context,
	string,
	genx.ModelContext,
	*genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func terminalError(chunks []*genx.MessageChunk) string {
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil {
			return chunk.Ctrl.Error
		}
	}
	return ""
}
