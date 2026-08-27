package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
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
		input := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: inputID}}
		lifecycle.observeAgentInputPush(input)
		lifecycle.observeAgentTransformStarted(input)
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, inputID, nil))
		text := &genx.MessageChunk{
			Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: outputID, BeginOfStream: true},
		}
		epoch := genx.NewResponseEpoch(inputID)
		attachTestResponseEpochWith(epoch, text)
		lifecycle.observeOutputProduced(text)
		lifecycle.observeOutput(context.Background(), text, func(context.Context) string { return "workspace-1" })
		eos := &genx.MessageChunk{
			Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: outputID, EndOfStream: true},
		}
		attachTestResponseEpochEnd(epoch, eos)
		lifecycle.observeOutputProduced(eos)
		lifecycle.observeOutput(context.Background(), eos, nil)
	}

	records := capturedTurnLifecycleRecords(t, capture)
	wantStages := []string{
		"turn_started", "input_first_event", "agent_input_first_push", "agent_transform_started", "input_terminal",
		"agent_output_produced", "output_first_event", "agent_output_delivered", "agent_terminal", "output_terminal", "turn_terminal",
		"turn_started", "input_first_event", "agent_input_first_push", "agent_transform_started", "input_terminal",
		"agent_output_produced", "output_first_event", "agent_output_delivered", "agent_terminal", "output_terminal", "turn_terminal",
	}
	if len(records) != len(wantStages) {
		t.Fatalf("records = %d, want %d", len(records), len(wantStages))
	}
	for i, record := range records {
		attrs := lifecycleRecordAttrs(record)
		if got := attrs["stage"]; got != wantStages[i] {
			t.Errorf("record[%d].stage = %#v, want %q", i, got, wantStages[i])
		}
		wantTurn := uint64(i/11 + 1)
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
	oldEpoch := genx.NewResponseEpoch("input-old")
	newEpoch := genx.NewResponseEpoch("input-new")
	oldBOS := attachTestResponseEpochWith(oldEpoch, &genx.MessageChunk{
		Part: genx.Text("old"), Ctrl: &genx.StreamCtrl{StreamID: "output-old", BeginOfStream: true},
	})
	lifecycle.observeOutputProduced(oldBOS)
	lifecycle.observeOutput(t.Context(), oldBOS, nil)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	newBOS := attachTestResponseEpochWith(newEpoch, &genx.MessageChunk{
		Part: genx.Text("new"), Ctrl: &genx.StreamCtrl{StreamID: "output-new", BeginOfStream: true},
	})
	lifecycle.observeOutputProduced(newBOS)
	lifecycle.observeOutput(t.Context(), newBOS, nil)
	oldEOS := attachTestResponseEpochEnd(oldEpoch, &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{
			StreamID: "output-old", EndOfStream: true, Error: "interrupted", ErrorCode: "STREAM_INTERRUPTED",
		},
	})
	lifecycle.observeOutputProduced(oldEOS)
	lifecycle.observeOutput(t.Context(), oldEOS, nil)
	newEOS := attachTestResponseEpochEnd(newEpoch, &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output-new", EndOfStream: true},
	})
	lifecycle.observeOutputProduced(newEOS)
	lifecycle.observeOutput(t.Context(), newEOS, nil)

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
	oldEpoch := genx.NewResponseEpoch("input-old")
	attachTestResponseEpochWith(oldEpoch, oldOutputBOS)
	lifecycle.observeOutputProduced(oldOutputBOS)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	lifecycle.observeOutput(t.Context(), oldOutputBOS, nil)
	lifecycle.observeOutput(t.Context(), attachTestResponseEpochEnd(oldEpoch, &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output-old", EndOfStream: true},
	}), nil)
	lifecycle.observeOutput(t.Context(), attachTestResponseEpoch("input-new", &genx.MessageChunk{
		Part: genx.Text("new output"), Ctrl: &genx.StreamCtrl{StreamID: "output-new", BeginOfStream: true},
	}), nil)

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

