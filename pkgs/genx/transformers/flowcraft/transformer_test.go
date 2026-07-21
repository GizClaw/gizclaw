package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestNewValidatesPublicContract(t *testing.T) {
	t.Parallel()
	valid := testConfig(&echoGenerator{})
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "ID", mutate: func(config *Config) { config.ID = "" }},
		{name: "Models", mutate: func(config *Config) { config.Models = nil }},
		{name: "Graph", mutate: func(config *Config) { config.Graph.Nodes = nil }},
		{name: "PublishNodes", mutate: func(config *Config) { config.PublishNodes = nil }},
		{name: "unknown publisher", mutate: func(config *Config) { config.PublishNodes = []string{"missing"} }},
		{name: "unsupported node", mutate: func(config *Config) { config.Graph.Nodes[0].Type = "shell" }},
		{name: "raw model ID", mutate: func(config *Config) { config.Graph.Nodes[0].Config["model"] = "provider/model" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.Graph = cloneTestGraph(valid.Graph)
			test.mutate(&config)
			if _, err := New(config); err == nil {
				t.Fatal("New() succeeded, want validation error")
			}
		})
	}
}

func TestNewOwnsMutableConfig(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	config := testConfig(generator)
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	config.PublishNodes[0] = "changed"
	config.Graph.Nodes[0].Config["model"] = "changed"
	output, err := transformer.Transform(context.Background(), textInput("owned"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "reply: owned" {
		t.Fatalf("output = %q", got)
	}
	if patterns := generator.patterns(); len(patterns) != 1 || patterns[0] != "model/chat" {
		t.Fatalf("patterns = %v", patterns)
	}
}

func TestTransformStreamsTextAndResolvesModelAlias(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), textInput("hello"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if got := joinedText(chunks); got != "reply: hello" {
		t.Fatalf("output text = %q, want %q", got, "reply: hello")
	}
	var streamID string
	var sawBOS, sawEOS bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if streamID == "" {
			streamID = chunk.Ctrl.StreamID
		}
		if chunk.Ctrl.StreamID != streamID {
			t.Fatalf("one response used StreamIDs %q and %q", streamID, chunk.Ctrl.StreamID)
		}
		sawBOS = sawBOS || chunk.IsBeginOfStream()
		sawEOS = sawEOS || chunk.IsEndOfStream()
	}
	if streamID == "" || !sawBOS || !sawEOS {
		t.Fatalf("stream lifecycle: id=%q BOS=%v EOS=%v", streamID, sawBOS, sawEOS)
	}
	if patterns := generator.patterns(); len(patterns) != 1 || patterns[0] != "model/chat" {
		t.Fatalf("model patterns = %v, want [model/chat]", patterns)
	}
}

func TestTransformAcceptsMatchingControlEOS(t *testing.T) {
	t.Parallel()
	transformer, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	streamID := "text-control-eos"
	if err := input.Add(
		genx.NewBeginOfStream(streamID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	); err != nil {
		t.Fatalf("build input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "reply: hello" {
		t.Fatalf("output = %q", got)
	}
}

func TestTransformBypassesUnrelatedControlEOSDuringTextInput(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := input.Add(
		genx.NewBeginOfStream("one"),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "two", EndOfStream: true}},
	); err != nil {
		t.Fatalf("build input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if len(chunks) != 1 || chunks[0].Ctrl == nil || chunks[0].Ctrl.StreamID != "two" || !chunks[0].IsEndOfStream() {
		t.Fatalf("bypass chunks = %#v", chunks)
	}
	if len(generator.patterns()) != 0 {
		t.Fatal("unclosed text route invoked a model")
	}
}

func TestTransformerSupportsConcurrentTransformCalls(t *testing.T) {
	t.Parallel()
	transformer, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	const count = 24
	var wait sync.WaitGroup
	errorsCh := make(chan error, count)
	for index := range count {
		wait.Go(func() {
			input := fmt.Sprintf("turn-%d", index)
			output, err := transformer.Transform(context.Background(), textInput(input))
			if err != nil {
				errorsCh <- err
				return
			}
			chunks, err := drainResult(output)
			if err != nil {
				errorsCh <- err
				return
			}
			if got := joinedText(chunks); got != "reply: "+input {
				errorsCh <- fmt.Errorf("text = %q", got)
			}
		})
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Errorf("concurrent Transform: %v", err)
	}
}

func TestNewBOSInterruptsPriorTurnAndPersistsDeliveredPrefix(t *testing.T) {
	t.Parallel()
	generator := &interruptGenerator{started: make(chan struct{})}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	inputBuilder := newInputBuilder()
	if err := addTextTurn(inputBuilder, "first"); err != nil {
		t.Fatalf("add first turn: %v", err)
	}
	output, err := transformer.Transform(context.Background(), inputBuilder.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	stream, ok := output.(*sessionStream)
	if !ok {
		t.Fatalf("output type = %T", output)
	}
	var firstID string
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read first prefix: %v", nextErr)
		}
		if chunk.Ctrl != nil {
			firstID = chunk.Ctrl.StreamID
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "partial" {
			break
		}
	}
	if err := addTextTurn(inputBuilder, "second"); err != nil {
		t.Fatalf("add second turn: %v", err)
	}
	if err := inputBuilder.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish input: %v", err)
	}
	remaining := drain(t, output)
	var interruptedEOS bool
	for _, chunk := range remaining {
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == firstID && chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
			interruptedEOS = true
		}
	}
	if !interruptedEOS {
		t.Fatal("interrupted response did not emit interrupted EOS")
	}
	if got := joinedText(remaining); !strings.Contains(got, "reply: second") {
		t.Fatalf("replacement output = %q", got)
	}
	select {
	case <-stream.session.done:
	case <-time.After(2 * time.Second):
		t.Fatal("session did not finish")
	}
	history, err := stream.session.history.load(context.Background())
	if err != nil {
		t.Fatalf("load History: %v", err)
	}
	if len(history) != 4 || history[1].Content() != "partial" || history[3].Content() != "reply: second" {
		t.Fatalf("History = %#v", history)
	}
	if !hasInterruptionMarker(history[1]) {
		t.Fatal("interrupted assistant History lacks marker")
	}
}

