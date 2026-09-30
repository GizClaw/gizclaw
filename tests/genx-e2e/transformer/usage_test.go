//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoast"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtimeduplex"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"google.golang.org/genai"
)

const (
	geminiAPIKeyEnv  = "GIZCLAW_GENX_E2E_GEMINI_API_KEY"
	geminiUsageModel = "gemini-2.5-flash"
	openAIUsageModel = "gpt-4o-mini"
)

// liveUsage collects the provider usage a live call reports.
type liveUsage struct {
	mu      sync.Mutex
	records []genx.UsageRecord
	changed chan struct{}
}

func newLiveUsage() *liveUsage {
	return &liveUsage{changed: make(chan struct{}, 1)}
}

func (u *liveUsage) record(record genx.UsageRecord) {
	u.mu.Lock()
	u.records = append(u.records, record)
	u.mu.Unlock()
	select {
	case u.changed <- struct{}{}:
	default:
	}
}

func (u *liveUsage) snapshot() []genx.UsageRecord {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]genx.UsageRecord(nil), u.records...)
}

// wrap returns a Transformer whose calls report usage to u.
func (u *liveUsage) wrap(inner genx.Transformer) genx.Transformer {
	return usageTransformer{inner: inner, usage: u}
}

type usageTransformer struct {
	inner genx.Transformer
	usage *liveUsage
}

func (t usageTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.inner.Transform(genx.WithUsageRecorder(ctx, t.usage.record), input)
}

// usageTotals sums usage per modality.
type usageTotals map[genx.UsageModality]genx.UsageRecord

// sumUsage requires every record to carry the expected identity and unit and
// sums them per modality.
func sumUsage(t *testing.T, records []genx.UsageRecord, provider, model string, unit genx.UsageUnit) usageTotals {
	t.Helper()
	totals := usageTotals{}
	for _, record := range records {
		if record.Provider != provider || record.Model != model || record.Unit != unit {
			t.Fatalf("usage record %+v, want provider %q model %q unit %q", record, provider, model, unit)
		}
		if record.Input < 0 || record.CachedInput < 0 || record.Output < 0 {
			t.Fatalf("usage record %+v has a negative quantity", record)
		}
		total := totals[record.Modality]
		total.Input += record.Input
		total.CachedInput += record.CachedInput
		total.Output += record.Output
		totals[record.Modality] = total
	}
	return totals
}

// runLiveUsageTurn drives one input turn and returns once the reported usage
// satisfies done. Realtime providers report usage after the response, so the
// session stays open until then; the output is drained without assertions,
// which the provider's conversation tests own.
func runLiveUsageTurn(
	t *testing.T,
	transformer genx.Transformer,
	push func(context.Context, *genx.RealtimeStream) error,
	provider, model string,
	done func(usageTotals) bool,
) usageTotals {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	usage := newLiveUsage()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := usage.wrap(transformer).Transform(ctx, input)
	if err != nil {
		t.Fatalf("Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)
	_, outputErrors := collectDuplexOutput(output)
	pushErrors := make(chan error, 1)
	go func() { pushErrors <- push(ctx, input) }()

	for {
		totals := sumUsage(t, usage.snapshot(), provider, model, genx.UsageUnitToken)
		if done(totals) {
			t.Logf("%s %s usage: %+v", provider, model, totals)
			return totals
		}
		select {
		case <-usage.changed:
		case err := <-pushErrors:
			if err != nil {
				t.Fatalf("push input turn: %v", err)
			}
		case err := <-outputErrors:
			t.Fatalf("output failed before usage was reported: %v (usage %+v)", err, totals)
		case <-ctx.Done():
			t.Fatalf("usage was not reported: %v (usage %+v)", ctx.Err(), totals)
		}
	}
}

func hasTextAndAudioResponseUsage(totals usageTotals) bool {
	text, audio := totals[genx.UsageModalityText], totals[genx.UsageModalityAudio]
	return audio.Input > 0 && text.Output > 0 && audio.Output > 0
}

func TestOpenAIGeneratorLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	client := openai.NewClient(option.WithAPIKey(firstEnv(einoAPIKeyEnv)))
	generator := &genx.OpenAIGenerator{
		Client: &client, Provider: "openai", Model: openAIUsageModel,
		TextOnly: true, SupportJSONOutput: true,
	}
	runGeneratorLiveUsage(t, generator, "openai", openAIUsageModel)
}

func TestGeminiGeneratorLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	client, err := genai.NewClient(t.Context(), &genai.ClientConfig{APIKey: firstEnv(geminiAPIKeyEnv)})
	if err != nil {
		t.Fatalf("genai.NewClient() failed: %v", err)
	}
	runGeneratorLiveUsage(t, &genx.GeminiGenerator{Client: client, Model: geminiUsageModel}, "gemini", geminiUsageModel)
}