func TestPeerStreamLifecycleBindsFirstLateObservationByEpochOwner(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-late-epoch", "peer-late-epoch")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-old", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-old"}})
	oldEpoch := genx.NewResponseEpoch("input-old")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	lifecycle.observeAgentInputPush(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-new"}})
	late := attachTestResponseEpochWith(oldEpoch, &genx.MessageChunk{
		Role: genx.RoleModel, Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "output-old", BeginOfStream: true},
	})
	lifecycle.observeOutputProduced(late)
	lifecycle.observeOutput(t.Context(), late, nil)

	var output map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "output_first_event" {
			output = attrs
		}
	}
	if output["turn_index"] != uint64(1) || output["output_stream_id_hash"] != safeStreamIDHash("output-old") {
		t.Fatalf("late epoch output = %#v", output)
	}
}

func TestPeerStreamLifecycleLeavesOutputWithoutProvenanceUnowned(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-unowned", "peer-unowned")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))
	chunk := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("unowned"), Ctrl: &genx.StreamCtrl{StreamID: "output"}}
	lifecycle.observeOutputProduced(chunk)
	lifecycle.observeOutput(t.Context(), chunk, nil)
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		stage := lifecycleRecordAttrs(record)["stage"]
		if stage == "agent_output_produced" || stage == "output_first_event" || stage == "agent_output_delivered" {
			t.Fatalf("unowned output produced per-turn record: %#v", lifecycleRecordAttrs(record))
		}
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
	attachTestResponseEpoch("input-new", newOutput)
	lifecycle.observeOutputProduced(newOutput)
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
	lifecycle.observeOutput(t.Context(), attachTestResponseEpoch("input-failure", &genx.MessageChunk{
		Part: genx.Text("partial"), Ctrl: &genx.StreamCtrl{StreamID: "output-failure", BeginOfStream: true},
	}), nil)
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

func TestPeerStreamLifecycleCorrelatesMultimodalAgentBoundaries(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-modalities", "peer-modalities")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-modalities", nil))
	input := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-modalities"}}
	lifecycle.observeAgentInputPush(input)
	lifecycle.observeAgentTransformStarted(input)

	chunks := []*genx.MessageChunk{
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("private transcript"), Ctrl: &genx.StreamCtrl{StreamID: "input-modalities", Label: "transcript", BeginOfStream: true}},
		{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input-modalities", Label: "transcript", EndOfStream: true}},
		{Role: genx.RoleModel, Name: "answer", Part: genx.Text("private assistant"), Ctrl: &genx.StreamCtrl{StreamID: "output-modalities", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Name: "answer", Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private audio")}, Ctrl: &genx.StreamCtrl{StreamID: "output-modalities", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output-modalities", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Name: "answer", Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "output-modalities", Label: "assistant", EndOfStream: true}},
	}
	transcriptEpoch := genx.NewResponseEpoch("input-modalities")
	for _, chunk := range chunks[:2] {
		attachTestResponseEpochWith(transcriptEpoch, chunk)
	}
	assistantEpoch := genx.NewResponseEpoch("input-modalities")
	for _, chunk := range chunks[2:] {
		attachTestResponseEpochWith(assistantEpoch, chunk)
	}
	chunks[len(chunks)-1].Ctrl.ResponseEpochEnd = true
	for index, chunk := range chunks {
		lifecycle.observeOutputProduced(chunk)
		lifecycle.observeOutput(t.Context(), chunk, nil)
		if index < len(chunks)-1 {
			assertNoTurnStage(t, capture, "turn_terminal")
		}
	}

	var produced, delivered, agentTerminal, turnTerminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		switch attrs["stage"] {
		case "agent_output_produced":
			produced = attrs
		case "agent_output_delivered":
			delivered = attrs
		case "agent_terminal":
			agentTerminal = attrs
		case "turn_terminal":
			turnTerminal = attrs
		}
	}
	if produced["output_modality"] != "transcript_text" || delivered["output_modality"] != "transcript_text" {
		t.Fatalf("first output modalities produced=%#v delivered=%#v", produced, delivered)
	}
	if agentTerminal["terminal_class"] != "completed" || agentTerminal["last_agent_stage"] != "agent_output_delivered" {
		t.Fatalf("Agent terminal = %#v", agentTerminal)
	}
	if turnTerminal["produced_modalities"] != "transcript_text,assistant_text,assistant_audio,assistant_eos" ||
		turnTerminal["delivered_modalities"] != "transcript_text,assistant_text,assistant_audio,assistant_eos" {
		t.Fatalf("turn terminal modalities = %#v", turnTerminal)
	}
	for key, want := range map[string]string{
		"source_part_classes":      "audio,text",
		"source_label_classes":     "assistant,transcript",
		"peer_event_types":         "bos,eos,text_delta,text_done",
		"peer_event_kinds":         "audio,text,unspecified",
		"peer_event_label_classes": "assistant,transcript",
	} {
		if got := turnTerminal[key]; got != want {
			t.Errorf("turn terminal %s = %#v, want %q", key, got, want)
		}
	}
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		if rendered := fmt.Sprint(lifecycleRecordAttrs(record)); strings.Contains(rendered, "private") {
			t.Fatalf("lifecycle record exposed payload: %s", rendered)
		}
	}
}

