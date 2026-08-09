package doubaorealtime

import (
	"io"
	"reflect"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoRealtimeSpokenResponseSelectsTTSOnce(t *testing.T) {
	var response doubaoRealtimeSpokenResponse
	if got := response.chat("duplicate chat"); len(got.text) != 0 {
		t.Fatalf("chat transition = %#v, want buffered", got)
	}
	first := response.ttsStarted("first sentence ")
	if !first.openText || !first.openAudio || !reflect.DeepEqual(first.text, []string{"first sentence "}) {
		t.Fatalf("first TTS transition = %#v", first)
	}
	second := response.ttsStarted("second sentence")
	if second.openText || second.openAudio || !reflect.DeepEqual(second.text, []string{"second sentence"}) {
		t.Fatalf("second TTS transition = %#v", second)
	}
	if got := response.finishChat(); got.closeText || len(got.text) != 0 {
		t.Fatalf("ChatEnded transition = %#v, want no output before TTS terminal", got)
	}
	finished := response.finishTTS()
	if !finished.closeAudio || !finished.closeText || len(finished.text) != 0 {
		t.Fatalf("TTSFinished transition = %#v", finished)
	}
	if got := response.finishTTS(); got.closeAudio || got.closeText || len(got.text) != 0 {
		t.Fatalf("duplicate TTSFinished transition = %#v, want idempotent", got)
	}
	if got := response.ttsStarted("late sentence"); got.openAudio || got.closeText || len(got.text) != 0 {
		t.Fatalf("late TTSStarted transition = %#v, want ignored after terminal", got)
	}
}

func TestDoubaoRealtimeSpokenResponseFallsBackToChatAfterBothTerminals(t *testing.T) {
	var response doubaoRealtimeSpokenResponse
	response.chat("first ")
	response.chat("second")
	if got := response.finishChat(); got.closeText || len(got.text) != 0 {
		t.Fatalf("ChatEnded transition = %#v, want buffered fallback", got)
	}
	if got := response.audioStarted(); !got.openAudio {
		t.Fatalf("audio transition = %#v, want one BOS", got)
	}
	finished := response.finishTTS()
	if !finished.openText || !reflect.DeepEqual(finished.text, []string{"first ", "second"}) ||
		!finished.closeText || !finished.closeAudio {
		t.Fatalf("fallback transition = %#v", finished)
	}
	if got := response.finishChat(); got.closeText || len(got.text) != 0 {
		t.Fatalf("duplicate ChatEnded transition = %#v, want idempotent", got)
	}
}

func TestDoubaoRealtimeHistoryRouteUsesCanonicalMIMEForCompleteLifecycle(t *testing.T) {
	routes := newDoubaoRealtimeHistoryRoutes()
	output := newBufferStream(4)
	source := &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: " Audio/L16; rate=16000; channels=1 ", Data: []byte("audio")},
		Ctrl: &genx.StreamCtrl{StreamID: "source", BeginOfStream: true, EndOfStream: true, Error: "source error"},
	}
	if err := routes.push(output, source, "history"); err != nil {
		t.Fatalf("push() error = %v", err)
	}
	if err := routes.close(output, "history", "audio/l16; channels=1; rate=16000"); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	chunks := make([]*genx.MessageChunk, 3)
	for index := range chunks {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("Next(%d) error = %v", index, err)
		}
		chunks[index] = chunk
	}
	for index, chunk := range chunks {
		mimeType, ok := chunk.MIMEType()
		if !ok || mimeType != "audio/l16; channels=1; rate=16000" {
			t.Fatalf("chunk %d MIME = %q, %t", index, mimeType, ok)
		}
	}
	if !chunks[0].IsBeginOfStream() || chunks[0].IsEndOfStream() ||
		chunks[1].IsBeginOfStream() || chunks[1].IsEndOfStream() || chunks[1].Ctrl.Error != "" ||
		chunks[2].IsBeginOfStream() || !chunks[2].IsEndOfStream() {
		t.Fatalf("history lifecycle = %#v, want BOS/data/EOS", chunks)
	}
	if len(routes.open) != 0 {
		t.Fatalf("open history routes = %#v, want none", routes.open)
	}
	if !source.IsBeginOfStream() || !source.IsEndOfStream() || source.Ctrl.Error != "source error" {
		t.Fatalf("source chunk was mutated = %#v", source.Ctrl)
	}
}

type sliceRealtimeStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *sliceRealtimeStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *sliceRealtimeStream) Close() error               { return nil }
func (s *sliceRealtimeStream) CloseWithError(error) error { return nil }

type gatedRealtimeStream struct {
	first            []*genx.MessageChunk
	rest             []*genx.MessageChunk
	gate             <-chan struct{}
	firstDrained     chan<- struct{}
	firstDrainedOnce sync.Once
	index            int
}

func (s *gatedRealtimeStream) Next() (*genx.MessageChunk, error) {
	if s.index < len(s.first) {
		chunk := s.first[s.index]
		s.index++
		if s.index == len(s.first) && s.firstDrained != nil {
			s.firstDrainedOnce.Do(func() { close(s.firstDrained) })
		}
		return chunk, nil
	}
	if s.gate != nil {
		<-s.gate
		s.gate = nil
	}
	restIndex := s.index - len(s.first)
	if restIndex >= len(s.rest) {
		return nil, io.EOF
	}
	chunk := s.rest[restIndex]
	s.index++
	return chunk, nil
}

