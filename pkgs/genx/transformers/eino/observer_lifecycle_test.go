package eino

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Exercise the session/turn completion chain with a delivery claimed by a
// composer, then abandoned by its final consumer after producer completion.
func TestSessionAbortAfterOutputCompletionReleasesTurn(t *testing.T) {
	transformer, err := New(t.Context(), textConfig())
	if err != nil {
		t.Fatal(err)
	}
	input := newInputBuilder()
	output, err := transformer.Transform(t.Context(), input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	stream := output.(*sessionStream)
	stream.DeferOutputObservation()
	t.Cleanup(func() {
		stream.AbandonDeferredObservations()
		_ = stream.CloseWithError(context.Canceled)
		select {
		case <-stream.session.done:
		case <-time.After(time.Second):
			t.Error("Eino session did not finish cleanup")
		}
	})
	addTextTurn(t, input, "pending delivery")
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" && chunk.Role == genx.RoleModel {
			break
		}
	}
	_ = stream.Output.Close()
	<-stream.session.invocation.Context().Done()
	if err := stream.CloseWithError(context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream.session.done:
	case <-time.After(time.Second):
		t.Fatal("Eino session retained WaitForObservers/interruptionDone after abort")
	}
	stream.WaitForObservers()
}
