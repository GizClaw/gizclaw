package eino

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func TestNativeLambdaTranscriptionDoesNotBlockSiblingState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		config := textConfig()
		config.Lambdas = &lambdaMapResolver{lambdas: map[string]*compose.Lambda{
			"wait": compose.InvokableLambda(func(_ context.Context, input map[string]any) (map[string]any, error) {
				return input, nil
			}),
			"gate": compose.InvokableLambda(func(ctx context.Context, input map[string]any) (map[string]any, error) {
				select {
				case <-entered:
					return input, nil
				case <-ctx.Done():
					return nil, context.Cause(ctx)
				}
			}),
		}}
		config.Graph.Compile.NodeTriggerMode = NodeTriggerAllPredecessor
		config.Graph.State.Fields = []StateField{
			{Name: "seed", Type: StateString, Merge: MergeReplace},
			{Name: "heard", Type: StateString, Merge: MergeReplace},
			{Name: "gate", Type: StateString, Merge: MergeReplace},
			{Name: "sibling", Type: StateString, Merge: MergeReplace},
			{Name: "answer", Type: StateString, Merge: MergeReplace},
		}
		config.Graph.Nodes = []NodeDefinition{
			lambdaNode("wait", "wait", "heard"),
			{ID: "gate", Inputs: map[string]Binding{"value": {From: "seed"}}, Outputs: map[string]string{"value": "gate"}, Lambda: &LambdaRefNode{Lambda: "gate"}},
			{ID: "sibling", Outputs: map[string]string{"text": "sibling"}, Script: constantTextScript("independent")},
			{ID: "join", Inputs: map[string]Binding{"value": {From: "heard"}}, Outputs: map[string]string{"value": "answer"}, Transform: &TransformNode{Operation: TransformSelect}},
		}
		config.Graph.Edges = []EdgeDefinition{
			{From: "start", To: "wait"}, {From: "start", To: "gate"}, {From: "gate", To: "sibling"},
			{From: "wait", To: "join"}, {From: "sibling", To: "join"}, {From: "join", To: "end"},
		}
		config.Graph.Outputs[0].Node = "join"
		normalized, err := normalizeConfig(config)
		if err != nil {
			t.Fatal(err)
		}
		graph, err := buildGraph(t.Context(), normalized, normalized.graphCopy, "test")
		if err != nil {
			t.Fatal(err)
		}
		state, err := newRunState(graph.fields, graphInput{ResolveText: func(ctx context.Context) (string, error) {
			close(entered)
			select {
			case <-release:
				return "final transcript", nil
			case <-ctx.Done():
				return "", context.Cause(ctx)
			}
		}}, map[string]any{"seed": "ready"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		finished := make(chan error, 1)
		go func() { finished <- graph.execute(t.Context(), state) }()
		synctest.Wait()
		if got, err := state.value("sibling"); err != nil || got != "independent" {
			t.Errorf("sibling could not progress while Lambda awaited ASR: %v, %v", got, err)
		}
		close(release)
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		if got, err := state.value("answer"); err != nil || got != "final transcript" {
			t.Fatalf("Lambda lost final transcription: %v, %v", got, err)
		}
	})
}

func TestNativePendingFinalizationAcceptsTextReplacement(t *testing.T) {
	for _, controlBOS := range []bool{false, true} {
		t.Run(fmt.Sprintf("control=%t", controlBOS), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				release := make(chan struct{})
				config := audioInputConfig(&componentMapResolver{chat: &fakeChatModel{chunks: []*schema.Message{schema.AssistantMessage("reply", nil)}}})
				config.Memory = &MemoryConfig{Store: &recordingMemoryStore{events: &eventRecorder{}}, Scope: memory.Scope{AppID: "app"}, Observe: ObservePolicy{Enabled: true, TextFrom: "input.text"}}
				core, err := New(ctx, config)
				if err != nil {
					t.Fatal(err)
				}
				defer core.Close()
				dock := transcriptionDock(t, core, func(ctx context.Context, _ []*genx.Blob) (string, error) {
					select {
					case <-release:
						return "old final transcript", nil
					case <-ctx.Done():
						return "", context.Cause(ctx)
					}
				}, true)
				input := streamkit.NewOutput(streamkit.OutputConfig{})
				defer input.Close()
				output, err := dock.Transform(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				for _, chunk := range []*genx.MessageChunk{
					{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "old", BeginOfStream: true}},
					{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "old", EndOfStream: true}},
				} {
					if err := input.Push(chunk); err != nil {
						t.Fatal(err)
					}
				}
				for {
					chunk, err := output.Next()
					if err != nil {
						t.Fatal(err)
					}
					if chunk.Role == genx.RoleModel && chunk.Part == genx.Text("reply") {
						break
					}
				}
				synctest.Wait()
				if controlBOS {
					if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "new", BeginOfStream: true}}); err != nil {
						t.Fatal(err)
					}
				}
				if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("typed replacement"), Ctrl: &genx.StreamCtrl{StreamID: "new", BeginOfStream: !controlBOS, EndOfStream: true}}); err != nil {
					t.Fatal(err)
				}
				// ASR remains pending: the replacement must complete independently.
				var replacementID string
				for {
					chunk, err := output.Next()
					if err != nil {
						t.Fatalf("text replacement waited for old ASR: %v", err)
					}
					if chunk.Role == genx.RoleModel && chunk.Part == genx.Text("reply") {
						replacementID = chunk.Ctrl.StreamID
					}
					if replacementID != "" && chunk.Ctrl != nil && chunk.Ctrl.StreamID == replacementID && chunk.IsEndOfStream() {
						if err := genx.StreamError(chunk.Ctrl); err != nil {
							t.Fatalf("replacement terminal: %v", err)
						}
						break
					}
				}
				close(release)
			})
		})
	}
}

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
