//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
	embeddingopenai "github.com/GizClaw/flowcraft/sdkx/embedding/openai"
	llmbytedance "github.com/GizClaw/flowcraft/sdkx/llm/bytedance"
	llmdeepseek "github.com/GizClaw/flowcraft/sdkx/llm/deepseek"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/openai/openai-go/option"
)

const (
	defaultDatasetPath   = "tests/locomo-e2e/testdata/locomo10_smoke.jsonl"
	defaultModelProvider = "deepseek"
	defaultModel         = "deepseek-v4-flash"
	defaultEmbedding     = "qwen3.7-text-embedding"
	defaultEmbeddingURL  = "https://dashscope.aliyuncs.com/compatible-mode/v1"
)

type liveSettings struct {
	datasetPath      string
	reportDir        string
	apiKey           string
	baseURL          string
	region           string
	modelProvider    string
	extractionModel  string
	embeddingModel   string
	embeddingDims    int
	embeddingAPIKey  string
	embeddingBaseURL string
	rerankModel      string
	answerModel      string
	topK             int
	ingestTimeout    time.Duration
	qaTimeout        time.Duration
	minF1            float64
	minEvidenceHit   float64
}

type liveNeeds struct {
	embedding bool
}

func requireLiveSettings(t *testing.T, needs liveNeeds) liveSettings {
	t.Helper()
	values := map[string]string{
		"GIZCLAW_LOCOMO_E2E_MODEL_API_KEY": os.Getenv("GIZCLAW_LOCOMO_E2E_MODEL_API_KEY"),
	}
	required := []string{"GIZCLAW_LOCOMO_E2E_MODEL_API_KEY"}
	settings := liveSettings{
		datasetPath:      envOr("GIZCLAW_LOCOMO_E2E_DATASET", defaultDatasetPath),
		reportDir:        envOr("GIZCLAW_LOCOMO_E2E_REPORT_DIR", "tests/locomo-e2e/reports"),
		apiKey:           values["GIZCLAW_LOCOMO_E2E_MODEL_API_KEY"],
		baseURL:          os.Getenv("GIZCLAW_LOCOMO_E2E_MODEL_BASE_URL"),
		region:           envOr("GIZCLAW_LOCOMO_E2E_MODEL_REGION", "cn-beijing"),
		modelProvider:    envOr("GIZCLAW_LOCOMO_E2E_MODEL_PROVIDER", defaultModelProvider),
		extractionModel:  envOr("GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL", defaultModel),
		embeddingModel:   envOr("GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL", defaultEmbedding),
		embeddingAPIKey:  os.Getenv("GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY"),
		embeddingBaseURL: envOr("GIZCLAW_LOCOMO_E2E_EMBEDDING_BASE_URL", defaultEmbeddingURL),
		rerankModel:      os.Getenv("GIZCLAW_LOCOMO_E2E_RERANK_MODEL"),
		answerModel:      envOr("GIZCLAW_LOCOMO_E2E_ANSWER_MODEL", defaultModel),
		topK:             envInt(t, "GIZCLAW_LOCOMO_E2E_TOP_K", 10),
		ingestTimeout:    envDuration(t, "GIZCLAW_LOCOMO_E2E_INGEST_TIMEOUT", 10*time.Minute),
		qaTimeout:        envDuration(t, "GIZCLAW_LOCOMO_E2E_QA_TIMEOUT", 2*time.Minute),
		minF1:            envRatio(t, "GIZCLAW_LOCOMO_E2E_MIN_F1", 0.05),
		minEvidenceHit:   envRatio(t, "GIZCLAW_LOCOMO_E2E_MIN_EVIDENCE_HIT_RATE", 0.50),
	}
	if needs.embedding {
		settings.embeddingDims = envInt(t, "GIZCLAW_LOCOMO_E2E_EMBEDDING_DIMENSIONS", 1024)
		values["GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL"] = settings.embeddingModel
		values["GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY"] = settings.embeddingAPIKey
		required = append(required,
			"GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL",
			"GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY",
		)
	}
	switch settings.modelProvider {
	case "bytedance", "deepseek":
	default:
		t.Fatalf("GIZCLAW_LOCOMO_E2E_MODEL_PROVIDER must be bytedance or deepseek, got %q", settings.modelProvider)
	}
	if err := validateRequired(values, required...); err != nil {
		t.Fatal(err)
	}
	return settings
}

func validateRequired(values map[string]string, names ...string) error {
	var invalid []string
	for _, name := range names {
		value := strings.TrimSpace(values[name])
		lower := strings.ToLower(value)
		if value == "" || strings.Contains(lower, "dummy") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "placeholder") ||
			strings.Contains(lower, "replace") || strings.Contains(lower, "changeme") {
			invalid = append(invalid, name)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("LoCoMo preflight invalid or missing required inputs: %s", strings.Join(invalid, ", "))
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s must be a positive integer", name)
	}
	return parsed
}

func envDuration(t *testing.T, name string, fallback time.Duration) time.Duration {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s must be a positive duration", name)
	}
	return parsed
}

func envRatio(t *testing.T, name string, fallback float64) float64 {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		t.Fatalf("%s must be a number between 0 and 1", name)
	}
	return parsed
}

type modelLoader struct {
	provider         string
	apiKey           string
	baseURL          string
	region           string
	embeddingAPIKey  string
	embeddingBaseURL string
}

func (l modelLoader) LoadLLM(_ context.Context, name string) (llm.LLM, error) {
	return newLLM(l.provider, name, l.apiKey, l.baseURL, l.region)
}

func (l modelLoader) LoadEmbedder(_ context.Context, name string) (embedding.Embedder, error) {
	var options []option.RequestOption
	if l.embeddingBaseURL != "" {
		options = append(options, option.WithBaseURL(l.embeddingBaseURL))
	}
	embedder := embeddingopenai.New(l.embeddingAPIKey, name, options...)
	if embedder == nil {
		return nil, errors.New("OpenAI-compatible embedder is unavailable")
	}
	return embedder, nil
}

func (s liveSettings) loader() modelLoader {
	return modelLoader{
		provider:         s.modelProvider,
		apiKey:           s.apiKey,
		baseURL:          s.baseURL,
		region:           s.region,
		embeddingAPIKey:  s.embeddingAPIKey,
		embeddingBaseURL: s.embeddingBaseURL,
	}
}

func newAnswerModel(settings liveSettings) (llm.LLM, error) {
	return newLLM(settings.modelProvider, settings.answerModel, settings.apiKey, settings.baseURL, settings.region)
}

func newLLM(provider, model, apiKey, baseURL, region string) (llm.LLM, error) {
	var raw llm.LLM
	var err error
	switch provider {
	case "bytedance":
		raw, err = llmbytedance.New(model, apiKey, baseURL, region, 2)
	case "deepseek":
		raw, err = llmdeepseek.New(model, apiKey, baseURL)
	default:
		return nil, fmt.Errorf("unsupported LoCoMo model provider %q", provider)
	}
	if err != nil {
		return nil, err
	}
	spec := llm.DefaultRegistry.LookupModelSpec(provider, model)
	return llm.WithDefaults(llm.WithCaps(llm.WithLimits(raw, spec.Limits), spec.Caps), spec.Defaults), nil
}

func closeStore(store memorystore.Store, closer io.Closer) error {
	if closer != nil {
		return closer.Close()
	}
	if storeCloser, ok := store.(io.Closer); ok {
		return storeCloser.Close()
	}
	return nil
}

func repoPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	_, current, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(current), "..", "..", path)
}

func configFingerprint(values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = io.WriteString(hash, value)
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil))
}
