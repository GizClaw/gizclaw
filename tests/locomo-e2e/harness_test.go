//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/eval/dataset"
	"github.com/GizClaw/flowcraft/eval/locomo"
	"github.com/GizClaw/flowcraft/eval/locomo/runners"
	"github.com/GizClaw/flowcraft/eval/metrics"
	"github.com/GizClaw/flowcraft/sdk/embedding"
	"github.com/GizClaw/flowcraft/sdk/llm"
	embeddingopenai "github.com/GizClaw/flowcraft/sdkx/embedding/openai"
	llmopenai "github.com/GizClaw/flowcraft/sdkx/llm/openai"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/openai/openai-go/option"
)

type liveSettings struct {
	datasetPath       string
	reportDir         string
	apiKey            string
	baseURL           string
	extractionModel   string
	embeddingModel    string
	rerankModel       string
	answerModel       string
	judgeModel        string
	topK              int
	concurrency       int
	ingestConcurrency int
	ingestTimeout     time.Duration
	qaTimeout         time.Duration
}

type liveNeeds struct {
	extraction bool
	embedding  bool
}

func requireLiveSettings(t *testing.T, needs liveNeeds) liveSettings {
	t.Helper()
	values := map[string]string{
		"GIZCLAW_LOCOMO_E2E_DATASET":        os.Getenv("GIZCLAW_LOCOMO_E2E_DATASET"),
		"GIZCLAW_LOCOMO_E2E_OPENAI_API_KEY": os.Getenv("GIZCLAW_LOCOMO_E2E_OPENAI_API_KEY"),
		"GIZCLAW_LOCOMO_E2E_ANSWER_MODEL":   os.Getenv("GIZCLAW_LOCOMO_E2E_ANSWER_MODEL"),
	}
	required := []string{"GIZCLAW_LOCOMO_E2E_DATASET", "GIZCLAW_LOCOMO_E2E_OPENAI_API_KEY", "GIZCLAW_LOCOMO_E2E_ANSWER_MODEL"}
	if needs.extraction {
		values["GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL"] = os.Getenv("GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL")
		required = append(required, "GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL")
	}
	if needs.embedding {
		values["GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL"] = os.Getenv("GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL")
		required = append(required, "GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL")
	}
	if err := validateRequired(values, required...); err != nil {
		t.Fatal(err)
	}
	return liveSettings{
		datasetPath:       values["GIZCLAW_LOCOMO_E2E_DATASET"],
		reportDir:         envOr("GIZCLAW_LOCOMO_E2E_REPORT_DIR", "tests/locomo-e2e/reports"),
		apiKey:            values["GIZCLAW_LOCOMO_E2E_OPENAI_API_KEY"],
		baseURL:           os.Getenv("GIZCLAW_LOCOMO_E2E_OPENAI_BASE_URL"),
		extractionModel:   os.Getenv("GIZCLAW_LOCOMO_E2E_EXTRACTION_MODEL"),
		embeddingModel:    os.Getenv("GIZCLAW_LOCOMO_E2E_EMBEDDING_MODEL"),
		rerankModel:       os.Getenv("GIZCLAW_LOCOMO_E2E_RERANK_MODEL"),
		answerModel:       values["GIZCLAW_LOCOMO_E2E_ANSWER_MODEL"],
		judgeModel:        os.Getenv("GIZCLAW_LOCOMO_E2E_JUDGE_MODEL"),
		topK:              envInt(t, "GIZCLAW_LOCOMO_E2E_TOP_K", 30),
		concurrency:       envInt(t, "GIZCLAW_LOCOMO_E2E_CONCURRENCY", 1),
		ingestConcurrency: envInt(t, "GIZCLAW_LOCOMO_E2E_INGEST_CONCURRENCY", 1),
		ingestTimeout:     envDuration(t, "GIZCLAW_LOCOMO_E2E_INGEST_TIMEOUT", 20*time.Minute),
		qaTimeout:         envDuration(t, "GIZCLAW_LOCOMO_E2E_QA_TIMEOUT", 2*time.Minute),
	}
}