func TestPeerStreamLifecycleEmptyControlEOSRemainsOtherAtPeerBoundary(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-control", "peer-control")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input", nil))
	chunk := attachTestResponseEpochEnd(genx.NewResponseEpoch("input"), &genx.MessageChunk{
		Role: genx.RoleModel,
		Ctrl: &genx.StreamCtrl{StreamID: "output", EndOfStream: true},
	})
	lifecycle.observeOutputProduced(chunk)
	lifecycle.observeOutput(t.Context(), chunk, nil)
	var terminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "turn_terminal" {
			terminal = attrs
		}
	}
	if terminal["produced_modalities"] != "assistant_eos" || terminal["delivered_modalities"] != "other" ||
		terminal["source_part_classes"] != "control" || terminal["source_label_classes"] != "empty" ||
		terminal["peer_event_types"] != "eos" || terminal["peer_event_kinds"] != "unspecified" ||
		terminal["peer_event_label_classes"] != "empty" {
		t.Fatalf("empty control terminal = %#v", terminal)
	}
}

func TestPeerEventOutputModalityUsesEmptyBlobAssistantFallback(t *testing.T) {
	chunk := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "video/mp4"},
		Ctrl: &genx.StreamCtrl{StreamID: "output", EndOfStream: true},
	}
	events := peerStreamEventsFromChunk(chunk)
	if len(events) != 1 || peerEventOutputModality(chunk, events[0]) != "assistant_eos" {
		t.Fatalf("empty-label blob events = %#v", events)
	}
}

func TestPeerStreamLifecycleSuppressedAudioEOSCompletesWithoutDeliveredEOS(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-audio", "peer-audio")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input", nil))
	epoch := genx.NewResponseEpoch("input")
	bos := attachTestResponseEpochWith(epoch, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private")},
		Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", BeginOfStream: true},
	})
	lifecycle.observeOutputProduced(bos)
	for _, event := range peerStreamEventsFromChunk(bos) {
		lifecycle.observePeerEventDelivered(t.Context(), bos, event, nil)
	}
	eos := attachTestResponseEpochEnd(epoch, &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", EndOfStream: true},
	})
	lifecycle.observeOutputProduced(eos)
	// The aggregate audio route suppressed this source EOS, but MixerOutput has
	// drained its track and successfully completed the source observation.
	lifecycle.observeOutputDrained(eos)

	var terminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "turn_terminal" {
			terminal = attrs
		}
	}
	if terminal["produced_modalities"] != "assistant_audio,assistant_eos" ||
		terminal["delivered_modalities"] != "assistant_audio" || terminal["output_terminal_observed"] != true {
		t.Fatalf("suppressed audio EOS terminal = %#v", terminal)
	}
}

func TestPeerStreamLifecycleIgnoresHistorySidebandForFirstAgentModality(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-sideband", "peer-sideband")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-sideband", nil))
	sideband := &genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private audio")},
		Ctrl: &genx.StreamCtrl{StreamID: "input-sideband", Label: genx.HistoryUserAudioLabel},
	}
	transcript := &genx.MessageChunk{
		Role: genx.RoleUser, Name: "transcript", Part: genx.Text("private transcript"),
		Ctrl: &genx.StreamCtrl{StreamID: "input-sideband", Label: "transcript"},
	}
	attachTestResponseEpoch("input-sideband", sideband)
	attachTestResponseEpoch("input-sideband", transcript)
	for _, chunk := range []*genx.MessageChunk{sideband, transcript} {
		lifecycle.observeOutputProduced(chunk)
		lifecycle.observeOutput(t.Context(), chunk, nil)
	}

	var produced, delivered map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		switch attrs["stage"] {
		case "agent_output_produced":
			produced = attrs
		case "agent_output_delivered":
			delivered = attrs
		}
	}
	if produced["output_modality"] != "transcript_text" || delivered["output_modality"] != "transcript_text" {
		t.Fatalf("sideband changed first Agent modalities produced=%#v delivered=%#v", produced, delivered)
	}
}

