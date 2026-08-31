//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestLoCoMoFlowcraftHybridSinglePass(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{embedding: true})
	profile := "flowcraft_redis8_hybrid_single_pass"
	store, closer := newFlowcraftRedis8Store(t, profile, memoryflowcraft.Config{
		Loader: settings.loader(),
		Extraction: memoryflowcraft.ExtractionConfig{
			Model: settings.extractionModel, Mode: recall.LLMExtractionSinglePass,
		},
		Embedding: memoryflowcraft.EmbeddingConfig{Model: settings.embeddingModel},
		Rerank:    memoryflowcraft.RerankConfig{Model: settings.rerankModel},
	})
	fingerprint := configFingerprint(profile, settings.extractionModel, settings.embeddingModel, settings.rerankModel)
	runLiveProfile(t, settings, profile, fingerprint, reportModels{
		Extraction: settings.extractionModel, Embedding: settings.embeddingModel, Rerank: settings.rerankModel,
	}, store, closer)
}
