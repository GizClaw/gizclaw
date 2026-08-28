package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
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
		if chunk.IsBeginOfStream() {
			bos = true
			if mimeType, ok := chunk.MIMEType(); !ok || mimeType != "text/plain" {
				t.Fatalf("BOS MIME = %q, present=%v; chunk=%#v", mimeType, ok, chunk)
			}
		}
		eos = eos || chunk.IsEndOfStream()
		if chunk.Ctrl.Error != "" {
			t.Fatalf("output error = %q", chunk.Ctrl.Error)
		}
	}
	if streamID == "" || !bos || !eos {
		t.Fatalf("lifecycle stream_id=%q BOS=%v EOS=%v", streamID, bos, eos)
	}
}

func TestOutputRouteBeginCarriesDeclaredMIME(t *testing.T) {
	for _, test := range []struct {
		name          string
		mimeType      string
		wantCanonical string
		wantText      bool
	}{
		{name: "text", mimeType: " Text/Plain ", wantCanonical: "text/plain", wantText: true},
		{name: "parameterized text", mimeType: "text/plain; charset=utf-8", wantCanonical: "text/plain; charset=utf-8"},
		{name: "declared text subtype", mimeType: "text/markdown", wantCanonical: "text/markdown"},
		{name: "blob", mimeType: "application/octet-stream", wantCanonical: "application/octet-stream"},
	} {
		t.Run(test.name, func(t *testing.T) {
			chunk := newOutputRouteBegin("route", test.mimeType)
			if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "route" || !chunk.IsBeginOfStream() || chunk.IsEndOfStream() {
				t.Fatalf("route BOS = %#v", chunk)
			}
			if mimeType, ok := chunk.MIMEType(); !ok || mimeType != test.wantCanonical {
				t.Fatalf("route BOS MIME = %q, present=%v, want %q", mimeType, ok, test.wantCanonical)
			}
			_, isText := chunk.Part.(genx.Text)
			if isText != test.wantText {
				t.Fatalf("route BOS part = %T, want text=%v", chunk.Part, test.wantText)
			}
		})
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

func TestInitiativeClaimIsAtomicAcrossConcurrentStreams(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnReload
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	const count = 32
	var claimed atomic.Int32
	var wait sync.WaitGroup
	failures := make(chan error, count)
	for range count {
		wait.Go(func() {
			ok, claimErr := transformer.claimInitiative(t.Context())
			if claimErr != nil {
				failures <- claimErr
				return
			}
			if ok {
				claimed.Add(1)
			}
		})
	}
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
	if got := claimed.Load(); got != 1 {
		t.Fatalf("initiative claims = %d, want 1", got)
	}
}

func TestInitiativeOnceWhenEmptySkipsExistingHistory(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnceWhenEmpty
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	transformer.history.live = []*schema.Message{{Role: schema.User, Content: "existing turn"}}
	claimed, err := transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("once_when_empty claimed initiative with existing history")
	}
	claimed, err = transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("once_when_empty retried after existing-history decision")
	}
}

func TestFailedInitiativeCanBeClaimedAgain(t *testing.T) {
	t.Parallel()
	config := textConfig()
	config.Initiative = InitiativeOnReload
	config.Graph.Nodes[0].Transform = nil
	config.Graph.Nodes[0].Script = &ScriptNode{
		Language: ScriptStarlark,
		Source:   "def run(input):\n  fail(\"initiative failed\")\n",
		Limits: ScriptLimits{
			MaxExecutionSteps: 1_000,
			Timeout:           time.Second,
			MaxInputBytes:     1 << 10,
			MaxOutputBytes:    1 << 10,
		},
	}
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	output, err := transformer.Transform(t.Context(), textInput(""))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, output)

	claimed, err := transformer.claimInitiative(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("failed initiative remained permanently claimed")
	}
}

