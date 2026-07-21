package flowcraft

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/flowcraft/sdk/engine"
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
		Type: engine.StreamDeltaParallelBranchCancel, ForkID: "fork", BranchID: "two",
	})
	emit("answer", engine.StreamDeltaPayload{
		Type: engine.StreamDeltaParallelBranchAccept, ForkID: "fork", BranchID: "one",
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
