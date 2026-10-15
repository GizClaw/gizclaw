package giztestcmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/minimaxtts"
	"github.com/GizClaw/minimax-go"
)

// Exercise the public Transformer, SDK HTTP streaming protocol, PCM-to-Ogg encoder
// and Giztest's existing packet-duration playback check on a real host clock.
// The remote provider is a local fixture with a controlled 400 ms first-packet
// latency. This is not a live provider or a physical playback acceptance test.
func TestTTSBatchingMiniMaxPlaybackPacing(t *testing.T) {
	for _, mode := range []string{"short_audio", "long_audio", "long_audio_cap"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			requests := make(chan ttsPacingRequest, 8)
			provider := newTTSBatchingPacingServer(t, ctx, mode, requests)
			firstFrames := 50
			if mode != "short_audio" {
				firstFrames = 150
			}
			wantPackets := firstFrames + 200
			t.Cleanup(provider.Close)
			client, err := minimax.NewClient(minimax.Config{BaseURL: provider.URL, APIKey: "local-protocol-fixture", HTTPClient: provider.Client()})
			if err != nil {
				t.Fatal(err)
			}
			transformer, err := minimaxtts.New(minimaxtts.Config{Client: client, VoiceID: "local-voice", Model: "local-speech", Format: "ogg_opus"})
			if err != nil {
				t.Fatal(err)
			}
			config := genx.DefaultTTSBatchConfig()
			if mode == "long_audio_cap" {
				config.MaxWait = 100 * time.Millisecond
			}
			ctx, err = genx.WithTTSBatching(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
			output, err := transformer.Transform(ctx, input.Stream())
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			firstAudio := make(chan time.Time, 1)
			allAudio := make(chan struct{}, 1)
			firstClip := make(chan struct{}, 1)
			readDone := make(chan error, 1)
			var pacing peerAudioPacing
			go func() {
				for {
					chunk, err := output.Next()
					if err != nil {
						readDone <- err
						return
					}
					blob, ok := chunk.Part.(*genx.Blob)
					if !ok || len(blob.Data) == 0 {
						continue
					}
					for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(blob.Data)) {
						if err != nil {
							readDone <- fmt.Errorf("read Ogg packet: %w", err)
							return
						}
						at := time.Now()
						if len(pacing.packetDurations) == 0 {
							firstAudio <- at
						}
						pacing.observe(at, [][]byte{packet})
						if len(pacing.packetDurations) == firstFrames {
							firstClip <- struct{}{}
						}
						if len(pacing.packetDurations) == wantPackets {
							allAudio <- struct{}{}
						}
					}
				}
			}()
			addText := func(text string) {
				t.Helper()
				if err := input.Add(&genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "narration"}}); err != nil {
					t.Fatal(err)
				}
			}
			wait := func(ch <-chan struct{}) {
				t.Helper()
				select {
				case <-ch:
				case err := <-readDone:
					t.Fatalf("audio ended early: %v", err)
				case <-ctx.Done():
					t.Fatal(context.Cause(ctx))
				}
			}
			addText("好的呀。后")
			var firstAt time.Time
			select {
			case firstAt = <-firstAudio:
			case err := <-readDone:
				t.Fatalf("audio ended early: %v", err)
			case <-ctx.Done():
				t.Fatal(context.Cause(ctx))
			}
			if mode != "short_audio" {
				wait(firstClip)
			}
			time.Sleep(max(0, time.Until(firstAt.Add(50*time.Millisecond))))
			// A real observed space confirms the sentence; leave no unreadable
			// lookahead that could become a third synthesis when input closes.
			addText("面的内容已经安排好了。 ")
			wait(allAudio)
			if err := input.Done(genx.Usage{}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-readDone:
				if !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
					t.Fatalf("TTS output: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(context.Cause(ctx))
			}
			summary := pacing.summary()
			if summary["packets"] != wantPackets || summary["audio_ms"] != int64(wantPackets*20) {
				t.Fatalf("unexpected audio: %#v", summary)
			}
			if summary["underruns"] != 0 {
				t.Fatalf("automatic audio budget underruns: %#v", summary)
			}
			firstRequest, nextRequest := <-requests, <-requests
			if firstRequest.text != "好的呀。" || nextRequest.text != "后面的内容已经安排好了。" {
				t.Fatalf("unexpected synthesis batches: %+v / %+v", firstRequest, nextRequest)
			}
			delay := nextRequest.at.Sub(firstAt)
			if mode == "long_audio" {
				if delay < 2*time.Second || delay > 3*time.Second {
					t.Fatalf("three seconds of audio did not allow a long collection budget: %s", delay)
				}
			} else if delay > 700*time.Millisecond {
				t.Fatalf("short/explicitly capped budget waited too long: %s", delay)
			}
			summary["next_submission_after_first_audio_ms"] = float64(delay) / float64(time.Millisecond)
			t.Logf("real MiniMax adapter with local protocol fixture: %#v", summary)
		})
	}
}

type ttsPacingRequest struct {
	text string
	at   time.Time
}

func newTTSBatchingPacingServer(t *testing.T, ctx context.Context, mode string, requests chan<- ttsPacingRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serve := func() error {
			var message map[string]any
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				return err
			}
			options, _ := message["stream_options"].(map[string]any)
			if r.Method != http.MethodPost || r.URL.Path != "/v1/t2a_v2" || message["stream"] != true || options["exclude_aggregated_audio"] != true {
				return fmt.Errorf("invalid HTTP stream request: %s %v", r.URL.Path, message)
			}
			audio, _ := message["audio_setting"].(map[string]any)
			if audio["format"] != "pcm" || audio["sample_rate"] != float64(16000) {
				return fmt.Errorf("expected canonical 16kHz PCM request, got %v", audio)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			write := func(audio []byte, final bool) error {
				status := 1
				if final {
					status = 2
				}
				payload, err := json.Marshal(map[string]any{"data": map[string]any{"audio": hex.EncodeToString(audio), "status": status}, "base_resp": map[string]any{"status_code": 0}})
				if err != nil {
					return err
				}
				if _, err = fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
					return err
				}
				w.(http.Flusher).Flush()
				return nil
			}
			text, _ := message["text"].(string)
			requests <- ttsPacingRequest{text: text, at: time.Now()}
			count := 200
			if text == "好的呀。" {
				count = 50
				if mode != "short_audio" {
					count = 150
				}
			} else if text != "后面的内容已经安排好了。" {
				return fmt.Errorf("unexpected synthesis batch %q", text)
			}
			timer := time.NewTimer(400 * time.Millisecond)
			defer timer.Stop()
			for i := range count {
				if i > 0 {
					timer.Reset(4 * time.Millisecond)
				}
				select {
				case <-timer.C:
				case <-ctx.Done():
					return context.Cause(ctx)
				case <-r.Context().Done():
					return context.Cause(r.Context())
				}
				if text == "好的呀。" && mode != "short_audio" {
					// Deliver a complete clip in controlled chunks so audio duration, not
					// the number of network writes, determines the next submission budget.
					for remaining := count; remaining > 0; {
						frames := min(24, remaining)
						if err := write(bytes.Repeat([]byte{0x00, 0x10}, 320*frames), false); err != nil {
							return err
						}
						remaining -= frames
					}
					break
				}
				if err := write(bytes.Repeat([]byte{0x00, 0x10}, 320), false); err != nil {
					return err
				}
			}
			return write(nil, true)
		}
		if err := serve(); err != nil && ctx.Err() == nil && r.Context().Err() == nil {
			t.Errorf("local speech protocol: %v", err)
		}
	}))
}
