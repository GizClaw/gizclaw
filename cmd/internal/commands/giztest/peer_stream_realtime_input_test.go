package giztestcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

type realtimeInputProbe struct {
	*fakeRelayStream
	push func(*genx.MessageChunk) error
}

func (s *realtimeInputProbe) Push(_ context.Context, chunk *genx.MessageChunk) error {
	return s.push(chunk)
}

func TestRealtimeSilenceRetainsRouteAndStopsOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		stream := newFakeRelayStream()
		defer stream.Close()
		var sent atomic.Int64
		packet := []byte{0xf8, 0xff, 0xfe}
		done := make(chan error, 1)
		go func() { done <- pushRealtimeSilence(ctx, stream, "microphone", "user", packet, &sent) }()
		synctest.Wait()
		time.Sleep(60 * time.Millisecond)
		synctest.Wait()
		if sent.Load() != 3 {
			t.Fatalf("sent %d silence packets, want three paced frames", sent.Load())
		}
		for range 3 {
			chunk := <-stream.pushes
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok || !bytes.Equal(blob.Data, packet) || chunk.Ctrl.StreamID != "microphone" || chunk.Ctrl.Label != "user" || chunk.IsBeginOfStream() || chunk.IsEndOfStream() {
				t.Fatalf("silence changed the microphone route: %#v", chunk)
			}
		}
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
		time.Sleep(time.Second)
		if sent.Load() != 3 {
			t.Fatal("cancelled microphone continued uploading")
		}
	})
}

func TestPeerStreamRealtimeContinuesInputWhileReplyDrains(t *testing.T) {
	ogg, packets := testOggOpus(t)
	for _, outcome := range []string{"reply", "upload-failure", "idle-timeout"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				stream := &realtimeInputProbe{fakeRelayStream: newFakeRelayStream()}
				dataPackets, begins := 0, 0
				initialPackets := len(packets) + int(realtimeTailSilence/(20*time.Millisecond))
				uploadErr := errors.New("silence upload failed")
				stream.push = func(chunk *genx.MessageChunk) error {
					if chunk.IsEndOfStream() || chunk.Ctrl.Label != "microphone" {
						return fmt.Errorf("unexpected microphone boundary: %#v", chunk.Ctrl)
					}
					if chunk.IsBeginOfStream() {
						begins++
						return nil
					}
					dataPackets++
					if dataPackets > initialPackets && outcome == "upload-failure" {
						return uploadErr
					}
					if dataPackets == initialPackets+3 && outcome == "reply" {
						stream.in <- assistantText("reply", "translation", true)
					}
					return nil
				}
				step := giztest.Step{ID: "turn", Client: "peer", PeerStream: &giztest.PeerStreamOperation{
					Mode: "realtime", Label: "microphone", Pacing: "1ms", RequireAudio: new(false),
				}}
				if outcome == "idle-timeout" {
					step.PeerStream.IdleTimeout = "100ms"
				}
				result, err := invokePeerStream(ctx, nil, func() (peerStream, error) { return stream, nil }, step, ogg, 0)
				atReturn := dataPackets
				time.Sleep(time.Second)
				if dataPackets != atReturn {
					t.Fatal("completed operation retained a microphone uploader")
				}
				if outcome == "idle-timeout" {
					if err == nil || !strings.Contains(err.Error(), "deadline=idle_timeout") || dataPackets <= initialPackets {
						t.Fatalf("silence suppressed the output inactivity deadline: %v, packets=%d", err, dataPackets)
					}
					return
				}
				if outcome == "upload-failure" {
					if !errors.Is(err, uploadErr) {
						t.Fatalf("upload failure = %v, want %v", err, uploadErr)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				object := result.assertion.(map[string]any)
				if begins != 1 || dataPackets < initialPackets+3 || object["input_packets"] != len(packets) || object["pushed_packets"] != dataPackets || object["text_eos"] != true {
					t.Fatalf("microphone stopped before reply completion: packets=%d begins=%d result=%#v", dataPackets, begins, object)
				}
			})
		})
	}
}
