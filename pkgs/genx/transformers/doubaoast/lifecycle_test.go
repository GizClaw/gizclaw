package doubaoast

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type lifecycleTestSession struct {
	sent       chan struct{}
	finish     chan struct{}
	closed     chan struct{}
	received   chan struct{}
	failure    error
	afterEOS   chan struct{}
	sentOnce   sync.Once
	finishOnce sync.Once
	closeOnce  sync.Once
}

func (s *lifecycleTestSession) SendAudio(context.Context, []byte) error {
	s.sentOnce.Do(func() { close(s.sent) })
	return nil
}
func (s *lifecycleTestSession) Finish(context.Context) error {
	s.finishOnce.Do(func() { close(s.finish) })
	return nil
}
func (s *lifecycleTestSession) Close() error { s.closeOnce.Do(func() { close(s.closed) }); return nil }
func (s *lifecycleTestSession) Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error] {
	return func(yield func(*doubaospeech.ASTTranslateEvent, error) bool) {
		if s.afterEOS != nil {
			<-s.afterEOS
		}
		if s.failure != nil {
			yield(nil, s.failure)
			close(s.received)
			return
		}
		close(s.received)
		<-s.closed
	}
}
func startLifecycleTest(t *testing.T, mode InputMode, failure error, afterEOS ...bool) (context.CancelFunc, *lifecycleTestSession, *bufferStream, genx.Stream) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	fake := &lifecycleTestSession{sent: make(chan struct{}), finish: make(chan struct{}), closed: make(chan struct{}), received: make(chan struct{}), failure: failure}
	tr := newTransformer(doubaospeech.NewClient("test"), withInputMode(mode), withRealtimePacing(false))
	tr.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return fake, nil
	}
	input := newBufferStream(4)
	var source genx.Stream = input
	if len(afterEOS) > 0 && afterEOS[0] {
		fake.afterEOS = make(chan struct{})
		source = &lifecycleTestInput{Stream: input, afterEOS: fake.afterEOS}
	}
	out, err := tr.transform(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = input.CloseWithError(context.Canceled)
		_ = fake.Close()
		_ = out.CloseWithError(context.Canceled)
	})
	_ = input.Push(genx.NewBeginOfStream("test"))
	_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "test"}})
	select {
	case <-fake.sent:
	case <-time.After(time.Second):
		t.Fatal("audio never sent")
	}
	return cancel, fake, input, out
}
func TestProviderErrorIsVisibleWithoutEOS(t *testing.T) {
	for _, mode := range []InputMode{InputModeRealtime, InputModePushToTalk} {
		t.Run(string(mode), func(t *testing.T) {
			sentinel := errors.New("provider failure")
			_, fake, _, out := startLifecycleTest(t, mode, sentinel)
			<-fake.received
			result := make(chan error, 1)
			go func() { _, err := out.Next(); result <- err }()
			select {
			case err := <-result:
				if !errors.Is(err, sentinel) {
					t.Fatalf("error=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("provider error consumed but output remained blocked before EOS")
			}
		})
	}
}
func TestCancellationClosesSilentSession(t *testing.T) {
	for _, mode := range []InputMode{InputModeRealtime, InputModePushToTalk} {
		t.Run(string(mode), func(t *testing.T) {
			cancel, fake, input, _ := startLifecycleTest(t, mode, nil)
			<-fake.received
			_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "test", EndOfStream: true}})
			select {
			case <-fake.finish:
			case <-time.After(time.Second):
				t.Fatal("Finish not called")
			}
			cancel()
			select {
			case <-fake.closed:
			case <-time.After(200 * time.Millisecond):
				t.Fatal("cancel did not close provider session after EOS")
			}
		})
	}
}

type lifecycleTestInput struct {
	genx.Stream
	count    int
	afterEOS chan struct{}
}

func (s *lifecycleTestInput) Next() (*genx.MessageChunk, error) {
	s.count++
	if s.count == 4 {
		close(s.afterEOS)
	}
	return s.Stream.Next()
}
func TestRealtimeProviderErrorAfterEOSIsVisible(t *testing.T) {
	sentinel := errors.New("provider failure after EOS")
	_, fake, input, out := startLifecycleTest(t, InputModeRealtime, sentinel, true)
	_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "test", EndOfStream: true}})
	<-fake.received
	result := make(chan error, 1)
	go func() { _, err := out.Next(); result <- err }()
	select {
	case err := <-result:
		if !errors.Is(err, sentinel) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("provider error after EOS consumed but output remained blocked")
	}
}

func TestClosingOutputClosesSilentSession(t *testing.T) {
	for _, mode := range []InputMode{InputModeRealtime, InputModePushToTalk} {
		t.Run(string(mode), func(t *testing.T) {
			_, session, _, output := startLifecycleTest(t, mode, nil)
			<-session.received
			if err := output.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-session.closed:
			case <-time.After(time.Second):
				t.Fatal("closing output did not close provider session")
			}
		})
	}
}
