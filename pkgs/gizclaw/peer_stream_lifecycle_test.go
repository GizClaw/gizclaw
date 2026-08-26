package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestPeerStreamLifecycleCorrelatesSequentialTurns(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-1", "peer-1")
	for index := 1; index <= 2; index++ {
		inputID := fmt.Sprintf("input-%d-secret", index)
		outputID := fmt.Sprintf("output-%d-secret", index)
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, inputID, nil))
		lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: inputID}})
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, inputID, nil))
		lifecycle.observeOutput(context.Background(), &genx.MessageChunk{
			Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: outputID, BeginOfStream: true},
		}, func(context.Context) string { return "workspace-1" })
		lifecycle.observeOutput(context.Background(), &genx.MessageChunk{
			Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: outputID, EndOfStream: true},
		}, nil)
	}

	records := capturedTurnLifecycleRecords(t, capture)
	wantStages := []string{
		"turn_started", "input_first_event", "agent_input_first_push", "input_terminal",
		"output_first_event", "output_terminal", "turn_terminal",
		"turn_started", "input_first_event", "agent_input_first_push", "input_terminal",
		"output_first_event", "output_terminal", "turn_terminal",
	}
	if len(records) != len(wantStages) {
		t.Fatalf("records = %d, want %d", len(records), len(wantStages))
	}
	for i, record := range records {
		attrs := lifecycleRecordAttrs(record)
		if got := attrs["stage"]; got != wantStages[i] {
			t.Errorf("record[%d].stage = %#v, want %q", i, got, wantStages[i])
		}
		wantTurn := uint64(i/7 + 1)
		if got := attrs["turn_index"]; got != wantTurn {
			t.Errorf("record[%d].turn_index = %#v, want %d", i, got, wantTurn)
		}
		if attrs["tunnel_session_id"] != "session-1" || attrs["peer_public_key"] != "peer-1" {
			t.Errorf("record[%d] correlation = %#v", i, attrs)
		}
		for key, value := range attrs {
			switch value.(type) {
			case string, int64, uint64, bool:
			default:
				t.Errorf("record[%d].%s has non-scalar value %T", i, key, value)
			}
		}
	}
	for _, record := range records {
		attrs := lifecycleRecordAttrs(record)
		formatted := fmt.Sprint(attrs)
		if strings.Contains(formatted, "input-1-secret") || strings.Contains(formatted, "output-1-secret") {
			t.Fatalf("lifecycle logs exposed raw untrusted data: %s", formatted)
		}
	}
}

func TestPeerStreamLifecycleKeepsLateOutputTerminalOnReplacedTurn(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-replace", "peer-replace")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-old", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-old"}})
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text("old"), Ctrl: &genx.StreamCtrl{StreamID: "output-old", BeginOfStream: true},
	}, nil)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text("new"), Ctrl: &genx.StreamCtrl{StreamID: "output-new", BeginOfStream: true},
	}, nil)
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{
			StreamID: "output-old", EndOfStream: true, Error: "interrupted", ErrorCode: "STREAM_INTERRUPTED",
		},
	}, nil)
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output-new", EndOfStream: true},
	}, nil)

	var oldOutputTerminal, oldTurnTerminal, newOutputTerminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		switch {
		case attrs["stage"] == "output_terminal" && attrs["turn_index"] == uint64(1):
			oldOutputTerminal = attrs
		case attrs["stage"] == "turn_terminal" && attrs["turn_index"] == uint64(1):
			oldTurnTerminal = attrs
		case attrs["stage"] == "output_terminal" && attrs["turn_index"] == uint64(2):
			newOutputTerminal = attrs
		}
	}
	if oldOutputTerminal["output_stream_id_hash"] != safeStreamIDHash("output-old") ||
		oldOutputTerminal["reason"] != "expected_interruption" {
		t.Fatalf("old output terminal = %#v", oldOutputTerminal)
	}
	if oldTurnTerminal["result"] != "replaced" || oldTurnTerminal["reason"] != "input_replaced" {
		t.Fatalf("old turn terminal = %#v", oldTurnTerminal)
	}
	if newOutputTerminal["output_stream_id_hash"] != safeStreamIDHash("output-new") {
		t.Fatalf("new output terminal = %#v", newOutputTerminal)
	}
}

func TestPeerStreamLifecycleKeepsDelayedFirstOutputOnReplacedTurn(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-delayed", "peer-delayed")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-old", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-old"}})
	oldOutputBOS := &genx.MessageChunk{
		Part: genx.Text("late old output"), Ctrl: &genx.StreamCtrl{StreamID: "output-old", BeginOfStream: true},
	}
	lifecycle.bindOutputOwner(oldOutputBOS)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	lifecycle.observeOutput(t.Context(), oldOutputBOS, nil)
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output-old", EndOfStream: true},
	}, nil)
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text("new output"), Ctrl: &genx.StreamCtrl{StreamID: "output-new", BeginOfStream: true},
	}, nil)

	var oldOutput, newOutput map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		switch {
		case attrs["stage"] == "output_first_event" && attrs["turn_index"] == uint64(1):
			oldOutput = attrs
		case attrs["stage"] == "output_first_event" && attrs["turn_index"] == uint64(2):
			newOutput = attrs
		}
	}
	if oldOutput["output_stream_id_hash"] != safeStreamIDHash("output-old") {
		t.Fatalf("old delayed output = %#v", oldOutput)
	}
	if newOutput["output_stream_id_hash"] != safeStreamIDHash("output-new") {
		t.Fatalf("new output = %#v", newOutput)
	}
}