func TestTransformBypassesNonTextStream(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	transformer, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	streamID := "audio-input"
	blob := &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1, 2, 3}}
	if err := input.Add(
		genx.NewBeginOfStream(streamID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: blob, Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}},
	); err != nil {
		t.Fatalf("build audio input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if len(chunks) != 3 {
		t.Fatalf("bypass chunks = %d, want 3", len(chunks))
	}
	if got := chunks[1].Part.(*genx.Blob).Data; len(got) != 3 || got[2] != 3 {
		t.Fatalf("bypass blob = %v", got)
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("bypass route = %#v", chunk.Ctrl)
		}
	}
	if len(generator.patterns()) != 0 {
		t.Fatal("non-text input invoked a model")
	}
}

func TestTransformRestoresBypassStreamIDFromBOS(t *testing.T) {
	t.Parallel()
	agent, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	streamID := "implicit-audio-route"
	if err := input.Add(
		genx.NewBeginOfStream(streamID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1}}},
		genx.NewEndOfStream("audio/ogg"),
	); err != nil {
		t.Fatalf("build audio input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	if len(chunks) != 3 {
		t.Fatalf("bypass chunks = %d, want 3", len(chunks))
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("bypass route = %#v, want %q", chunk.Ctrl, streamID)
		}
	}
}

func TestTransformPreservesInterleavedNonTextBoundaries(t *testing.T) {
	t.Parallel()
	agent, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	textID := "text-input"
	audioID := "audio-input"
	if err := input.Add(
		genx.NewBeginOfStream(textID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: textID}},
		genx.NewBeginOfStream(audioID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: audioID}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: audioID, EndOfStream: true}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: textID, EndOfStream: true}},
	); err != nil {
		t.Fatalf("build interleaved input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks := drain(t, output)
	var audioBOS, audioData, audioEOS bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != audioID {
			continue
		}
		audioBOS = audioBOS || chunk.IsBeginOfStream()
		audioEOS = audioEOS || chunk.IsEndOfStream()
		if _, ok := chunk.Part.(*genx.Blob); ok {
			audioData = true
		}
	}
	if !audioBOS || !audioData || !audioEOS {
		t.Fatalf("audio lifecycle: BOS=%v data=%v EOS=%v", audioBOS, audioData, audioEOS)
	}
	if got := joinedText(chunks); got != "reply: hello" {
		t.Fatalf("assistant output = %q", got)
	}
}

func TestTransformPropagatesErroredTextEOS(t *testing.T) {
	t.Parallel()
	agent, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	streamID := "failed-input"
	if err := input.Add(
		genx.NewBeginOfStream(streamID),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: streamID}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true, Error: "asr failed"}},
	); err != nil {
		t.Fatalf("build failed input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if _, err := output.Next(); err == nil || !strings.Contains(err.Error(), "asr failed") {
		t.Fatalf("Next() error = %v, want upstream error", err)
	}
}