// runGeneratorLiveUsage requires one text token record from a streamed
// response and one from a structured invocation.
func runGeneratorLiveUsage(t *testing.T, generator genx.Generator, provider, model string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	usage := newLiveUsage()
	ctx = genx.WithUsageRecorder(ctx, usage.record)

	builder := &genx.ModelContextBuilder{}
	builder.UserText("user", "Reply with the single word OK.")
	stream, err := generator.GenerateStream(ctx, "", builder.Build())
	if err != nil {
		t.Fatalf("GenerateStream() failed: %v", err)
	}
	var reply strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			if !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF) {
				t.Fatalf("stream failed: %v", err)
			}
			break
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			reply.WriteString(string(text))
		}
	}
	streamed := usage.snapshot()
	if len(streamed) != 1 {
		t.Fatalf("streamed usage records = %+v, want one text record", streamed)
	}

	tool := genx.MustNewFuncTool[struct {
		Answer string `json:"answer"`
	}]("answer", "Returns the answer.")
	if _, _, err := generator.Invoke(ctx, "", builder.Build(), tool); err != nil {
		t.Fatalf("Invoke() failed: %v", err)
	}
	records := usage.snapshot()
	if len(records) != 2 {
		t.Fatalf("usage records = %+v, want one per call", records)
	}
	for _, record := range records {
		if record.Modality != genx.UsageModalityText || record.Input <= 0 || record.Output <= 0 {
			t.Fatalf("usage record %+v, want positive text input and output", record)
		}
	}
	sumUsage(t, records, provider, model, genx.UsageUnitToken)
	t.Logf("reply=%q usage=%+v", reply.String(), records)
}

func TestDoubaoRealtimeLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	model := string(doubaospeech.RealtimeModelO20)
	transformer, err := doubaorealtime.New(doubaorealtime.Config{
		Client:         liveDoubaoClient(t),
		Model:          model,
		Mode:           doubaorealtime.ModeRealtime,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtime.New() failed: %v", err)
	}
	packets := embeddedPromptOpusPackets(t)
	runLiveUsageTurn(t, transformer, func(ctx context.Context, input *genx.RealtimeStream) error {
		return pushDuplexTurn(ctx, input, "doubao-realtime-usage", packets)
	}, "volc", model, hasTextAndAudioResponseUsage)
}

func TestDoubaoRealtimeDuplexLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	transcode := false
	transformer, err := doubaorealtimeduplex.New(doubaorealtimeduplex.Config{
		Client:         liveDoubaoClient(t),
		Model:          doubaospeech.RealtimeDuplexModelDefault,
		Instructions:   "Reply in one short English sentence.",
		InputTranscode: &transcode,
	})
	if err != nil {
		t.Fatalf("doubaorealtimeduplex.New() failed: %v", err)
	}
	packets := embeddedPromptOpusPackets(t)
	runLiveUsageTurn(t, transformer, func(ctx context.Context, input *genx.RealtimeStream) error {
		return pushDuplexTurn(ctx, input, "doubao-duplex-usage", packets)
	}, "volc", doubaospeech.RealtimeDuplexModelDefault, hasTextAndAudioResponseUsage)
}

func TestDoubaoASTLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	pacing := false
	transformer, err := doubaoast.New(doubaoast.Config{
		Client: liveDoubaoClient(t), Mode: doubaospeech.ASTTranslateModeS2S,
		InputMode: doubaoast.InputModeRealtime, SourceLanguage: "zhen", TargetLanguage: "zhen",
		SourceLanguageDetect: true, SpeakerID: "zh_female_xiaohe_uranus_bigtts", RealtimePacing: &pacing,
	})
	if err != nil {
		t.Fatalf("doubaoast.New() failed: %v", err)
	}
	packets := embeddedPromptOpusPackets(t)
	runLiveUsageTurn(t, transformer, func(ctx context.Context, input *genx.RealtimeStream) error {
		return pushDuplexTurn(ctx, input, "doubao-ast-usage", packets)
	}, "volc", doubaospeech.ResourceASTTranslate, hasTextAndAudioResponseUsage)
}

func TestDashScopeRealtimeLiveUsage(t *testing.T) {
	loadGenXE2EEnv(t)
	transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
		Client:       dashscope.NewClient(firstEnv(dashScopeAPIKeyEnv)),
		Model:        dashscope.ModelQwen35OmniPlusRealtime,
		VAD:          dashscope.VADModeDisabled,
		Instructions: "Reply in one short English sentence.",
	})
	if err != nil {
		t.Fatalf("dashscoperealtime.New() failed: %v", err)
	}
	packets := embeddedPromptOpusPackets(t)
	runLiveUsageTurn(t, transformer, func(ctx context.Context, input *genx.RealtimeStream) error {
		return pushDashScopeToolTurn(ctx, input, "dashscope-usage", packets)
	}, "dashscope", dashscope.ModelQwen35OmniPlusRealtime, hasTextAndAudioResponseUsage)
}

// requireTTSCharacterUsage sums the characters one TTS turn billed.
func requireTTSCharacterUsage(t *testing.T, usage *liveUsage, provider, model string) int64 {
	t.Helper()
	totals := sumUsage(t, usage.snapshot(), provider, model, genx.UsageUnitCharacter)
	if len(totals) != 1 || totals[genx.UsageModalityText].Input <= 0 {
		t.Fatalf("%s TTS usage = %+v, want billed text characters", provider, totals)
	}
	return totals[genx.UsageModalityText].Input
}

// requireVolcTTSCharacters checks Volc's billing rule: one character per
// Unicode character of the synthesized text.
func requireVolcTTSCharacters(t *testing.T, usage *liveUsage, model, text string) {
	t.Helper()
	if got, want := requireTTSCharacterUsage(t, usage, "volc", model), int64(utf8.RuneCountInString(text)); got != want {
		t.Fatalf("Volc TTS billed %d characters, want %d for %q", got, want, text)
	}
}
