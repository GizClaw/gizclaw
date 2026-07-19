package doubaorealtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/doubao-speech-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestNewValidatesFunctionCallModel(t *testing.T) {
	toolkit := commonagent.EmptyToolkit()
	if _, err := New(Config{Transformer: &testTransformer{}, Pattern: "model/demo", Model: "1.2.5.0", Toolkit: toolkit}); err == nil || !strings.Contains(err.Error(), Model) {
		t.Fatalf("New(unsupported model) error = %v", err)
	}
	agent, err := New(Config{Transformer: &testTransformer{}, Pattern: "model/demo", Model: Model, Toolkit: toolkit})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if agent.config.Model != doubaospeech.RealtimeDuplexModelDefault {
		t.Fatalf("config model = %q", agent.config.Model)
	}
}

func TestAgentInvokesToolkitStrictlyInProviderOrder(t *testing.T) {
	var order []string
	toolkit := commonagent.ToolkitFunc{
		List: func() []commonagent.Tool {
			return []commonagent.Tool{{Name: "first"}, {Name: "second"}}
		},
		InvokeFunc: func(_ context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			order = append(order, call.ID)
			return commonagent.ToolResult{ID: call.ID, Content: json.RawMessage(`{"ok":true}`)}, nil
		},
	}
	agent, err := New(Config{Transformer: &testTransformer{}, Pattern: "model/demo", Toolkit: toolkit})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := agent.invoke(t.Context(), []doubaospeech.RealtimeDuplexFunctionCall{
		{CallID: "call-2", Name: "second", Arguments: `{}`},
		{CallID: "call-1", Name: "first", Arguments: `{}`},
	})
	if err != nil {
		t.Fatalf("invoke() error = %v", err)
	}
	if !slices.Equal(order, []string{"call-2", "call-1"}) {
		t.Fatalf("invoke order = %v", order)
	}
	if outputs[0].CallID != "call-2" || outputs[1].CallID != "call-1" {
		t.Fatalf("outputs = %#v", outputs)
	}
}

func TestAgentReturnsStructuredBusinessErrorToProvider(t *testing.T) {
	toolkit := commonagent.ToolkitFunc{
		List: func() []commonagent.Tool { return []commonagent.Tool{{Name: "device"}} },
		InvokeFunc: func(_ context.Context, call commonagent.ToolCall) (commonagent.ToolResult, error) {
			return commonagent.ErrorToolResult(call.ID, "offline", "device disconnected"), nil
		},
	}
	agent, err := New(Config{Transformer: &testTransformer{}, Pattern: "model/demo", Toolkit: toolkit})
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := agent.invoke(t.Context(), []doubaospeech.RealtimeDuplexFunctionCall{{CallID: "call-1", Name: "device", Arguments: `{}`}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || !json.Valid([]byte(outputs[0].Output)) || !strings.Contains(outputs[0].Output, "offline") {
		t.Fatalf("outputs = %#v", outputs)
	}
}

func TestProviderToolsRejectUnsupportedSchemaWithoutWeakening(t *testing.T) {
	_, err := providerTools([]commonagent.Tool{{
		ID:   "tool-1",
		Name: "demo",
		InputSchema: &jsonschema.Schema{
			Type:    "object",
			Pattern: "unsupported-at-object-level",
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported JSON Schema") {
		t.Fatalf("providerTools() error = %v", err)
	}
}

func TestResponseStreamReplacesProviderIDsWithFreshIDs(t *testing.T) {
	provider := &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("one"), Ctrl: &genx.StreamCtrl{StreamID: "provider-1"}},
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider-1", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("two"), Ctrl: &genx.StreamCtrl{StreamID: "provider-2"}},
	}}
	stream := &responseStream{Stream: provider, ids: make(map[string]string)}
	first, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	firstEOS, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	second, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Ctrl.StreamID == "provider-1" || first.Ctrl.StreamID != firstEOS.Ctrl.StreamID {
		t.Fatalf("first response IDs = %q / %q", first.Ctrl.StreamID, firstEOS.Ctrl.StreamID)
	}
	if second.Ctrl.StreamID == "provider-2" || second.Ctrl.StreamID == first.Ctrl.StreamID {
		t.Fatalf("second response ID = %q, first = %q", second.Ctrl.StreamID, first.Ctrl.StreamID)
	}
}

func TestResponseStreamCreatesFreshIDsWhenProviderOmitsThem(t *testing.T) {
	provider := &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text("one")},
		{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("two")},
	}}
	stream := &responseStream{Stream: provider, ids: make(map[string]string)}
	first, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	firstEOS, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	second, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Ctrl.StreamID == "" || first.Ctrl.StreamID != firstEOS.Ctrl.StreamID {
		t.Fatalf("first response IDs = %q / %q", first.Ctrl.StreamID, firstEOS.Ctrl.StreamID)
	}
	if second.Ctrl.StreamID == "" || second.Ctrl.StreamID == first.Ctrl.StreamID {
		t.Fatalf("second response ID = %q, first = %q", second.Ctrl.StreamID, first.Ctrl.StreamID)
	}
}

