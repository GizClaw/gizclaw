package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
)

// FlowcraftConfig configures one embedded Flowcraft memory partition.
type FlowcraftConfig struct {
	Dir             string               `yaml:"dir"`
	RuntimeID       string               `yaml:"runtime_id"`
	AgentID         string               `yaml:"agent_id"`
	UserID          string               `yaml:"user_id"`
	ExtractionModel string               `yaml:"extraction_model"`
	EmbeddingModel  string               `yaml:"embedding_model"`
	RerankModel     string               `yaml:"rerank_model"`
	ExtractionMode  string               `yaml:"extraction_mode"`
	SystemPrompt    string               `yaml:"system_prompt"`
	SchemaName      string               `yaml:"schema_name"`
	Temperature     float64              `yaml:"temperature"`
	StageTimeout    time.Duration        `yaml:"stage_timeout"`
	Async           FlowcraftAsyncConfig `yaml:"async"`
}

// FlowcraftAsyncConfig enables caller-driven asynchronous extraction.
type FlowcraftAsyncConfig struct {
	Enabled  bool   `yaml:"enabled"`
	WorkerID string `yaml:"worker_id"`
}

func (c FlowcraftConfig) validate() error {
	if strings.TrimSpace(c.RuntimeID) == "" {
		return fmt.Errorf("%w: flowcraft runtime_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(c.UserID) == "" {
		return fmt.Errorf("%w: flowcraft user_id is required", ErrInvalidInput)
	}
	switch recall.LLMExtractionMode(c.ExtractionMode) {
	case "", recall.LLMExtractionSinglePass, recall.LLMExtractionTwoPass:
	default:
		return fmt.Errorf("%w: flowcraft extraction_mode %q is invalid", ErrInvalidInput, c.ExtractionMode)
	}
	if c.StageTimeout < 0 {
		return fmt.Errorf("%w: flowcraft stage_timeout must not be negative", ErrInvalidInput)
	}
	if c.Async.Enabled && strings.TrimSpace(c.ExtractionModel) == "" {
		return fmt.Errorf("%w: flowcraft async extraction requires extraction_model", ErrInvalidInput)
	}
	return nil
}

func (c FlowcraftConfig) scope() recall.Scope {
	return recall.Scope{RuntimeID: c.RuntimeID, AgentID: c.AgentID, UserID: c.UserID}
}
