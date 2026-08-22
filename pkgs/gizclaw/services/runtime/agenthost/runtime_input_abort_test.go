package agenthost

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// TestAbortingATurnByClosingTheInputSourceIsReportedAsUnexpectedOutputEnd
// reproduces the #934 failure shape through the real Audio Dock composition:
// closing the runtime input source while a turn is still producing output makes
// every stage finish cleanly, so the runtime observes a clean EOF while it is
// still active and fails the turn. Pushing a control-only interrupt BOS instead
// aborts the turn without ending the pipeline.
//
// The cascade under test is production code:
//
//	input source ends -> audiodock routeInput returns -> producers.Wait()
//	  -> agentInput.Close() -> Agent sees stream end and closes its output
//	  -> audiodock closes its own output -> agenthost.consume sees a clean EOF.
func TestAbortingATurnByClosingTheInputSourceIsReportedAsUnexpectedOutputEnd(t *testing.T) {
	t.Run("closing the source ends the pipeline (pre-fix behavior)", func(t *testing.T) {
		harness := newAbortHarness(t)
		defer harness.stop()
		harness.startTurn("tell me a story")
		harness.awaitTurnInProgress()

		// Pre-fix abortAgentInputTurn: close the whole runtime input source.
		if err := harness.source.Close(); err != nil {
			t.Fatalf("close input source: %v", err)
		}

		err := harness.awaitConsumerError(3 * time.Second)
		if err == nil {
			t.Fatal("closing the input source mid-turn did not fail the runtime; the bug did not reproduce")
		}
		if !errors.Is(err, errUnexpectedOutputEnd) {
			t.Fatalf("consumer error = %v, want %v", err, errUnexpectedOutputEnd)
		}
		t.Logf("reproduced: %v", err)
	})

	t.Run("control-only interrupt BOS keeps the pipeline open (post-fix behavior)", func(t *testing.T) {
		harness := newAbortHarness(t)
		defer harness.stop()
		harness.startTurn("tell me a story")
		harness.awaitTurnInProgress()

		// Post-fix abortAgentInputTurn: interrupt the turn, keep the source open.
		harness.push(&genx.MessageChunk{
			Role: genx.RoleUser,
			Ctrl: &genx.StreamCtrl{StreamID: genx.NewStreamID(), BeginOfStream: true},
		})

		if err := harness.awaitConsumerError(1500 * time.Millisecond); err != nil {
			t.Fatalf("interrupting the turn failed the runtime: %v", err)
		}
		// The runtime must still accept a following turn.
		harness.startTurn("continue the story")
		harness.awaitTurnInProgress()
		if err := harness.awaitConsumerError(500 * time.Millisecond); err != nil {
			t.Fatalf("runtime failed during the next turn: %v", err)
		}
	})
}

type abortHarness struct {
	t        *testing.T
	svc      *Service
	source   *PushSource
	agent    *turnScopedAgent
	errCh    chan error
	turnSeen chan struct{}
}

func newAbortHarness(t *testing.T) *abortHarness {
	t.Helper()
	ctx := context.Background()
	publicKey := testPublicKey(t)
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, publicKey, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	agent := &turnScopedAgent{turnStarted: make(chan struct{}, 8)}
	dock, err := audiodock.New(audiodock.Config{Agent: agent})
	if err != nil {
		t.Fatalf("audiodock.New() error = %v", err)
	}
	harness := &abortHarness{
		t:        t,
		source:   NewPushSource(16),
		agent:    agent,
		errCh:    make(chan error, 4),
		turnSeen: agent.turnStarted,
	}
	harness.svc = &Service{
		Host:      dockMux{dock: dock},
		PeerRun:   store,
		PublicKey: publicKey,
		Source:    harness.source,
		Consumer: StreamConsumerFunc(func(_ context.Context, stream genx.Stream) error {
			for {
				if _, err := stream.Next(); err != nil {
					if IsStreamDone(err) {
						return nil
					}
					return err
				}
			}
		}),
		OnConsumerError: func(_ context.Context, _ string, err error) {
			select {
			case harness.errCh <- err:
			default:
			}
		},
	}
	if _, err := harness.svc.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	return harness
}

func (h *abortHarness) push(chunk *genx.MessageChunk) {
	h.t.Helper()
	if err := h.source.Push(context.Background(), chunk); err != nil {
		h.t.Fatalf("push input: %v", err)
	}
}

