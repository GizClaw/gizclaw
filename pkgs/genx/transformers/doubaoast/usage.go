package doubaoast

import (
	"context"
	"log/slog"
	"math"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// recordUsage reports the tokens one AST usage response bills. The provider
// itemizes tokens per modality and direction; it reports no cache.
func (t *Transformer) recordUsage(ctx context.Context, usage *doubaospeech.ASTTranslateUsage) {
	if usage == nil {
		return
	}
	var report genx.TokenUsage
	for _, item := range usage.Items {
		quantity := int64(math.Round(float64(item.Quantity)))
		switch item.Unit {
		case "input_text_tokens":
			report.InputTokens += quantity
		case "input_audio_tokens":
			report.InputTokens += quantity
			report.InputAudioTokens += quantity
		case "output_text_tokens":
			report.OutputTokens += quantity
		case "output_audio_tokens":
			report.OutputTokens += quantity
			report.OutputAudioTokens += quantity
		default:
			slog.WarnContext(ctx, "doubao ast: unrecorded billing item", "unit", item.Unit, "quantity", item.Quantity)
		}
	}
	genx.RecordUsage(ctx, report.Records("volc", t.resourceID)...)
}