func TestPeerStreamLifecycleClassifiesFirstAssistantModalities(t *testing.T) {
	for _, test := range []struct {
		name           string
		chunks         []*genx.MessageChunk
		wantModality   string
		wantTerminal   string
		wantModalities string
	}{
		{
			name: "text",
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: genx.Text("private"), Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", BeginOfStream: true}},
				{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", EndOfStream: true}},
			},
			wantModality: "assistant_text", wantTerminal: "completed", wantModalities: "assistant_text,assistant_eos",
		},
		{
			name: "audio only",
			chunks: []*genx.MessageChunk{
				{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private")}, Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", BeginOfStream: true}},
				{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", EndOfStream: true}},
			},
			wantModality: "assistant_audio", wantTerminal: "completed", wantModalities: "assistant_audio,assistant_eos",
		},
		{
			name:         "eos only",
			chunks:       []*genx.MessageChunk{{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", EndOfStream: true}}},
			wantModality: "assistant_eos", wantTerminal: "completed", wantModalities: "assistant_eos",
		},
		{
			name:         "interrupt only",
			chunks:       []*genx.MessageChunk{{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "output", Label: "assistant", EndOfStream: true, Error: "interrupted", ErrorCode: "STREAM_INTERRUPTED"}}},
			wantModality: "interrupt", wantTerminal: "interrupted", wantModalities: "interrupt",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &slogCapture{}
			lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-"+test.name, "peer")
			lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input", nil))
			epoch := genx.NewResponseEpoch("input")
			for _, chunk := range test.chunks {
				attachTestResponseEpochWith(epoch, chunk)
			}
			test.chunks[len(test.chunks)-1].Ctrl.ResponseEpochEnd = true
			for _, chunk := range test.chunks {
				lifecycle.observeOutputProduced(chunk)
				lifecycle.observeOutput(t.Context(), chunk, nil)
			}
			var produced, delivered, terminal map[string]any
			for _, record := range capturedTurnLifecycleRecords(t, capture) {
				attrs := lifecycleRecordAttrs(record)
				switch attrs["stage"] {
				case "agent_output_produced":
					produced = attrs
				case "agent_output_delivered":
					delivered = attrs
				case "agent_terminal":
					terminal = attrs
				}
			}
			if produced["output_modality"] != test.wantModality || delivered["output_modality"] != test.wantModality {
				t.Fatalf("first modalities produced=%#v delivered=%#v", produced, delivered)
			}
			if terminal["terminal_class"] != test.wantTerminal || terminal["produced_modalities"] != test.wantModalities {
				t.Fatalf("terminal = %#v", terminal)
			}
		})
	}
}

func TestPeerStreamLifecycleReplacementTerminatesTranscriptOnlyTurn(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-transcript-only", "peer-transcript-only")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-old", nil))
	transcript := &genx.MessageChunk{
		Role: genx.RoleUser, Name: "transcript", Part: genx.Text("private transcript"),
		Ctrl: &genx.StreamCtrl{StreamID: "input-old", Label: "transcript"},
	}
	attachTestResponseEpoch("input-old", transcript)
	lifecycle.observeOutputProduced(transcript)
	lifecycle.observeOutput(t.Context(), transcript, nil)
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-new", nil))

	var agentTerminal, turnTerminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["turn_index"] != uint64(1) {
			continue
		}
		switch attrs["stage"] {
		case "agent_terminal":
			agentTerminal = attrs
		case "turn_terminal":
			turnTerminal = attrs
		}
	}
	if agentTerminal["terminal_class"] != "interrupted" || agentTerminal["produced_modalities"] != "transcript_text" {
		t.Fatalf("transcript-only Agent terminal = %#v", agentTerminal)
	}
	if turnTerminal["reason"] != "input_replaced" {
		t.Fatalf("transcript-only turn terminal = %#v", turnTerminal)
	}
}

