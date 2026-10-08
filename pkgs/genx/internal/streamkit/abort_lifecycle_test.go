package streamkit

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func awaitObservers(t *testing.T, output *Output) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		output.WaitForObservers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delivery observers remained after cancellation")
	}
}

func TestOutputAbortAfterCompletion(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "fail"}[fail], func(t *testing.T) {
			var delivered, abandoned atomic.Int32
			output := NewOutput(OutputConfig{Observe: func(*genx.MessageChunk) { delivered.Add(1) }})
			output.DeferOutputObservation()
			t.Cleanup(output.AbandonDeferredObservations)
			for range 3 {
				chunk := &genx.MessageChunk{Part: genx.Text("prefix")}
				if err := output.PushTracked(chunk, nil, func(*genx.MessageChunk) { abandoned.Add(1) }); err != nil {
					t.Fatal(err)
				}
			}
			prefix, err := output.Next()
			if err != nil {
				t.Fatal(err)
			}
			output.ObserveOutput(prefix)
			producerErr := errors.New("producer failed")
			pending, err := output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				_ = output.Fail(producerErr)
			} else {
				_ = output.Close()
			}
			for range 2 {
				if err := output.CloseWithError(context.Canceled); err != nil {
					t.Fatal(err)
				}
			}
			awaitObservers(t, output)
			output.ObserveOutput(pending)
			if delivered.Load() != 1 || abandoned.Load() != 2 {
				t.Fatalf("delivered=%d abandoned=%d; want delivered prefix and two discarded chunks", delivered.Load(), abandoned.Load())
			}
			if chunk, err := output.Next(); err != nil || chunk == nil {
				t.Fatalf("completed production lost its readable buffer: %v", err)
			}
			want := io.EOF
			if fail {
				want = producerErr
			}
			if _, err := output.Next(); !errors.Is(err, want) {
				t.Fatalf("completed production terminal = %v, want %v", err, want)
			}
		})
	}
}

func TestInvocationCancelAfterCompletion(t *testing.T) {
	for _, viaOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "invocation", true: "output"}[viaOutput], func(t *testing.T) {
			invocation := NewInvocation(t.Context(), OutputConfig{Observe: func(*genx.MessageChunk) {}})
			output := invocation.Output()
			output.DeferOutputObservation()
			t.Cleanup(output.AbandonDeferredObservations)
			if err := output.Push(&genx.MessageChunk{Part: genx.Text("pending delivery")}); err != nil {
				t.Fatal(err)
			}
			if _, err := output.Next(); err != nil {
				t.Fatal(err)
			}
			if viaOutput {
				_ = output.Close()
				select {
				case <-invocation.Context().Done():
				case <-time.After(time.Second):
					t.Fatal("production completion did not cancel invocation work")
				}
			} else {
				_ = invocation.Close()
			}
			for range 2 {
				if err := invocation.Cancel(context.Canceled); err != nil {
					t.Fatal(err)
				}
			}
			awaitObservers(t, output)
		})
	}
}

func TestInvocationCancelledTerminalsNeedNoDeliveryAcknowledgement(t *testing.T) {
	invocation := NewInvocation(t.Context(), OutputConfig{Observe: func(*genx.MessageChunk) {}})
	output := invocation.Output()
	output.DeferOutputObservation()
	t.Cleanup(output.AbandonDeferredObservations)
	response, err := invocation.StartResponse(ResponseConfig{Role: genx.RoleModel}, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Part: genx.Text("first")}); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Cancel(context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Cancel(context.Canceled); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := output.Next(); err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatal(err)
			}
			break
		}
	}
	awaitObservers(t, output)
}

func TestParentCancellationAfterProductionCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	invocation := NewInvocation(ctx, OutputConfig{Observe: func(*genx.MessageChunk) {}})
	output := invocation.Output()
	output.DeferOutputObservation()
	t.Cleanup(output.AbandonDeferredObservations)
	if err := output.Push(&genx.MessageChunk{Part: genx.Text("pending")}); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Next(); err != nil {
		t.Fatal(err)
	}
	_ = output.Close()
	<-invocation.Context().Done()
	cancel()
	awaitObservers(t, output)
}

