package eino

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/buffer"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestTransformerDefensiveAPIsAndStreamHelpers(t *testing.T) {
	t.Parallel()
	transformer, err := New(nil, textConfig())
	if err != nil {
		t.Fatalf("New(nil context) error = %v", err)
	}
	for _, candidate := range []*Transformer{nil, &Transformer{}} {
		if output, err := candidate.Transform(t.Context(), textInput("x")); err == nil || output != nil {
			t.Fatalf("Transform() = %#v, %v, want nil Transformer error", output, err)
		}
	}
	if output, err := transformer.Transform(t.Context(), nil); err == nil || output != nil {
		t.Fatalf("Transform(nil input) = %#v, %v", output, err)
	}
	output, err := transformer.Transform(nil, textInput("nil-context"))
	if err != nil {
		t.Fatalf("Transform(nil context) error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "nil-context" {
		t.Fatalf("output = %q", got)
	}

	var nilStream *sessionStream
	if err := nilStream.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if err := nilStream.CloseWithError(nil); err != nil {
		t.Fatalf("nil CloseWithError() error = %v", err)
	}
	if !isStreamEnd(nil) || !isStreamEnd(io.EOF) || !isStreamEnd(buffer.ErrIteratorDone) ||
		!isStreamEnd(genx.Done(genx.Usage{})) || isStreamEnd(errors.New("failure")) {
		t.Fatal("isStreamEnd() classification mismatch")
	}
	if messageStreamID(nil) != "" || messageStreamID(&genx.MessageChunk{}) != "" {
		t.Fatal("messageStreamID(nil control) was non-empty")
	}
}

func TestTurnRunEmitterAdversarialBoundaries(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	session := newSession(t.Context(), transformer, textInput("unused"))
	runCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(io.EOF)
	run := &turnRun{
		session: session, ctx: runCtx, cancel: cancel,
		routes: make(map[string]outputRoute), streamIDs: make(map[string]struct{}),
		changed: make(chan struct{}, 1),
	}
	output := transformer.graph.primary
	if err := run.Emit(output, "inactive"); !errors.Is(err, streamkit.ErrInactiveResponse) {
		t.Fatalf("Emit(inactive) error = %v", err)
	}
	run.accepting = true
	if err := run.Emit(output, "missing"); err == nil {
		t.Fatal("Emit(missing route) succeeded")
	}
	response, err := session.invocation.StartResponse(streamkit.ResponseConfig{
		Role: genx.RoleModel, Name: output.Name, Label: output.Name,
	}, output.MIMEType)
	if err != nil {
		t.Fatalf("StartResponse() error = %v", err)
	}
	route := outputRoute{definition: output, response: response}
	run.routes[output.Name] = route
	run.primary = route
	run.streamIDs[response.StreamID()] = struct{}{}
	if err := run.Emit(output, 42); err == nil {
		t.Fatal("Emit(unsupported) succeeded")
	}
	if err := run.Emit(output, "text"); err != nil {
		t.Fatalf("Emit(text) error = %v", err)
	}
	blobOutput := output
	blobOutput.Name = "blob"
	blobOutput.MIMEType = "application/octet-stream"
	blobResponse, err := session.invocation.StartResponse(streamkit.ResponseConfig{
		Role: genx.RoleModel, Name: blobOutput.Name, Label: blobOutput.Name,
	}, blobOutput.MIMEType)
	if err != nil {
		t.Fatalf("StartResponse(blob) error = %v", err)
	}
	run.routes[blobOutput.Name] = outputRoute{definition: blobOutput, response: blobResponse}
	if err := run.Emit(blobOutput, []byte{1, 2, 3}); err != nil {
		t.Fatalf("Emit(blob) error = %v", err)
	}
	run.observe(nil)
	run.observe(&genx.MessageChunk{})
	run.observe(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{
		StreamID: response.StreamID(), EndOfStream: true,
	}})
	run.observe(&genx.MessageChunk{
		Part: &genx.Blob{Data: []byte{1, 2}},
		Ctrl: &genx.StreamCtrl{StreamID: response.StreamID()},
	})
	if run.deliveredBytes != 2 {
		t.Fatalf("delivered bytes = %d, want 2", run.deliveredBytes)
	}
	run.terminal = true
	run.interrupt()
	run.terminal = false
	run.interrupted = true
	run.interrupt()
	_ = session.invocation.Cancel(io.EOF)
}

func TestGraphBindingDiscoveryCoversEveryCompositeLocation(t *testing.T) {
	t.Parallel()
	source := "input.parts"
	direct := GraphDefinition{Nodes: []NodeDefinition{{
		Inputs: map[string]Binding{"parts": {From: source}},
	}}}
	if !graphUsesBinding(direct, source) {
		t.Fatal("direct binding was not found")
	}
	retrieverGraph := GraphDefinition{Nodes: []NodeDefinition{{
		Retriever: &RetrieverNode{Query: Binding{From: source}},
	}}}
	if !graphUsesBinding(retrieverGraph, source) {
		t.Fatal("Retriever binding was not found")
	}
	batchItems := GraphDefinition{Nodes: []NodeDefinition{{
		Batch: &BatchNode{Items: Binding{From: source}},
	}}}
	if !graphUsesBinding(batchItems, source) {
		t.Fatal("Batch Items binding was not found")
	}
	for name, graph := range map[string]GraphDefinition{
		"subgraph": {Nodes: []NodeDefinition{{
			Subgraph: &SubgraphNode{Graph: direct},
		}}},
		"batch child": {Nodes: []NodeDefinition{{
			Batch: &BatchNode{Graph: direct},
		}}},
		"race branch": {Nodes: []NodeDefinition{{
			Race: &RaceNode{Branches: []RaceBranch{{Graph: direct}}},
		}}},
	} {
		if !graphUsesBinding(graph, source) {
			t.Fatalf("%s binding was not found", name)
		}
	}
	if graphUsesBinding(GraphDefinition{}, source) {
		t.Fatal("empty Graph reported a binding")
	}
}