func TestPeerStreamLifecycleAssignsReplacementOnlyOutputToReplacement(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-replacement-only", "peer-replacement-only")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-old", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-old"}})
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	newOutput := &genx.MessageChunk{
		Part: genx.Text("replacement output"), Ctrl: &genx.StreamCtrl{StreamID: "output-new", BeginOfStream: true},
	}
	lifecycle.bindOutputOwner(newOutput)
	lifecycle.observeOutput(t.Context(), newOutput, nil)

	var output map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "output_first_event" {
			output = attrs
		}
	}
	if output["turn_index"] != uint64(2) || output["output_stream_id_hash"] != safeStreamIDHash("output-new") {
		t.Fatalf("replacement output = %#v", output)
	}
}

func TestPeerStreamLifecycleAgentOutputFailureTerminatesOpenTurns(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-failure", "peer-failure")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-failure", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-failure"}})
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
		Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "output-failure", BeginOfStream: true},
	}, nil)
	lifecycle.finish("agent_output", errors.New("credential-bearing provider failure"))

	var outputTerminal, turnTerminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		switch attrs["stage"] {
		case "output_terminal":
			outputTerminal = attrs
		case "turn_terminal":
			turnTerminal = attrs
		}
	}
	for _, attrs := range []map[string]any{outputTerminal, turnTerminal} {
		if attrs["result"] != "runtime_error" || attrs["reason"] != "internal_error" {
			t.Fatalf("failure terminal = %#v", attrs)
		}
		if strings.Contains(fmt.Sprint(attrs), "credential-bearing") {
			t.Fatalf("failure terminal exposed raw error: %#v", attrs)
		}
	}
}

func TestPeerStreamLifecycleSecondTurnStallHasIndependentTerminal(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-stall", "peer-stall")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "first", nil))
	lifecycle.observeOutput(t.Context(), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "first-output", EndOfStream: true}}, nil)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "second", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "second"}})
	lifecycle.finish("peer_input", io.EOF)

	var terminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "turn_terminal" && attrs["turn_index"] == uint64(2) {
			terminal = attrs
		}
	}
	for key, want := range map[string]any{
		"result": "closed", "reason": "stream_closed", "agent_input_pushed": true,
		"output_event_observed": false, "output_terminal_observed": false,
	} {
		if got := terminal[key]; got != want {
			t.Errorf("terminal.%s = %#v, want %#v; terminal=%#v", key, got, want, terminal)
		}
	}
}

func TestPeerStreamLifecycleStateAndRecordVolumeAreBounded(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-bounded", "peer-bounded")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-0", nil))
	for range 100 {
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA, "input-0", nil))
		lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-0"}})
		lifecycle.observeOutput(t.Context(), &genx.MessageChunk{Part: genx.Text("delta"), Ctrl: &genx.StreamCtrl{StreamID: "output-0"}}, nil)
	}
	if got := len(capturedTurnLifecycleRecords(t, capture)); got != 4 {
		t.Fatalf("records after repeated chunks = %d, want fixed four stages", got)
	}
	for index := 1; index <= peerStreamLifecycleMaxRetainedTurns+20; index++ {
		inputID := fmt.Sprintf("input-%d", index)
		outputID := fmt.Sprintf("output-%d", index)
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, inputID, nil))
		lifecycle.observeOutput(t.Context(), &genx.MessageChunk{
			Part: genx.Text("open"), Ctrl: &genx.StreamCtrl{StreamID: outputID, BeginOfStream: true},
		}, nil)
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if len(lifecycle.turns) > peerStreamLifecycleMaxRetainedTurns ||
		len(lifecycle.turnOrder) > peerStreamLifecycleMaxRetainedTurns ||
		len(lifecycle.outputTurns) > peerStreamLifecycleMaxOutputRoutes ||
		len(lifecycle.outputOrder) > peerStreamLifecycleMaxOutputRoutes {
		t.Fatalf("retained state turns=%d order=%d routes=%d route_order=%d", len(lifecycle.turns), len(lifecycle.turnOrder), len(lifecycle.outputTurns), len(lifecycle.outputOrder))
	}
}

