package flowcraft

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
)

// ModelLoader resolves application-owned model resource names. The adapter
// never reads model or provider configuration files itself.
type ModelLoader interface {
	LoadLLM(ctx context.Context, name string) (llm.LLM, error)
	LoadEmbedder(ctx context.Context, name string) (embedding.Embedder, error)
}

// Config contains only constructed runtime dependencies and adapter policy.
// All injected dependencies remain caller-owned.
type Config struct {
	Loader ModelLoader

	Extraction ExtractionConfig
	Embedding  EmbeddingConfig
	Rerank     RerankConfig

	RetrievalIndex   retrieval.Index
	TemporalStore    recall.TemporalStore
	EvidenceStore    recall.EvidenceStore
	AsyncQueue       recall.AsyncSemanticQueue
	SideEffectOutbox recall.SideEffectOutbox

	GraphEnabled bool
}

type ExtractionConfig struct {
	Model        string
	Mode         recall.LLMExtractionMode
	SystemPrompt string
	SchemaName   string
	Temperature  *float64
	StageTimeout time.Duration
}

type EmbeddingConfig struct {
	Model string
}

type RerankConfig struct {
	Model string
}

func (c Config) validate() error {
	switch c.Extraction.Mode {
	case "", recall.LLMExtractionSinglePass, recall.LLMExtractionTwoPass:
	default:
		return fmt.Errorf("%w: flowcraft extraction mode %q is invalid", errInvalidInput, c.Extraction.Mode)
	}
	if c.Extraction.StageTimeout < 0 {
		return fmt.Errorf("%w: flowcraft extraction stage timeout must not be negative", errInvalidInput)
	}
	if strings.TrimSpace(c.Extraction.Model) == "" && c.AsyncQueue != nil {
		return fmt.Errorf("%w: flowcraft async queue requires an extraction model", errInvalidInput)
	}
	if c.Loader == nil && (strings.TrimSpace(c.Extraction.Model) != "" || strings.TrimSpace(c.Embedding.Model) != "" || strings.TrimSpace(c.Rerank.Model) != "") {
		return fmt.Errorf("%w: configured flowcraft models require a model loader", errInvalidInput)
	}
	return nil
}

func nativeScope(scope scope) recall.Scope {
	return recall.Scope{RuntimeID: "gizclaw", UserID: string(scope)}
}
