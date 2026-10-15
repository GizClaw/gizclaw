package streamkit

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/audiostream"
)

func newTTSAudioBudgetHarness(t *testing.T, config genx.TTSBatchConfig, synthesize TTSSynthesizer) *ttsBatchHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ctx, err := genx.WithTTSBatching(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	h := &ttsBatchHarness{t: t, input: NewOutput(OutputConfig{}), calls: make(chan ttsBatchCall, 64), cancel: cancel}
	h.output = NewMeasuredTTSStream(ctx, h.input, OutputConfig{}, "audio/pcm;rate=16000", func(ctx context.Context, text string, meta TTSMeta, mime string, emit func([]byte) error) error {
		h.calls <- ttsBatchCall{text: text, meta: meta, at: time.Now()}
		return synthesize(ctx, text, meta, mime, emit)
	}, func() func([]byte) time.Duration { return audiostream.NewDurationMeter("audio/pcm", 16000, 1).Add })
	return h
}

func ttsBudgetAudio(duration time.Duration) []byte {
	return make([]byte, int(duration*32000/time.Second))
}

func TestTTSAudioBudgetFirstSegmentSubmitsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
			select {
			case <-time.After(time.Second):
				return emit(ttsBudgetAudio(3 * time.Second))
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
		defer h.stop()
		start := time.Now()
		h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
		if call := h.nextCall(); call.text != "好，" || call.at != start {
			t.Fatalf("first two-rune segment was delayed: %+v", call)
		}
		h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
		if call := h.nextCall(); call.text != "继续先关窗，" || call.at != start {
			t.Fatalf("cold-start lookahead was delayed: %+v", call)
		}
		h.send("检查电源，最后确认，接", &genx.StreamCtrl{StreamID: "a"})
		time.Sleep(900 * time.Millisecond)
		synctest.Wait()
		h.noCall() // Both synthesis slots are occupied; later text stays mergeable.
	})
}

func TestTTSAudioBudgetColdLookaheadCoversShortFirstPhrase(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, meta TTSMeta, _ string, emit func([]byte) error) error {
			select {
			case <-time.After(1400 * time.Millisecond):
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			duration := 3 * time.Second
			if meta.SegmentIndex == 1 {
				duration = 720 * time.Millisecond
			}
			return emit(ttsBudgetAudio(duration))
		})
		defer h.stop()
		start := time.Now()
		h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
		h.nextCall()
		time.Sleep(160 * time.Millisecond)
		h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
		if call := h.nextCall(); call.at.Sub(start) != 160*time.Millisecond {
			t.Fatalf("empty audio budget delayed lookahead: %+v", call)
		}
		var arrivals []time.Time
		for len(arrivals) < 2 {
			chunk, err := h.output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				arrivals = append(arrivals, time.Now())
			}
		}
		if arrivals[0].Sub(start) != 1400*time.Millisecond || !arrivals[1].Before(arrivals[0].Add(720*time.Millisecond)) {
			t.Fatalf("short first phrase underruns: %v", arrivals)
		}
	})
}

func TestTTSAudioBudgetCollectsMoreWithLongerPreviousAudio(t *testing.T) {
	for _, duration := range []time.Duration{time.Second, 3 * time.Second, 6 * time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
					select {
					case <-time.After(200 * time.Millisecond):
						return emit(ttsBudgetAudio(duration))
					case <-ctx.Done():
						return context.Cause(ctx)
					}
				})
				defer h.stop()
				start := time.Now()
				h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
				h.nextCall()
				time.Sleep(200 * time.Millisecond)
				synctest.Wait()
				h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
				laterSegments := int((duration - 50*time.Millisecond) / (400 * time.Millisecond))
				generated := make(chan error, 1)
				go func() {
					for index := range laterSegments {
						time.Sleep(time.Until(start.Add(time.Duration(index+1) * 400 * time.Millisecond)))
						if err := h.input.Push(&genx.MessageChunk{Part: genx.Text("检查电源，再"), Ctrl: &genx.StreamCtrl{StreamID: "a"}}); err != nil {
							generated <- err
							return
						}
					}
					generated <- nil
				}()
				h.noCall()
				// First audio begins at 200ms; reserve that observed latency plus
				// 50ms margin from its full duration. There is no fixed wait cap.
				deadline := start.Add(duration - 50*time.Millisecond)
				time.Sleep(time.Until(deadline) - time.Nanosecond)
				synctest.Wait()
				h.noCall()
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				call := h.nextCall()
				want := "继续先关窗，" + strings.Repeat("再检查电源，", laterSegments)
				if err := <-generated; err != nil {
					t.Fatal(err)
				}
				if call.at != deadline || call.text != want {
					t.Fatalf("audio-budget batch = %q after %s", call.text, call.at.Sub(start))
				}
				t.Logf("previous_audio=%s merged_segments=%d submit_after_first_request=%s", duration, laterSegments+1, call.at.Sub(start))
				// The next synthesis returns before the preceding audio runs out.
				if !call.at.Add(200 * time.Millisecond).Before(start.Add(200*time.Millisecond + duration)) {
					t.Fatal("next first audio missed the preceding audio's media end")
				}
			})
		})
	}
}

