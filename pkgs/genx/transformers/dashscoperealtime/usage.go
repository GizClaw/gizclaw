package dashscoperealtime

import (
	"context"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// recordUsage reports the tokens response.done bills for one response. The
// service reports no cache for realtime responses.
func (t *Transformer) recordUsage(ctx context.Context, usage *dashscope.UsageStats) {
	if usage == nil {
		return
	}
	report := genx.TokenUsage{
		InputTokens:  int64(usage.InputTokens),
		OutputTokens: int64(usage.OutputTokens),
	}
	if usage.InputTokenDetails != nil {
		report.InputAudioTokens = int64(usage.InputTokenDetails.AudioTokens)
	}
	if usage.OutputTokenDetails != nil {
		report.OutputAudioTokens = int64(usage.OutputTokenDetails.AudioTokens)
	}
	genx.RecordUsage(ctx, report.Records("dashscope", t.model)...)
}