func validateRequired(values map[string]string, names ...string) error {
	var missing []string
	for _, name := range names {
		if strings.TrimSpace(values[name]) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("LoCoMo preflight missing required inputs: %s", strings.Join(missing, ", "))
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

type openAIModelLoader struct {
	apiKey  string
	baseURL string
}

func (l openAIModelLoader) LoadLLM(_ context.Context, name string) (llm.LLM, error) {
	return llmopenai.New(name, l.apiKey, l.baseURL)
}

func (l openAIModelLoader) LoadEmbedder(_ context.Context, name string) (embedding.Embedder, error) {
	var options []option.RequestOption
	if l.baseURL != "" {
		options = append(options, option.WithBaseURL(l.baseURL))
	}
	embedder := embeddingopenai.New(l.apiKey, name, options...)
	if embedder == nil {
		return nil, errors.New("openai embedder is unavailable")
	}
	return embedder, nil
}

type storeRunner struct {
	name   string
	store  memorystore.Store
	closer io.Closer
	close  sync.Once
	err    error
}

func (r *storeRunner) Name() string { return r.name }

func (r *storeRunner) Save(ctx context.Context, scope runners.Scope, messages []llm.Message) (int, time.Duration, error) {
	turns := make([]memorystore.Turn, 0, len(messages))
	for index, message := range messages {
		text := strings.TrimSpace(message.Content())
		if text == "" {
			continue
		}
		turns = append(turns, memorystore.Turn{ID: fmt.Sprintf("turn-%d", index), Role: memorystore.Role(message.Role), Text: text})
	}
	return r.observe(ctx, scope, memorystore.Observation{Turns: turns})
}

// SaveSourceTurns preserves LoCoMo evidence IDs as provider-neutral turn IDs,
// allowing the pinned evaluator to compute evidence-level retrieval metrics.
func (r *storeRunner) SaveSourceTurns(ctx context.Context, scope runners.Scope, turns []runners.RawTurn) (int, time.Duration, error) {
	converted := make([]memorystore.Turn, 0, len(turns))
	observationID := ""
	for _, turn := range turns {
		text := strings.TrimSpace(turn.Content)
		if text == "" {
			continue
		}
		if observationID == "" {
			observationID = turn.SessionID
		}
		converted = append(converted, memorystore.Turn{ID: turn.EvidenceID, Role: memorystore.Role(turn.Role), Text: text})
	}
	return r.observe(ctx, scope, memorystore.Observation{ID: observationID, Turns: converted})
}

func (r *storeRunner) observe(ctx context.Context, scope runners.Scope, observation memorystore.Observation) (int, time.Duration, error) {
	started := time.Now()
	observation.Scope = evalScope(scope)
	result, err := r.store.Observe(ctx, observation)
	if err != nil {
		return 0, time.Since(started), err
	}
	if result.Operation != nil && result.Operation.Status == memorystore.OperationPending {
		waiter, ok := r.store.(memorystore.OperationWaiter)
		if !ok {
			return 0, time.Since(started), errors.New("memory store returned a pending operation without OperationWaiter")
		}
		result, err = waiter.Wait(ctx, result.Operation.ID)
		if err != nil {
			return 0, time.Since(started), err
		}
		if result.Operation != nil && result.Operation.Status == memorystore.OperationFailed {
			return 0, time.Since(started), fmt.Errorf("memory operation failed: %s", result.Operation.Error)
		}
	}
	return len(result.Facts), time.Since(started), nil
}

func (r *storeRunner) Recall(ctx context.Context, scope runners.Scope, query string, topK int) ([]runners.RecallArtifact, time.Duration, error) {
	started := time.Now()
	result, err := r.store.Recall(ctx, memorystore.Query{Scope: evalScope(scope), Text: query, Limit: topK})
	if err != nil {
		return nil, time.Since(started), err
	}
	artifacts := make([]runners.RecallArtifact, len(result.Matches))
	for index, match := range result.Matches {
		var evidenceIDs []string
		for _, source := range match.Fact.Sources {
			evidenceIDs = append(evidenceIDs, source.TurnIDs...)
		}
		kind, _ := match.Fact.Attributes["kind"].(string)
		artifacts[index] = runners.RecallArtifact{
			ID: match.Fact.ID, Content: match.Fact.Text, Score: match.Score,
			Kind: kind, EvidenceIDs: evidenceIDs, Metadata: match.Fact.Attributes,
		}
	}
	return artifacts, time.Since(started), nil
}

func (r *storeRunner) Close() error {
	r.close.Do(func() {
		if r.closer != nil {
			r.err = r.closer.Close()
			return
		}
		if closer, ok := r.store.(io.Closer); ok {
			r.err = closer.Close()
		}
	})
	return r.err
}

func evalScope(scope runners.Scope) memorystore.Scope {
	return memorystore.Scope(scope.RuntimeID + ":" + scope.AgentID + ":" + scope.UserID)
}

type reportEnvelope struct {
	Profile           string         `json:"profile"`
	ConfigFingerprint string         `json:"config_fingerprint"`
	DatasetIdentity   string         `json:"dataset_identity"`
	Report            *locomo.Report `json:"report"`
}

func runLiveProfile(t *testing.T, settings liveSettings, profile, fingerprint string, store memorystore.Store, closer io.Closer) {
	t.Helper()
	runner := &storeRunner{name: profile, store: store, closer: closer}
	t.Cleanup(func() {
		if err := runner.Close(); err != nil {
			t.Errorf("close %s: %v", profile, err)
		}
	})
	ds, identity, err := loadDataset(settings.datasetPath)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := llmopenai.New(settings.answerModel, settings.apiKey, settings.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	var judge metrics.Judge = metrics.EMJudge{}
	if settings.judgeModel != "" {
		judgeLLM, err := llmopenai.New(settings.judgeModel, settings.apiKey, settings.baseURL)
		if err != nil {
			t.Fatal(err)
		}
		judge = metrics.LLMJudge{LLM: judgeLLM, Prompt: metrics.LocoMoLLMJudgePrompt}
	}
	unique := configFingerprint(profile, time.Now().UTC().Format(time.RFC3339Nano))[:16]
	report, err := locomo.Run(context.Background(), runner, ds, locomo.Options{
		TopK: settings.topK, Judge: judge, UseExtractor: true, AnswerLLM: answer,
		Concurrency: settings.concurrency, IngestConcurrency: settings.ingestConcurrency,
		IngestTimeout: settings.ingestTimeout, QATimeout: settings.qaTimeout,
		RuntimeID: "gizclaw-locomo", UserID: profile + "-" + unique,
		RetrievalBackend: profile, RunName: profile,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := reportEnvelope{Profile: profile, ConfigFingerprint: fingerprint, DatasetIdentity: identity, Report: report}
	if err := writeReport(settings.reportDir, envelope); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: n=%d em=%.4f f1=%.4f judge=%.4f", profile, report.N, report.Aggregate.EM, report.Aggregate.F1, report.Aggregate.Judge)
}

func loadDataset(path string) (*dataset.Dataset, string, error) {
	if path == "synthetic" {
		return dataset.Synthetic(), "flowcraft:synthetic", nil
	}
	absolute := repoPath(path)
	raw, err := os.ReadFile(absolute)
	if err != nil {
		return nil, "", err
	}
	ds, err := dataset.LoadJSONL(absolute)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return ds, filepath.Base(path) + ":sha256:" + hex.EncodeToString(sum[:]), nil
}

func writeReport(dir string, envelope reportEnvelope) error {
	dir = repoPath(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	name := envelope.Profile + "-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
	return os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0o600)
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
