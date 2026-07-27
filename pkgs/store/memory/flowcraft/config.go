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

	// Tier applies Flowcraft's write-time salience intent to every observation.
	Tier string
	// LaneNames are the closed portable Layout lane names. Extracted facts use
	// the required "<lane>: ..." content prefix, which the adapter projects
	// back into the provider-neutral "lane" attribute for Graph filtering.
	LaneNames []string
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
	switch c.Tier {
	case "", "core", "general", "data", "storage":
	default:
		return fmt.Errorf("%w: flowcraft memory tier %q is invalid", errInvalidInput, c.Tier)
	}
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

func (c Config) normalized() Config {
	c.Extraction.Model = strings.TrimSpace(c.Extraction.Model)
	c.Embedding.Model = strings.TrimSpace(c.Embedding.Model)
	c.Rerank.Model = strings.TrimSpace(c.Rerank.Model)
	c.Tier = strings.TrimSpace(c.Tier)
	lanes := make([]string, 0, len(c.LaneNames))
	seen := make(map[string]struct{}, len(c.LaneNames))
	for _, lane := range c.LaneNames {
		lane = strings.TrimSpace(lane)
		if lane == "" {
			continue
		}
		if _, duplicate := seen[lane]; duplicate {
			continue
		}
		seen[lane] = struct{}{}
		lanes = append(lanes, lane)
	}
	c.LaneNames = lanes
	return c
}

func nativeScope(input scope) (recall.Scope, error) {
	input.AppID = strings.TrimSpace(input.AppID)
	input.UserID = strings.TrimSpace(input.UserID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.RunID = strings.TrimSpace(input.RunID)
	if input.AppID == "" {
		return recall.Scope{}, fmt.Errorf("%w: flowcraft memory app id is required", errInvalidInput)
	}
	if input.RunID != "" {
		return recall.Scope{}, fmt.Errorf("%w: flowcraft memory does not support run id", errUnsupported)
	}
	return recall.Scope{RuntimeID: input.AppID, UserID: input.UserID, AgentID: input.AgentID}, nil
}
