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
	if len(chunks) != 5 {
		t.Fatalf("chunks = %#v, want one BOS, two deltas, and two final EOS", chunks)
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
	if begins != 1 || !seenAfter {
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

type fakeDashScopeRealtimeSession struct {
	events     []*dashscope.RealtimeEvent
	pauseAfter int
	paused     chan struct{}
	release    <-chan struct{}
	finished   chan struct{}
	canceled   chan struct{}

	mu         sync.Mutex
	submitted  []string
	creates    int
	cancelOnce sync.Once
}

func (s *fakeDashScopeRealtimeSession) UpdateSession(*dashscope.SessionConfig) error { return nil }
func (s *fakeDashScopeRealtimeSession) AppendAudio([]byte) error                     { return nil }
func (s *fakeDashScopeRealtimeSession) CommitInput() error                           { return nil }
func (s *fakeDashScopeRealtimeSession) CreateResponse(*dashscope.ResponseCreateOptions) error {
	s.mu.Lock()
	s.creates++
	s.mu.Unlock()
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
