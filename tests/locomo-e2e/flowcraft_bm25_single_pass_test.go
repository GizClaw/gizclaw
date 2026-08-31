//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestLoCoMoFlowcraftBM25SinglePass(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{})
	profile := "flowcraft_redis8_bm25_single_pass"
	store, closer := newFlowcraftRedis8Store(t, profile, memoryflowcraft.Config{
		Loader: settings.loader(),
		Extraction: memoryflowcraft.ExtractionConfig{
			Model: settings.extractionModel, Mode: recall.LLMExtractionSinglePass,
		},
		Rerank: memoryflowcraft.RerankConfig{Model: settings.rerankModel},
	})
	fingerprint := configFingerprint(profile, settings.extractionModel, settings.rerankModel)
	runLiveProfile(t, settings, profile, fingerprint, reportModels{
		Extraction: settings.extractionModel, Rerank: settings.rerankModel,
	}, store, closer)
}
