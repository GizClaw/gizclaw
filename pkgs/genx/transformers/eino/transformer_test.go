package eino

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformStreamsLifecycle(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(t.Context(), textInput("hello"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	var streamID string
	var bos, eos bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if streamID == "" {
			streamID = chunk.Ctrl.StreamID
		}
		if streamID != chunk.Ctrl.StreamID {
			t.Fatalf("StreamID changed from %q to %q", streamID, chunk.Ctrl.StreamID)
		}
		bos = bos || chunk.IsBeginOfStream()
		eos = eos || chunk.IsEndOfStream()
		if chunk.Ctrl.Error != "" {
			t.Fatalf("output error = %q", chunk.Ctrl.Error)
		}
	}
	if streamID == "" || !bos || !eos {
		t.Fatalf("lifecycle stream_id=%q BOS=%v EOS=%v", streamID, bos, eos)
	}
}

func TestTransformerSupportsConcurrentCalls(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const count = 24
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for range count {
		wait.Go(func() {
			text := genx.NewStreamID()
			output, transformErr := transformer.Transform(t.Context(), textInput(text))
			if transformErr != nil {
				failures <- transformErr
				return
			}
			if got := joinedText(drain(t, output)); got != text {
				failures <- errors.New("cross-run output")
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
}

func textConfig() Config {
	return Config{
		Agent:  AgentConfig{ID: "assistant", Name: "Assistant"},
		Limits: Limits{MaxOutputBytes: 1 << 20},
		Graph: GraphDefinition{
			Name: "text",
			State: StateDefinition{Fields: []StateField{{
				Name: "answer", Type: StateString, Merge: MergeReplace,
			}}},
			Nodes: []NodeDefinition{{
				ID: "answer",
				Inputs: map[string]Binding{
					"input": {From: "input.text"},
				},
				Outputs: map[string]string{"text": "answer"},
				Transform: &TransformNode{
					Operation: TransformConcatText, Order: []string{"input"},
				},
			}},
			Edges: []EdgeDefinition{{From: "start", To: "answer"}, {From: "answer", To: "end"}},
			Outputs: []OutputDefinition{{
				Node: "answer", Field: "answer", Name: "assistant", MIMEType: "text/plain", Primary: true,
			}},
		},
	}
}

func newInputBuilder() *genx.StreamBuilder {
	return genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
}

func textInput(text string) genx.Stream {
	builder := newInputBuilder()
	_ = builder.Add(
		genx.NewBeginOfStream(genx.NewStreamID()),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)},
		genx.NewTextEndOfStream(),
	)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func drain(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return chunks
		}
		if err != nil {
			t.Fatalf("drain Stream: %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func joinedText(chunks []*genx.MessageChunk) string {
	var result strings.Builder
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && !chunk.IsEndOfStream() {
			result.WriteString(string(text))
		}
	}
	return result.String()
}
