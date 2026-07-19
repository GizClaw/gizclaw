package memory

import (
	"context"
	"fmt"

	"github.com/GizClaw/flowcraft/memory/recall"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
)

// FlowcraftModelLoader resolves model resource names from application config.
type FlowcraftModelLoader interface {
	LoadLLM(ctx context.Context, name string) (llm.LLM, error)
	LoadEmbedder(ctx context.Context, name string) (embedding.Embedder, error)
}

// FlowcraftOption adds provider-specific recall options.
type FlowcraftOption func(*flowcraftOptions)

type flowcraftOptions struct {
	recallOptions []recall.Option
}

// WithFlowcraftRecallOptions supplies optional Flowcraft extensions without
// weakening the provider-neutral Store contract.
func WithFlowcraftRecallOptions(options ...recall.Option) FlowcraftOption {
	return func(config *flowcraftOptions) {
		config.recallOptions = append(config.recallOptions, options...)
	}
}

// OpenFlowcraftStore constructs an embedded Flowcraft memory. A non-empty Dir
// selects its durable workspace backend; an empty Dir uses process memory.
func OpenFlowcraftStore(ctx context.Context, config FlowcraftConfig, loader FlowcraftModelLoader, options ...FlowcraftOption) (*FlowcraftStore, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	resolved := flowcraftOptions{}
	for _, option := range options {
		if option != nil {
			option(&resolved)
		}
	}
	recallOptions := append([]recall.Option(nil), resolved.recallOptions...)
	var backend *flowworkspace.Backend
	var temporal recall.TemporalStore
	var queue *flowcraftAsyncQueue
	opened := false
	defer func() {
		if !opened && backend != nil {
			_ = backend.Close()
		}
	}()
	if config.Dir != "" {
		var err error
		backend, err = flowworkspace.Open(config.Dir)
		if err != nil {
			return nil, mapFlowcraftError("open workspace", err)
		}
		temporal = backend.TemporalStore()
		queue = newFlowcraftAsyncQueue(backend.AsyncSemanticQueue())
		recallOptions = append(recallOptions,
			recall.WithTemporalStore(temporal),
			recall.WithEvidenceStore(backend.EvidenceStore()),
			recall.WithSideEffectOutbox(backend.SideEffectOutbox()),
			recall.WithAsyncSemanticQueue(queue),
		)
	} else {
		temporal = recall.NewInMemoryTemporalStore()
		recallOptions = append(recallOptions, recall.WithTemporalStore(temporal))
		if config.Async.Enabled {
			queue = newFlowcraftAsyncQueue(recall.NewInMemoryAsyncSemanticQueue())
			recallOptions = append(recallOptions, recall.WithAsyncSemanticQueue(queue))
		}
	}
	if config.ExtractionModel != "" {
		if loader == nil {
			return nil, fmt.Errorf("%w: flowcraft extraction_model requires a model loader", ErrInvalidInput)
		}
		model, err := loader.LoadLLM(ctx, config.ExtractionModel)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft extraction model %q: %w", config.ExtractionModel, err)
		}
		if model == nil {
			return nil, fmt.Errorf("%w: flowcraft extraction model %q resolved to nil", ErrUnavailable, config.ExtractionModel)
		}
		extractorOptions := []recall.LLMExtractorOption{
			recall.WithLLMExtractionMode(recall.LLMExtractionMode(config.ExtractionMode)),
			recall.WithLLMExtractorTemperature(config.Temperature),
		}
		if config.SystemPrompt != "" {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorSystemPrompt(config.SystemPrompt))
		}
		if config.SchemaName != "" {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorSchemaName(config.SchemaName))
		}
		if config.StageTimeout > 0 {
			extractorOptions = append(extractorOptions, recall.WithLLMExtractorStageTimeout(config.StageTimeout))
		}
		recallOptions = append(recallOptions, recall.WithLLMExtractor(model, extractorOptions...))
	}
	if config.EmbeddingModel != "" {
		if loader == nil {
			return nil, fmt.Errorf("%w: flowcraft embedding_model requires a model loader", ErrInvalidInput)
		}
		embedder, err := loader.LoadEmbedder(ctx, config.EmbeddingModel)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft embedding model %q: %w", config.EmbeddingModel, err)
		}
		if embedder == nil {
			return nil, fmt.Errorf("%w: flowcraft embedding model %q resolved to nil", ErrUnavailable, config.EmbeddingModel)
		}
		recallOptions = append(recallOptions, recall.WithEmbedder(embedder))
	}
	if config.RerankModel != "" {
		if loader == nil {
			return nil, fmt.Errorf("%w: flowcraft rerank_model requires a model loader", ErrInvalidInput)
		}
		model, err := loader.LoadLLM(ctx, config.RerankModel)
		if err != nil {
			return nil, fmt.Errorf("load flowcraft rerank model %q: %w", config.RerankModel, err)
		}
		if model == nil {
			return nil, fmt.Errorf("%w: flowcraft rerank model %q resolved to nil", ErrUnavailable, config.RerankModel)
		}
		recallOptions = append(recallOptions, recall.WithReranker(recall.NewLLMReranker(model)))
	}
	memory, err := recall.New(recallOptions...)
	if err != nil {
		if backend != nil {
			_ = backend.Close()
		}
		return nil, mapFlowcraftError("construct memory", err)
	}
	opened = true
	store := newFlowcraftStore(config, memory, temporal, queue, backend)
	if err := store.rehydrateOperations(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}
