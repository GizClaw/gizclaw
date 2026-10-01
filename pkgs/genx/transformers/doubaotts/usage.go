package doubaotts

import (
	"context"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// recordUsage reports the characters the provider billed for one synthesis
// request. Only the final chunk carries usage.
func recordUsage(ctx context.Context, resourceID string, chunk *doubaospeech.TTSV2Chunk) {
	if chunk == nil || chunk.Usage == nil {
		return
	}
	genx.RecordUsage(ctx, genx.UsageRecord{
		Provider: "volc",
		Model:    resourceID,
		Modality: genx.UsageModalityText,
		Unit:     genx.UsageUnitCharacter,
		Input:    int64(chunk.Usage.TextWords),
	})
}