func TestResponseStreamKeepsMultipleTerminalRoutesOnOneID(t *testing.T) {
	provider := &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider-1", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "provider-1", EndOfStream: true}},
		{Role: genx.RoleModel, Part: genx.Text("next"), Ctrl: &genx.StreamCtrl{StreamID: "provider-2"}},
	}}
	stream := &responseStream{Stream: provider, ids: make(map[string]string)}
	textEOS, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	audioEOS, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	next, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if textEOS.Ctrl.StreamID == "" || textEOS.Ctrl.StreamID != audioEOS.Ctrl.StreamID {
		t.Fatalf("terminal route IDs = %q / %q", textEOS.Ctrl.StreamID, audioEOS.Ctrl.StreamID)
	}
	if next.Ctrl.StreamID == "" || next.Ctrl.StreamID == textEOS.Ctrl.StreamID {
		t.Fatalf("next response ID = %q, previous = %q", next.Ctrl.StreamID, textEOS.Ctrl.StreamID)
	}
}

func TestResponseStreamPreservesOutputObservationWithProviderID(t *testing.T) {
	provider := &recordingObservationStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{{
		Role: genx.RoleModel, Part: genx.Text("visible"), Ctrl: &genx.StreamCtrl{StreamID: "provider-1"},
	}}}}
	stream := &responseStream{Stream: provider, ids: make(map[string]string), providerIDs: make(map[string]string)}
	stream.DeferOutputObservation()
	chunk, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Ctrl.StreamID == "provider-1" {
		t.Fatalf("public StreamID = %q, want rewritten ID", chunk.Ctrl.StreamID)
	}
	observed := chunk.Clone()
	stream.ObserveOutput(observed)
	if !provider.deferred {
		t.Fatal("DeferOutputObservation() was not forwarded")
	}
	if provider.observed == nil || provider.observed.Ctrl == nil || provider.observed.Ctrl.StreamID != "provider-1" {
		t.Fatalf("observed chunk = %+v, want provider StreamID", provider.observed)
	}
	if observed.Ctrl.StreamID != chunk.Ctrl.StreamID {
		t.Fatalf("ObserveOutput() mutated caller chunk: got %q, want %q", observed.Ctrl.StreamID, chunk.Ctrl.StreamID)
	}
}

func TestTransformUsesConfiguredPattern(t *testing.T) {
	transformer := &testTransformer{}
	agent, err := New(Config{Transformer: transformer, Pattern: "model/doubao", Toolkit: commonagent.EmptyToolkit()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := agent.Transform(t.Context(), "ignored", &sliceStream{})
	if err != nil {
		t.Fatal(err)
	}
	if transformer.pattern != "model/doubao" {
		t.Fatalf("transform pattern = %q", transformer.pattern)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next() error = %v", err)
	}
}

type testTransformer struct {
	pattern string
}

func (t *testTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	t.pattern = pattern
	return &sliceStream{}, nil
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *sliceStream) Close() error               { return nil }
func (s *sliceStream) CloseWithError(error) error { return nil }

type recordingObservationStream struct {
	sliceStream
	deferred bool
	observed *genx.MessageChunk
}

func (s *recordingObservationStream) DeferOutputObservation() {
	s.deferred = true
}

func (s *recordingObservationStream) ObserveOutput(chunk *genx.MessageChunk) {
	s.observed = chunk.Clone()
}
