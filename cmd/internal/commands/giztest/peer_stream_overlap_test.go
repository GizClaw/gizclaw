package giztestcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestOverlappingPeerInput(t *testing.T) {
	for _, mode := range []string{"push-to-talk", "realtime"} {
		for _, outcome := range []string{"complete", "interrupted", "already-ended", "second-error", "second-code-error", "second-no-audio", "missing-second-audio"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				audio, _ := testOggOpus(t)
				stream := newFakeRelayStream()
				defer stream.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					opens := 0
					result, err := invokePeerStream(ctx, nil, func() (peerStream, error) {
						opens++
						return stream, nil
					}, giztest.Step{PeerStream: &giztest.PeerStreamOperation{Mode: mode, Pacing: "0s", OverlapInput: true}}, audio, 0)
					if opens != 1 {
						done <- fmt.Errorf("opened %d streams", opens)
						return
					}
					if err == nil && result.evidence["input_overlap"] != true {
						done <- context.Canceled
						return
					}
					done <- err
				}()
				firstBOS := nextPush(t, stream)
				if !firstBOS.IsBeginOfStream() {
					t.Fatal("missing first BOS")
				}
				nextPush(t, stream)
				// Realtime can announce an empty provisional response before ASR.
				empty := assistantText("provisional", "", false)
				empty.Ctrl.BeginOfStream = true
				stream.in <- empty
				stream.in <- assistantText("provisional", "", true)
				stream.in <- assistantText("first", "story", false)
				stream.in <- assistantBlob("first", []byte{1}, outcome == "already-ended")
				// Drain the first input and wait for a fresh BOS on the SAME stream.
				for {
					select {
					case err := <-done:
						if outcome != "already-ended" || err == nil || !strings.Contains(err.Error(), "first audio EOS") {
							t.Fatalf("early completion: %v", err)
						}
						return
					case chunk := <-stream.pushes:
						if chunk.IsBeginOfStream() {
							if chunk.Ctrl.StreamID == firstBOS.Ctrl.StreamID {
								t.Fatal("second input reused message ID")
							}
							goto second
						}
					case <-ctx.Done():
						t.Fatal("second input did not start")
					}
				}
			second:
				// Wait for two packets so the first successful audio Push has returned.
				nextPush(t, stream)
				nextPush(t, stream)
				firstEOS := assistantBlob("first", nil, true)
				if outcome == "interrupted" {
					firstEOS.Ctrl.Error = "interrupted"
				}
				stream.in <- assistantText("first", "", true)
				stream.in <- firstEOS
				// Orphan terminals must not stand in for the second response.
				stream.in <- assistantText("stale", "", true)
				stream.in <- assistantBlob("stale", nil, true)
				stream.in <- assistantText("second", "reply", false)
				if outcome != "second-no-audio" {
					stream.in <- assistantBlob("second", []byte{1}, false)
				}
				stream.in <- assistantText("second", "", true)
				if outcome != "missing-second-audio" {
					eos := assistantBlob("second", nil, true)
					if outcome == "second-error" {
						eos.Ctrl.Error = "provider failed"
					}
					if outcome == "second-code-error" {
						eos.Ctrl.ErrorCode = "PROVIDER_FAILED"
					}
					stream.in <- eos
				}
				for {
					select {
					case <-stream.pushes:
					case err := <-done:
						if outcome == "complete" || outcome == "interrupted" {
							if err != nil {
								t.Fatal(err)
							}
						} else if err == nil {
							t.Fatal("invalid overlap passed")
						}
						if outcome == "missing-second-audio" && !errors.Is(err, context.DeadlineExceeded) {
							t.Fatal(err)
						}
						wantError := map[string]string{"second-error": "provider failed", "second-code-error": "PROVIDER_FAILED", "second-no-audio": "did not complete with text and audio"}[outcome]
						if wantError != "" && !strings.Contains(err.Error(), wantError) {
							t.Fatal(err)
						}
						return
					}
				}
			})
		}
	}
}

func TestOverlappingInputDocuments(t *testing.T) {
	for _, workflow := range []string{"doubao-realtime-conversation", "eino-concurrency-assistant", "flowcraft-voice-assistant"} {
		for _, mode := range []string{"push-to-talk", "realtime"} {
			name := "../../../../tests/gizclaw-e2e/giztest/" + workflow + "." + mode + "-overlapping-input.giztest.yaml"
			if _, err := giztest.LoadDocument(name, newDriver(false, nil)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}
