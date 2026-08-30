package flowcraft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/core/graph"
	memoryhook "github.com/GizClaw/flowcraft/core/memory/hook"
	flowmodel "github.com/GizClaw/flowcraft/core/message"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
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
		{name: "raw model ID", mutate: func(config *Config) {
			config.Graph.Nodes[0].Config = testInferenceConfig("provider/model")
		}},
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
	config.Graph.Nodes[0].Config = testInferenceConfig("changed")
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
		if chunk.IsBeginOfStream() {
			sawBOS = true
			if mimeType, ok := chunk.MIMEType(); !ok || mimeType != "text/plain" {
				t.Fatalf("route BOS MIME = %q, present=%v; chunk=%#v", mimeType, ok, chunk)
			}
		}
		if chunk.IsEndOfStream() {
			sawEOS = true
			if chunk.Name != assistantLabel || chunk.Ctrl.Label != assistantLabel {
				t.Fatalf("route EOS name/label = %q/%q, want %q/%q", chunk.Name, chunk.Ctrl.Label, assistantLabel, assistantLabel)
			}
		}
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

func TestAgentInitiativeRunsOnceAcrossTransformCalls(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	config := testConfig(generator)
	config.Initiative = InitiativeOnReload
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	emptyInput := newInputBuilder()
	if err := emptyInput.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish empty input: %v", err)
	}
	first, err := transformer.Transform(context.Background(), emptyInput.Stream())
	if err != nil {
		t.Fatalf("Transform(first) error = %v", err)
	}
	firstChunks := drain(t, first)
	if got := joinedText(firstChunks); got != "reply: " {
		t.Fatalf("first output = %q, want initiative", got)
	}
	initiativeID := ""
	if len(firstChunks) > 0 && firstChunks[0].Ctrl != nil {
		initiativeID = firstChunks[0].Ctrl.StreamID
	}
	if initiativeID == "" {
		t.Fatal("initiative StreamID is empty")
	}
	second, err := transformer.Transform(context.Background(), textInput("hello"))
	if err != nil {
		t.Fatalf("Transform(second) error = %v", err)
	}
	secondChunks := drain(t, second)
	if got := joinedText(secondChunks); got != "reply: hello" {
		t.Fatalf("second output = %q, want no repeated initiative", got)
	}
	if len(secondChunks) == 0 || secondChunks[0].Ctrl == nil || secondChunks[0].Ctrl.StreamID == initiativeID {
		t.Fatalf("second response reused initiative StreamID %q", initiativeID)
	}
	third, err := transformer.Transform(context.Background(), textInput("again"))
	if err != nil {
		t.Fatalf("Transform(third) error = %v", err)
	}
	if got := joinedText(drain(t, third)); got != "reply: again" {
		t.Fatalf("third output = %q, want no repeated initiative", got)
	}
	if patterns := generator.patterns(); len(patterns) != 3 {
		t.Fatalf("generator calls = %v, want one initiative and two peer turns", patterns)
	}
}

func TestOnceWhenEmptyUsesStableConversationHistory(t *testing.T) {
	config := testConfig(&echoGenerator{})
	config.ContextID = "workspace-agent"
	config.Initiative = InitiativeOnceWhenEmpty
	transformer, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if transformer.contextID != "workspace-agent" {
		t.Fatalf("context ID = %q", transformer.contextID)
	}
	transformer.history.live = []flowmodel.Message{flowmodel.NewTextMessage(flowmodel.RoleUser, "existing")}
	claimed, err := transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("once_when_empty claimed initiative with existing history")
	}
}

