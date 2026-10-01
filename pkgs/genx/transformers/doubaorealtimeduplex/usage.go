package doubaorealtimeduplex

import (
	"context"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// recordUsage reports the tokens response.done bills for one interaction. The
// provider reports cached input apart from input tokens; cache without a
// modality split is attributed to text.
func (t *Transformer) recordUsage(ctx context.Context, usage *doubaospeech.RealtimeDuplexUsage) {
	if usage == nil {
		return
	}
	details := usage.InputTokenDetails
	cachedAudio := int64(details.CachedTokensDetails.AudioTokens)
	cachedText := max(int64(details.CachedTokens)-cachedAudio, 0)
	genx.RecordUsage(ctx, genx.TokenUsage{
		InputTokens:       int64(usage.InputTokens) + int64(details.CachedTokens),
		InputAudioTokens:  int64(details.AudioTokens) + cachedAudio,
		CachedTextTokens:  cachedText,
		CachedAudioTokens: cachedAudio,
		OutputTokens:      int64(usage.OutputTokens),
		OutputAudioTokens: int64(usage.OutputTokenDetails.AudioTokens),
	}.Records("volc", t.model)...)
}
