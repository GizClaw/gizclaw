package audiodock

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestConsumerCloseReleasesCompletedDockDelivery(t *testing.T) {
	for _, withError := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "error-close"}[withError], func(t *testing.T) {
			invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{Observe: func(*genx.MessageChunk) {}})
			stream := &dockStream{Output: invocation.Output(), invocation: invocation}
			stream.DeferOutputObservation()
			t.Cleanup(stream.AbandonDeferredObservations)
			_ = stream.Push(&genx.MessageChunk{Part: genx.Text("pending delivery")})
			if _, err := stream.Next(); err != nil {
				t.Fatal(err)
			}
			_ = invocation.Close()
			for range 2 {
				var err error
				if withError {
					err = stream.CloseWithError(context.Canceled)
				} else {
					err = stream.Close()
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan struct{})
			go func() { stream.WaitForObservers(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("consumer close retained completed Audio Dock delivery")
			}
		})
	}
}