func TestMIMEBearingNonTextBOSDoesNotInterruptActiveTextTurn(t *testing.T) {
	t.Parallel()
	generator := &interruptGenerator{started: make(chan struct{})}
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := addTextTurn(input, "first"); err != nil {
		t.Fatalf("add first turn: %v", err)
	}
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var responseID string
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read first prefix: %v", nextErr)
		}
		if chunk.Ctrl != nil {
			responseID = chunk.Ctrl.StreamID
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "partial" {
			break
		}
	}
	audioID := "audio-during-run"
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: audioID, BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg"}, Ctrl: &genx.StreamCtrl{StreamID: audioID, EndOfStream: true}},
	); err != nil {
		t.Fatalf("add audio route: %v", err)
	}
	for range 2 {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read audio bypass: %v", nextErr)
		}
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != audioID {
			t.Fatalf("audio chunk route = %#v", chunk.Ctrl)
		}
		if chunk.Ctrl.StreamID == responseID && chunk.IsEndOfStream() {
			t.Fatal("non-text BOS interrupted the active text response")
		}
	}
	if err := addTextTurn(input, "second"); err != nil {
		t.Fatalf("add second turn: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish input: %v", err)
	}
	remaining := drain(t, output)
	if got := joinedText(remaining); !strings.Contains(got, "reply: second") {
		t.Fatalf("replacement output = %q", got)
	}
}

func TestControlOnlyBOSInterruptsBeforeFirstTextDelta(t *testing.T) {
	t.Parallel()
	generator := &interruptGenerator{started: make(chan struct{})}
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := addTextTurn(input, "first"); err != nil {
		t.Fatalf("add first turn: %v", err)
	}
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var firstID string
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read first prefix: %v", nextErr)
		}
		if chunk.Ctrl != nil {
			firstID = chunk.Ctrl.StreamID
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "partial" {
			break
		}
	}
	replacementID := "replacement"
	if err := input.Add(genx.NewBeginOfStream(replacementID)); err != nil {
		t.Fatalf("add replacement BOS: %v", err)
	}
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("wait for interruption: %v", nextErr)
		}
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == firstID && chunk.IsEndOfStream() {
			if chunk.Ctrl.Error != "interrupted" {
				t.Fatalf("interruption error = %q", chunk.Ctrl.Error)
			}
			break
		}
	}
	if err := input.Add(
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: replacementID}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: replacementID, EndOfStream: true}},
	); err != nil {
		t.Fatalf("finish replacement turn: %v", err)
	}
	_ = input.Done(genx.Usage{})
	if got := joinedText(drain(t, output)); got != "reply: second" {
		t.Fatalf("replacement output = %q", got)
	}
}

