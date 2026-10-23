package giztestcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// Hold the first input push until the reader has timestamped an earlier
// response's tail. This proves arrival order without wall-clock sleeps.
type gatedOverlapTextStream struct {
	*fakeRelayStream
	started, release, tailRead chan struct{}
	firstPush                  bool
}

func (s *gatedOverlapTextStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if !s.firstPush {
		s.firstPush = true
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.fakeRelayStream.Push(ctx, chunk)
}

func (s *gatedOverlapTextStream) observePacketArrival(chunk *genx.MessageChunk, _ time.Time) {
	if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID == "earlier" && chunk.IsEndOfStream() {
		select {
		case s.tailRead <- struct{}{}:
		default:
		}
	}
}

func TestOverlappingTextInputIgnoresEarlierAudio(t *testing.T) {
	for _, ended := range []bool{false, true} {
		t.Run(fmt.Sprintf("current_response_already_ended=%t", ended), func(t *testing.T) {
			stream := &gatedOverlapTextStream{fakeRelayStream: newFakeRelayStream(), started: make(chan struct{}), release: make(chan struct{}), tailRead: make(chan struct{}, 1)}
			defer stream.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			type outcome struct {
				result operationResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				r, err := invokeOverlappingPeerInput(ctx, stream, &giztest.PeerStreamOperation{Mode: "text", OverlapInput: true, Pacing: "0s"}, []any{"slow-first", "replacement"})
				done <- outcome{r, err}
			}()
			select {
			case <-stream.started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			stream.in <- assistantBlob("earlier", testAudibleOpus(t), false)
			stream.in <- assistantBlob("earlier", nil, true)
			select {
			case <-stream.tailRead:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			close(stream.release)
			for range 3 {
				select {
				case <-stream.pushes:
				case got := <-done:
					t.Fatalf("earlier audio ended the current turn: %v", got.err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			// More content on an identified earlier response stays excluded even
			// after the new input has committed.
			stream.in <- assistantBlob("earlier", testAudibleOpus(t), false)
			stream.in <- assistantBlob("earlier", nil, true)
			stream.in <- assistantText("first", "current", false)
			stream.in <- assistantBlob("first", testAudibleOpus(t), ended)
			if ended {
				select {
				case got := <-done:
					if got.err == nil || !strings.Contains(got.err.Error(), "first audio EOS") {
						t.Fatalf("premature current response passed: %v", got.err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				return
			}
			for range 3 {
				select {
				case <-stream.pushes:
				case got := <-done:
					t.Fatalf("replacement input missing: %v", got.err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			finishAssistantTurn(stream.fakeRelayStream, "first")
			stream.in <- assistantText("second", "replacement", false)
			stream.in <- assistantBlob("second", testAudibleOpus(t), false)
			finishAssistantTurn(stream.fakeRelayStream, "second")
			select {
			case got := <-done:
				if got.err != nil {
					t.Fatal(got.err)
				}
				if got.result.evidence["input_overlap"] != true {
					t.Fatalf("overlap was not proven: %+v", got.result.evidence)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

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
				if mode == "push-to-talk" {
					audioBOS := nextPush(t, stream)
					if firstBOS.Part != nil || !audioBOS.IsBeginOfStream() || audioBOS.Ctrl.StreamID != firstBOS.Ctrl.StreamID {
						t.Fatal("missing same-input control/audio BOS")
					}
				}
				nextPush(t, stream)
				// Realtime can announce an empty provisional response before ASR.
				empty := assistantText("provisional", "", false)
				empty.Ctrl.BeginOfStream = true
				stream.in <- empty
				stream.in <- assistantText("provisional", "", true)
				stream.in <- assistantText("first", "story", false)
				stream.in <- assistantBlob("first", testAudibleOpus(t), outcome == "already-ended")
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
				if mode == "push-to-talk" {
					if chunk := nextPush(t, stream); !chunk.IsBeginOfStream() {
						t.Fatal("missing second audio BOS")
					}
				}
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
					stream.in <- assistantBlob("second", testAudibleOpus(t), false)
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
	for _, workflow := range []string{"doubao-realtime-conversation", "eino-concurrency-assistant", "eino-voice-assistant"} {
		for _, mode := range []string{"push-to-talk", "realtime"} {
			name := "../../../../tests/gizclaw-e2e/giztest/" + workflow + "." + mode + "-overlapping-input.giztest.yaml"
			if _, err := giztest.LoadDocument(name, newDriver(false, nil)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}
