package streamkit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type ttsBatchCall struct {
	text string
	meta TTSMeta
	at   time.Time
}

type ttsBatchHarness struct {
	t      *testing.T
	input  *Output
	output *Output
	calls  chan ttsBatchCall
	cancel context.CancelFunc
}

func newTTSBatchHarness(t *testing.T, config genx.TTSBatchConfig, synthesize TTSSynthesizer) *ttsBatchHarness {
	t.Helper()
	// These text-only fixtures do not encode audio duration. Supply a known
	// external budget and explicit cap for boundary, timer and lifecycle tests.
	// Default adaptive scheduling is tested with measured PCM separately.
	if config.MaxWait == 0 {
		config.MaxWait = 200 * time.Millisecond
	}
	if config.WaitBudget == nil {
		config.WaitBudget = func() time.Duration { return time.Hour }
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx, err := genx.WithTTSBatching(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	h := &ttsBatchHarness{t: t, input: NewOutput(OutputConfig{}), calls: make(chan ttsBatchCall, 64), cancel: cancel}
	h.output = NewTTSStream(ctx, h.input, OutputConfig{}, "audio/ogg", func(ctx context.Context, text string, meta TTSMeta, mime string, emit func([]byte) error) error {
		h.calls <- ttsBatchCall{text: text, meta: meta, at: time.Now()}
		if synthesize != nil {
			return synthesize(ctx, text, meta, mime, emit)
		}
		return emit([]byte(text))
	})
	return h
}

func (h *ttsBatchHarness) send(text string, ctrl *genx.StreamCtrl) {
	h.t.Helper()
	if err := h.input.Push(&genx.MessageChunk{Part: genx.Text(text), Ctrl: ctrl}); err != nil {
		h.t.Fatal(err)
	}
	synctest.Wait()
}

func (h *ttsBatchHarness) nextCall() ttsBatchCall {
	h.t.Helper()
	select {
	case call := <-h.calls:
		return call
	default:
		h.t.Fatal("expected a synthesis request")
		return ttsBatchCall{}
	}
}

func (h *ttsBatchHarness) noCall() {
	h.t.Helper()
	select {
	case call := <-h.calls:
		h.t.Fatalf("unexpected synthesis request: %q", call.text)
	default:
	}
}

func (h *ttsBatchHarness) startFirst(route string) {
	h.t.Helper()
	h.send("第一句话已经说完了。", &genx.StreamCtrl{StreamID: route})
	h.noCall()
	h.send("后", &genx.StreamCtrl{StreamID: route})
	if got := h.nextCall().text; got != "第一句话已经说完了。" {
		h.t.Fatalf("startup request = %q", got)
	}
}

func (h *ttsBatchHarness) stop() {
	h.cancel()
	_ = h.input.CloseWithError(context.Canceled)
	_ = h.output.CloseWithError(context.Canceled)
	synctest.Wait()
}

func TestTTSBatchingMergesAfterFirstAudio(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = time.Second
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(100 * time.Millisecond)
		h.send("检查一下电源。接", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(config.MaxWait - 100*time.Millisecond)
		synctest.Wait()
		if got := h.nextCall().text; got != "后面先关好窗户，再检查一下电源。" {
			t.Fatalf("merged synthesis = %q", got)
		}
		h.noCall()
	})
}

func TestTTSBatchingDeadlineDoesNotNeedMoreInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = 100 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		started := time.Now()
		h.noCall()
		time.Sleep(99 * time.Millisecond)
		synctest.Wait()
		h.noCall()
		time.Sleep(time.Millisecond)
		synctest.Wait()
		call := h.nextCall()
		if call.text != "后面先关好窗户，" || call.at.Sub(started) != config.MaxWait {
			t.Fatalf("deadline request = %q after %s", call.text, call.at.Sub(started))
		}
		time.Sleep(config.MaxWait)
		synctest.Wait()
		h.noCall() // The unconfirmed tail "再" is never forced through a deadline.
	})
}