func TestAbortWaitsForStartedDeliveryCallbacks(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(release) }) }
	t.Cleanup(finish)
	output := NewOutput(OutputConfig{Observe: func(*genx.MessageChunk) {
		close(started)
		<-release
	}})
	output.DeferOutputObservation()
	chunk := &genx.MessageChunk{Part: genx.Text("delivered")}
	_ = output.Push(chunk)
	_, _ = output.Next()
	go output.ObserveOutput(chunk)
	<-started
	_ = output.Close()
	_ = output.CloseWithError(context.Canceled)
	done := make(chan struct{})
	go func() { output.WaitForObservers(); close(done) }()
	select {
	case <-done:
		t.Fatal("abort released an already started persistence callback")
	default:
	}
	finish()
	awaitObservers(t, output)
	<-done
}

func TestCancellationAbandonmentCanReenterInvocation(t *testing.T) {
	invocation := NewInvocation(t.Context(), OutputConfig{})
	done := make(chan struct{})
	if err := invocation.Output().PushTracked(&genx.MessageChunk{}, nil, func(*genx.MessageChunk) {
		_, _ = invocation.StartResponse(ResponseConfig{})
		close(done)
	}); err != nil {
		t.Fatal(err)
	}
	go func() { _ = invocation.Cancel(context.Canceled) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation invoked abandonment under the invocation mutex")
	}
}

// Client operations may race producer completion, downstream pulls and each
// other. Every ordering must retire delivery once, preserve its delivered
// prefix, and terminate all waiters without requiring a downstream drain.
func TestInvocationTeardownOrderings(t *testing.T) {
	for _, completion := range []string{"active", "close", "fail", "output-close"} {
		for _, teardown := range []string{"cancel", "error-close", "parent-cancel"} {
			t.Run(completion+"/"+teardown, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var delivered atomic.Int32
				invocation := NewInvocation(ctx, OutputConfig{Observe: func(*genx.MessageChunk) { delivered.Add(1) }})
				output := invocation.Output()
				output.DeferOutputObservation()
				t.Cleanup(output.AbandonDeferredObservations)
				for range 3 {
					_ = output.Push(&genx.MessageChunk{Part: genx.Text("prefix")})
				}
				prefix, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				output.ObserveOutput(prefix)
				pending, err := output.Next()
				if err != nil {
					t.Fatal(err)
				}
				switch completion {
				case "close":
					_ = invocation.Close()
				case "fail":
					_ = invocation.Fail(errors.New("provider failed"))
				case "output-close":
					_ = output.Close()
					<-invocation.Context().Done()
				}
				var waiters sync.WaitGroup
				for range 4 {
					waiters.Go(output.WaitForObservers)
				}
				for range 2 {
					switch teardown {
					case "cancel":
						_ = invocation.Cancel(context.Canceled)
					case "error-close":
						_ = output.CloseWithError(context.Canceled)
					case "parent-cancel":
						cancel()
					}
				}
				awaitObservers(t, output)
				waiters.Wait()
				output.ObserveOutput(pending)
				if delivered.Load() != 1 {
					t.Fatalf("delivered prefix changed: %d", delivered.Load())
				}
				select {
				case <-output.settled:
				case <-time.After(time.Second):
					t.Fatal("teardown retained the output cancellation watch")
				}
			})
		}
	}
}

func TestUnobservedBufferedCompletionNeedsNoCancellationWatch(t *testing.T) {
	invocation := NewInvocation(t.Context(), OutputConfig{})
	output := invocation.Output()
	_ = output.Push(&genx.MessageChunk{Part: genx.Text("buffered")})
	_ = invocation.Close()
	select {
	case <-output.settled:
	default:
		t.Fatal("unobserved producer buffer retained a cancellation watch")
	}
	if chunk, err := output.Next(); err != nil || chunk == nil {
		t.Fatalf("unobserved completion lost its readable buffer: %v", err)
	}
}
