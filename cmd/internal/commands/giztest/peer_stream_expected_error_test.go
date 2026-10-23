package giztestcmd

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func bindingErrorStep() giztest.Step {
	return giztest.Step{ID: "missing", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
		Mode: "text", Input: "hello", ExpectedTerminalError: &giztest.PeerStreamErrorExpectation{
			Code: "MODEL_REQUEST_BINDING_MISSING", Message: "Model request metadata could not be resolved.",
			Label: "assistant", Retryable: false,
		},
	}}
}

func bindingErrorChunk() *genx.MessageChunk {
	return &genx.MessageChunk{Ctrl: &genx.StreamCtrl{
		StreamID: "server-response", Label: "assistant", EndOfStream: true,
		ErrorCode: "MODEL_REQUEST_BINDING_MISSING", Error: "Model request metadata could not be resolved.",
	}}
}

func TestPeerStreamExpectedErrorRecordsTerminalAndPayload(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	terminal := bindingErrorChunk()
	terminal.Part = genx.Text("partial response")
	stream.in <- terminal
	result, err := invokePeerStream(t.Context(), nil, func() (peerStream, error) { return stream, nil }, bindingErrorStep(), "hello", 0)
	if err != nil {
		t.Fatal(err)
	}
	object := result.assertion.(map[string]any)
	got := object["terminal_error"].(map[string]any)
	if got["code"] != "MODEL_REQUEST_BINDING_MISSING" || got["stream_id"] != "server-response" || got["end_of_stream"] != true || got["retryable"] != false {
		t.Fatalf("terminal details = %#v", got)
	}
	if object["reply"] != "partial response" || result.evidence["terminal_error"] == nil {
		t.Fatalf("terminal hid payload or evidence: %#v", result)
	}
}

func TestPeerStreamExpectedErrorRejectsOtherOutcomes(t *testing.T) {
	for name, change := range map[string]func(*genx.MessageChunk){
		"code":      func(c *genx.MessageChunk) { c.Ctrl.ErrorCode = "OTHER_ERROR" },
		"message":   func(c *genx.MessageChunk) { c.Ctrl.Error = "other failure" },
		"label":     func(c *genx.MessageChunk) { c.Ctrl.Label = "transcript" },
		"retryable": func(c *genx.MessageChunk) { c.Ctrl.ErrorRetryable = true },
		"stream":    func(c *genx.MessageChunk) { c.Ctrl.StreamID = "" },
		"non-EOS":   func(c *genx.MessageChunk) { c.Ctrl.EndOfStream = false },
		"success": func(c *genx.MessageChunk) {
			c.Ctrl.ErrorCode, c.Ctrl.Error = "", ""
			c.Part = genx.Text("success is not the expected failure")
		},
		"silence": nil,
	} {
		t.Run(name, func(t *testing.T) {
			stream := newFakeRelayStream()
			defer stream.Close()
			if change != nil {
				terminal := bindingErrorChunk()
				change(terminal)
				stream.in <- terminal
				stream.in <- assistantText("server-response", "", true)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			if _, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return stream, nil }, bindingErrorStep(), "hello", 0); err == nil {
				t.Fatal("unexpected outcome passed the terminal error assertion")
			}
		})
	}
}

func TestPeerStreamExpectedErrorCountsTerminalAudio(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	begin := assistantBlob("server-response", nil, false)
	begin.Ctrl.BeginOfStream = true
	stream.in <- begin
	packet := testAudibleOpus(t)
	terminal := bindingErrorChunk()
	terminal.Part = &genx.Blob{MIMEType: "audio/opus", Data: packet}
	stream.in <- terminal
	result, err := invokePeerStream(t.Context(), nil, func() (peerStream, error) { return stream, nil }, bindingErrorStep(), "hello", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.assertion.(map[string]any)["audio_bytes"] != len(packet) {
		t.Fatalf("terminal hid audio: %#v", result.assertion)
	}
}

func TestPeerStreamTerminalErrorStillFailsWithoutExpectation(t *testing.T) {
	stream := newFakeRelayStream()
	defer stream.Close()
	stream.in <- bindingErrorChunk()
	step := bindingErrorStep()
	step.PeerStream.ExpectedTerminalError = nil
	if _, err := invokePeerStream(t.Context(), nil, func() (peerStream, error) { return stream, nil }, step, "hello", 0); err == nil {
		t.Fatal("terminal error passed without an explicit expectation")
	}
}
