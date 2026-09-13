package dashscoperealtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type connectTestOpener struct {
	calls int
	open  func(int) (dashScopeRealtimeSession, error)
}

func (o *connectTestOpener) Connect(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
	o.calls++
	return o.open(o.calls)
}

type connectTestSession struct {
	fakeDashScopeSession
	setupErr    error
	providerErr *dashscope.EventError
	updateErr   error
	closed      int
}

func (s *connectTestSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		if s.setupErr != nil {
			yield(nil, s.setupErr)
			return
		}
		if s.providerErr != nil {
			yield(&dashscope.RealtimeEvent{Type: dashscope.EventTypeError, Error: s.providerErr}, nil)
			return
		}
		yield(&dashscope.RealtimeEvent{Type: dashscope.EventTypeSessionCreated}, nil)
	}
}
func (s *connectTestSession) UpdateSession(*dashscope.SessionConfig) error { return s.updateErr }
func (s *connectTestSession) Close() error                                 { s.closed++; return nil }

func TestConnectTemporaryFailures(t *testing.T) {
	for _, phase := range []string{"connect", "created", "update", "provider_error"} {
		t.Run(phase, func(t *testing.T) {
			failed := &connectTestSession{}
			success := &connectTestSession{}
			opener := &connectTestOpener{open: func(n int) (dashScopeRealtimeSession, error) {
				if n > 1 {
					return success, nil
				}
				switch phase {
				case "connect":
					return nil, &dashscope.Error{Code: "ServiceBusy", HTTPStatus: 503}
				case "created":
					failed.setupErr = io.EOF
				case "provider_error":
					failed.providerErr = &dashscope.EventError{Code: "ServiceBusy", Message: "busy"}
				case "update":
					failed.updateErr = syscall.EPIPE
				}
				return failed, nil
			}}
			var waits []time.Duration
			transformer := &Transformer{realtime: opener, retryWait: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
			session, err := transformer.connect(t.Context(), &dashscope.SessionConfig{})
			if err != nil || session != success || opener.calls != 2 {
				t.Fatalf("session=%v err=%v calls=%d", session, err, opener.calls)
			}
			if !reflect.DeepEqual(waits, []time.Duration{100 * time.Millisecond}) {
				t.Fatalf("waits=%v", waits)
			}
			if phase != "connect" && failed.closed != 1 {
				t.Fatalf("failed closes=%d", failed.closed)
			}
			if success.closed != 0 {
				t.Fatal("closed successful session")
			}
		})
	}
}

func TestConnectRetryBoundAndCancellation(t *testing.T) {
	busy := &dashscope.Error{HTTPStatus: 503}
	opener := &connectTestOpener{open: func(int) (dashScopeRealtimeSession, error) { return nil, busy }}
	var waits []time.Duration
	transformer := &Transformer{realtime: opener, retryWait: func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }}
	_, err := transformer.connect(t.Context(), nil)
	if !errors.Is(err, busy) || opener.calls != 6 {
		t.Fatalf("err=%v calls=%d", err, opener.calls)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1600 * time.Millisecond}
	if !reflect.DeepEqual(waits, want) {
		t.Fatalf("waits=%v", waits)
	}
	ctx, cancel := context.WithCancel(t.Context())
	transformer.retryWait = func(ctx context.Context, d time.Duration) error {
		cancel()
		return waitDashScopeConnect(ctx, time.Hour)
	}
	opener.calls = 0
	_, err = transformer.connect(ctx, nil)
	if !errors.Is(err, context.Canceled) || opener.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, opener.calls)
	}
	_, err = transformer.connect(ctx, nil)
	if !errors.Is(err, context.Canceled) || opener.calls != 1 {
		t.Fatalf("pre-canceled err=%v calls=%d", err, opener.calls)
	}
}

