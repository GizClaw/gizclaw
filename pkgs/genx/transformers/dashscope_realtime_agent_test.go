package transformers

import (
	"context"
	"errors"
	"io"
	"iter"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDashScopeRealtimeEnforcesToolLimitAcrossModelRounds(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "round-1"},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, ResponseID: "round-1", CallID: "call-1", Name: "first", Arguments: `{}`},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "round-1"},
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "round-2"},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, ResponseID: "round-2", CallID: "call-2", Name: "second", Arguments: `{}`},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "round-2"},
		},
		finished: eventsFinished,
	}
	input := &dashScopeConcurrentInput{firstRead: make(chan struct{}), eventsFinished: eventsFinished}
	invocations := 0
	output := newBufferStream(8)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{
			MaxToolCalls: 1,
			FunctionCallHandler: func(_ context.Context, calls []DashScopeRealtimeFunctionCall) ([]DashScopeRealtimeFunctionCallOutput, error) {
				invocations++
				return []DashScopeRealtimeFunctionCallOutput{{CallID: calls[0].CallID, Output: `null`}}, nil
			},
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish")
	}
	for {
		_, err := output.Next()
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "tool call limit exceeded") {
			t.Fatalf("output error = %v", err)
		}
		if errors.Is(err, io.EOF) {
			t.Fatal("tool limit failure was not observable")
		}
		break
	}
	if invocations != 1 {
		t.Fatalf("handler invocations = %d, want 1", invocations)
	}
}

func TestDashScopeRealtimeProviderEndClosesBlockedInput(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{finished: eventsFinished}
	input := newBlockingRealtimeStream()
	output := newBufferStream(1)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop remained blocked in input.Next after provider events ended")
	}
	select {
	case <-input.done:
	default:
		t.Fatal("blocked input was not closed")
	}
}

func TestDashScopeRealtimeAudioEOSIsNotCommittedAgainAtEOF(t *testing.T) {
	eofRead := make(chan struct{})
	responseCreated := make(chan struct{})
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events:           []*dashscope.RealtimeEvent{{Type: dashscope.EventTypeResponseDone, ResponseID: "response-1"}},
		waitBeforeEvents: eofRead,
		finished:         eventsFinished,
		canceled:         make(chan struct{}),
		created:          responseCreated,
	}
	input := &dashScopeEOSInput{responseCreated: responseCreated, eofRead: eofRead}
	output := newBufferStream(4)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish after the committed response completed")
	}
	if got := session.createCount(); got != 1 {
		t.Fatalf("CreateResponse calls = %d, want 1", got)
	}
}

func TestDashScopeRealtimeTranscriptEOSPreservesLabel(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{{
			Type:       dashscope.EventTypeInputAudioTranscriptionCompleted,
			Transcript: "hello",
		}},
		finished: eventsFinished,
	}
	input := &dashScopeConcurrentInput{firstRead: make(chan struct{}), eventsFinished: eventsFinished}
	output := newBufferStream(2)
	(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})

	textChunk, err := output.Next()
	if err != nil {
		t.Fatal(err)
	}
	eosChunk, err := output.Next()
	if err != nil {
		t.Fatal(err)
	}
	if textChunk.Ctrl.StreamID != eosChunk.Ctrl.StreamID {
		t.Fatalf("transcript StreamIDs = %q and %q", textChunk.Ctrl.StreamID, eosChunk.Ctrl.StreamID)
	}
	if textChunk.Ctrl.Label != dashScopeRealtimeTranscriptLabel || eosChunk.Ctrl.Label != dashScopeRealtimeTranscriptLabel {
		t.Fatalf("transcript labels = %q and %q, want %q", textChunk.Ctrl.Label, eosChunk.Ctrl.Label, dashScopeRealtimeTranscriptLabel)
	}
	if !eosChunk.IsEndOfStream() {
		t.Fatalf("transcript EOS = %#v", eosChunk)
	}
}

func TestDashScopeRealtimeStartsResponseWithoutCreatedEvent(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "provider-response", Delta: "hello"},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "provider-response"},
		},
		finished: eventsFinished,
	}
	input := &dashScopeConcurrentInput{firstRead: make(chan struct{}), eventsFinished: eventsFinished}
	output := newBufferStream(4)
	(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})

	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want text and text EOS chunks", chunks)
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || chunks[0].IsBeginOfStream() {
		t.Fatalf("response text = %#v", chunks[0])
	}
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("response chunk = %#v, want StreamID %q", chunk, streamID)
		}
	}
}