func TestTransformCancellationClosesIdleInput(t *testing.T) {
	t.Parallel()
	agent, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := agent.Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	stream := output.(*sessionStream)
	cancel()
	select {
	case <-stream.session.done:
	case <-time.After(time.Second):
		t.Fatal("input pump remained blocked after Transform cancellation")
	}
	if err := input.Push(context.Background(), &genx.MessageChunk{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Push() after cancellation = %v, want closed pipe", err)
	}
}

func TestInlineScriptPersistsSerializableBoardState(t *testing.T) {
	t.Parallel()
	state := kv.NewMemory(nil)
	config := testConfig(&echoGenerator{})
	config.State = state
	config.Graph = flowgraph.GraphDefinition{Name: "script", Entry: "script", Nodes: []flowgraph.NodeDefinition{{
		ID: "script", Type: "script", Config: map[string]any{"source": `board.setVar("kept", "yes"); board.setVar("tmp_drop", "no");`},
	}}}
	config.PublishNodes = []string{"script"}
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := transformer.Transform(context.Background(), textInput("run"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	stream := output.(*sessionStream)
	drain(t, output)
	data, err := state.Get(context.Background(), kv.Key{stream.session.contextID})
	if err != nil {
		t.Fatalf("load saved State: %v", err)
	}
	if got := string(data); !strings.Contains(got, `"kept":"yes"`) || strings.Contains(got, "tmp_drop") {
		t.Fatalf("saved State = %s", got)
	}
}

func TestObservationBuilderReceivesFilteredBoardSnapshot(t *testing.T) {
	store := &waitingMemoryStore{}
	config := testConfig(&echoGenerator{})
	config.Memory = store
	config.MemoryScope = "runtime/user/agent"
	config.ObserveEnabled = true
	config.Graph = flowgraph.GraphDefinition{Name: "script", Entry: "script", Nodes: []flowgraph.NodeDefinition{{
		ID: "script", Type: "script", Config: map[string]any{
			"source": `board.setVar("kept", {"value": "yes"}); board.setVar("tmp_drop", "no");`,
		},
	}}}
	config.PublishNodes = []string{"script"}
	var observed ObservationInput
	config.ObservationBuilder = func(_ context.Context, input ObservationInput) (memory.Observation, error) {
		observed = input
		input.BoardVariables["mutated"] = true
		return DefaultObservationBuilder(context.Background(), input)
	}
	agent, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := agent.Transform(context.Background(), textInput("run"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	drain(t, output)
	if observed.UserText != "run" || observed.DeliveredAssistantText != "" {
		t.Fatalf("ObservationInput text = %#v", observed)
	}
	if _, ok := observed.BoardVariables["kept"]; !ok {
		t.Fatalf("ObservationInput BoardVariables = %#v", observed.BoardVariables)
	}
	if _, ok := observed.BoardVariables["tmp_drop"]; ok {
		t.Fatalf("transient Board variable leaked: %#v", observed.BoardVariables)
	}
}

func TestDefaultRecallRendererPreservesMatchOrder(t *testing.T) {
	t.Parallel()
	result, err := DefaultRecallRenderer(context.Background(), []memory.Match{
		{Fact: memory.Fact{Text: "first"}},
		{Fact: memory.Fact{Text: "  "}},
		{Fact: memory.Fact{Text: "second"}},
	})
	if err != nil {
		t.Fatalf("DefaultRecallRenderer() error = %v", err)
	}
	if result != "Relevant memory:\n- first\n- second" {
		t.Fatalf("DefaultRecallRenderer() = %q", result)
	}
}

func TestNewClonesNilRecallFilterValues(t *testing.T) {
	t.Parallel()
	config := testConfig(&echoGenerator{})
	config.Memory = &waitingMemoryStore{}
	config.MemoryScope = "runtime/user/agent"
	config.RecallProfiles = []MemoryRecallProfile{{
		BoardVariable: "memory", Limit: 1,
		Filters: []memory.Filter{{Field: "kind", Operator: memory.FilterIn, Value: []any{nil}}},
	}}
	if _, err := New(config); err != nil {
		t.Fatalf("New() error = %v", err)
	}
}

func TestObserveWaitOrdersTurnsWithoutBlockingInputPump(t *testing.T) {
	t.Parallel()
	store := &waitingMemoryStore{waitStarted: make(chan struct{}), release: make(chan struct{})}
	generator := &echoGenerator{}
	config := testConfig(generator)
	config.Memory = store
	config.MemoryScope = "runtime/user/agent"
	config.ObserveEnabled = true
	config.ObserveWaitForCompletion = true
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	_ = addTextTurn(input, "first")
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read first response: %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "reply: first" {
			break
		}
	}
	if err := addTextTurn(input, "second"); err != nil {
		t.Fatalf("add second turn: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish input: %v", err)
	}
	next := make(chan error, 1)
	go func() {
		_, nextErr := output.Next()
		next <- nextErr
	}()
	select {
	case <-store.waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Memory Wait was not called")
	}
	if got := len(generator.patterns()); got != 1 {
		t.Fatalf("model calls before Memory completion = %d, want 1", got)
	}
	close(store.release)
	if err := <-next; err != nil {
		t.Fatalf("read first EOS: %v", err)
	}
	remaining := drain(t, output)
	if got := joinedText(remaining); got != "reply: second" {
		t.Fatalf("second output = %q", got)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(store.observations))
	}
	for _, observation := range store.observations {
		if observation.Scope != config.MemoryScope || len(observation.Turns) != 2 {
			t.Fatalf("observation = %#v", observation)
		}
	}
}

func hasInterruptionMarker(message flowmodel.Message) bool {
	for _, part := range message.Parts {
		if part.Type == flowmodel.PartData && part.Data != nil && part.Data.MimeType == "application/vnd.genx.interruption+json" {
			return true
		}
	}
	return false
}

func testConfig(generator genx.Generator) Config {
	return Config{
		ID: "assistant", Name: "Assistant", Models: generator,
		Graph: flowgraph.GraphDefinition{Name: "chat", Entry: "chat", Nodes: []flowgraph.NodeDefinition{{
			ID: "chat", Type: "llm", Config: map[string]any{"model": "chat"},
		}}},
		PublishNodes: []string{"chat"},
	}
}

func cloneTestGraph(source flowgraph.GraphDefinition) flowgraph.GraphDefinition {
	result := source
	result.Nodes = append([]flowgraph.NodeDefinition(nil), source.Nodes...)
	for index := range result.Nodes {
		config := make(map[string]any, len(source.Nodes[index].Config))
		maps.Copy(config, source.Nodes[index].Config)
		result.Nodes[index].Config = config
	}
	return result
}

type echoGenerator struct {
	mu           sync.Mutex
	patternsSeen []string
}

func (g *echoGenerator) GenerateStream(_ context.Context, pattern string, modelContext genx.ModelContext) (genx.Stream, error) {
	g.mu.Lock()
	g.patternsSeen = append(g.patternsSeen, pattern)
	g.mu.Unlock()
	return responseStream(modelContext, "reply: "+lastUserText(modelContext)), nil
}

func (*echoGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (g *echoGenerator) patterns() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.patternsSeen...)
}

type interruptGenerator struct {
	started chan struct{}
	once    sync.Once
}

type waitingMemoryStore struct {
	mu           sync.Mutex
	observations []memory.Observation
	waitStarted  chan struct{}
	release      chan struct{}
	once         sync.Once
}

func (s *waitingMemoryStore) Observe(_ context.Context, observation memory.Observation) (memory.ObserveResult, error) {
	s.mu.Lock()
	s.observations = append(s.observations, observation)
	operationID := fmt.Sprintf("operation-%d", len(s.observations))
	s.mu.Unlock()
	return memory.ObserveResult{Operation: &memory.Operation{ID: operationID, Status: memory.OperationPending}}, nil
}

func (*waitingMemoryStore) Recall(_ context.Context, _ memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}

func (*waitingMemoryStore) Update(_ context.Context, _ memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, errors.New("not supported")
}

func (*waitingMemoryStore) Delete(_ context.Context, _ memory.DeleteRequest) error {
	return errors.New("not supported")
}

func (s *waitingMemoryStore) Wait(ctx context.Context, operationID string) (memory.ObserveResult, error) {
	s.once.Do(func() { close(s.waitStarted) })
	select {
	case <-ctx.Done():
		return memory.ObserveResult{}, ctx.Err()
	case <-s.release:
		return memory.ObserveResult{Operation: &memory.Operation{ID: operationID, Status: memory.OperationSucceeded}}, nil
	}
}

func (g *interruptGenerator) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	user := lastUserText(modelContext)
	if user != "first" {
		return responseStream(modelContext, "reply: "+user), nil
	}
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial")})
		g.once.Do(func() { close(g.started) })
		<-ctx.Done()
		_ = builder.Abort(context.Cause(ctx))
	}()
	return builder.Stream(), nil
}