func TestConnectErrorClassification(t *testing.T) {
	for _, test := range []struct {
		err   error
		retry bool
	}{
		{&dashscope.Error{HTTPStatus: 503}, true},
		{&dashscope.Error{HTTPStatus: 429}, true},
		{&dashscope.Error{Code: "ServiceBusy"}, true},
		{&dashscope.Error{Code: "InvalidApiKey", HTTPStatus: 503}, false},
		{&dashscope.Error{Code: "InvalidParameter", HTTPStatus: 503}, false},
		{&dashscope.Error{Code: "ServiceBusy", HTTPStatus: 403}, false},
		{&dashscope.Error{HTTPStatus: 400}, false},
		{&dashscope.Error{HTTPStatus: 404}, false},
		{&dashscope.Error{HTTPStatus: 500}, false},
		{io.EOF, true}, {io.ErrUnexpectedEOF, true}, {syscall.EPIPE, true}, {syscall.ECONNRESET, true},
		{context.DeadlineExceeded, true}, {context.Canceled, false}, {errors.New("invalid configuration"), false},
	} {
		t.Run(test.err.Error(), func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", test.err)
			if got := dashScopeConnectRecoverable(err); got != test.retry {
				t.Fatalf("retry=%v want=%v", got, test.retry)
			}
			if !test.retry {
				opener := &connectTestOpener{open: func(int) (dashScopeRealtimeSession, error) { return nil, err }}
				transformer := &Transformer{realtime: opener, retryWait: func(context.Context, time.Duration) error { t.Fatal("unexpected retry"); return nil }}
				if _, got := transformer.connect(t.Context(), nil); !errors.Is(got, test.err) || opener.calls != 1 {
					t.Fatalf("err=%v calls=%d", got, opener.calls)
				}
			}
		})
	}
}

// No recovery is attempted once setup returns an output stream, even for a
// transient error arriving immediately after the first response chunk.
func TestConnectDoesNotReplayEstablishedOutput(t *testing.T) {
	observed := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	session := &establishedConnectSession{ctx: ctx, observed: observed}
	var calls atomic.Int32
	transformer := &Transformer{realtime: connectOpenerFunc(func(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
		calls.Add(1)
		return session, nil
	}), retryWait: func(context.Context, time.Duration) error { t.Error("replayed established session"); return nil }}
	output, err := transformer.Transform(ctx, emptyDashScopeStream{})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	var text string
	for {
		chunk, err := output.Next()
		if err != nil {
			if !errors.Is(err, syscall.ECONNRESET) {
				t.Fatalf("terminal=%v", err)
			}
			break
		}
		if part, ok := chunk.Part.(genx.Text); ok {
			text += string(part)
			if text == "first" {
				close(observed)
			}
		}
	}
	if text != "first" || calls.Load() != 1 {
		t.Fatalf("text=%q connections=%d", text, calls.Load())
	}
}

type connectOpenerFunc func(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error)

func (f connectOpenerFunc) Connect(ctx context.Context, c *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
	return f(ctx, c)
}

type cancelConnectSession struct {
	fakeDashScopeSession
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (s *cancelConnectSession) Close() error { s.once.Do(func() { close(s.closed) }); return nil }
func (s *cancelConnectSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		close(s.entered)
		<-s.closed
		yield(nil, io.EOF)
	}
}
func TestConnectCancelWhileWaitingForSessionCreated(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	session := &cancelConnectSession{entered: make(chan struct{}), closed: make(chan struct{})}
	transformer := &Transformer{realtime: connectOpenerFunc(func(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
		return session, nil
	})}
	done := make(chan error, 1)
	go func() { _, err := transformer.connect(ctx, nil); done <- err }()
	select {
	case <-session.entered:
	case <-time.After(time.Second):
		t.Fatal("did not enter handshake")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close handshake")
	}
}

type establishedConnectSession struct {
	fakeDashScopeSession
	ctx      context.Context
	observed <-chan struct{}
	calls    int
}

func (s *establishedConnectSession) Events() iter.Seq2[*dashscope.RealtimeEvent, error] {
	s.calls++
	call := s.calls
	return func(yield func(*dashscope.RealtimeEvent, error) bool) {
		if call == 1 {
			yield(&dashscope.RealtimeEvent{Type: dashscope.EventTypeSessionCreated}, nil)
			return
		}
		if !yield(&dashscope.RealtimeEvent{Type: dashscope.EventTypeResponseTextDelta, Delta: "first"}, nil) {
			return
		}
		select {
		case <-s.observed:
			yield(nil, syscall.ECONNRESET)
		case <-s.ctx.Done():
			yield(nil, s.ctx.Err())
		}
	}
}