func (s *gatedRealtimeStream) Close() error               { return nil }
func (s *gatedRealtimeStream) CloseWithError(error) error { return nil }

type blockingRealtimeStream struct {
	started     chan struct{}
	done        chan struct{}
	startedOnce sync.Once
	doneOnce    sync.Once
	errMu       sync.Mutex
	err         error
}

func newBlockingRealtimeStream() *blockingRealtimeStream {
	return &blockingRealtimeStream{started: make(chan struct{}), done: make(chan struct{})}
}

func (s *blockingRealtimeStream) Next() (*genx.MessageChunk, error) {
	s.startedOnce.Do(func() { close(s.started) })
	<-s.done
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return nil, s.err
}

func (s *blockingRealtimeStream) Close() error {
	s.close(nil)
	return nil
}

func (s *blockingRealtimeStream) CloseWithError(err error) error {
	s.close(err)
	return nil
}

func (s *blockingRealtimeStream) close(err error) {
	s.doneOnce.Do(func() {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
		close(s.done)
	})
}

func TestRealtimeAssistantLifecycleInterruptsCurrentEpoch(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	epoch := assistant.markStarted("turn-1")
	if !assistant.canPush(epoch) {
		t.Fatal("started response rejected current epoch")
	}
	streamID, interrupted := assistant.interrupt("fallback", false)
	if !interrupted || streamID != "turn-1" {
		t.Fatalf("interrupt() = (%q, %v), want (turn-1, true)", streamID, interrupted)
	}
	if assistant.canPush(epoch) {
		t.Fatal("interrupted response still accepts old epoch")
	}
	assistant.setAccept(true)
	next := assistant.nextEpoch()
	assistant.markPending("turn-2", next)
	if !assistant.canPush(next) {
		t.Fatal("next response rejected current epoch")
	}
	assistant.markDone(next)
	if _, interrupted := assistant.interrupt("turn-2", false); interrupted {
		t.Fatal("completed response remained active")
	}
}

func TestRealtimeAssistantLifecyclePushHoldsInterruptLock(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	epoch := assistant.markStarted("turn-1")
	lockHeld := false
	accepted, err := assistant.pushIfCurrent(epoch, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true},
	}, func() error {
		if assistant.mu.TryLock() {
			assistant.mu.Unlock()
			t.Fatal("push callback ran without holding the interruption lock")
		}
		lockHeld = true
		return nil
	})
	if err != nil || !accepted || !lockHeld {
		t.Fatalf("current push = accepted %t lock held %t error %v", accepted, lockHeld, err)
	}

	interruption := assistant.interruptRoutes("turn-2", false)
	if !interruption.interrupted || !interruption.audioStarted {
		t.Fatalf("interruptRoutes() = %#v, want started audio route", interruption)
	}

	called := false
	accepted, err = assistant.pushIfCurrent(epoch, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("late")},
	}, func() error {
		called = true
		return nil
	})
	if err != nil || accepted || called {
		t.Fatalf("stale push = accepted %t called %t error %v", accepted, called, err)
	}
}

func TestRealtimeAssistantLifecycleIgnoresStaleStreamCompletion(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	assistant.markStarted("turn-1")
	assistant.markStarted("turn-2")
	assistant.markDoneStream("turn-1")
	streamID, interrupted := assistant.interrupt("fallback", false)
	if !interrupted || streamID != "turn-2" {
		t.Fatalf("interrupt() after stale completion = (%q, %v), want (turn-2, true)", streamID, interrupted)
	}
}

func TestBufferStreamDefersRealtimeCompletionUntilFinalObservation(t *testing.T) {
	assistant := newRealtimeAssistantLifecycle()
	assistant.markStarted("turn-1")
	output := newBufferStream(2)
	defer output.Close()
	output.setOutputObserver(func(chunk *genx.MessageChunk) {
		observeRealtimeAssistantOutput(assistant, "assistant", chunk)
	})
	output.DeferOutputObservation()
	textEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true},
	}
	audioEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Label: "assistant", EndOfStream: true},
	}
	if err := output.Push(textEOS); err != nil {
		t.Fatalf("Push(text EOS) error = %v", err)
	}
	if err := output.Push(audioEOS); err != nil {
		t.Fatalf("Push(audio EOS) error = %v", err)
	}
	for range 2 {
		if _, err := output.Next(); err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	if interruption := assistant.interruptRoutes("turn-2", false); !interruption.interrupted {
		t.Fatal("buffered response became non-interruptible before final observation")
	}

	assistant.markStarted("turn-3")
	textEOS.Ctrl.StreamID = "turn-3"
	audioEOS.Ctrl.StreamID = "turn-3"
	output.ObserveOutput(textEOS)
	output.ObserveOutput(audioEOS)
	if interruption := assistant.interruptRoutes("turn-4", false); interruption.interrupted {
		t.Fatal("fully observed response remained interruptible")
	}
}