// startTurn pushes one complete text turn, which the Agent answers slowly so
// the abort lands while the turn is still producing output.
func (h *abortHarness) startTurn(text string) {
	h.t.Helper()
	streamID := genx.NewStreamID()
	h.push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, BeginOfStream: true}})
	h.push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: streamID}})
	h.push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: true}})
}

func (h *abortHarness) awaitTurnInProgress() {
	h.t.Helper()
	select {
	case <-h.turnSeen:
	case <-time.After(3 * time.Second):
		h.t.Fatal("timed out waiting for the Agent to start a turn")
	}
}

func (h *abortHarness) awaitConsumerError(wait time.Duration) error {
	select {
	case err := <-h.errCh:
		return err
	case <-time.After(wait):
		return nil
	}
}

func (h *abortHarness) stop() {
	_, _ = h.svc.Shutdown(context.Background())
	_ = h.source.Close()
}

type dockMux struct{ dock *audiodock.Dock }

func (m dockMux) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return m.dock.Transform(ctx, input)
}

// turnScopedAgent mimics the Flowcraft contract that matters here: a finished
// turn keeps the output stream open, a control-only BOS interrupts the active
// turn, and the output stream ends only when the input stream ends.
type turnScopedAgent struct {
	turnStarted chan struct{}
}

func (a *turnScopedAgent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := newAgentOutput()
	go a.run(ctx, input, output)
	return output, nil
}

func (a *turnScopedAgent) run(ctx context.Context, input genx.Stream, output *agentOutput) {
	var text strings.Builder
	var turn sync.WaitGroup
	cancelTurn := func() {}
	for {
		chunk, err := input.Next()
		if err != nil {
			cancelTurn()
			turn.Wait()
			// Flowcraft closes cleanly once its input ends.
			_ = output.Close()
			return
		}
		if chunk == nil {
			continue
		}
		if chunk.IsBeginOfStream() && chunk.Part == nil {
			// Control-only BOS interrupts the active turn but keeps the stream.
			cancelTurn()
			turn.Wait()
			text.Reset()
			continue
		}
		part, ok := chunk.Part.(genx.Text)
		if !ok {
			continue
		}
		text.WriteString(string(part))
		if !chunk.IsEndOfStream() || strings.TrimSpace(text.String()) == "" {
			continue
		}
		text.Reset()
		turnCtx, cancel := context.WithCancel(ctx)
		cancelTurn = cancel
		turn.Add(1)
		go func() {
			defer turn.Done()
			defer cancel()
			a.emitTurn(turnCtx, output)
		}()
	}
}

// emitTurn answers over several chunks so an abort can land mid-turn.
func (a *turnScopedAgent) emitTurn(ctx context.Context, output *agentOutput) {
	streamID := genx.NewStreamID()
	if err := output.push(&genx.MessageChunk{
		Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", BeginOfStream: true},
	}); err != nil {
		return
	}
	select {
	case a.turnStarted <- struct{}{}:
	default:
	}
	for range 20 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
		if err := output.push(&genx.MessageChunk{
			Role: genx.RoleModel, Name: "assistant", Part: genx.Text("word "),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant"},
		}); err != nil {
			return
		}
	}
	_ = output.push(&genx.MessageChunk{
		Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "assistant", EndOfStream: true},
	})
}

// agentOutput is a minimal pull stream for the fake Agent.
type agentOutput struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []*genx.MessageChunk
	closed bool
	err    error
}

func newAgentOutput() *agentOutput {
	o := &agentOutput{}
	o.cond = sync.NewCond(&o.mu)
	return o
}

func (o *agentOutput) push(chunk *genx.MessageChunk) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return io.ErrClosedPipe
	}
	o.queue = append(o.queue, chunk)
	o.cond.Broadcast()
	return nil
}

func (o *agentOutput) Next() (*genx.MessageChunk, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for len(o.queue) == 0 && !o.closed {
		o.cond.Wait()
	}
	if len(o.queue) > 0 {
		chunk := o.queue[0]
		o.queue = o.queue[1:]
		return chunk, nil
	}
	if o.err != nil {
		return nil, o.err
	}
	return nil, io.EOF
}

func (o *agentOutput) Close() error { return o.closeWith(nil) }

func (o *agentOutput) CloseWithError(err error) error { return o.closeWith(err) }

func (o *agentOutput) closeWith(err error) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.closed {
		o.closed = true
		o.err = err
		o.cond.Broadcast()
	}
	return nil
}