func TestNewBOSInterruptsRunningInitiative(t *testing.T) {
	t.Parallel()
	generator := &initiativeInterruptGenerator{started: make(chan struct{})}
	config := testConfig(generator)
	config.Initiative = InitiativeOnReload
	transformer, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	select {
	case <-generator.started:
	case <-time.After(time.Second):
		t.Fatal("initiative did not start")
	}
	if err := addTextTurn(input, "hello"); err != nil {
		t.Fatalf("add replacement turn: %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("finish input: %v", err)
	}
	type result struct {
		chunks []*genx.MessageChunk
		err    error
	}
	done := make(chan result, 1)
	go func() {
		chunks, drainErr := drainResult(output)
		done <- result{chunks: chunks, err: drainErr}
	}()
	var chunks []*genx.MessageChunk
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("drain output: %v", got.err)
		}
		chunks = got.chunks
	case <-time.After(2 * time.Second):
		t.Fatal("replacement BOS did not interrupt initiative")
	}
	if got := joinedText(chunks); got != "reply: hello" {
		t.Fatalf("output = %q, want replacement reply", got)
	}
	var interrupted bool
	for _, chunk := range chunks {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil && chunk.Ctrl.Error == "interrupted" {
			interrupted = true
		}
	}
	if !interrupted {
		t.Fatal("initiative did not emit interrupted EOS")
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
	if len(history) != 4 || history[1].Content.Text() != "partial" || history[3].Content.Text() != "reply: second" {
		t.Fatalf("History = %#v", history)
	}
	if !hasInterruptionMarker(history[1]) {
		t.Fatal("interrupted assistant History lacks marker")
	}
}

func TestRepeatedBOSInterruptsPriorTurnsWithoutClosingSession(t *testing.T) {
	t.Parallel()
	const interruptions = 3
	generator := &repeatedInterruptGenerator{started: make(chan string, interruptions)}
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}

	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	results := make(chan result, 32)
	go func() {
		for {
			chunk, nextErr := output.Next()
			results <- result{chunk: chunk, err: nextErr}
			if nextErr != nil {
				return
			}
		}
	}()

	responseIDs := make(map[string]int)
	responseEOS := make(map[string]int)
	interrupted := make(map[string]int)
	for turn := 1; turn <= interruptions; turn++ {
		user := fmt.Sprintf("turn-%d", turn)
		if err := addTextTurn(input, user); err != nil {
			t.Fatalf("add %s: %v", user, err)
		}
		select {
		case started := <-generator.started:
			if started != user {
				t.Fatalf("started turn = %q, want %q", started, user)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not start", user)
		}
		for {
			got := <-results
			if got.err != nil {
				t.Fatalf("read %s prefix: %v", user, got.err)
			}
			if got.chunk != nil && got.chunk.Ctrl != nil && got.chunk.IsEndOfStream() {
				responseEOS[got.chunk.Ctrl.StreamID]++
				if got.chunk.Ctrl.Error == "interrupted" {
					interrupted[got.chunk.Ctrl.StreamID]++
				}
			}
			text, ok := got.chunk.Part.(genx.Text)
			if ok && text == genx.Text("partial: "+user) {
				responseIDs[got.chunk.Ctrl.StreamID]++
				break
			}
		}
	}
	if err := addTextTurn(input, "final"); err != nil {
		t.Fatalf("add final turn: %v", err)
	}
	_ = input.Done(genx.Usage{})

	var finalID string
	for got := range results {
		if got.err != nil {
			if !errors.Is(got.err, io.EOF) && !errors.Is(got.err, genx.ErrDone) {
				t.Fatalf("read output: %v", got.err)
			}
			break
		}
		if got.chunk == nil || got.chunk.Ctrl == nil {
			continue
		}
		if got.chunk.IsEndOfStream() {
			responseEOS[got.chunk.Ctrl.StreamID]++
			if got.chunk.Ctrl.Error == "interrupted" {
				interrupted[got.chunk.Ctrl.StreamID]++
			}
		}
		if text, ok := got.chunk.Part.(genx.Text); ok && text == "reply: final" {
			finalID = got.chunk.Ctrl.StreamID
		}
	}
	if len(responseIDs) != interruptions {
		t.Fatalf("interrupted response IDs = %#v, want %d fresh IDs", responseIDs, interruptions)
	}
	for id := range responseIDs {
		if responseEOS[id] != 1 {
			t.Errorf("EOS count for %s = %d, want 1", id, responseEOS[id])
		}
		if interrupted[id] != 1 {
			t.Errorf("interrupted EOS count for %s = %d, want 1", id, interrupted[id])
		}
		if id == finalID {
			t.Errorf("final response reused interrupted StreamID %s", id)
		}
	}
	if finalID == "" {
		t.Fatal("final replacement response was not delivered")
	}
}