func TestTTSAudioBudgetGrowsWithStreamingAudio(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
			select {
			case <-time.After(200 * time.Millisecond):
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			if err := emit(ttsBudgetAudio(time.Second)); err != nil {
				return err
			}
			select {
			case <-time.After(500 * time.Millisecond):
				return emit(ttsBudgetAudio(2 * time.Second))
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
		defer h.stop()
		start := time.Now()
		h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
		h.nextCall()
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
		time.Sleep(800 * time.Millisecond)
		h.send("检查电源，最后确认。接", &genx.StreamCtrl{StreamID: "a"})
		h.noCall() // A 1s fixed budget would already have frozen the second batch.
		time.Sleep(1950 * time.Millisecond)
		synctest.Wait()
		call := h.nextCall()
		if call.at.Sub(start) != 2950*time.Millisecond || call.text != "继续先关窗，再检查电源，最后确认。" {
			t.Fatalf("growing audio budget = %q after %s", call.text, call.at.Sub(start))
		}
	})
}

func TestTTSAudioBudgetLongBatchStreamsBeforeProviderCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		finished := make(chan struct{})
		h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, meta TTSMeta, _ string, emit func([]byte) error) error {
			if meta.SegmentIndex == 1 {
				return emit(ttsBudgetAudio(3 * time.Second))
			}
			defer close(finished)
			if err := emit(ttsBudgetAudio(20 * time.Millisecond)); err != nil {
				return err
			}
			select {
			case <-release:
				return emit(ttsBudgetAudio(20 * time.Millisecond))
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
		defer h.stop()
		h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
		h.nextCall()
		synctest.Wait()
		text := "继续" + strings.Repeat("这是一段完整的讲述。", 50)
		h.send(strings.TrimPrefix(text, "继")+"接", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(2950 * time.Millisecond)
		synctest.Wait()
		if call := h.nextCall(); call.text != text {
			t.Fatalf("long batch was fragmented: %q", call.text)
		}
		var audioChunks int
		for audioChunks < 2 {
			chunk, err := h.output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if audio, ok := chunk.Part.(*genx.Blob); ok && len(audio.Data) > 0 {
				audioChunks++
			}
		}
		select {
		case <-finished:
			t.Fatal("long batch waited for the whole synthesis before output")
		default:
		}
		close(release)
		synctest.Wait()
	})
}

func TestTTSAudioBudgetDeliveryWaitConsumesAudioTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ctx, err := genx.WithTTSBatching(ctx, genx.DefaultTTSBatchConfig())
		if err != nil {
			t.Fatal(err)
		}
		ctx = WithTTSBackpressure(ctx)
		h := &ttsBatchHarness{t: t, input: NewOutput(OutputConfig{}), calls: make(chan ttsBatchCall, 8), cancel: cancel}
		h.output = NewMeasuredTTSStream(ctx, h.input, OutputConfig{}, "audio/pcm", func(ctx context.Context, text string, meta TTSMeta, _ string, emit func([]byte) error) error {
			h.calls <- ttsBatchCall{text: text, meta: meta, at: time.Now()}
			select {
			case <-time.After(200 * time.Millisecond):
				return emit(ttsBudgetAudio(3 * time.Second))
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		}, func() func([]byte) time.Duration { return audiostream.NewDurationMeter("audio/pcm", 16000, 1).Add })
		h.output.DeferOutputObservation()
		defer h.stop()
		start := time.Now()
		h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
		h.nextCall()
		time.Sleep(200 * time.Millisecond)
		var audio *genx.MessageChunk
		for audio == nil {
			chunk, err := h.output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				audio = chunk
			}
		}
		h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
		// Feeding a large chunk to a paced consumer may take seconds. Those
		// seconds consume the previous clip even while its acknowledgement waits.
		time.Sleep(2 * time.Second)
		h.noCall()
		h.output.ObserveOutput(audio)
		synctest.Wait()
		time.Sleep(750 * time.Millisecond)
		synctest.Wait()
		if call := h.nextCall(); call.at.Sub(start) != 2950*time.Millisecond {
			t.Fatalf("delivery wait restarted the audio clock: %+v", call)
		}
	})
}