func TestDashScopeRealtimeExecutesFunctionCallsInOutputIndexOrder(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "provider-response"},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, CallID: "call-2", Name: "second", Arguments: `{}`, OutputIndex: 1},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, CallID: "call-1", Name: "first", Arguments: `{}`, OutputIndex: 0},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "provider-response"},
		},
		finished: eventsFinished,
	}
	inputRead := make(chan struct{})
	input := &dashScopeConcurrentInput{firstRead: inputRead, eventsFinished: eventsFinished}
	var gotCalls []DashScopeRealtimeFunctionCall
	runtime := DashScopeRealtimeCtxOptions{FunctionCallHandler: func(_ context.Context, calls []DashScopeRealtimeFunctionCall) ([]DashScopeRealtimeFunctionCallOutput, error) {
		select {
		case <-inputRead:
		case <-time.After(time.Second):
			return nil, context.DeadlineExceeded
		}
		gotCalls = append(gotCalls, calls...)
		return []DashScopeRealtimeFunctionCallOutput{
			{CallID: calls[0].CallID, Output: `{"index":0}`},
			{CallID: calls[1].CallID, Output: `{"index":1}`},
		}, nil
	}}
	output := newBufferStream(4)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, runtime)
		close(done)
	}()

	select {
	case <-inputRead:
	case <-time.After(time.Second):
		t.Fatal("input was not read while the Tool handler was active")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish")
	}
	if len(gotCalls) != 2 {
		t.Fatalf("handler calls = %#v", gotCalls)
	}
	if got := []string{gotCalls[0].CallID, gotCalls[1].CallID}; !slices.Equal(got, []string{"call-1", "call-2"}) {
		t.Fatalf("handler call order = %v", got)
	}
	if got := session.submittedIDs(); !slices.Equal(got, []string{"call-1", "call-2"}) {
		t.Fatalf("submitted result order = %v", got)
	}
	if session.createCount() != 1 {
		t.Fatalf("CreateResponse calls = %d, want 1", session.createCount())
	}
	if chunk, err := output.Next(); chunk != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("tool-only round exposed output chunk=%#v error=%v", chunk, err)
	}
}

func TestDashScopeRealtimeKeepsOneExternalStreamAcrossToolRounds(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "round-1"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "round-1", Delta: "before"},
			{Type: dashscope.EventTypeResponseTextDone, ResponseID: "round-1"},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, ResponseID: "round-1", CallID: "call-1", Name: "tool", Arguments: `{}`},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "round-1"},
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "round-2"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "round-2", Delta: "after"},
			{Type: dashscope.EventTypeResponseTextDone, ResponseID: "round-2"},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "round-2"},
		},
		finished: eventsFinished,
	}
	input := &dashScopeConcurrentInput{firstRead: make(chan struct{}), eventsFinished: eventsFinished}
	output := newBufferStream(2)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{
			FunctionCallHandler: func(_ context.Context, calls []DashScopeRealtimeFunctionCall) ([]DashScopeRealtimeFunctionCallOutput, error) {
				return []DashScopeRealtimeFunctionCallOutput{{CallID: calls[0].CallID, Output: `null`}}, nil
			},
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish")
	}

	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want two deltas and one final text EOS", chunks)
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" {
		t.Fatal("external StreamID is empty")
	}
	begins := 0
	seenAfter := false
	for _, chunk := range chunks {
		if chunk.Ctrl.StreamID != streamID {
			t.Fatalf("chunk StreamID = %q, want %q", chunk.Ctrl.StreamID, streamID)
		}
		if chunk.IsBeginOfStream() {
			begins++
		}
		if text, ok := chunk.Part.(genx.Text); ok && text == "after" {
			seenAfter = true
		}
		if chunk.IsEndOfStream() && !seenAfter {
			t.Fatalf("premature EOS before post-Tool output: %#v", chunk)
		}
	}
	if begins != 0 || !seenAfter {
		t.Fatalf("BOS count = %d, saw post-Tool output = %t, chunks=%#v", begins, seenAfter, chunks)
	}
}

