package flowcraft

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	flowagent "github.com/GizClaw/flowcraft/core/agent"
	"github.com/GizClaw/flowcraft/core/event"
	"github.com/GizClaw/flowcraft/core/message"
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
	emit := func(nodeID string, delta flowagent.StreamDeltaPayload) {
		t.Helper()
		if err := flowagent.EmitStreamDelta(context.Background(), host, "run", "agent.node."+nodeID, delta); err != nil {
			t.Fatalf("EmitStreamDelta(%s): %v", delta.Type, err)
		}
	}

	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "accepted"}, Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "cancelled"}, Speculative: true,
		ForkID: "fork", BranchID: "two",
	})
	emit("hidden", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "hidden"}, Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	if len(emitted) != 0 {
		t.Fatalf("speculative output escaped before acceptance: %v", emitted)
	}
	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaParallelBranchCancel, ForkID: "fork", BranchID: "two", Speculative: true,
	})
	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaParallelBranchAccept, ForkID: "fork", BranchID: "one", Speculative: true,
	})
	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "late-accepted"}, Speculative: true,
		ForkID: "fork", BranchID: "one",
	})
	emit("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "late-cancelled"}, Speculative: true,
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
		Subject: flowagent.SubjectStreamDelta("run", "agent.node.answer"),
		Payload: []byte("{"),
	}); err == nil {
		t.Fatal("Publish(malformed delta) succeeded")
	}
	if err := host.emitLocked("answer", flowagent.StreamDeltaPayload{
		Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "visible"},
	}); err == nil || !strings.Contains(err.Error(), emitErr.Error()) {
		t.Fatalf("emitLocked() error = %v", err)
	}
	for index, delta := range []flowagent.StreamDeltaPayload{
		{Type: flowagent.StreamDeltaFinish, FinishReason: "completed"},
		{Type: flowagent.StreamDeltaPart},
		{Type: flowagent.StreamDeltaPart, Part: message.TextPart{Text: "hidden"}},
	} {
		nodeID := "answer"
		if index == 2 {
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