func TestSafeStreamIDHashIsBoundedAndStable(t *testing.T) {
	const untrusted = "  prompt-secret\nBearer credential-secret  "
	first := safeStreamIDHash(untrusted)
	second := safeStreamIDHash(strings.TrimSpace(untrusted))
	if first == "" || first != second || len(first) != 32 {
		t.Fatalf("safeStreamIDHash() = %q / %q", first, second)
	}
	if strings.Contains(first, "secret") || safeStreamIDHash("  ") != "" {
		t.Fatalf("safeStreamIDHash() exposed input or retained empty input: %q", first)
	}
	if got := safeStreamIDHash("  stream-42\n"); got != "0f3a788cbbee0b932cfcac7d71645f31" {
		t.Fatalf("safeStreamIDHash(test vector) = %q", got)
	}
}

func TestPeerStreamLifecycleResultIsExhaustiveAndBounded(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		wantResult string
		wantReason string
	}{
		{name: "success", wantResult: "success", wantReason: "completed"},
		{name: "canceled", err: context.Canceled, wantResult: "canceled", wantReason: "context_canceled"},
		{name: "timeout", err: context.DeadlineExceeded, wantResult: "timeout", wantReason: "deadline_exceeded"},
		{name: "closed", err: io.EOF, wantResult: "closed", wantReason: "stream_closed"},
		{name: "runtime", err: errors.New("provider secret"), wantResult: "runtime_error", wantReason: "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, reason := peerStreamLifecycleResult(test.err)
			if result != test.wantResult || reason != test.wantReason {
				t.Fatalf("peerStreamLifecycleResult() = (%q, %q), want (%q, %q)", result, reason, test.wantResult, test.wantReason)
			}
			if strings.Contains(result+reason, "secret") {
				t.Fatal("bounded lifecycle result exposed raw error")
			}
		})
	}
}

func TestPeerStreamControlOutcomeUsesClosedValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		message    string
		code       string
		wantResult string
		wantReason string
	}{
		{name: "completed", wantResult: "success", wantReason: "completed"},
		{name: "interrupted", message: "interrupted", wantResult: "interrupted", wantReason: "expected_interruption"},
		{name: "caller cancellation", code: "STREAM_CANCELED", wantResult: "canceled", wantReason: "caller_canceled"},
		{name: "deadline", message: context.DeadlineExceeded.Error(), wantResult: "timeout", wantReason: "deadline_exceeded"},
		{name: "closed", code: "STREAM_CLOSED", wantResult: "closed", wantReason: "stream_closed"},
		{name: "runtime", message: "Bearer raw-provider-secret", code: "PROVIDER_FAILURE", wantResult: "runtime_error", wantReason: "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, reason := peerStreamControlOutcome(test.message, test.code)
			if result != test.wantResult || reason != test.wantReason {
				t.Fatalf("peerStreamControlOutcome() = (%q, %q), want (%q, %q)", result, reason, test.wantResult, test.wantReason)
			}
			if strings.Contains(result+reason, "secret") {
				t.Fatal("bounded outcome exposed raw error")
			}
		})
	}
}

func TestPeerStreamLifecycleTerminalLocalizesZeroEvent(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-zero", "peer-zero")
	lifecycle.accepted()
	lifecycle.eventStreamAccepted()
	lifecycle.finish("peer_input", context.DeadlineExceeded)
	lifecycle.finish("server_tunnel", context.DeadlineExceeded)

	records := capturedLifecycleRecords(t, capture)
	peerTerminal := lifecycleRecordAttrs(records[len(records)-2])
	if peerTerminal["component"] != "peer_input" || peerTerminal["last_stage"] != "event_stream_accepted" {
		t.Errorf("peer input terminal = %#v", peerTerminal)
	}
	terminal := lifecycleRecordAttrs(records[len(records)-1])
	for key, want := range map[string]any{
		"result": "timeout", "reason": "deadline_exceeded", "last_stage": "event_stream_accepted",
		"input_event_observed": false, "agent_input_opened": false,
		"agent_input_pushed": false, "output_event_observed": false,
	} {
		if got := terminal[key]; got != want {
			t.Errorf("terminal.%s = %#v, want %#v", key, got, want)
		}
	}
}

func peerInputEvent(eventType eventpb.PeerEventType, streamID string, eventErr *eventpb.EventError) *eventpb.PeerEvent {
	if eventType == eventpb.PeerEventType_PEER_EVENT_TYPE_BOS {
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: streamID}},
		}
	}
	if eventType == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS {
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{StreamId: streamID, Error: eventErr}},
		}
	}
	return &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventType,
		Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{StreamId: streamID}},
	}
}

func capturedTurnLifecycleRecords(t *testing.T, capture *slogCapture) []slog.Record {
	t.Helper()
	var records []slog.Record
	for _, record := range capturedLifecycleRecords(t, capture) {
		if _, ok := lifecycleRecordAttrs(record)["turn_index"]; ok {
			records = append(records, record)
		}
	}
	return records
}

func capturedLifecycleRecords(t *testing.T, capture *slogCapture) []slog.Record {
	t.Helper()
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return append([]slog.Record(nil), capture.records...)
}

func lifecycleRecordAttrs(record slog.Record) map[string]any {
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	return attrs
}