func TestRepeatedBOSClosesInterruptedTurnBeforeReplacementBOS(t *testing.T) {
	t.Parallel()
	generator := newOrderedInterruptGenerator(3)
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	output, err := agent.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	type nextResult struct {
		chunk *genx.MessageChunk
		err   error
	}
	results := make(chan nextResult, 16)
	go func() {
		for {
			chunk, nextErr := output.Next()
			results <- nextResult{chunk: chunk, err: nextErr}
			if nextErr != nil {
				return
			}
		}
	}()
	var chunks []*genx.MessageChunk
	waitForText := func(want string) {
		t.Helper()
		for {
			select {
			case result := <-results:
				if result.err != nil {
					t.Fatalf("read output before %q: %v", want, result.err)
				}
				chunks = append(chunks, result.chunk)
				if text, ok := result.chunk.Part.(genx.Text); ok && text == genx.Text(want) {
					return
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("output %q did not arrive", want)
			}
		}
	}

	if err := addTextTurn(input, "turn-1"); err != nil {
		t.Fatal(err)
	}
	waitForText("partial: turn-1")
	if err := addTextTurn(input, "turn-2"); err != nil {
		t.Fatal(err)
	}
	generator.waitCancelled(t, 0)
	close(generator.release[0])
	waitForText("partial: turn-2")
	if err := addTextTurn(input, "turn-3"); err != nil {
		t.Fatal(err)
	}
	generator.waitCancelled(t, 1)
	close(generator.release[1])
	waitForText("reply: turn-3")
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	for result := range results {
		if errors.Is(result.err, io.EOF) || errors.Is(result.err, genx.ErrDone) {
			break
		}
		if result.err != nil {
			t.Fatalf("read output: %v", result.err)
		}
		chunks = append(chunks, result.chunk)
	}

	type lifecycle struct {
		streamID string
		bosIndex int
		eosIndex int
		bosCount int
		eosCount int
		error    string
	}
	turns := []lifecycle{{bosIndex: -1, eosIndex: -1}, {bosIndex: -1, eosIndex: -1}, {bosIndex: -1, eosIndex: -1}}
	wantText := []genx.Text{"partial: turn-1", "partial: turn-2", "reply: turn-3"}
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok {
			continue
		}
		for turn := range turns {
			if text == wantText[turn] {
				turns[turn].streamID = chunk.Ctrl.StreamID
			}
		}
	}
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		for turn := range turns {
			if turns[turn].streamID == "" || chunk.Ctrl.StreamID != turns[turn].streamID {
				continue
			}
			if chunk.IsBeginOfStream() {
				turns[turn].bosIndex = index
				turns[turn].bosCount++
			}
			if chunk.IsEndOfStream() {
				turns[turn].eosIndex = index
				turns[turn].eosCount++
				turns[turn].error = chunk.Ctrl.Error
			}
		}
	}
	for turn, lifecycle := range turns {
		if lifecycle.streamID == "" || lifecycle.bosCount != 1 || lifecycle.eosCount != 1 || lifecycle.bosIndex >= lifecycle.eosIndex {
			t.Fatalf("turn %d lifecycle = %#v; chunks=%#v", turn+1, lifecycle, chunks)
		}
		wantError := ""
		if turn < len(turns)-1 {
			wantError = "interrupted"
		}
		if lifecycle.error != wantError {
			t.Errorf("turn %d EOS error = %q, want %q", turn+1, lifecycle.error, wantError)
		}
		if turn > 0 && turns[turn-1].eosIndex >= lifecycle.bosIndex {
			t.Errorf("turn %d BOS index %d preceded turn %d EOS index %d", turn+1, lifecycle.bosIndex, turn, turns[turn-1].eosIndex)
		}
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
		&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: audioID, BeginOfStream: true}},
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

