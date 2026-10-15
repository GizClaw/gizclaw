package streamkit

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// The fixture streams 20 ms PCM frames through the production text batcher and
// ordered emitter. The receiver consumes that audio on its own media clock,
// after collecting the same 500 ms prebuffer used by Giztest's pacing check.
// A synthesis completing, or a first provider chunk arriving, does not imply
// that the receiver has enough audio to cover the next request's latency.
func TestTTSBatchingPlaybackPacing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wait        time.Duration
		latency     time.Duration
		firstFrames int
		textDelay   time.Duration
		framePeriod time.Duration
		laterPeriod time.Duration
		laterStall  time.Duration
		useBudget   bool
		wantStall   time.Duration
	}{
		{name: "short_start_fixed_1s", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, wantStall: 754 * time.Millisecond},
		{name: "short_start_fixed_200ms", wait: 200 * time.Millisecond, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond},
		{name: "slow_first_packet_fixed_200ms", wait: 200 * time.Millisecond, latency: 600 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, wantStall: 154 * time.Millisecond},
		{name: "short_start_buffer_budget", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, useBudget: true},
		{name: "slow_first_packet_buffer_budget", wait: time.Second, latency: 600 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, useBudget: true},
		{name: "long_start_fixed_1s", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 150, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond},
		{name: "long_start_buffer_budget", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 150, textDelay: 500 * time.Millisecond, framePeriod: 4 * time.Millisecond, useBudget: true},
		{name: "text_starvation_fixed_1s", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 2 * time.Second, framePeriod: 4 * time.Millisecond, wantStall: 2704 * time.Millisecond},
		{name: "text_starvation_buffer_budget", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 2 * time.Second, framePeriod: 4 * time.Millisecond, useBudget: true, wantStall: 1704 * time.Millisecond},
		{name: "provider_too_slow_even_without_wait", wait: time.Second, latency: time.Second, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, useBudget: true, wantStall: 354 * time.Millisecond},
		{name: "provider_stream_stalls_after_first_packet", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, laterStall: time.Second, useBudget: true, wantStall: 754 * time.Millisecond},
		{name: "provider_stream_slower_than_playback", wait: time.Second, latency: 400 * time.Millisecond, firstFrames: 30, textDelay: 50 * time.Millisecond, framePeriod: 4 * time.Millisecond, laterPeriod: 30 * time.Millisecond, useBudget: true, wantStall: 1744 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var trace ttsPlaybackTrace
				config := genx.DefaultTTSBatchConfig()
				config.MaxRunes, config.MaxWait = 1000, tc.wait
				if tc.useBudget {
					// This deliberately assumes playback began at the first frame,
					// not after the prebuffer, so the estimate is conservative. The
					// latency is fixture knowledge, not a production measurement.
					config.WaitBudget = func() time.Duration {
						return trace.available(time.Now()) - tc.latency - 50*time.Millisecond
					}
				}
				ctx, err := genx.WithTTSBatching(ctx, config)
				if err != nil {
					t.Fatal(err)
				}
				input := NewOutput(OutputConfig{})
				calls := make(chan ttsBatchCall, 8)
				output := NewTTSStream(ctx, input, OutputConfig{}, "audio/pcm;rate=16000", func(ctx context.Context, text string, meta TTSMeta, _ string, emit func([]byte) error) error {
					calls <- ttsBatchCall{text: text, meta: meta, at: time.Now()}
					timer := time.NewTimer(tc.latency)
					defer timer.Stop()
					select {
					case <-timer.C:
					case <-ctx.Done():
						return context.Cause(ctx)
					}
					count, period := 200, 20*time.Millisecond
					if meta.SegmentIndex == 1 {
						count, period = tc.firstFrames, tc.framePeriod
					} else if tc.laterPeriod > 0 {
						period = tc.laterPeriod
					}
					for i := range count {
						if i > 0 {
							wait := period
							if i == 30 && meta.SegmentIndex == 2 {
								wait += tc.laterStall
							}
							timer.Reset(wait)
							select {
							case <-timer.C:
							case <-ctx.Done():
								return context.Cause(ctx)
							}
						}
						// 16 kHz, mono, signed 16-bit PCM: 640 bytes = 20 ms.
						if err := emit(make([]byte, 640)); err != nil {
							return err
						}
					}
					return nil
				})
				firstAudio := make(chan time.Time, 1)
				readDone := make(chan error, 1)
				go func() {
					for {
						chunk, err := output.Next()
						if err != nil {
							readDone <- err
							return
						}
						if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
							if trace.firstAt.Load() == 0 {
								trace.firstAt.Store(time.Now().UnixNano())
								firstAudio <- time.Now()
							}
							duration := time.Duration(len(blob.Data)) * time.Second / 32000
							trace.received.Add(int64(duration))
							trace.frames = append(trace.frames, ttsPlaybackFrame{at: time.Now(), duration: duration})
						}
					}
				}()
				push := func(text string, end bool) {
					t.Helper()
					if err := input.Push(&genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "narration", EndOfStream: end}}); err != nil {
						t.Fatal(err)
					}
					synctest.Wait()
				}
				push("好的呀。后", false)
				firstCall := <-calls
				at := <-firstAudio
				synctest.Wait()
				time.Sleep(max(0, at.Add(tc.textDelay).Sub(time.Now())))
				push("面先把门窗关好，再", false)
				secondCall := <-calls
				synctest.Wait()
				// Wait for the two actual syntheses, then close without adding an
				// EOS-triggered synthesis to the measured arrival trace.
				time.Sleep(tc.latency + 7*time.Second)
				synctest.Wait()
				cancel()
				_ = input.CloseWithError(context.Canceled)
				_ = output.CloseWithError(context.Canceled)
				if err := <-readDone; err != io.EOF && err != context.Canceled {
					t.Fatalf("audio reader: %v", err)
				}
				stall := trace.stall(500 * time.Millisecond)
				if firstCall.text != "好的呀。" || secondCall.text != "后面先把门窗关好，" {
					t.Fatalf("synthesis batches = %q, %q", firstCall.text, secondCall.text)
				}
				if got, want := len(trace.frames), tc.firstFrames+200; got != want {
					t.Fatalf("received %d audio frames, want %d", got, want)
				}
				if stall != tc.wantStall {
					t.Fatalf("playback underrun = %s, want %s", stall, tc.wantStall)
				}
				t.Logf("first_audio_ms=%d second_submit_after_first_audio_ms=%d frames=%d underrun_ms=%d", at.Sub(firstCall.at).Milliseconds(), secondCall.at.Sub(at).Milliseconds(), len(trace.frames), stall.Milliseconds())
			})
		})
	}
}