func (*interruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func responseStream(modelContext genx.ModelContext, text string) genx.Stream {
	builder := genx.NewGrowableStreamBuilder(modelContext, 2)
	_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text)})
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func lastUserText(modelContext genx.ModelContext) string {
	var result string
	for message := range modelContext.Messages() {
		if message.Role != genx.RoleUser {
			continue
		}
		contents, ok := message.Payload.(genx.Contents)
		if !ok {
			continue
		}
		result = ""
		for _, part := range contents {
			if text, ok := part.(genx.Text); ok {
				result += string(text)
			}
		}
	}
	return result
}

func newInputBuilder() *genx.StreamBuilder {
	return genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
}

func textInput(text string) genx.Stream {
	builder := newInputBuilder()
	_ = addTextTurn(builder, text)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func addTextTurn(builder *genx.StreamBuilder, text string) error {
	return builder.Add(
		genx.NewBeginOfStream(genx.NewStreamID()),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text)},
		genx.NewTextEndOfStream(),
	)
}

func drain(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	chunks, err := drainResult(stream)
	if err != nil {
		t.Fatalf("drain Stream: %v", err)
	}
	return chunks
}

func drainResult(stream genx.Stream) ([]*genx.MessageChunk, error) {
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return chunks, nil
			}
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
}

func joinedText(chunks []*genx.MessageChunk) string {
	var result strings.Builder
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			result.WriteString(string(text))
		}
	}
	return result.String()
}