func TestDashScopeRealtimeInterruptDiscardsBufferedOutputAndEmitsEOS(t *testing.T) {
	textSent := make(chan struct{})
	releaseEvents := make(chan struct{})
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "provider-response"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "provider-response", Delta: "stale"},
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "late-response"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "late-response", Delta: "must-not-leak"},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "late-response"},
		},
		pauseAfter: 1,
		paused:     textSent,
		release:    releaseEvents,
		finished:   eventsFinished,
		canceled:   make(chan struct{}),
	}
	input := &dashScopeInterruptInput{textSent: textSent, eventsFinished: eventsFinished}
	output := newBufferStream(8)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})
		close(done)
	}()
	select {
	case <-session.canceled:
	case <-time.After(time.Second):
		t.Fatal("provider response was not canceled")
	}
	close(releaseEvents)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish")
	}

	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 2 {
		t.Fatalf("interrupt chunks = %#v, want text/audio EOS only", chunks)
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || streamID == "turn-2" {
		t.Fatalf("assistant StreamID = %q", streamID)
	}
	for _, chunk := range chunks {
		if !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" || chunk.Ctrl.StreamID != streamID {
			t.Fatalf("interrupt chunk = %#v", chunk)
		}
	}
}

func TestDashScopeRealtimeServerVADStartsFreshResponseAfterInterrupt(t *testing.T) {
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "old-response"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "old-response", Delta: "stale"},
			{Type: dashscope.EventTypeInputSpeechStarted},
			{Type: dashscope.EventTypeInputSpeechStopped},
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "new-response"},
			{Type: dashscope.EventTypeResponseTextDelta, ResponseID: "new-response", Delta: "fresh"},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "new-response"},
		},
		finished: eventsFinished,
		canceled: make(chan struct{}),
	}
	input := &dashScopeConcurrentInput{firstRead: make(chan struct{}), eventsFinished: eventsFinished}
	output := newBufferStream(8)
	(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{})

	var chunks []*genx.MessageChunk
	for {
		chunk, err := output.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 4 {
		t.Fatalf("chunks = %#v, want two interrupted route EOS chunks, fresh text, and fresh EOS", chunks)
	}
	for _, chunk := range chunks[:2] {
		if chunk.Ctrl == nil || chunk.Ctrl.Error != "interrupted" || !chunk.IsEndOfStream() || chunk.Ctrl.StreamID != chunks[0].Ctrl.StreamID {
			t.Fatalf("interrupted chunk = %#v", chunk)
		}
	}
	if text, ok := chunks[2].Part.(genx.Text); !ok || text != "fresh" {
		t.Fatalf("fresh response chunk = %#v", chunks[2])
	}
	if chunks[2].Ctrl.StreamID == chunks[0].Ctrl.StreamID {
		t.Fatalf("fresh response reused interrupted StreamID %q", chunks[2].Ctrl.StreamID)
	}
	if chunks[3].Ctrl == nil || chunks[3].Ctrl.StreamID != chunks[2].Ctrl.StreamID || !chunks[3].IsEndOfStream() || chunks[3].Ctrl.Error != "" {
		t.Fatalf("fresh response EOS = %#v", chunks[3])
	}
}