func TestControlOnlyBOSDiscardsUnfinishedInputText(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := input.Add(
		genx.NewBeginOfStream("stale"),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("stale text"), Ctrl: &genx.StreamCtrl{StreamID: "stale"}},
		genx.NewBeginOfStream("replacement"),
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "stale", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("fresh text"), Ctrl: &genx.StreamCtrl{StreamID: "replacement"}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "replacement", EndOfStream: true}},
	); err != nil {
		t.Fatalf("build replacement input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "reply: fresh text" {
		t.Fatalf("output = %q", got)
	}
	if patterns := generator.patterns(); len(patterns) != 1 {
		t.Fatalf("model calls = %d, want 1", len(patterns))
	}
}

func TestTextBearingBOSDiscardsUnfinishedInputText(t *testing.T) {
	t.Parallel()
	generator := &echoGenerator{}
	agent, err := New(testConfig(generator))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := input.Add(
		genx.NewBeginOfStream("stale"),
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("stale text"), Ctrl: &genx.StreamCtrl{StreamID: "stale"}},
		&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("fresh text"), Ctrl: &genx.StreamCtrl{StreamID: "replacement", BeginOfStream: true}},
		&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "replacement", EndOfStream: true}},
	); err != nil {
		t.Fatalf("build replacement input: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := joinedText(drain(t, output)); got != "reply: fresh text" {
		t.Fatalf("output = %q", got)
	}
	if patterns := generator.patterns(); len(patterns) != 1 {
		t.Fatalf("model calls = %d, want 1", len(patterns))
	}
}