type ttsPlaybackFrame struct {
	at       time.Time
	duration time.Duration
}

type ttsPlaybackTrace struct {
	frames   []ttsPlaybackFrame // reader-owned; inspected only after readDone
	firstAt  atomic.Int64
	received atomic.Int64
}

func (p *ttsPlaybackTrace) available(now time.Time) time.Duration {
	firstAt := p.firstAt.Load()
	if firstAt == 0 {
		return 0
	}
	return max(0, time.Duration(p.received.Load())-now.Sub(time.Unix(0, firstAt)))
}

func (p *ttsPlaybackTrace) stall(prebuffer time.Duration) time.Duration {
	var buffered time.Duration
	var end time.Time
	var stalled time.Duration
	for _, frame := range p.frames {
		if end.IsZero() {
			buffered += frame.duration
			if buffered >= prebuffer {
				end = frame.at.Add(buffered)
			}
			continue
		}
		if frame.at.After(end) {
			stalled += frame.at.Sub(end)
			end = frame.at
		}
		end = end.Add(frame.duration)
	}
	return stalled
}

func TestTTSPlaybackTrace(t *testing.T) {
	for _, gap := range []time.Duration{400 * time.Millisecond, 2 * time.Second} {
		t.Run(fmt.Sprint(gap), func(t *testing.T) {
			var trace ttsPlaybackTrace
			at := time.Unix(1, 0)
			for i := range 200 {
				if i == 100 {
					at = at.Add(gap)
				}
				trace.frames = append(trace.frames, ttsPlaybackFrame{at: at, duration: 20 * time.Millisecond})
				at = at.Add(20 * time.Millisecond)
			}
			// These are the positive/negative controls in Giztest's existing
			// TestPeerAudioPacingExposesSegmentBoundaryStall and ...AcceptsGaps.
			if got, want := trace.stall(500*time.Millisecond), max(0, gap-480*time.Millisecond); got != want {
				t.Fatalf("stall = %s, want %s", got, want)
			}
		})
	}
}
