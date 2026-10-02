//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/GizClaw/flowcraft/sdk/llm"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type benchmarkAnswerer interface {
	Answer(context.Context, string, []memorystore.Match) (string, error)
}

type llmAnswerer struct {
	model llm.LLM
}

const answerSystemPrompt = "Answer using only the recalled memory evidence. Check that facts concern the person in the question. Combine relevant facts across memories, preserve negation, and calculate relative dates from the source conversation time rather than today's date. For lists and counts, include all supported distinct items. Return only a concise answer. If evidence is insufficient, return unknown."

const answerMaxTokens = 512

func (a llmAnswerer) Answer(ctx context.Context, question string, matches []memorystore.Match) (string, error) {
	var evidence strings.Builder
	for index, match := range matches {
		fmt.Fprintf(&evidence, "[%d] %s\n", index+1, strings.TrimSpace(match.Fact.Text))
	}
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleSystem, answerSystemPrompt),
		llm.NewTextMessage(llm.RoleUser, "Memory evidence:\n"+evidence.String()+"\nQuestion: "+question),
	}
	message, _, err := a.model.Generate(ctx, messages,
		llm.WithTemperature(0),
		llm.WithMaxTokens(answerMaxTokens),
		llm.WithThinking(false),
	)
	if err != nil {
		return "", err
	}
	answer := strings.TrimSpace(message.Content())
	if answer == "" {
		return "", errors.New("answer model returned empty content")
	}
	return answer, nil
}

type reportEnvelope struct {
	Profile           string                  `json:"profile"`
	ConfigFingerprint string                  `json:"config_fingerprint"`
	DatasetIdentity   string                  `json:"dataset_identity"`
	Models            reportModels            `json:"models"`
	StartedAt         time.Time               `json:"started_at"`
	FinishedAt        time.Time               `json:"finished_at"`
	Duration          time.Duration           `json:"duration_ns"`
	Ingest            []ingestResult          `json:"ingest"`
	Questions         []questionResult        `json:"questions"`
	Aggregate         aggregateResult         `json:"aggregate"`
	ByCategory        map[int]aggregateResult `json:"by_category"`
	Protocol          reportProtocol          `json:"protocol"`
	QualityGate       qualityGate             `json:"quality_gate"`
	Cleanup           []scopeCleanupResult    `json:"cleanup,omitempty"`
}

type reportModels struct {
	ExtractionPolicyFingerprint string `json:"extraction_policy_fingerprint,omitempty"`
	SDKVersion                  string `json:"sdk_version,omitempty"`
	Provider                    string `json:"provider,omitempty"`
	Extraction                  string `json:"extraction,omitempty"`
	ExtractionProvider          string `json:"extraction_provider,omitempty"`
	ExtractionThinking          string `json:"extraction_thinking,omitempty"`
	ExtractionServiceTier       string `json:"extraction_service_tier,omitempty"`
	ExtractionMaxTokens         int    `json:"extraction_max_tokens,omitempty"`
	Embedding                   string `json:"embedding,omitempty"`
	EmbeddingDimensions         int    `json:"embedding_dimensions,omitempty"`
	EmbeddingProtocol           string `json:"embedding_protocol,omitempty"`
	EmbeddingPolicyFingerprint  string `json:"embedding_policy_fingerprint,omitempty"`
	Rerank                      string `json:"rerank,omitempty"`
	Answer                      string `json:"answer"`
}

type reportProtocol struct {
	Version                 string `json:"version"`
	ObservationGranularity  string `json:"observation_granularity"`
	TopK                    int    `json:"top_k"`
	AnswerMaxTokens         int    `json:"answer_max_tokens"`
	AnswerPromptFingerprint string `json:"answer_prompt_fingerprint"`
}

type ingestResult struct {
	ConversationID string        `json:"conversation_id"`
	Observations   int           `json:"observations"`
	Turns          int           `json:"turns"`
	Facts          int           `json:"facts"`
	Duration       time.Duration `json:"duration_ns"`
}

type questionResult struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Category       int            `json:"category"`
	Answerable     bool           `json:"answerable"`
	Query          string         `json:"-"`
	GoldAnswers    []string       `json:"-"`
	Prediction     string         `json:"-"`
	ExactMatch     bool           `json:"exact_match"`
	F1             float64        `json:"f1"`
	AdversarialOK  bool           `json:"adversarial_rejection"`
	EvidenceHit    *bool          `json:"evidence_hit,omitempty"`
	RecallDuration time.Duration  `json:"recall_duration_ns"`
	AnswerDuration time.Duration  `json:"answer_duration_ns"`
	Error          string         `json:"error,omitempty"`
	Recalled       []recalledFact `json:"recalled"`
}