func TestTTSBatchingCompleteShortFirstSentenceBypassesStartupThreshold(t *testing.T) {
	for _, sample := range []struct{ name, input, want string }{
		{"chinese", "好。后", "好。"},
		{"english", "I! Next", "I! "},
		{"quoted_chinese", "“好。”后", "“好。”"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				config := genx.DefaultTTSBatchConfig()
				config.FirstMinRunes = 8
				config.FirstWait = time.Hour
				h := newTTSBatchHarness(t, config, nil)
				defer h.stop()
				started := time.Now()
				h.send(sample.input, &genx.StreamCtrl{StreamID: "startup"})
				call := h.nextCall()
				if call.text != sample.want || !call.at.Equal(started) {
					t.Fatalf("complete first sentence = %q after %s, want %q immediately", call.text, call.at.Sub(started), sample.want)
				}
				h.noCall()
			})
		})
	}
}

func TestTTSBatchingStartupMinimumDoesNotRestartBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.FirstMinRunes = 8
		config.FirstWait = 50 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.send("好的，我", &genx.StreamCtrl{StreamID: "a"})
		time.Sleep(time.Second)
		synctest.Wait()
		h.noCall()
		h.send("们先把窗户关好，再", &genx.StreamCtrl{StreamID: "a"})
		started := time.Now()
		call := h.nextCall()
		if call.text != "好的，我们先把窗户关好，" || call.at.Sub(started) != 0 {
			t.Fatalf("startup budget = %q after %s", call.text, call.at.Sub(started))
		}
	})
}

func TestTTSBatchingCollectsWhileFirstSynthesisReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		config := genx.DefaultTTSBatchConfig()
		h := newTTSBatchHarness(t, config, func(ctx context.Context, text string, meta TTSMeta, _ string, emit func([]byte) error) error {
			if meta.SegmentIndex == 1 {
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return emit([]byte(text))
		})
		defer h.stop()
		h.startFirst("a")
		h.send("面先把门窗全部关好，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		h.send("检查厨房里的电源，最后", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		close(release)
		synctest.Wait()
		h.send("确认全部完成。接", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		if got := h.nextCall().text; got != "后面先把门窗全部关好，再检查厨房里的电源，最后确认全部完成。" {
			t.Fatalf("segments collected while awaiting audio = %q", got)
		}
	})
}

func TestTTSBatchingFirstWaitIsConfigurable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.FirstWait = 50 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		started := time.Now()
		h.send("先把门窗全部关好，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(49 * time.Millisecond)
		synctest.Wait()
		h.noCall()
		time.Sleep(time.Millisecond)
		synctest.Wait()
		call := h.nextCall()
		if call.text != "先把门窗全部关好，" || call.at.Sub(started) != config.FirstWait {
			t.Fatalf("startup budget = %q after %s", call.text, call.at.Sub(started))
		}
	})
}

func TestTTSBatchingNewSegmentsDoNotExtendDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = 100 * time.Millisecond
		config.WaitBudget = func() time.Duration { return time.Hour }
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		started := time.Now()
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		time.Sleep(60 * time.Millisecond)
		h.send("检查电源，最", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(40 * time.Millisecond)
		synctest.Wait()
		call := h.nextCall()
		if call.text != "后面先关好窗户，再检查电源，" || call.at.Sub(started) != config.MaxWait {
			t.Fatalf("collection extended its deadline: %q after %s", call.text, call.at.Sub(started))
		}
	})
}

func TestTTSBatchingLiveBudgetMakesPendingUrgent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var budget atomic.Int64
		budget.Store(int64(time.Second))
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = time.Second
		config.WaitBudget = func() time.Duration { return time.Duration(budget.Load()) }
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(40 * time.Millisecond)
		budget.Store(0)
		h.send("检", &genx.StreamCtrl{StreamID: "a"})
		if got := h.nextCall().text; got != "后面先关好窗户，" {
			t.Fatalf("urgent batch = %q", got)
		}
	})
}

