package giztestcmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestPeerStreamRearmRejectsKnownResponseTerminalErrors(t *testing.T) {
	for _, afterRearm := range []bool{false, true} {
		for _, terminal := range []genx.StreamCtrl{
			{ErrorCode: "STREAM_ERROR", Error: "provider failed"},
			{Error: "interrupted"},
			{ErrorCode: "MODEL_ERROR"},
		} {
			t.Run(fmt.Sprintf("after_rearm=%t/code=%s/message=%s", afterRearm, terminal.ErrorCode, terminal.Error), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				stream := newFakeRelayStream()
				sessions := newPeerStreamSessions()
				defer sessions.Close()
				session := newPeerStreamSession("peer", stream)
				session.streamID = "old-input"
				// This ID was already observed in the previous operation. Its
				// payload is excluded, but re-arm must still reject its error EOS.
				session.history.observe(assistantText("old-response", "opening", false), time.Now())
				session.startReader()
				if err := sessions.add("microphone", session); err != nil {
					t.Fatal(err)
				}
				terminal.StreamID, terminal.Label, terminal.EndOfStream = "old-response", "assistant", true
				failed := &genx.MessageChunk{Part: genx.Text(""), Ctrl: &terminal}
				if afterRearm {
					stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{
						StreamID: "old-input", Label: "user", EndOfStream: true,
						ErrorCode: "INPUT_ROUTE_RELOADED", Error: "input route reloaded", ErrorRetryable: true,
					}}
					done := make(chan struct{})
					defer func() { cancel(); <-done }()
					go func() {
						defer close(done)
						for i := range 202 {
							select {
							case <-stream.pushes:
								if i == 0 {
									stream.in <- failed
								}
							case <-ctx.Done():
								return
							}
						}
						// A good replacement must not hide the old response's error.
						finishAssistantTurn(stream, "replacement")
					}()
				} else {
					stream.in <- failed
				}
				result, err := invokePeerStreamWithSessions(ctx, nil, func() (peerStream, error) {
					t.Fatal("re-arm must reuse its retained session")
					return nil, nil
				}, sessions, giztest.Step{ID: "rearm", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
					Mode: "realtime", Session: "microphone", AwaitRearm: "INPUT_ROUTE_RELOADED",
				}}, []byte{0xf8}, 0)
				if err == nil || !strings.Contains(err.Error(), "assistant terminal error") {
					t.Fatalf("known response terminal was hidden: error=%v evidence=%v", err, result.evidence)
				}
				if result.evidence["reload_eos_observed"] != afterRearm {
					t.Fatalf("reload evidence=%v, want %t", result.evidence, afterRearm)
				}
				select {
				case <-stream.closed:
				default:
					t.Fatal("failed retained session was not closed")
				}
			})
		}
	}
}

func TestPeerStreamPTTInterruptPreservesAdmittedTerminal(t *testing.T) {
	for _, providerError := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider_error=%t", providerError), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			packet := testAudibleOpus(t)
			first := &replyAtCommitStream{fakeRelayStream: newFakeRelayStream(), chunks: []*genx.MessageChunk{
				assistantText("first", "first reply", false), assistantBlob("first", packet, false),
			}}
			first.chunks[0].Ctrl.BeginOfStream, first.chunks[1].Ctrl.BeginOfStream = true, true
			terminal := assistantText("first", "", true)
			terminal.Ctrl.Error = "interrupted"
			if providerError {
				terminal.Ctrl.ErrorCode, terminal.Ctrl.Error = "MODEL_ERROR", "provider failed"
			}
			replacement := &replyAtCommitStream{fakeRelayStream: newFakeRelayStream(), chunks: []*genx.MessageChunk{
				assistantText("first", "late payload", false), assistantBlob("first", packet, false), terminal, assistantBlob("first", nil, true),
				assistantText("second", "replacement", false), assistantBlob("second", packet, false),
				assistantText("second", "", true), assistantBlob("second", nil, true),
			}}
			replacement.chunks[4].Ctrl.BeginOfStream, replacement.chunks[5].Ctrl.BeginOfStream = true, true
			defer first.Close()
			defer replacement.Close()
			opened := 0
			input, _ := testOggOpus(t)
			result, err := invokePeerStream(ctx, nil, func() (peerStream, error) {
				opened++
				if opened == 1 {
					return first, nil
				}
				return replacement, nil
			}, giztest.Step{ID: "interrupt", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
				Mode: "push-to-talk", HoldBeforeAudio: "0ms", Pacing: "0ms", InterruptAfter: "1ms",
			}}, input, 0)
			if providerError {
				if err == nil || !strings.Contains(err.Error(), "MODEL_ERROR") {
					t.Fatalf("admitted response provider error was hidden: error=%v evidence=%v", err, result.evidence)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			object := result.assertion.(map[string]any)
			if opened != 2 || object["interrupted"] != true || object["interrupt_observed"] != true || object["response_count"] != 1 || result.evidence["response_count"] != 1 {
				t.Fatalf("interruption bookkeeping: opened=%d assertion=%v evidence=%v", opened, object, result.evidence)
			}
			if object["reply"] != "first replyreplacement" || object["audio_bytes"] != 2*len(packet) {
				t.Fatalf("later old payload was included: %v", object)
			}
			if integrity := object["audio_integrity"].(map[string]any); integrity["open"] != 0 || integrity["violations"] != 0 {
				t.Fatalf("admitted terminal did not close its audio lifecycle: %#v", integrity)
			}
		})
	}
}
