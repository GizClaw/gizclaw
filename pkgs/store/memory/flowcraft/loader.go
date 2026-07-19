package flowcraft

import (
	"context"
	"fmt"

	"github.com/GizClaw/flowcraft/memory/recall"
)

// New constructs a Flowcraft-backed memory Store from one in-memory config.
// It starts no background worker and never closes caller-owned dependencies.
func New(ctx context.Context, config Config) (*Store, error) {
	config = config.normalized()
	if err := config.validate(); err != nil {
		return nil, err
	}
	recallOptions := make([]recall.Option, 0, 10)
	temporal := config.TemporalStore
	if temporal == nil {
		temporal = recall.NewInMemoryTemporalStore()
	} else {
		temporal = nonClosingTemporalStore{TemporalStore: temporal}
	}
	recallOptions = append(recallOptions, recall.WithTemporalStore(temporal))
	if config.EvidenceStore != nil {
		recallOptions = append(recallOptions, recall.WithEvidenceStore(nonClosingEvidenceStore{EvidenceStore: config.EvidenceStore}))
	}
	queue := newFlowcraftAsyncQueue(config.AsyncQueue)
	if queue != nil {
		recallOptions = append(recallOptions, recall.WithAsyncSemanticQueue(queue))
	}
	if config.SideEffectOutbox != nil {
		recallOptions = append(recallOptions, recall.WithSideEffectOutbox(nonClosingSideEffectOutbox{SideEffectOutbox: config.SideEffectOutbox}))
	}
	if config.RetrievalIndex != nil {
		recallOptions = append(recallOptions, recall.WithRetrievalIndex(nonClosingRetrievalIndex{Index: config.RetrievalIndex}))
	}
	if config.GraphEnabled {
		recallOptions = append(recallOptions, recall.WithGraphEnabled(true))
	}
	if config.Extraction.Model != "" {
		model, err := config.Loader.LoadLLM(ctx, config.Extraction.Model)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft extraction model %q: %w", config.Extraction.Model, err)
		}
		if model == nil {
			return nil, fmt.Errorf("%w: flowcraft extraction model %q resolved to nil", errUnavailable, config.Extraction.Model)
		}
		extractorOptions := []recall.LLMExtractorOption{recall.WithLLMExtractionMode(config.Extraction.Mode)}
		if config.Extraction.Temperature != nil {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorTemperature(*config.Extraction.Temperature))
		}
		if config.Extraction.SystemPrompt != "" {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorSystemPrompt(config.Extraction.SystemPrompt))
		}
		if config.Extraction.SchemaName != "" {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorSchemaName(config.Extraction.SchemaName))
		}
		if config.Extraction.StageTimeout > 0 {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorStageTimeout(config.Extraction.StageTimeout))
		}
		recallOptions = append(recallOptions, recall.WithLLMExtractor(model, extractorOptions...))
	}
	if config.Embedding.Model != "" {
		embedder, err := config.Loader.LoadEmbedder(ctx, config.Embedding.Model)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft embedding model %q: %w", config.Embedding.Model, err)
		}
		if embedder == nil {
			return nil, fmt.Errorf("%w: flowcraft embedding model %q resolved to nil", errUnavailable, config.Embedding.Model)
		}
		recallOptions = append(recallOptions, recall.WithEmbedder(embedder))
	}
	if config.Rerank.Model != "" {
		model, err := config.Loader.LoadLLM(ctx, config.Rerank.Model)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft rerank model %q: %w", config.Rerank.Model, err)
		}
		if model == nil {
			return nil, fmt.Errorf("%w: flowcraft rerank model %q resolved to nil", errUnavailable, config.Rerank.Model)
		}
		recallOptions = append(recallOptions, recall.WithReranker(recall.NewLLMReranker(model)))
	}
	memory, err := recall.New(recallOptions...)
	if err != nil {
		return nil, mapFlowcraftError("construct memory", err)
	}
	store := newStore(config, memory, temporal, queue)
	if queue != nil {
		queue.setStatusWriter(store.recordOperationStatus)
	}
	if err := store.rehydrateOperations(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}