func TestOrphanControlBOSIsDroppedAtInputEOF(t *testing.T) {
	t.Parallel()
	agent, err := New(testConfig(&echoGenerator{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := newInputBuilder()
	if err := input.Add(genx.NewBeginOfStream("orphan")); err != nil {
		t.Fatalf("add BOS: %v", err)
	}
	_ = input.Done(genx.Usage{})
	output, err := agent.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if chunks := drain(t, output); len(chunks) != 0 {
		t.Fatalf("orphan BOS produced %d output chunks", len(chunks))
	}
}

func TestEarlyInterruptionDoesNotPersistEmptyAssistant(t *testing.T) {
	t.Parallel()
	generator := &silentInterruptGenerator{started: make(chan struct{})}
	memoryStore := &waitingMemoryStore{}
	config := testConfig(generator)
	config.Memory = memoryStore
	config.MemoryScope = memory.Scope{AppID: "runtime", UserID: "user", AgentID: "agent"}
	config.MemoryTurn = &memoryhook.TurnSettings{Channel: "inference.chat"}
	agent, err := New(config)
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
	select {
	case <-generator.started:
	case <-time.After(time.Second):
		t.Fatal("first model call did not start")
	}
	if err := addTextTurn(input, "second"); err != nil {
		t.Fatalf("add replacement turn: %v", err)
	}
	_ = input.Done(genx.Usage{})
	if got := joinedText(drain(t, output)); got != "reply: second" {
		t.Fatalf("replacement output = %q", got)
	}
	stream := output.(*sessionStream)
	history, err := stream.session.history.load(context.Background())
	if err != nil {
		t.Fatalf("load History: %v", err)
	}
	if len(history) != 2 || history[0].Content.Text() != "second" || history[1].Content.Text() != "reply: second" {
		t.Fatalf("History = %#v", history)
	}
	memoryStore.mu.Lock()
	observations := append([]memory.Observation(nil), memoryStore.observations...)
	memoryStore.mu.Unlock()
	if len(observations) != 1 || len(observations[0].Turns) != 2 || observations[0].Turns[0].Text != "second" {
		t.Fatalf("Memory observations = %#v", observations)
	}
}

func TestOfficialMemoryHooksUseConfiguredStore(t *testing.T) {
	store := &waitingMemoryStore{}
	config := testConfig(&echoGenerator{})
	config.Memory = store
	config.MemoryScope = memory.Scope{AppID: "runtime", UserID: "user", AgentID: "agent"}
	config.MemoryContext = &memoryhook.ContextSettings{
		Query:  memoryhook.QuerySettings{CurrentMessage: true},
		Budget: memoryhook.BudgetSettings{MaxItems: 3},
		Output: "memory_items",
	}
	config.MemoryTurn = &memoryhook.TurnSettings{Channel: "inference.chat"}
	agent, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(agent.agent.Prepare) != 1 || len(agent.agent.Commit) != 1 {
		t.Fatalf("Agent hooks prepare=%d commit=%d, want 1/1", len(agent.agent.Prepare), len(agent.agent.Commit))
	}
	output, err := agent.Transform(t.Context(), textInput("remember blue lantern"))
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	drain(t, output)
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.queries) != 1 || store.queries[0].Text != "remember blue lantern" || store.queries[0].Limit != 3 {
		t.Fatalf("Recall queries = %#v", store.queries)
	}
	if len(store.observations) != 1 || len(store.observations[0].Turns) != 2 {
		t.Fatalf("Memory observations = %#v", store.observations)
	}
	if got := store.observations[0].Turns[1].Text; got != "reply: remember blue lantern" {
		t.Fatalf("assistant turn = %q", got)
	}
}

func TestInterruptedTurnReportsFinalizeFailure(t *testing.T) {
	t.Parallel()
	generator := &interruptGenerator{started: make(chan struct{})}
	config := testConfig(generator)
	config.History = &failingHistoryStore{}
	agent, err := New(config)
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
	if err := input.Add(genx.NewBeginOfStream("replacement")); err != nil {
		t.Fatalf("add replacement BOS: %v", err)
	}
	_ = input.Done(genx.Usage{})
	var reported bool
	for _, chunk := range drain(t, output) {
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == responseID && chunk.IsEndOfStream() {
			reported = strings.Contains(chunk.Ctrl.Error, "history failed")
		}
	}
	if !reported {
		t.Fatal("interrupted EOS did not report History failure")
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
		ID: "script", Type: "script", Config: testNodeConfig(map[string]any{"runtime": "js", "source": `board.setVar("kept", "yes"); board.setVar("tmp_drop", "no");`}),
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

func hasInterruptionMarker(message flowmodel.Message) bool {
	for _, raw := range message.Content.Parts {
		part, err := flowmodel.NormalizePart(raw)
		if err == nil {
			if data, ok := part.(flowmodel.DataPart); ok && data.MediaType == "application/vnd.genx.interruption+json" {
				return true
			}
		}
	}
	return false
}

func testConfig(generator genx.Generator) Config {
	return Config{
		ID: "assistant", Name: "Assistant", Models: generator,
		Graph: flowgraph.GraphDefinition{Name: "chat", Entry: "chat", Nodes: []flowgraph.NodeDefinition{{
			ID: "chat", Type: "inference", Config: testInferenceConfig("chat"),
		}}},
		PublishNodes: []string{"chat"},
	}
}

func cloneTestGraph(source flowgraph.GraphDefinition) flowgraph.GraphDefinition {
	result := source
	result.Nodes = append([]flowgraph.NodeDefinition(nil), source.Nodes...)
	for index := range result.Nodes {
		result.Nodes[index].Config = append([]byte(nil), source.Nodes[index].Config...)
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

type initiativeInterruptGenerator struct {
	started chan struct{}
	once    sync.Once
}

type silentInterruptGenerator struct {
	started chan struct{}
	once    sync.Once
}

type repeatedInterruptGenerator struct {
	started chan string
}

type orderedInterruptGenerator struct {
	mu        sync.Mutex
	calls     int
	started   []chan struct{}
	cancelled []chan struct{}
	release   []chan struct{}
}

func newOrderedInterruptGenerator(turns int) *orderedInterruptGenerator {
	generator := &orderedInterruptGenerator{
		started:   make([]chan struct{}, turns),
		cancelled: make([]chan struct{}, turns-1),
		release:   make([]chan struct{}, turns-1),
	}
	for index := range generator.started {
		generator.started[index] = make(chan struct{})
	}
	for index := range generator.cancelled {
		generator.cancelled[index] = make(chan struct{})
		generator.release[index] = make(chan struct{})
	}
	return generator
}

type waitingMemoryStore struct {
	mu           sync.Mutex
	observations []memory.Observation
	queries      []memory.Query
	waitStarted  chan struct{}
	release      chan struct{}
	once         sync.Once
}

type failingHistoryStore struct{}

func (*failingHistoryStore) Append(context.Context, []logstore.Record) ([]logstore.RecordKey, error) {
	return nil, errors.New("history failed")
}

func (*failingHistoryStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}

func (*failingHistoryStore) Replace(context.Context, logstore.Record) error { return nil }

func (*failingHistoryStore) Delete(context.Context, logstore.RecordKey) error { return nil }

func (*failingHistoryStore) Close() error { return nil }

func (s *waitingMemoryStore) Observe(_ context.Context, observation memory.Observation) (memory.ObserveResult, error) {
	s.mu.Lock()
	s.observations = append(s.observations, observation)
	operationID := fmt.Sprintf("operation-%d", len(s.observations))
	s.mu.Unlock()
	return memory.ObserveResult{Operation: &memory.Operation{ID: operationID, Status: memory.OperationPending}}, nil
}

func (s *waitingMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.mu.Lock()
	s.queries = append(s.queries, query)
	s.mu.Unlock()
	return memory.RecallResult{}, nil
}

func (*waitingMemoryStore) Update(_ context.Context, _ memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, errors.New("not supported")
}

func (*waitingMemoryStore) Delete(_ context.Context, _ memory.DeleteRequest) error {
	return errors.New("not supported")
}

func (s *waitingMemoryStore) Wait(ctx context.Context, request memory.OperationRequest) (memory.ObserveResult, error) {
	s.once.Do(func() { close(s.waitStarted) })
	select {
	case <-ctx.Done():
		return memory.ObserveResult{}, ctx.Err()
	case <-s.release:
		return memory.ObserveResult{Operation: &memory.Operation{ID: request.ID, Status: memory.OperationSucceeded}}, nil
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

func (g *initiativeInterruptGenerator) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	user := lastUserText(modelContext)
	if user != "" {
		return responseStream(modelContext, "reply: "+user), nil
	}
	g.once.Do(func() { close(g.started) })
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func (*initiativeInterruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (*interruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (g *silentInterruptGenerator) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	if lastUserText(modelContext) != "first" {
		return responseStream(modelContext, "reply: "+lastUserText(modelContext)), nil
	}
	g.once.Do(func() { close(g.started) })
	<-ctx.Done()
	return nil, context.Cause(ctx)
}

func (*silentInterruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (g *repeatedInterruptGenerator) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	user := lastUserText(modelContext)
	if user == "final" {
		return responseStream(modelContext, "reply: final"), nil
	}
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial: " + user)})
		g.started <- user
		<-ctx.Done()
		_ = builder.Abort(context.Cause(ctx))
	}()
	return builder.Stream(), nil
}

func (*repeatedInterruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (g *orderedInterruptGenerator) GenerateStream(ctx context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	g.mu.Lock()
	turn := g.calls
	g.calls++
	g.mu.Unlock()
	if turn >= len(g.started) {
		return nil, fmt.Errorf("unexpected generator turn %d", turn+1)
	}
	user := lastUserText(modelContext)
	if turn == len(g.started)-1 {
		close(g.started[turn])
		return responseStream(modelContext, "reply: "+user), nil
	}
	builder := genx.NewGrowableStreamBuilder(modelContext, 1)
	go func() {
		_ = builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial: " + user)})
		close(g.started[turn])
		<-ctx.Done()
		close(g.cancelled[turn])
		<-g.release[turn]
		_ = builder.Abort(context.Cause(ctx))
	}()
	return builder.Stream(), nil
}

func (*orderedInterruptGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func (g *orderedInterruptGenerator) waitCancelled(t *testing.T, turn int) {
	t.Helper()
	select {
	case <-g.cancelled[turn]:
	case <-time.After(5 * time.Second):
		t.Fatalf("turn %d was not cancelled", turn+1)
	}
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
	if result == providerSafeEmptyUserText {
		return ""
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
