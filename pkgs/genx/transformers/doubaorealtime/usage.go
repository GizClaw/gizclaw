package doubaorealtime

import (
	"context"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// recordUsage reports the tokens of one realtime response. The provider
// reports cached tokens separately from input tokens.
func (t *Transformer) recordUsage(ctx context.Context, usage *doubaospeech.RealtimeUsage) {
	if usage == nil {
		return
	}
	genx.RecordUsage(ctx, genx.TokenUsage{
		InputTokens:       int64(usage.InputTextTokens + usage.InputAudioTokens + usage.CachedTextTokens + usage.CachedAudioTokens),
		InputAudioTokens:  int64(usage.InputAudioTokens + usage.CachedAudioTokens),
		CachedTextTokens:  int64(usage.CachedTextTokens),
		CachedAudioTokens: int64(usage.CachedAudioTokens),
		OutputTokens:      int64(usage.OutputTextTokens + usage.OutputAudioTokens),
		OutputAudioTokens: int64(usage.OutputAudioTokens),
	}.Records("volc", t.model)...)
}