func TestTTSBatchingKeepsQuotedSentencesTogether(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSBatchHarness(t, genx.DefaultTTSBatchConfig(), nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面他说：“先关门。再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		h.send("开窗，最后检查电源。”随后", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		if got := h.nextCall().text; got != "后面他说：“先关门。再开窗，最后检查电源。”" {
			t.Fatalf("quoted batch = %q", got)
		}
	})
}

func TestTTSBatchingMergesCompleteSentencesWithinBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = 100 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面第一句已经说完。第", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(60 * time.Millisecond)
		h.send("二句也已经说完。后", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(40 * time.Millisecond)
		synctest.Wait()
		if got := h.nextCall().text; got != "后面第一句已经说完。第二句也已经说完。" {
			t.Fatalf("complete sentences were submitted individually: %q", got)
		}
	})
}

func TestTTSBatchingBoundsRequestsAndFlushesEOS(t *testing.T) {
	for _, limit := range []int{8, 16, 80, 240, 1000} {
		t.Run(fmt.Sprintf("max_%d", limit), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				config := genx.DefaultTTSBatchConfig()
				config.MaxRunes = limit
				config.FirstWait, config.MaxWait = time.Hour, time.Hour
				h := newTTSBatchHarness(t, config, nil)
				defer h.stop()
				text := strings.Repeat("先关好窗户，再检查电源，最后确认完成。", 20)
				if limit == 1000 {
					text = strings.Repeat("先关好窗户，再检查电源，最后确认完成。", 160)
				}
				started := time.Now()
				h.send(text, &genx.StreamCtrl{StreamID: "a", EndOfStream: true})
				_ = h.input.Close()
				var spoken strings.Builder
				for _, chunk := range collectTransformerChunks(t, h.output) {
					if audio, ok := chunk.Part.(*genx.Blob); ok {
						spoken.Write(audio.Data)
					}
				}
				if spoken.String() != text {
					t.Fatalf("EOS changed text: %q", spoken.String())
				}
				close(h.calls)
				for call := range h.calls {
					if !utf8.ValidString(call.text) || utf8.RuneCountInString(call.text) > limit {
						t.Errorf("request exceeds %d runes: %q", limit, call.text)
					}
					if !call.at.Equal(started) {
						t.Errorf("EOS waited for a collection timer: %s", call.at.Sub(started))
					}
				}
			})
		})
	}
}

func TestTTSBatchingDoesNotMergeSpeechIdentities(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.FirstWait, config.MaxWait = time.Second, time.Second
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.send("这一条消息属于甲，后", &genx.StreamCtrl{StreamID: "a", MessageID: "one"})
		h.noCall()
		h.send("下一条消息属于乙，后", &genx.StreamCtrl{StreamID: "a", MessageID: "two", MessageEnd: true})
		first, second := h.nextCall(), h.nextCall()
		if first.meta.MessageID != "one" || first.text != "这一条消息属于甲，后" || second.meta.MessageID != "two" || second.text != "下一条消息属于乙，后" {
			t.Fatalf("speech identities were merged: %+v / %+v", first, second)
		}
		h.send("", &genx.StreamCtrl{StreamID: "a", EndOfStream: true})
		_ = h.input.Close()
		var ends []string
		for _, chunk := range collectTransformerChunks(t, h.output) {
			if chunk.Ctrl != nil && chunk.Ctrl.MessageEnd {
				ends = append(ends, chunk.Ctrl.MessageID)
			}
		}
		if !slices.Equal(ends, []string{"two"}) {
			t.Fatalf("message boundaries = %q", ends)
		}
	})
}

func TestTTSBatchingCancellationDiscardsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSBatchHarness(t, genx.DefaultTTSBatchConfig(), nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		h.cancel()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		h.noCall()
		select {
		case <-h.output.Done():
		default:
			t.Fatal("cancellation did not close the blocked pipeline")
		}
	})
}