func TestPeerStreamLifecycleCancellationPreservesLastAgentStage(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-cancel", "peer-cancel")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-cancel", nil))
	input := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-cancel"}}
	lifecycle.observeAgentInputPush(input)
	lifecycle.observeAgentTransformStarted(input)
	lifecycle.finish("agent_output", context.Canceled)

	var terminal map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "agent_terminal" {
			terminal = attrs
		}
	}
	for key, want := range map[string]any{
		"terminal_class":       "caller_canceled",
		"last_agent_stage":     "agent_transform_started",
		"produced_modalities":  "",
		"delivered_modalities": "",
	} {
		if got := terminal[key]; got != want {
			t.Errorf("terminal.%s = %#v, want %#v; terminal=%#v", key, got, want, terminal)
		}
	}
}

func TestPeerStreamLifecyclePreservesProviderTerminalAcrossCancellation(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-provider", "peer-provider")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-provider", nil))
	providerEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Ctrl: &genx.StreamCtrl{
			StreamID: "output-provider", Label: "assistant", EndOfStream: true,
			Error: "private provider detail", FailureClass: genx.FailureClassProvider,
		},
	}
	attachTestResponseEpochEnd(genx.NewResponseEpoch("input-provider"), providerEOS)
	lifecycle.observeOutputProduced(providerEOS)
	lifecycle.finish("agent_output", context.Canceled)

	var terminals []map[string]any
	for _, record := range capturedTurnLifecycleRecords(t, capture) {
		attrs := lifecycleRecordAttrs(record)
		if attrs["stage"] == "agent_terminal" {
			terminals = append(terminals, attrs)
		}
		if strings.Contains(fmt.Sprint(attrs), "private provider detail") {
			t.Fatalf("terminal exposed raw provider error: %#v", attrs)
		}
	}
	if len(terminals) != 1 || terminals[0]["terminal_class"] != "provider_error" || terminals[0]["delivered_modalities"] != "" {
		t.Fatalf("provider terminals = %#v", terminals)
	}
}

func TestPeerStreamLifecycleConcurrentPeersDoNotExchangeTurnState(t *testing.T) {
	type peerFixture struct {
		session string
		peer    string
		input   string
		output  string
		capture *slogCapture
	}
	fixtures := []peerFixture{
		{session: "session-a", peer: "peer-a", input: "input-a-secret", output: "output-a-secret", capture: &slogCapture{}},
		{session: "session-b", peer: "peer-b", input: "input-b-secret", output: "output-b-secret", capture: &slogCapture{}},
	}
	var wg sync.WaitGroup
	for _, fixture := range fixtures {
		wg.Go(func() {
			lifecycle := newPeerStreamLifecycle(slog.New(fixture.capture), fixture.session, fixture.peer)
			lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, fixture.input, nil))
			input := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: fixture.input}}
			lifecycle.observeAgentInputPush(input)
			lifecycle.observeAgentTransformStarted(input)
			text := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("private"), Ctrl: &genx.StreamCtrl{StreamID: fixture.output, Label: "assistant"}}
			attachTestResponseEpoch(fixture.input, text)
			lifecycle.observeOutputProduced(text)
			lifecycle.observeOutput(t.Context(), text, func(context.Context) string { return "shared-workspace" })
			lifecycle.finish("agent_output", context.Canceled)
		})
	}
	wg.Wait()

	for index, fixture := range fixtures {
		other := fixtures[1-index]
		for _, record := range capturedTurnLifecycleRecords(t, fixture.capture) {
			attrs := lifecycleRecordAttrs(record)
			if attrs["tunnel_session_id"] != fixture.session || attrs["peer_public_key"] != fixture.peer || attrs["turn_index"] != uint64(1) {
				t.Fatalf("peer %s correlation = %#v", fixture.session, attrs)
			}
			rendered := fmt.Sprint(attrs)
			if strings.Contains(rendered, other.session) || strings.Contains(rendered, other.input) || strings.Contains(rendered, other.output) {
				t.Fatalf("peer %s record contains peer %s state: %s", fixture.session, other.session, rendered)
			}
			if attrs["input_stream_id_hash"] != nil && attrs["input_stream_id_hash"] != safeStreamIDHash(fixture.input) {
				t.Fatalf("peer %s input correlation = %#v", fixture.session, attrs)
			}
			if attrs["output_stream_id_hash"] != nil && attrs["output_stream_id_hash"] != safeStreamIDHash(fixture.output) {
				t.Fatalf("peer %s output correlation = %#v", fixture.session, attrs)
			}
		}
	}
}