func TestTTSAudioBudgetRoutesAreIndependent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSAudioBudgetHarness(t, genx.DefaultTTSBatchConfig(), func(ctx context.Context, _ string, meta TTSMeta, _ string, emit func([]byte) error) error {
			select {
			case <-time.After(200 * time.Millisecond):
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			duration := time.Second
			if meta.StreamID == "b" {
				duration = 3 * time.Second
			}
			return emit(ttsBudgetAudio(duration))
		})
		defer h.stop()
		for _, route := range []string{"a", "b"} {
			h.send("好，继", &genx.StreamCtrl{StreamID: route})
			h.nextCall()
		}
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		for _, route := range []string{"a", "b"} {
			h.send("续先关窗，再", &genx.StreamCtrl{StreamID: route})
		}
		time.Sleep(750 * time.Millisecond)
		synctest.Wait()
		if call := h.nextCall(); call.meta.StreamID != "a" {
			t.Fatalf("short route budget = %+v", call)
		}
		h.noCall()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if call := h.nextCall(); call.meta.StreamID != "b" {
			t.Fatalf("long route budget = %+v", call)
		}
	})
}

func TestTTSAudioBudgetReservesLatencyAndMargin(t *testing.T) {
	for _, tc := range []struct{ lead, margin, submit time.Duration }{
		{0, 50 * time.Millisecond, 2950 * time.Millisecond},
		{500 * time.Millisecond, 50 * time.Millisecond, 2650 * time.Millisecond},
		{500 * time.Millisecond, 200 * time.Millisecond, 2500 * time.Millisecond},
		{time.Duration(1<<63 - 1), time.Duration(1<<63 - 1), 200 * time.Millisecond},
	} {
		t.Run(tc.submit.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				config := genx.DefaultTTSBatchConfig()
				config.SynthesisLead, config.SafetyMargin = tc.lead, tc.margin
				h := newTTSAudioBudgetHarness(t, config, func(ctx context.Context, _ string, _ TTSMeta, _ string, emit func([]byte) error) error {
					select {
					case <-time.After(200 * time.Millisecond):
						return emit(ttsBudgetAudio(3 * time.Second))
					case <-ctx.Done():
						return context.Cause(ctx)
					}
				})
				defer h.stop()
				start := time.Now()
				h.send("好，继", &genx.StreamCtrl{StreamID: "a"})
				h.nextCall()
				time.Sleep(200*time.Millisecond - time.Nanosecond)
				synctest.Wait()
				h.noCall()
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				h.send("续先关窗，再", &genx.StreamCtrl{StreamID: "a"})
				if wait := tc.submit - 200*time.Millisecond; wait > 0 {
					time.Sleep(wait - time.Nanosecond)
					synctest.Wait()
					h.noCall()
					time.Sleep(time.Nanosecond)
				}
				synctest.Wait()
				if call := h.nextCall(); call.at.Sub(start) != tc.submit {
					t.Fatalf("latency reserve = %+v", call)
				}
			})
		})
	}
}

func TestTTSAudioBudgetRecoversAfterExhaustion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := newTTSSynthesisBudget(genx.DefaultTTSBatchConfig(), true)
		budget.start()
		budget.observe(20*time.Millisecond, 200*time.Millisecond)
		budget.markOutput()
		time.Sleep(2 * time.Second)
		budget.observe(3*time.Second, 0)
		deadline, awaiting := budget.deadline(time.Now())
		if awaiting || time.Until(deadline) != 2750*time.Millisecond {
			t.Fatalf("recovered audio retained an old deficit: deadline=%s awaiting=%v", time.Until(deadline), awaiting)
		}
		budget.finish()
	})
}

func TestTTSAudioBudgetCallbackRunsOutsideLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		var budget *ttsSynthesisBudget
		config.WaitBudget = func() time.Duration {
			budget.start()
			budget.finish()
			return time.Second
		}
		budget = newTTSSynthesisBudget(config, true)
		budget.observe(3*time.Second, 200*time.Millisecond)
		budget.markOutput()
		deadline, _ := budget.deadline(time.Now())
		if time.Until(deadline) != time.Second {
			t.Fatalf("external shorter budget = %s", time.Until(deadline))
		}
	})
}