func TestTTSBatchingProviderFailureClosesStalledInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		want := errors.New("provider failed before first audio")
		h := newTTSBatchHarness(t, genx.DefaultTTSBatchConfig(), func(context.Context, string, TTSMeta, string, func([]byte) error) error { return want })
		defer h.stop()
		h.send("第一句话已经说完了。 ", &genx.StreamCtrl{StreamID: "a"})
		_, err := collectTransformerChunksUntilError(h.output)
		if !errors.Is(err, want) {
			t.Fatalf("provider failure = %v", err)
		}
	})
}

func TestTTSBatchingRoutesHaveIndependentDeadlines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = 100 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.startFirst("a")
		h.startFirst("b")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		time.Sleep(30 * time.Millisecond)
		h.send("面先检查电源，再", &genx.StreamCtrl{StreamID: "b"})
		time.Sleep(70 * time.Millisecond)
		synctest.Wait()
		if call := h.nextCall(); call.meta.StreamID != "a" || call.text != "后面先关好窗户，" {
			t.Fatalf("first route deadline = %+v", call)
		}
		h.noCall()
		time.Sleep(30 * time.Millisecond)
		synctest.Wait()
		if call := h.nextCall(); call.meta.StreamID != "b" || call.text != "后面先检查电源，" {
			t.Fatalf("second route deadline = %+v", call)
		}
	})
}

func TestTTSBatchingInterruptDiscardsPendingAndResetsStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newTTSBatchHarness(t, genx.DefaultTTSBatchConfig(), nil)
		defer h.stop()
		h.startFirst("a")
		h.send("面先关好窗户，再", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		h.send("", &genx.StreamCtrl{StreamID: "a", Error: "interrupted"})
		time.Sleep(time.Second)
		synctest.Wait()
		h.noCall()
		h.send("新的回复现在开始。 ", &genx.StreamCtrl{StreamID: "a"})
		call := h.nextCall()
		if call.text != "新的回复现在开始。" || call.meta.SegmentIndex != 1 {
			t.Fatalf("replacement inherited pending text or startup state: %+v", call)
		}
	})
}

func TestTTSBatchingQualityCorpus(t *testing.T) {
	for _, sample := range ttsSegmentationSamples() {
		t.Run(sample.name, func(t *testing.T) {
			for _, limit := range []int{80, 240, 1000} {
				t.Run(fmt.Sprintf("max_%d", limit), func(t *testing.T) {
					for _, chunking := range ttsSegmentationChunkings(sample.input) {
						if chunking.name != "whole" && chunking.name != "width_1" && chunking.name != "bytewise" && chunking.name != "mixed_0" {
							continue
						}
						t.Run(chunking.name, func(t *testing.T) {
							config := genx.DefaultTTSBatchConfig()
							config.MaxRunes = limit
							ctx, err := genx.WithTTSBatching(t.Context(), config)
							if err != nil {
								t.Fatal(err)
							}
							input := &testStream{doneErr: genx.ErrDone}
							for _, text := range chunking.chunks {
								input.chunks = append(input.chunks, &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "corpus"}})
							}
							input.chunks = append(input.chunks, &genx.MessageChunk{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "corpus", EndOfStream: true}})
							output := NewTTSStream(ctx, input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, _ TTSMeta, _ string, emit func([]byte) error) error {
								return emit([]byte(text))
							})
							var requests []string
							for _, chunk := range collectTransformerChunks(t, output) {
								if audio, ok := chunk.Part.(*genx.Blob); ok && len(audio.Data) != 0 {
									requests = append(requests, string(audio.Data))
								}
							}
							ttsCheckSegments(t, requests, sample.spoken, sample.protected, limit)
						})
					}
				})
			}
		})
	}
}

func TestTTSBatchingKeepsCurlyApostropheInsideQuotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.FirstWait = 50 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.send("He said, ‘don’t stop. K", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		h.send("eep going.’ Next", &genx.StreamCtrl{StreamID: "a"})
		if got := strings.TrimSpace(h.nextCall().text); got != "He said, ‘don’t stop. Keep going.’" {
			t.Fatalf("curly apostrophe closed the quotation: %q", got)
		}
	})
}

type ttsCloseTrackingInput struct {
	genx.Stream
	canceled atomic.Int32
}

