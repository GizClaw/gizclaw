package eino

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/schema"
)

func TestNativeAudioStreamsBeforePostFunctionNeedsTranscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	release := make(chan struct{})
	chat := &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("reply", nil)}}
	config := audioInputConfig(&componentMapResolver{chat: chat})
	config.Graph.State.Fields = append(config.Graph.State.Fields, StateField{Name: "heard", Type: StateString, Merge: MergeReplace})
	config.Graph.Nodes = append(config.Graph.Nodes, NodeDefinition{
		ID: "after", Inputs: map[string]Binding{"value": {From: "input.text"}},
		Outputs: map[string]string{"value": "heard"}, Transform: &TransformNode{Operation: TransformSelect},
	})
	config.Graph.Edges = []EdgeDefinition{{From: "start", To: "prompt"}, {From: "prompt", To: "model"}, {From: "model", To: "after"}, {From: "after", To: "end"}}
	// Observe the exact same binding after the reply, through the real Memory
	// consumer rather than inspecting the declaration or source code.
	memories := &recordingMemoryStore{events: &eventRecorder{}}
	config.Memory = &MemoryConfig{Store: memories, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: true, TextFrom: "heard"}}
	core, err := New(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	// History must accompany the original audio to the response model.
	previous, err := core.Transform(ctx, textInput("earlier context"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, previous)
	dock := transcriptionDock(t, core, func(ctx context.Context, _ []*genx.Blob) (string, error) {
		select {
		case <-release:
			return "final transcript", nil
		case <-ctx.Done():
			return "", context.Cause(ctx)
		}
	}, true)
	output, err := dock.Transform(ctx, audioInput("speech", [][]byte{{1, 2, 3}}))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var reply strings.Builder
	for reply.String() != "reply" {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("reply waited for recognition: %v", err)
		}
		if chunk.Role == genx.RoleModel {
			if text, ok := chunk.Part.(genx.Text); ok {
				reply.WriteString(string(text))
			}
		}
	}
	close(release)
	drain(t, output)
	chat.mu.Lock()
	input := chat.inputs[len(chat.inputs)-1]
	chat.mu.Unlock()
	last := input[len(input)-1]
	if len(input) < 4 || input[1].Content != "earlier context" || last.Content != "" || len(last.UserInputMultiContent) != 1 {
		t.Fatalf("history or original audio lost: %#v", input)
	}
	data, err := base64.StdEncoding.DecodeString(*last.UserInputMultiContent[0].Audio.Base64Data)
	if err != nil || !slices.Equal(data, []byte{1, 2, 3}) {
		t.Fatalf("model audio changed: %v, %v", data, err)
	}
	memories.mu.Lock()
	defer memories.mu.Unlock()
	if len(memories.observations) == 0 || memories.observations[len(memories.observations)-1].Text != "final transcript" {
		t.Fatalf("post-function did not receive final transcription: %#v", memories.observations)
	}
}

func TestNativeTextBindingUsesNodeCancellationOutsideStateLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	entered := make(chan struct{})
	state, err := newRunState(map[string]StateField{"answer": {Name: "answer", Type: StateString, Merge: MergeReplace}}, graphInput{
		ResolveText: func(ctx context.Context) (string, error) {
			close(entered)
			<-ctx.Done()
			return "", context.Cause(ctx)
		},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, stop := context.WithCancel(ctx)
	defer stop()
	finished := make(chan error, 1)
	go func() { _, err := state.binding(child, Binding{From: "input.text"}); finished <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("binding never started")
	}
	written := make(chan error, 1)
	go func() { written <- state.set("answer", "other branch") }()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("transcription waiter held the State lock")
	}
	stop()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("child cancellation: %v", err)
	}
}

func TestNativeTextNodesCancelWithTheirExecution(t *testing.T) {
	for _, kind := range []string{"transform", "passthrough"} {
		t.Run(kind, func(t *testing.T) {
			buildCtx, cancelBuild := context.WithCancel(t.Context())
			defer cancelBuild()
			node := NodeDefinition{Inputs: map[string]Binding{"value": {From: "input.text"}}}
			if kind == "transform" {
				node.Transform = &TransformNode{Operation: TransformSelect}
			} else {
				node.Passthrough = &PassthroughNode{}
			}
			compiled, err := compileNode(buildCtx, &normalizedConfig{}, node, nil, "test")
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan context.Context, 1)
			state, err := newRunState(nil, graphInput{ResolveText: func(ctx context.Context) (string, error) {
				entered <- ctx
				<-ctx.Done()
				return "", context.Cause(ctx)
			}}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			runCtx, cancelRun := context.WithCancel(buildCtx)
			defer cancelRun()
			finished := make(chan error, 1)
			go func() { _, _, err := compiled.run(runCtx, state); finished <- err }()
			var waiting context.Context
			select {
			case waiting = <-entered:
			case <-time.After(time.Second):
				t.Fatal("node never requested its transcription")
			}
			cancelRun()
			select {
			case <-waiting.Done():
			default:
				t.Fatal("node waited using its build context instead of its execution context")
			}
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("node cancellation = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled node is still waiting for transcription")
			}
		})
	}
}