type recalledFact struct {
	ID          string   `json:"id"`
	Text        string   `json:"-"`
	Score       float64  `json:"score"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type aggregateResult struct {
	Questions       int     `json:"questions"`
	Answerable      int     `json:"answerable_questions"`
	Adversarial     int     `json:"adversarial_questions"`
	AdversarialOK   int     `json:"adversarial_rejections"`
	AdversarialRate float64 `json:"adversarial_accuracy"`
	Succeeded       int     `json:"succeeded"`
	Failed          int     `json:"failed"`
	ExactMatch      float64 `json:"exact_match"`
	F1              float64 `json:"f1"`
	EvidenceScored  int     `json:"evidence_scored"`
	EvidenceHitRate float64 `json:"evidence_hit_rate"`
}

type qualityGate struct {
	MinimumF1              float64 `json:"minimum_f1"`
	MinimumEvidenceHitRate float64 `json:"minimum_evidence_hit_rate"`
}

type scopeCleanupResult struct {
	AppID         string `json:"app_id"`
	VerifiedEmpty bool   `json:"verified_empty"`
	Error         string `json:"error,omitempty"`
}

func runLiveProfile(t *testing.T, settings liveSettings, profile, fingerprint string, models reportModels, store memorystore.Store, closer io.Closer) {
	t.Helper()
	t.Cleanup(func() {
		if err := closeStore(store, closer); err != nil {
			t.Errorf("close %s: %v", profile, err)
		}
	})
	dataset, identity, err := loadDataset(settings.datasetPath)
	if err != nil {
		t.Fatal(err)
	}
	answerModel, err := newAnswerModel(settings)
	if err != nil {
		t.Fatal(err)
	}
	models.Answer = settings.answerModel
	models.Provider = settings.modelProvider
	if models.Embedding != "" {
		models.EmbeddingDimensions = settings.embeddingDims
	}
	var envelope *reportEnvelope
	var onScope func(memorystore.Scope)
	if settings.cleanupScopes {
		if _, ok := store.(memorystore.ScopePurger); !ok {
			t.Fatal("remote LoCoMo profile requires scoped purge and empty verification")
		}
		onScope = func(scope memorystore.Scope) {
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				err := purgeBenchmarkScope(ctx, store, scope, 2*time.Second)
				result := scopeCleanupResult{AppID: scope.AppID, VerifiedEmpty: err == nil}
				if err != nil {
					result.Error = err.Error()
					t.Errorf("purge LoCoMo scope %s: %v", scope.AppID, err)
				} else {
					t.Logf("purged LoCoMo scope %s: verified empty", scope.AppID)
				}
				if envelope != nil {
					envelope.Cleanup = append(envelope.Cleanup, result)
					if err := writeReport(settings.reportDir, *envelope); err != nil {
						t.Errorf("write LoCoMo cleanup receipt: %v", err)
					}
				}
			})
		}
	}
	envelope, err = runBenchmark(context.Background(), benchmarkOptions{
		Profile:                profile,
		ConfigFingerprint:      fingerprint,
		DatasetIdentity:        identity,
		Models:                 models,
		TopK:                   settings.topK,
		IngestTimeout:          settings.ingestTimeout,
		QATimeout:              settings.qaTimeout,
		MinimumF1:              settings.minF1,
		MinimumEvidenceHit:     settings.minEvidenceHit,
		Logf:                   t.Logf,
		ObservationGranularity: settings.observationGranularity,
		OnScope:                onScope,
	}, store, dataset, llmAnswerer{model: answerModel})
	if envelope != nil {
		if writeErr := writeReport(settings.reportDir, *envelope); writeErr != nil {
			t.Errorf("write report: %v", writeErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if settings.baselineReport != "" {
		if err := compareBaseline(settings.baselineReport, *envelope, settings.maxF1Drop, settings.maxEvidenceDrop); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%s: n=%d em=%.4f f1=%.4f evidence_hit=%.4f adversarial=%.4f duration=%s", profile,
		envelope.Aggregate.Questions, envelope.Aggregate.ExactMatch,
		envelope.Aggregate.F1, envelope.Aggregate.EvidenceHitRate,
		envelope.Aggregate.AdversarialRate, envelope.Duration)
}

type benchmarkOptions struct {
	OnScope                func(memorystore.Scope)
	ObservationGranularity string
	Profile                string
	ConfigFingerprint      string
	DatasetIdentity        string
	Models                 reportModels
	TopK                   int
	IngestTimeout          time.Duration
	QATimeout              time.Duration
	MinimumF1              float64
	MinimumEvidenceHit     float64
	Logf                   func(string, ...any)
}

func runBenchmark(ctx context.Context, options benchmarkOptions, store memorystore.Store, dataset *benchmarkDataset, answerer benchmarkAnswerer) (*reportEnvelope, error) {
	started := time.Now().UTC()
	granularity := options.ObservationGranularity
	if granularity == "" {
		granularity = "session"
	}
	envelope := &reportEnvelope{
		Profile: options.Profile, ConfigFingerprint: options.ConfigFingerprint,
		DatasetIdentity: options.DatasetIdentity, StartedAt: started,
		Models: options.Models,
		Protocol: reportProtocol{Version: "gizclaw-locomo-v2", ObservationGranularity: granularity,
			TopK: options.TopK, AnswerMaxTokens: answerMaxTokens, AnswerPromptFingerprint: configFingerprint(answerSystemPrompt)},
		QualityGate: qualityGate{
			MinimumF1:              options.MinimumF1,
			MinimumEvidenceHitRate: options.MinimumEvidenceHit,
		},
	}
	runID := configFingerprint(options.Profile, started.Format(time.RFC3339Nano))[:16]
	scopes := make(map[string]memorystore.Scope, len(dataset.Conversations))
	for _, conversation := range dataset.Conversations {
		scope := memorystore.Scope{AppID: "locomo-" + runID + "-" + conversation.ID}
		scopes[conversation.ID] = scope
		if options.OnScope != nil {
			options.OnScope(scope)
		}
		result, err := ingestConversationMode(ctx, store, scope, conversation, options.IngestTimeout, options.Logf, granularity)
		envelope.Ingest = append(envelope.Ingest, result)
		if err != nil {
			finishReport(envelope)
			return envelope, fmt.Errorf("ingest %s: %w", conversation.ID, err)
		}
	}
	for _, question := range dataset.Questions {
		scope := scopes[question.ConversationID]
		result := runQuestion(ctx, store, answerer, scope, question, options.TopK, options.QATimeout)
		envelope.Questions = append(envelope.Questions, result)
		if options.Logf != nil {
			options.Logf("question %s complete: em=%t f1=%.4f error=%q", result.ID, result.ExactMatch, result.F1, result.Error)
		}
	}
	envelope.Aggregate = aggregateQuestions(envelope.Questions)
	envelope.ByCategory = make(map[int]aggregateResult)
	groups := make(map[int][]questionResult)
	for _, question := range envelope.Questions {
		groups[question.Category] = append(groups[question.Category], question)
	}
	for category, questions := range groups {
		envelope.ByCategory[category] = aggregateQuestions(questions)
	}
	finishReport(envelope)
	if envelope.Aggregate.Failed > 0 {
		return envelope, fmt.Errorf("%d of %d LoCoMo questions failed", envelope.Aggregate.Failed, envelope.Aggregate.Questions)
	}
	if envelope.Aggregate.F1 < options.MinimumF1 {
		return envelope, fmt.Errorf("LoCoMo F1 %.4f is below minimum %.4f", envelope.Aggregate.F1, options.MinimumF1)
	}
	if envelope.Aggregate.EvidenceScored > 0 && envelope.Aggregate.EvidenceHitRate < options.MinimumEvidenceHit {
		return envelope, fmt.Errorf("LoCoMo evidence hit rate %.4f is below minimum %.4f", envelope.Aggregate.EvidenceHitRate, options.MinimumEvidenceHit)
	}
	return envelope, nil
}

func finishReport(envelope *reportEnvelope) {
	envelope.FinishedAt = time.Now().UTC()
	envelope.Duration = envelope.FinishedAt.Sub(envelope.StartedAt)
}

func ingestConversation(ctx context.Context, store memorystore.Store, scope memorystore.Scope, conversation benchmarkConversation, timeout time.Duration, logf func(string, ...any)) (ingestResult, error) {
	return ingestConversationMode(ctx, store, scope, conversation, timeout, logf, "session")
}

func ingestConversationMode(ctx context.Context, store memorystore.Store, scope memorystore.Scope, conversation benchmarkConversation, timeout time.Duration, logf func(string, ...any), granularity string) (ingestResult, error) {
	result := ingestResult{ConversationID: conversation.ID}
	started := time.Now()
	observations := sessionObservations(scope, conversation)
	completed := 0
	for index, session := range observations {
		batch := []memorystore.Observation{session}
		if granularity == "turn" {
			batch = make([]memorystore.Observation, len(session.Turns))
			for turnIndex, turn := range session.Turns {
				batch[turnIndex] = memorystore.Observation{Scope: scope, ID: turn.ID, ObservedAt: turn.ObservedAt, Turns: []memorystore.Turn{turn}}
			}
		}
		sessionFacts := 0
		for _, observation := range batch {
			operationCtx, cancel := context.WithTimeout(ctx, timeout)
			observed, err := store.Observe(operationCtx, observation)
			if err == nil {
				observed, err = awaitObservation(operationCtx, store, observation.Scope, observed)
			}
			cancel()
			if err != nil {
				result.Duration = time.Since(started)
				return result, err
			}
			result.Observations++
			result.Turns += len(observation.Turns)
			result.Facts += len(observed.Facts)
			sessionFacts += len(observed.Facts)
			completed++
			if logf != nil {
				logf("ingest %s observation %d complete: turns=%d facts=%d", conversation.ID,
					completed, len(observation.Turns), len(observed.Facts))
			}
		}
		if sessionFacts < conversation.MinimumFactsPerSession {
			result.Duration = time.Since(started)
			return result, fmt.Errorf("session %q materialized %d facts, below dataset minimum %d",
				session.ID, sessionFacts, conversation.MinimumFactsPerSession)
		}
		if logf != nil {
			logf("ingest %s session %d/%d complete: turns=%d facts=%d", conversation.ID, index+1, len(observations), len(session.Turns), sessionFacts)
		}
	}
	result.Duration = time.Since(started)
	return result, nil
}

func sessionObservations(scope memorystore.Scope, conversation benchmarkConversation) []memorystore.Observation {
	var observations []memorystore.Observation
	for _, turn := range conversation.Turns {
		if len(observations) == 0 || observations[len(observations)-1].ID != turn.SessionID {
			observations = append(observations, memorystore.Observation{
				Scope: scope, ID: turn.SessionID, ObservedAt: turn.ObservedAt,
			})
		}
		current := &observations[len(observations)-1]
		current.Turns = append(current.Turns, memorystore.Turn{
			ID: turn.EvidenceID, Role: memorystore.Role(turn.Role), Speaker: turn.Speaker,
			Text: turn.Content, ObservedAt: turn.ObservedAt,
		})
	}
	return observations
}

func awaitObservation(ctx context.Context, store memorystore.Store, scope memorystore.Scope, result memorystore.ObserveResult) (memorystore.ObserveResult, error) {
	if result.Operation == nil {
		return result, nil
	}
	if result.Operation.Status == memorystore.OperationFailed {
		return result, fmt.Errorf("memory operation failed: %s", result.Operation.Error)
	}
	if result.Operation.Status != memorystore.OperationPending {
		return result, nil
	}
	waiter, ok := store.(memorystore.OperationWaiter)
	if !ok {
		return result, errors.New("memory store returned a pending operation without OperationWaiter")
	}
	result, err := waiter.Wait(ctx, memorystore.OperationRequest{Scope: scope, ID: result.Operation.ID})
	if err != nil {
		return result, err
	}
	if result.Operation == nil {
		return result, nil
	}
	switch result.Operation.Status {
	case memorystore.OperationSucceeded:
		return result, nil
	case memorystore.OperationFailed:
		return result, fmt.Errorf("memory operation failed: %s", result.Operation.Error)
	default:
		return result, fmt.Errorf("memory waiter returned non-terminal status %q", result.Operation.Status)
	}
}

func runQuestion(ctx context.Context, store memorystore.Store, answerer benchmarkAnswerer, scope memorystore.Scope, question benchmarkQuestion, topK int, timeout time.Duration) questionResult {
	result := questionResult{
		ID: question.ID, ConversationID: question.ConversationID,
		Category: question.Category, Answerable: question.Answerable != nil && *question.Answerable,
		Query: question.Query, GoldAnswers: question.GoldAnswers,
	}
	qaCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	recallStarted := time.Now()
	recalled, err := store.Recall(qaCtx, memorystore.Query{Scope: scope, Text: question.Query, Limit: topK})
	result.RecallDuration = time.Since(recallStarted)
	if err != nil {
		result.Error = "recall_error"
		return result
	}
	result.EvidenceHit = evidenceHit(recalled.Matches, question.EvidenceIDs)
	result.Recalled = reportRecalledFacts(recalled.Matches)
	if !matchesDescending(recalled.Matches) {
		result.Error = "recall: matches are not in descending score order"
		return result
	}
	answerStarted := time.Now()
	result.Prediction, err = answerer.Answer(qaCtx, question.Query, recalled.Matches)
	result.AnswerDuration = time.Since(answerStarted)
	if err != nil {
		result.Error = "answer_error"
		return result
	}
	if result.Answerable {
		result.ExactMatch = exactMatch(result.Prediction, question.GoldAnswers)
		result.F1 = tokenF1(result.Prediction, question.GoldAnswers)
	} else {
		result.AdversarialOK = isAdversarialRejection(result.Prediction)
	}
	return result
}

func matchesDescending(matches []memorystore.Match) bool {
	for index := 1; index < len(matches); index++ {
		if matches[index].Score > matches[index-1].Score {
			return false
		}
	}
	return true
}

func reportRecalledFacts(matches []memorystore.Match) []recalledFact {
	result := make([]recalledFact, 0, len(matches))
	for _, match := range matches {
		fact := recalledFact{ID: match.Fact.ID, Text: match.Fact.Text, Score: match.Score}
		for _, source := range match.Fact.Sources {
			fact.EvidenceIDs = append(fact.EvidenceIDs, source.TurnIDs...)
		}
		result = append(result, fact)
	}
	return result
}

func evidenceHit(matches []memorystore.Match, expected []string) *bool {
	if len(expected) == 0 {
		return nil
	}
	var recalled []string
	for _, match := range matches {
		for _, source := range match.Fact.Sources {
			recalled = append(recalled, source.TurnIDs...)
		}
	}
	hit := false
	for _, evidenceID := range expected {
		if slices.Contains(recalled, evidenceID) {
			hit = true
			break
		}
	}
	return &hit
}

func aggregateQuestions(results []questionResult) aggregateResult {
	aggregate := aggregateResult{Questions: len(results)}
	var exactMatches, evidenceHits int
	for _, result := range results {
		if result.Error != "" {
			aggregate.Failed++
			continue
		}
		aggregate.Succeeded++
		if result.Answerable {
			aggregate.Answerable++
			if result.ExactMatch {
				exactMatches++
			}
			aggregate.F1 += result.F1
		} else {
			aggregate.Adversarial++
			if result.AdversarialOK {
				aggregate.AdversarialOK++
			}
		}
		if result.Answerable && result.EvidenceHit != nil {
			aggregate.EvidenceScored++
			if *result.EvidenceHit {
				evidenceHits++
			}
		}
	}
	if aggregate.Answerable > 0 {
		aggregate.ExactMatch = float64(exactMatches) / float64(aggregate.Answerable)
		aggregate.F1 /= float64(aggregate.Answerable)
	}
	if aggregate.EvidenceScored > 0 {
		aggregate.EvidenceHitRate = float64(evidenceHits) / float64(aggregate.EvidenceScored)
	}
	if aggregate.Adversarial > 0 {
		aggregate.AdversarialRate = float64(aggregate.AdversarialOK) / float64(aggregate.Adversarial)
	}
	return aggregate
}

func isAdversarialRejection(prediction string) bool {
	switch normalizeAnswer(prediction) {
	case "unknown", "not mentioned", "no information available":
		return true
	default:
		return false
	}
}

func exactMatch(prediction string, answers []string) bool {
	normalizedPrediction := normalizeAnswer(prediction)
	if normalizedPrediction == "" {
		return false
	}
	for _, answer := range answers {
		normalizedAnswer := normalizeAnswer(answer)
		if normalizedAnswer == "" {
			continue
		}
		if normalizedPrediction == normalizedAnswer {
			return true
		}
	}
	return false
}

func tokenF1(prediction string, answers []string) float64 {
	predicted := strings.Fields(normalizeAnswer(prediction))
	best := 0.0
	for _, answer := range answers {
		gold := strings.Fields(normalizeAnswer(answer))
		if len(predicted) == 0 || len(gold) == 0 {
			continue
		}
		counts := make(map[string]int, len(predicted))
		for _, token := range predicted {
			counts[token]++
		}
		common := 0
		for _, token := range gold {
			if counts[token] > 0 {
				common++
				counts[token]--
			}
		}
		if common == 0 {
			continue
		}
		precision := float64(common) / float64(len(predicted))
		recall := float64(common) / float64(len(gold))
		best = max(best, 2*precision*recall/(precision+recall))
	}
	return best
}

func normalizeAnswer(value string) string {
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	normalized := words[:0]
	for _, word := range words {
		if word != "a" && word != "an" && word != "the" {
			normalized = append(normalized, word)
		}
	}
	return strings.Join(normalized, " ")
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
	name := envelope.Profile + "-" + envelope.StartedAt.Format("20060102T150405Z") + ".json"
	return os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0o600)
}