func (s *ttsCloseTrackingInput) CloseWithError(err error) error {
	s.canceled.Add(1)
	return s.Stream.CloseWithError(err)
}

func TestTTSBatchingNormalCompletionPreservesInputTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &ttsCloseTrackingInput{Stream: &testStream{
			chunks:  []*genx.MessageChunk{{Part: genx.Text("这是正常结束的正文。"), Ctrl: &genx.StreamCtrl{StreamID: "a", EndOfStream: true}}},
			doneErr: genx.ErrDone,
		}}
		output := NewTTSStream(context.Background(), input, OutputConfig{}, "audio/ogg", func(_ context.Context, text string, _ TTSMeta, _ string, emit func([]byte) error) error {
			return emit([]byte(text))
		})
		collectTransformerChunks(t, output)
		synctest.Wait()
		if got := input.canceled.Load(); got != 0 {
			t.Fatalf("successful upstream was canceled %d times", got)
		}
	})
}

func TestTTSBatchingMessageEndPreservesAudioStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = 200 * time.Millisecond
		h := newTTSBatchHarness(t, config, nil)
		defer h.stop()
		h.send("第一条消息已经说完。", &genx.StreamCtrl{StreamID: "a", MessageID: "one", MessageEnd: true})
		h.nextCall()
		h.send("第二条消息先关窗，再", &genx.StreamCtrl{StreamID: "a", MessageID: "two"})
		h.noCall()
		time.Sleep(config.MaxWait)
		synctest.Wait()
		call := h.nextCall()
		if call.text != "第二条消息先关窗，" || call.meta.MessageID != "two" {
			t.Fatalf("message boundary changed the stream's audio phase: %+v", call)
		}
	})
}

func TestTTSBatchingLargeSteadyBatchStreamsBeforeSynthesisFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		finished := make(chan struct{})
		config := genx.DefaultTTSBatchConfig()
		config.MaxWait = time.Second
		h := newTTSBatchHarness(t, config, func(ctx context.Context, _ string, meta TTSMeta, _ string, emit func([]byte) error) error {
			if meta.SegmentIndex != 2 {
				return emit([]byte("startup"))
			}
			defer close(finished)
			if err := emit([]byte("large-head")); err != nil {
				return err
			}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return emit([]byte("large-tail"))
		})
		defer h.stop()
		nextAudio := func() string {
			t.Helper()
			for {
				chunk, err := h.output.Next()
				if err != nil {
					t.Fatal(err)
				}
				if audio, ok := chunk.Part.(*genx.Blob); ok && len(audio.Data) != 0 {
					return string(audio.Data)
				}
			}
		}
		h.startFirst("a")
		if got := nextAudio(); got != "startup" {
			t.Fatalf("startup audio = %q", got)
		}
		text := "后" + strings.Repeat("这是一段完整的长篇讲述。", 50)
		if length := utf8.RuneCountInString(text); length <= 240 || length > 1000 {
			t.Fatalf("invalid long-batch fixture length %d", length)
		}
		started := time.Now()
		// startFirst left "后" as unconfirmed lookahead in the text stream.
		h.send(strings.TrimPrefix(text, "后")+"续", &genx.StreamCtrl{StreamID: "a"})
		h.noCall()
		time.Sleep(999 * time.Millisecond)
		synctest.Wait()
		h.noCall()
		time.Sleep(time.Millisecond)
		synctest.Wait()
		call := h.nextCall()
		if call.text != text || call.at.Sub(started) != time.Second {
			t.Fatalf("large batch = %q after %s", call.text, call.at.Sub(started))
		}
		if got := nextAudio(); got != "large-head" {
			t.Fatalf("first streaming audio = %q", got)
		}
		select {
		case <-finished:
			t.Fatal("first audio waited for the complete synthesis")
		default:
		}
		close(release)
		synctest.Wait()
		if got := nextAudio(); got != "large-tail" {
			t.Fatalf("remaining streaming audio = %q", got)
		}
	})
}
