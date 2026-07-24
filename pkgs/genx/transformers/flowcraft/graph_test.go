package flowcraft

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/GizClaw/flowcraft/sdk/engine"
	"github.com/GizClaw/flowcraft/sdk/event"
)

func TestRunHostPublishesOnlyAcceptedCandidateFromAllowedNode(t *testing.T) {
	t.Parallel()
	var emitted []string
	host := &runHost{
		publish: map[string]struct{}{"answer": {}},
		emit: func(_ string, content string) error {
			emitted = append(emitted, content)
			return nil
		},
		buffers: make(map[string][]bufferedDelta), terminal: make(map[string]struct{}),
	}
	emit := func(nodeID string, delta engine.StreamDeltaPayload) {
		t.Helper()
		if err := engine.EmitStreamDelta(context.Background(), host, "run", "agent.node."+nodeID, delta); err != nil {
			t.Fatalf("EmitStreamDelta(%s): %v", delta.Type, err)
		}
	}

	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "accepted", Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "cancelled", Speculative: true,
		ForkID: "fork", BranchID: "two",
	})
	emit("hidden", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "hidden", Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	if len(emitted) != 0 {
		t.Fatalf("speculative output escaped before acceptance: %v", emitted)
	}
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaParallelBranchCancel, ForkID: "fork", BranchID: "two", Speculative: true,
	})
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaParallelBranchAccept, ForkID: "fork", BranchID: "one", Speculative: true,
	})
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "late-accepted", Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "late-cancelled", Speculative: true,
		ForkID: "fork", BranchID: "two",
	})

	if !slices.Equal(emitted, []string{"accepted"}) {
		t.Fatalf("emitted = %v, want only accepted published candidate", emitted)
	}
	if host.tokenCount() != 1 {
		t.Fatalf("tokenCount() = %d, want 1", host.tokenCount())
	}
	if len(host.buffers) != 0 {
		t.Fatalf("late terminal events recreated buffers: %#v", host.buffers)
	}
}

func TestRunHostRejectsMalformedDeltasAndEmitterFailures(t *testing.T) {
	t.Parallel()
	emitErr := errors.New("downstream failed")
	host := &runHost{
		publish: map[string]struct{}{"answer": {}},
		emit: func(string, string) error {
			return emitErr
		},
		buffers: make(map[string][]bufferedDelta), terminal: make(map[string]struct{}),
	}
	if err := host.Publish(t.Context(), event.Envelope{
		Subject: engine.SubjectStreamDelta("run", "agent.node.answer"),
		Payload: []byte("{"),
	}); err == nil {
		t.Fatal("Publish(malformed delta) succeeded")
	}
	if err := host.emitLocked("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaToken, Content: "visible",
	}); err == nil || !strings.Contains(err.Error(), emitErr.Error()) {
		t.Fatalf("emitLocked() error = %v", err)
	}
	for _, delta := range []engine.StreamDeltaPayload{
		{Type: engine.StreamDeltaToolCall, Content: "ignored"},
		{Type: engine.StreamDeltaToken},
		{Type: engine.StreamDeltaToken, Content: "hidden"},
	} {
		nodeID := "answer"
		if delta.Content == "hidden" {
			nodeID = "hidden"
		}
		if err := host.emitLocked(nodeID, delta); err != nil {
			t.Fatalf("emitLocked(ignored) error = %v", err)
		}
	}
	if host.tokenCount() != 0 {
		t.Fatalf("ignored deltas changed token count to %d", host.tokenCount())
	}
}
