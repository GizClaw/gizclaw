package doubaorealtime

import (
	"io"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

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