func TestPeerStreamLifecycleSecondTurnStallHasIndependentTerminal(t *testing.T) {
	capture := &slogCapture{}
	lifecycle := newPeerStreamLifecycle(slog.New(capture), "session-stall", "peer-stall")
	lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "first", nil))
	lifecycle.observeOutput(t.Context(), attachTestResponseEpochEnd(genx.NewResponseEpoch("first"), &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "first-output", EndOfStream: true}}), nil)
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
	epoch := genx.NewResponseEpoch("input-0")
	for range 100 {
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA, "input-0", nil))
		input := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "input-0"}}
		output := &genx.MessageChunk{Part: genx.Text("delta"), Ctrl: &genx.StreamCtrl{StreamID: "output-0"}}
		attachTestResponseEpochWith(epoch, output)
		lifecycle.observeAgentInputPush(input)
		lifecycle.observeAgentTransformStarted(input)
		lifecycle.observeOutputProduced(output)
		lifecycle.observeOutput(t.Context(), output, nil)
	}
	if got := len(capturedTurnLifecycleRecords(t, capture)); got != 7 {
		t.Fatalf("records after repeated chunks = %d, want fixed seven stages", got)
	}
	for index := 1; index <= peerStreamLifecycleMaxRetainedTurns+20; index++ {
		inputID := fmt.Sprintf("input-%d", index)
		outputID := fmt.Sprintf("output-%d", index)
		lifecycle.observeInput(peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, inputID, nil))
		lifecycle.observeOutput(t.Context(), attachTestResponseEpoch(inputID, &genx.MessageChunk{
			Part: genx.Text("open"), Ctrl: &genx.StreamCtrl{StreamID: outputID, BeginOfStream: true},
		}), nil)
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if len(lifecycle.turns) > peerStreamLifecycleMaxRetainedTurns ||
		len(lifecycle.turnOrder) > peerStreamLifecycleMaxRetainedTurns ||
		len(lifecycle.outputTurns) > peerStreamLifecycleMaxOutputRoutes ||
		len(lifecycle.outputOrder) > peerStreamLifecycleMaxOutputRoutes ||
		len(lifecycle.epochTurns) > peerStreamLifecycleMaxOutputRoutes ||
		len(lifecycle.epochOrder) > peerStreamLifecycleMaxOutputRoutes {
		t.Fatalf("retained state turns=%d order=%d routes=%d route_order=%d epochs=%d epoch_order=%d", len(lifecycle.turns), len(lifecycle.turnOrder), len(lifecycle.outputTurns), len(lifecycle.outputOrder), len(lifecycle.epochTurns), len(lifecycle.epochOrder))
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

func TestPeerAgentOutputModalityAndTerminalClassUseClosedValues(t *testing.T) {
	for _, test := range []struct {
		name     string
		chunk    *genx.MessageChunk
		modality string
		terminal string
	}{
		{name: "transcript", chunk: &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("private")}, modality: "transcript_text"},
		{name: "assistant text", chunk: &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("private")}, modality: "assistant_text"},
		{name: "assistant audio", chunk: &genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("private")}}, modality: "assistant_audio"},
		{name: "completed", chunk: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true}}, modality: "assistant_eos", terminal: "completed"},
		{name: "interrupted", chunk: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true, ErrorCode: "STREAM_INTERRUPTED"}}, modality: "interrupt", terminal: "interrupted"},
		{name: "provider", chunk: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: "private provider detail", FailureClass: genx.FailureClassProvider}}, modality: "assistant_eos", terminal: "provider_error"},
		{name: "transform", chunk: &genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: "private transform detail", ErrorCode: "AGENT_RELOAD_FAILED"}}, modality: "assistant_eos", terminal: "transform_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := peerAgentOutputModality(test.chunk); got != test.modality {
				t.Fatalf("modality = %q, want %q", got, test.modality)
			}
			if test.terminal != "" {
				if got := peerAgentTerminalClass(test.chunk, nil); got != test.terminal {
					t.Fatalf("terminal class = %q, want %q", got, test.terminal)
				}
			}
		})
	}
	for _, test := range []struct {
		err  error
		want string
	}{
		{err: context.Canceled, want: "caller_canceled"},
		{err: context.DeadlineExceeded, want: "deadline_exceeded"},
		{err: errors.New("private stream detail"), want: "stream_error"},
	} {
		if got := peerAgentTerminalClass(nil, test.err); got != test.want {
			t.Errorf("terminal class for %v = %q, want %q", test.err, got, test.want)
		}
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

func TestPeerStreamLifecycleDisabledAtInfoAvoidsAllObservationWork(t *testing.T) {
	warnLogger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if lifecycle := newPeerStreamLifecycle(warnLogger, "session-disabled", "peer-disabled"); lifecycle != nil {
		t.Fatal("Warn logger constructed a lifecycle observer")
	}

	previous := slog.Default()
	slog.SetDefault(warnLogger)
	t.Cleanup(func() { slog.SetDefault(previous) })
	if lifecycle := newPeerStreamLifecycle(nil, "session-default-disabled", "peer-default-disabled"); lifecycle != nil {
		t.Fatal("nil logger did not resolve the disabled default logger")
	}

	var lifecycle *peerStreamLifecycle
	input := peerInputEvent(eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, "input-secret", nil)
	chunk := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: genx.Text("private output"),
		Ctrl: &genx.StreamCtrl{StreamID: "output-secret", BeginOfStream: true},
	}
	workspaceCalled := false
	workspaceName := func(context.Context) string {
		workspaceCalled = true
		return "private-workspace"
	}
	allocations := testing.AllocsPerRun(1000, func() {
		lifecycle.accepted()
		lifecycle.eventStreamAccepted()
		lifecycle.observeInput(input)
		lifecycle.observeAgentInputOpen()
		lifecycle.observeAgentInputPush(chunk)
		lifecycle.observeAgentTransformStarted(chunk)
		lifecycle.observeInterrupt()
		lifecycle.observeOutputProduced(chunk)
		lifecycle.observeOutput(context.Background(), chunk, workspaceName)
		lifecycle.finish("agent_output", nil)
		lifecycle.finish("peer_input", nil)
		lifecycle.finish("server_tunnel", nil)
	})
	if allocations != 0 {
		t.Fatalf("disabled lifecycle allocations = %v, want 0", allocations)
	}
	if workspaceCalled {
		t.Fatal("disabled lifecycle resolved the Workspace callback")
	}
}

