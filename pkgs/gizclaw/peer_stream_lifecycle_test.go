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

func TestPeerStreamLifecycleRecordsOrderedFirstEventsAndSafeTerminal(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-1", "peer-1")
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{StreamId: "input-1-secret"}},
	}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-1", BeginOfStream: true}}
	output := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "output-1", BeginOfStream: true}}

	lifecycle.accepted()
	lifecycle.eventStreamAccepted()
	lifecycle.observeInput(event)
	lifecycle.observeInput(event)
	lifecycle.observeAgentInputOpen()
	lifecycle.observeAgentInputPush(chunk)
	lifecycle.observeAgentInputPush(chunk)
	lifecycle.observeOutput(context.Background(), output, func(context.Context) string { return "workspace-1" })
	lifecycle.observeOutput(context.Background(), output, nil)
	lifecycle.finish("server_tunnel", errors.New("authorization: Bearer secret-provider-error"))
	lifecycle.finish("server_tunnel", nil)

	records := capturedLifecycleRecords(t, capture)
	wantStages := []string{
		"session_accepted",
		"event_stream_accepted",
		"input_first_event",
		"agent_input_opened",
		"agent_input_first_push",
		"output_first_event",
		"terminal",
	}
	if len(records) != len(wantStages) {
		t.Fatalf("records = %d, want %d", len(records), len(wantStages))
	}
	for i, record := range records {
		attrs := lifecycleRecordAttrs(record)
		if record.Message != peerStreamLifecycleMessage {
			t.Fatalf("record[%d].Message = %q", i, record.Message)
		}
		if got := attrs["stage"]; got != wantStages[i] {
			t.Errorf("record[%d].stage = %#v, want %q", i, got, wantStages[i])
		}
		if attrs["tunnel_session_id"] != "session-1" || attrs["peer_public_key"] != "peer-1" {
			t.Errorf("record[%d] correlation = %#v", i, attrs)
		}
		if value, ok := attrs["stream_id_hash"]; ok {
			if value != safeStreamIDHash(map[int]string{2: "input-1-secret", 4: "input-1", 5: "output-1"}[i]) {
				t.Errorf("record[%d].stream_id_hash = %#v", i, value)
			}
		}
		for key, value := range attrs {
			switch value.(type) {
			case string, int64, bool:
			default:
				t.Errorf("record[%d].%s has non-scalar value %T", i, key, value)
			}
		}
	}
	terminal := lifecycleRecordAttrs(records[len(records)-1])
	for key, want := range map[string]any{
		"result": "runtime_error", "reason": "internal_error", "last_stage": "output_first_event",
		"input_event_observed": true, "agent_input_opened": true,
		"agent_input_pushed": true, "output_event_observed": true,
	} {
		if got := terminal[key]; got != want {
			t.Errorf("terminal.%s = %#v, want %#v", key, got, want)
		}
	}
	for _, record := range records {
		attrs := lifecycleRecordAttrs(record)
		formatted := fmt.Sprint(attrs)
		if strings.Contains(formatted, "secret-provider-error") || strings.Contains(formatted, "input-1-secret") {
			t.Fatalf("lifecycle logs exposed raw untrusted data: %s", formatted)
		}
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