func TestPeerBOSInterruptsInitiativeWithoutMixingTurns(t *testing.T) {
	t.Parallel()
	chat := newBlockingChatModel()
	config := chatConfig(&componentMapResolver{chat: chat})
	config.Initiative = InitiativeOnReload
	transformer, err := New(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	var initiativeStreamID string
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			t.Fatalf("read initiative: %v", nextErr)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "first" {
			initiativeStreamID = chunk.Ctrl.StreamID
			break
		}
	}
	addTextTurn(t, input, "peer")
	select {
	case <-chat.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Peer BOS did not interrupt the initiative")
	}
	close(chat.release)
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	chunks := drain(t, output)
	var sawInterrupted, sawPeer bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID == initiativeStreamID && chunk.IsEndOfStream() &&
			chunk.Ctrl.Error == "interrupted" {
			sawInterrupted = true
		}
		if chunk.Ctrl.StreamID != initiativeStreamID {
			if text, ok := chunk.Part.(genx.Text); ok && text != "" {
				sawPeer = true
			}
		}
	}
	if !sawInterrupted || !sawPeer {
		t.Fatalf("interrupted=%v peer_output=%v chunks=%#v", sawInterrupted, sawPeer, chunks)
	}
}

func TestRepeatedBOSClosesInterruptedTurnBeforeReplacementBOS(t *testing.T) {
	t.Parallel()
	chat := newOrderedInterruptChatModel(3)
	transformer, err := New(t.Context(), chatConfig(&componentMapResolver{chat: chat}))
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
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

	addTextTurn(t, input, "turn-1")
	waitForText("reply-1")
	addTextTurn(t, input, "turn-2")
	chat.waitCancelled(t, 0)
	close(chat.release[0])
	waitForText("reply-2")
	addTextTurn(t, input, "turn-3")
	chat.waitCancelled(t, 1)
	close(chat.release[1])
	waitForText("reply-3")
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	for result := range results {
		if errors.Is(result.err, io.EOF) {
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
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			for turn := range turns {
				if text == genx.Text(fmt.Sprintf("reply-%d", turn+1)) {
					turns[turn].streamID = chunk.Ctrl.StreamID
				}
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

func TestInterruptedTranscriptReplacementKeepsSessionAlive(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		chunks func(oldStreamID, newStreamID string) []*genx.MessageChunk
	}{
		{
			name: "interrupted terminal before replacement BOS",
			chunks: func(oldStreamID, newStreamID string) []*genx.MessageChunk {
				return []*genx.MessageChunk{
					textChunk(oldStreamID, "stale partial", true, false),
					interruptedTextEnd(oldStreamID),
					genx.NewBeginOfStream("replacement-audio"),
					textChunk(newStreamID, "fresh", true, false),
					textChunk(newStreamID, "", false, true),
				}
			},
		},
		{
			name: "interrupted terminal after replacement BOS",
			chunks: func(oldStreamID, newStreamID string) []*genx.MessageChunk {
				return []*genx.MessageChunk{
					textChunk(oldStreamID, "stale partial", true, false),
					genx.NewBeginOfStream("replacement-audio"),
					interruptedTextEnd(oldStreamID),
					textChunk(newStreamID, "fresh", true, false),
					textChunk(newStreamID, "", false, true),
				}
			},
		},
		{
			name: "stale terminal after replacement transcript starts",
			chunks: func(oldStreamID, newStreamID string) []*genx.MessageChunk {
				return []*genx.MessageChunk{
					textChunk(oldStreamID, "stale partial", true, false),
					genx.NewBeginOfStream("replacement-audio"),
					textChunk(newStreamID, "fresh", true, false),
					interruptedTextEnd(oldStreamID),
					textChunk(newStreamID, "", false, true),
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			transformer, err := New(t.Context(), textConfig())
			if err != nil {
				t.Fatal(err)
			}
			oldStreamID := genx.NewStreamID()
			newStreamID := genx.NewStreamID()
			output, err := transformer.Transform(t.Context(), inputFromChunks(t, test.chunks(oldStreamID, newStreamID)...))
			if err != nil {
				t.Fatal(err)
			}
			if got := joinedText(drain(t, output)); got != "fresh" {
				t.Fatalf("output text = %q, want fresh", got)
			}
		})
	}
}

func TestRepeatedInterruptedTranscriptsDiscardPartialInput(t *testing.T) {
	t.Parallel()
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatal(err)
	}
	firstStreamID := genx.NewStreamID()
	secondStreamID := genx.NewStreamID()
	freshStreamID := genx.NewStreamID()
	output, err := transformer.Transform(t.Context(), inputFromChunks(t,
		textChunk(firstStreamID, "stale one", true, false),
		&genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "image/png", Data: []byte{1}},
			Ctrl: &genx.StreamCtrl{StreamID: firstStreamID},
		},
		interruptedTextEnd(firstStreamID),
		textChunk(secondStreamID, "stale two", true, false),
		interruptedTextEnd(secondStreamID),
		textChunk(freshStreamID, "fresh", true, false),
		textChunk(freshStreamID, "", false, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := joinedText(drain(t, output)); got != "fresh" {
		t.Fatalf("output text = %q, want fresh", got)
	}
	transformer.history.mu.Lock()
	history := append([]*schema.Message(nil), transformer.history.live...)
	transformer.history.mu.Unlock()
	if len(history) != 2 || history[0].Role != schema.User || history[0].Content != "fresh" ||
		history[1].Role != schema.Assistant || history[1].Content != "fresh" {
		t.Fatalf("History = %#v, want only fresh user and assistant messages", history)
	}
}

func textChunk(streamID, text string, begin, end bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: begin, EndOfStream: end},
	}
}

func interruptedTextEnd(streamID string) *genx.MessageChunk {
	chunk := textChunk(streamID, "", false, true)
	chunk.Ctrl.Error = "interrupted"
	return chunk
}

type orderedInterruptChatModel struct {
	mu        sync.Mutex
	calls     int
	started   []chan struct{}
	cancelled []chan struct{}
	release   []chan struct{}
}

func newOrderedInterruptChatModel(turns int) *orderedInterruptChatModel {
	chat := &orderedInterruptChatModel{
		started:   make([]chan struct{}, turns),
		cancelled: make([]chan struct{}, turns-1),
		release:   make([]chan struct{}, turns-1),
	}
	for index := range chat.started {
		chat.started[index] = make(chan struct{})
	}
	for index := range chat.cancelled {
		chat.cancelled[index] = make(chan struct{})
		chat.release[index] = make(chan struct{})
	}
	return chat
}

func (chat *orderedInterruptChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	reader, err := chat.Stream(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.Recv()
}

func (chat *orderedInterruptChatModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	chat.mu.Lock()
	turn := chat.calls
	chat.calls++
	chat.mu.Unlock()
	if turn >= len(chat.started) {
		return nil, fmt.Errorf("unexpected chat turn %d", turn+1)
	}
	reader, writer := schema.Pipe[*schema.Message](0)
	go func() {
		defer writer.Close()
		if writer.Send(schema.AssistantMessage(fmt.Sprintf("reply-%d", turn+1), nil), nil) {
			return
		}
		close(chat.started[turn])
		if turn >= len(chat.cancelled) {
			return
		}
		<-ctx.Done()
		close(chat.cancelled[turn])
		<-chat.release[turn]
	}()
	return reader, nil
}

func (chat *orderedInterruptChatModel) waitStarted(t *testing.T, turn int) {
	t.Helper()
	select {
	case <-chat.started[turn]:
	case <-time.After(5 * time.Second):
		t.Fatalf("turn %d did not start", turn+1)
	}
}

func (chat *orderedInterruptChatModel) waitCancelled(t *testing.T, turn int) {
	t.Helper()
	select {
	case <-chat.cancelled[turn]:
	case <-time.After(5 * time.Second):
		t.Fatalf("turn %d was not cancelled", turn+1)
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
