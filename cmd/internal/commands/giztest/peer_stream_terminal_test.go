package giztestcmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func revokedTerminalExpectation() *giztest.PeerStreamTerminalErrorExpectation {
	return &giztest.PeerStreamTerminalErrorExpectation{
		Code: "AGENT_OUTPUT_ERROR", Message: "sfu: workspace access revoked",
		StreamID: "agent-output-error", Label: "agent", Retryable: true,
	}
}

func revokedTerminalChunk() *genx.MessageChunk {
	return &genx.MessageChunk{Ctrl: &genx.StreamCtrl{
		ErrorCode: "AGENT_OUTPUT_ERROR", Error: "sfu: workspace access revoked",
		StreamID: "agent-output-error", Label: "agent", ErrorRetryable: true, EndOfStream: true,
	}}
}

func TestListenExpectedTerminalKeepsWindowAndAllAudio(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	step := listenStep("60ms")
	step.PeerStream.AllowedTerminalError = revokedTerminalExpectation()
	terminal := revokedTerminalChunk()
	packets := testAudibleOpusPackets(t, 2)
	terminal.Part = &genx.Blob{MIMEType: "audio/opus", Data: packets[0]}
	stream.in <- terminal
	stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packets[1]}}
	started := time.Now()
	result, err := listenPeerStream(t.Context(), stream, step, 0)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 60*time.Millisecond {
		t.Fatal("expected terminal shortened the listen window")
	}
	object := result.assertion.(map[string]any)
	if object["audio_bytes"] != len(packets[0])+len(packets[1]) || object["packets"] != 2 {
		t.Fatalf("terminal hid audio: %#v", object)
	}
	if result.evidence["terminal_errors"] != 1 || result.evidence["accepted_terminal_errors"] != 1 {
		t.Fatalf("terminal evidence = %#v", result.evidence)
	}
	events := result.evidence["terminal_error_events"].([]map[string]any)
	if events[0]["stream_id"] != "agent-output-error" || events[0]["code"] != "AGENT_OUTPUT_ERROR" || events[0]["accepted"] != true {
		t.Fatalf("terminal correlation = %#v", events)
	}
}

func TestListenExpectedTerminalRejectsOtherAndRepeatedFailures(t *testing.T) {
	for name, change := range map[string]func(*genx.MessageChunk){
		"code":      func(c *genx.MessageChunk) { c.Ctrl.ErrorCode = "SFU_ACCESS_CHECK_FAILED" },
		"message":   func(c *genx.MessageChunk) { c.Ctrl.Error += " unexpectedly" },
		"stream":    func(c *genx.MessageChunk) { c.Ctrl.StreamID = "new-runtime" },
		"label":     func(c *genx.MessageChunk) { c.Ctrl.Label = "another-participant" },
		"retryable": func(c *genx.MessageChunk) { c.Ctrl.ErrorRetryable = false },
		"non-EOS":   func(c *genx.MessageChunk) { c.Ctrl.EndOfStream = false },
		"repeat":    func(*genx.MessageChunk) {},
	} {
		t.Run(name, func(t *testing.T) {
			stream := newFakeRelayStream()
			defer stream.Close()
			step := listenStep("1s")
			step.PeerStream.AllowedTerminalError = revokedTerminalExpectation()
			if name == "repeat" {
				stream.in <- revokedTerminalChunk()
			}
			terminal := revokedTerminalChunk()
			change(terminal)
			stream.in <- terminal
			result, err := listenPeerStream(t.Context(), stream, step, 0)
			if err == nil || !strings.Contains(err.Error(), "terminal error") {
				t.Fatalf("unrelated terminal accepted: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestListenExpectedTerminalStillRejectsCloseAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		stream := newFakeRelayStream()
		step := listenStep("1s")
		step.PeerStream.AllowedTerminalError = revokedTerminalExpectation()
		stream.in <- revokedTerminalChunk()
		ctx, stop := context.WithCancel(t.Context())
		if cancel {
			stop()
		} else {
			stream.Close()
		}
		_, err := listenPeerStream(ctx, stream, step, 0)
		stop()
		stream.Close()
		if err == nil {
			t.Fatal("expected terminal concealed an early close or cancellation")
		}
	}
}

func TestListenTerminalRemainsFailureByDefault(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	stream.in <- revokedTerminalChunk()
	if _, err := listenPeerStream(t.Context(), stream, listenStep("1s"), 0); err == nil {
		t.Fatal("undeclared revocation error was accepted")
	}
}
