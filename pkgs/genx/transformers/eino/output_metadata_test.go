package eino

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestOutputMetadataFreezesBeforeFirstNonblankChunk(t *testing.T) {
	config := textConfig()
	calls := 0
	config.OutputMetadata = func(_ OutputDefinition, state map[string]any) map[string]string {
		calls++
		return map[string]string{"selected": state["answer"].(string)}
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	session := newSession(t.Context(), transformer, textInput("unused"))
	defer session.invocation.Output().Close()
	state, err := newRunState(transformer.config.fields, graphInput{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	output := transformer.graph.primary
	response, err := session.invocation.StartResponse(streamkit.ResponseConfig{Role: genx.RoleModel, Name: output.Name}, output.MIMEType)
	if err != nil {
		t.Fatal(err)
	}
	run := &turnRun{session: session, state: state, accepting: true, routes: map[string]outputRoute{output.Name: {definition: output, response: response}}}
	for _, delta := range []struct{ text, state, want string }{{" ", "initial", ""}, {"hello", "fox", "fox"}, {" world", "bird", "fox"}} {
		if err := state.set("answer", delta.state); err != nil {
			t.Fatal(err)
		}
		if err := run.Emit(output, delta.text); err != nil {
			t.Fatal(err)
		}
		chunk, err := session.invocation.Output().Next()
		if err != nil {
			t.Fatal(err)
		}
		if got := chunk.Metadata["selected"]; got != delta.want {
			t.Fatalf("%q metadata = %q, want %q", delta.text, got, delta.want)
		}
		chunk.Metadata = nil
	}
	if calls != 1 {
		t.Fatalf("metadata callback calls = %d", calls)
	}
}

func TestStreamingMetadataDoesNotWaitForModelCompletion(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	defer finish()
	chat := &metadataStreamingModel{release: release}
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "speaker", Type: StateString, Merge: MergeReplace})
	config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
		ID: "select", Inputs: map[string]Binding{"value": {From: "input.text"}}, Outputs: map[string]string{"value": "speaker"}, Passthrough: &PassthroughNode{},
	})
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "select"}, {From: "select", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "end"}}
	config.OutputMetadata = func(_ OutputDefinition, state map[string]any) map[string]string {
		return map[string]string{"speaker": state["speaker"].(string)}
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput("fox"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	first := make(chan *genx.MessageChunk, 1)
	done := make(chan error, 1)
	go func() {
		sent := false
		for {
			chunk, err := output.Next()
			if err != nil {
				done <- err
				return
			}
			if text, ok := chunk.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" && !sent {
				first <- chunk
				sent = true
			}
		}
	}()
	select {
	case chunk := <-first:
		if chunk.Metadata["speaker"] != "fox" {
			t.Fatalf("first text metadata = %#v", chunk.Metadata)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first text waited for model completion")
	}
	finish()
	select {
	case err := <-done:
		if !isStreamEnd(err) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("output did not finish")
	}
	transformer.history.mu.Lock()
	defer transformer.history.mu.Unlock()
	history := transformer.history.live
	if len(history) != 2 || history[0].Content != "fox" || history[1].Content != " hello world" {
		t.Fatalf("delivered History = %#v", history)
	}
}

type metadataStreamingModel struct {
	fakeChatModel
	release <-chan struct{}
}

func (m *metadataStreamingModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		if writer.Send(schema.AssistantMessage(" ", nil), nil) {
			return
		}
		if writer.Send(schema.AssistantMessage("hello", nil), nil) {
			return
		}
		select {
		case <-m.release:
		case <-ctx.Done():
			return
		}
		writer.Send(schema.AssistantMessage(" world", nil), nil)
	}()
	return reader, nil
}