func TestDashScopeRealtimeInterruptDropsSuccessfulLateToolOutput(t *testing.T) {
	handlerStarted := make(chan struct{})
	eventsFinished := make(chan struct{})
	session := &fakeDashScopeRealtimeSession{
		events: []*dashscope.RealtimeEvent{
			{Type: dashscope.EventTypeResponseCreated, ResponseID: "provider-response"},
			{Type: dashscope.EventTypeResponseFunctionCallArgumentsDone, ResponseID: "provider-response", CallID: "call-1", Name: "tool", Arguments: `{}`},
			{Type: dashscope.EventTypeResponseDone, ResponseID: "provider-response"},
		},
		finished: eventsFinished,
		canceled: make(chan struct{}),
	}
	input := &dashScopeInterruptInput{textSent: handlerStarted, eventsFinished: eventsFinished}
	output := newBufferStream(4)
	done := make(chan struct{})
	go func() {
		(&DashScopeRealtime{}).processLoop(t.Context(), input, output, session, DashScopeRealtimeCtxOptions{
			FunctionCallHandler: func(ctx context.Context, calls []DashScopeRealtimeFunctionCall) ([]DashScopeRealtimeFunctionCallOutput, error) {
				close(handlerStarted)
				<-ctx.Done()
				return []DashScopeRealtimeFunctionCallOutput{{CallID: calls[0].CallID, Output: `{"late":true}`}}, nil
			},
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("processLoop did not finish")
	}
	if got := session.submittedIDs(); len(got) != 0 {
		t.Fatalf("submitted stale Tool outputs = %v", got)
	}
	if got := session.createCount(); got != 0 {
		t.Fatalf("CreateResponse calls after interruption = %d, want 0", got)
	}
}

type fakeDashScopeRealtimeSession struct {
	events           []*dashscope.RealtimeEvent
	waitBeforeEvents <-chan struct{}
	pauseAfter       int
	paused           chan struct{}
	release          <-chan struct{}
	finished         chan struct{}
	canceled         chan struct{}
	created          chan struct{}

	mu         sync.Mutex
	submitted  []string
	creates    int
	cancelOnce sync.Once
	createOnce sync.Once
}

func (s *fakeDashScopeRealtimeSession) UpdateSession(*dashscope.SessionConfig) error { return nil }
func (s *fakeDashScopeRealtimeSession) AppendAudio([]byte) error                     { return nil }
func (s *fakeDashScopeRealtimeSession) CommitInput() error                           { return nil }
func (s *fakeDashScopeRealtimeSession) CreateResponse(*dashscope.ResponseCreateOptions) error {
	s.mu.Lock()
	s.creates++
	s.mu.Unlock()
	if s.created != nil {
		s.createOnce.Do(func() { close(s.created) })
	}
	return nil
}
func (s *fakeDashScopeRealtimeSession) SubmitFunctionCallOutput(callID, _ string) error {
	s.mu.Lock()
	s.submitted = append(s.submitted, callID)
	s.mu.Unlock()
	return nil
}
func (s *fakeDashScopeRealtimeSession) CancelResponse() error {
	s.cancelOnce.Do(func() {
		close(s.canceled)
	})
	return nil
}
func (s *fakeDashScopeRealtimeSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		defer close(s.finished)
		if s.waitBeforeEvents != nil {
			<-s.waitBeforeEvents
		}
		for i, event := range s.events {
			if !yield(event, nil) {
				return
			}
			if s.paused != nil && i == s.pauseAfter {
				close(s.paused)
				<-s.release
			}
		}
	}
}
func (s *fakeDashScopeRealtimeSession) Close() error { return nil }
func (s *fakeDashScopeRealtimeSession) submittedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.submitted...)
}
func (s *fakeDashScopeRealtimeSession) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

type dashScopeConcurrentInput struct {
	firstRead      chan struct{}
	eventsFinished <-chan struct{}
	once           sync.Once
}

type dashScopeEOSInput struct {
	responseCreated <-chan struct{}
	eofRead         chan struct{}
	index           int
}

func (s *dashScopeEOSInput) Next() (*genx.MessageChunk, error) {
	if s.index == 0 {
		s.index++
		return &genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{0, 0}},
			Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true, EndOfStream: true},
		}, nil
	}
	<-s.responseCreated
	close(s.eofRead)
	return nil, io.EOF
}

func (*dashScopeEOSInput) Close() error               { return nil }
func (*dashScopeEOSInput) CloseWithError(error) error { return nil }

func (s *dashScopeConcurrentInput) Next() (*genx.MessageChunk, error) {
	s.once.Do(func() { close(s.firstRead) })
	<-s.eventsFinished
	return nil, nil
}
func (*dashScopeConcurrentInput) Close() error               { return nil }
func (*dashScopeConcurrentInput) CloseWithError(error) error { return nil }

type dashScopeInterruptInput struct {
	textSent       <-chan struct{}
	eventsFinished <-chan struct{}
	index          int
}

func (s *dashScopeInterruptInput) Next() (*genx.MessageChunk, error) {
	if s.index == 0 {
		<-s.textSent
		s.index++
		return &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}, nil
	}
	<-s.eventsFinished
	return nil, nil
}
func (*dashScopeInterruptInput) Close() error               { return nil }
func (*dashScopeInterruptInput) CloseWithError(error) error { return nil }