func TestPeerStreamLifecycleUsesAnyInfoEnabledHandler(t *testing.T) {
	warn := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn})
	info := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	allWarn := slog.New(&lifecycleTestFanoutHandler{handlers: []slog.Handler{warn, warn}})
	if lifecycle := newPeerStreamLifecycle(allWarn, "session-warn", "peer-warn"); lifecycle != nil {
		t.Fatal("all-Warn fanout constructed a lifecycle observer")
	}
	mixed := slog.New(&lifecycleTestFanoutHandler{handlers: []slog.Handler{warn, info}})
	if lifecycle := newPeerStreamLifecycle(mixed, "session-mixed", "peer-mixed"); lifecycle == nil {
		t.Fatal("mixed fanout did not construct a lifecycle observer")
	}
}

func attachTestResponseEpoch(inputStreamID string, chunk *genx.MessageChunk) *genx.MessageChunk {
	return attachTestResponseEpochWith(genx.NewResponseEpoch(inputStreamID), chunk)
}

func attachTestResponseEpochWith(epoch *genx.ResponseEpoch, chunk *genx.MessageChunk) *genx.MessageChunk {
	if chunk == nil {
		return nil
	}
	if chunk.Ctrl == nil {
		chunk.Ctrl = &genx.StreamCtrl{}
	}
	chunk.Ctrl.ResponseEpoch = epoch
	return chunk
}

func attachTestResponseEpochEnd(epoch *genx.ResponseEpoch, chunk *genx.MessageChunk) *genx.MessageChunk {
	attachTestResponseEpochWith(epoch, chunk)
	if chunk != nil && chunk.Ctrl != nil {
		chunk.Ctrl.ResponseEpochEnd = true
	}
	return chunk
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

type lifecycleTestFanoutHandler struct {
	handlers []slog.Handler
}

func (h *lifecycleTestFanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *lifecycleTestFanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *lifecycleTestFanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return &lifecycleTestFanoutHandler{handlers: handlers}
}

func (h *lifecycleTestFanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return &lifecycleTestFanoutHandler{handlers: handlers}
}
