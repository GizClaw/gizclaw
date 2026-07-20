//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"testing"

	"github.com/GizClaw/flowcraft/memory/recall"
	memoryflowcraft "github.com/GizClaw/gizclaw-go/pkgs/store/memory/flowcraft"
)

func TestLoCoMoFlowcraftHybridTwoPass(t *testing.T) {
	settings := requireLiveSettings(t, liveNeeds{embedding: true})
	resources := newFlowcraftResources(t, "flowcraft-hybrid-two-pass")
	store, err := memoryflowcraft.New(context.Background(), memoryflowcraft.Config{
		Loader: settings.loader(),
		Extraction: memoryflowcraft.ExtractionConfig{
			Model: settings.extractionModel, Mode: recall.LLMExtractionTwoPass,
		},
		Embedding:        memoryflowcraft.EmbeddingConfig{Model: settings.embeddingModel},
		Rerank:           memoryflowcraft.RerankConfig{Model: settings.rerankModel},
		RetrievalIndex:   resources.index,
		TemporalStore:    resources.backend.TemporalStore(),
		EvidenceStore:    resources.backend.EvidenceStore(),
		AsyncQueue:       resources.backend.AsyncSemanticQueue(),
		SideEffectOutbox: resources.backend.SideEffectOutbox(),
		GraphEnabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := "flowcraft_hybrid_two_pass"
	fingerprint := configFingerprint(profile, settings.extractionModel, settings.embeddingModel, settings.rerankModel)
	runLiveProfile(t, settings, profile, fingerprint, reportModels{
		Extraction: settings.extractionModel, Embedding: settings.embeddingModel, Rerank: settings.rerankModel,
	}, store, resources.closer(store))
}
