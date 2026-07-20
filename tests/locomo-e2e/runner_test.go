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

func (a llmAnswerer) Answer(ctx context.Context, question string, matches []memorystore.Match) (string, error) {
	var evidence strings.Builder
	for index, match := range matches {
		fmt.Fprintf(&evidence, "[%d] %s\n", index+1, strings.TrimSpace(match.Fact.Text))
	}
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleSystem, "Answer the question using only the recalled memory evidence. Return only the concise answer. If the evidence is insufficient, return unknown."),
		llm.NewTextMessage(llm.RoleUser, "Memory evidence:\n"+evidence.String()+"\nQuestion: "+question),
	}
	message, _, err := a.model.Generate(ctx, messages,
		llm.WithTemperature(0),
		llm.WithMaxTokens(256),
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
	Profile           string           `json:"profile"`
	ConfigFingerprint string           `json:"config_fingerprint"`
	DatasetIdentity   string           `json:"dataset_identity"`
	Models            reportModels     `json:"models"`
	StartedAt         time.Time        `json:"started_at"`
	FinishedAt        time.Time        `json:"finished_at"`
	Ingest            []ingestResult   `json:"ingest"`
	Questions         []questionResult `json:"questions"`
	Aggregate         aggregateResult  `json:"aggregate"`
	QualityGate       qualityGate      `json:"quality_gate"`
}

type reportModels struct {
	Extraction string `json:"extraction,omitempty"`
	Embedding  string `json:"embedding,omitempty"`
	Rerank     string `json:"rerank,omitempty"`
	Answer     string `json:"answer"`
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
	Query          string         `json:"query"`
	GoldAnswers    []string       `json:"gold_answers"`
	Prediction     string         `json:"prediction,omitempty"`
	ExactMatch     bool           `json:"exact_match"`
	F1             float64        `json:"f1"`
	EvidenceHit    *bool          `json:"evidence_hit,omitempty"`
	RecallDuration time.Duration  `json:"recall_duration_ns"`
	AnswerDuration time.Duration  `json:"answer_duration_ns"`
	Error          string         `json:"error,omitempty"`
	Recalled       []recalledFact `json:"recalled"`
}

type recalledFact struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Score       float64  `json:"score"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type aggregateResult struct {
	Questions       int     `json:"questions"`
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
	envelope, err := runBenchmark(context.Background(), benchmarkOptions{
		Profile:            profile,
		ConfigFingerprint:  fingerprint,
		DatasetIdentity:    identity,
		Models:             models,
		TopK:               settings.topK,
		IngestTimeout:      settings.ingestTimeout,
		QATimeout:          settings.qaTimeout,
		MinimumF1:          settings.minF1,
		MinimumEvidenceHit: settings.minEvidenceHit,
		Logf:               t.Logf,
	}, store, dataset, llmAnswerer{model: answerModel})
	if envelope != nil {
		if writeErr := writeReport(settings.reportDir, *envelope); writeErr != nil {
			t.Errorf("write report: %v", writeErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: n=%d em=%.4f f1=%.4f evidence_hit=%.4f", profile,
		envelope.Aggregate.Questions, envelope.Aggregate.ExactMatch,
		envelope.Aggregate.F1, envelope.Aggregate.EvidenceHitRate)
}

type benchmarkOptions struct {
	Profile            string
	ConfigFingerprint  string
	DatasetIdentity    string
	Models             reportModels
	TopK               int
	IngestTimeout      time.Duration
	QATimeout          time.Duration
	MinimumF1          float64
	MinimumEvidenceHit float64
	Logf               func(string, ...any)
}

func runBenchmark(ctx context.Context, options benchmarkOptions, store memorystore.Store, dataset *benchmarkDataset, answerer benchmarkAnswerer) (*reportEnvelope, error) {
	started := time.Now().UTC()
	envelope := &reportEnvelope{
		Profile: options.Profile, ConfigFingerprint: options.ConfigFingerprint,
		DatasetIdentity: options.DatasetIdentity, StartedAt: started,
		Models: options.Models,
		QualityGate: qualityGate{
			MinimumF1:              options.MinimumF1,
			MinimumEvidenceHitRate: options.MinimumEvidenceHit,
		},
	}
	runID := configFingerprint(options.Profile, started.Format(time.RFC3339Nano))[:16]
	scopes := make(map[string]memorystore.Scope, len(dataset.Conversations))
	for _, conversation := range dataset.Conversations {
		scope := memorystore.Scope("locomo:" + runID + ":" + conversation.ID)
		scopes[conversation.ID] = scope
		result, err := ingestConversation(ctx, store, scope, conversation, options.IngestTimeout, options.Logf)
		envelope.Ingest = append(envelope.Ingest, result)
		if err != nil {
			envelope.FinishedAt = time.Now().UTC()
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
	envelope.FinishedAt = time.Now().UTC()
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

func ingestConversation(ctx context.Context, store memorystore.Store, scope memorystore.Scope, conversation benchmarkConversation, timeout time.Duration, logf func(string, ...any)) (ingestResult, error) {
	result := ingestResult{ConversationID: conversation.ID}
	started := time.Now()
	observations := sessionObservations(scope, conversation)
	for index, observation := range observations {
		operationCtx, cancel := context.WithTimeout(ctx, timeout)
		observed, err := store.Observe(operationCtx, observation)
		if err == nil {
			observed, err = awaitObservation(operationCtx, store, observed)
		}
		cancel()
		if err != nil {
			result.Duration = time.Since(started)
			return result, err
		}
		result.Observations++
		result.Turns += len(observation.Turns)
		result.Facts += len(observed.Facts)
		if logf != nil {
			logf("ingest %s session %d/%d complete: turns=%d facts=%d", conversation.ID,
				index+1, len(observations), len(observation.Turns), len(observed.Facts))
		}
		if len(observed.Facts) < conversation.MinimumFactsPerSession {
			result.Duration = time.Since(started)
			return result, fmt.Errorf("session %q materialized %d facts, below dataset minimum %d",
				observation.ID, len(observed.Facts), conversation.MinimumFactsPerSession)
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

func awaitObservation(ctx context.Context, store memorystore.Store, result memorystore.ObserveResult) (memorystore.ObserveResult, error) {
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
	result, err := waiter.Wait(ctx, result.Operation.ID)
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
		Query: question.Query, GoldAnswers: question.GoldAnswers,
	}
	qaCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	recallStarted := time.Now()
	recalled, err := store.Recall(qaCtx, memorystore.Query{Scope: scope, Text: question.Query, Limit: topK})
	result.RecallDuration = time.Since(recallStarted)
	if err != nil {
		result.Error = "recall: " + err.Error()
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
		result.Error = "answer: " + err.Error()
		return result
	}
	result.ExactMatch = exactMatch(result.Prediction, question.GoldAnswers)
	result.F1 = tokenF1(result.Prediction, question.GoldAnswers)
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
		if result.ExactMatch {
			exactMatches++
		}
		aggregate.F1 += result.F1
		if result.EvidenceHit != nil {
			aggregate.EvidenceScored++
			if *result.EvidenceHit {
				evidenceHits++
			}
		}
	}
	if aggregate.Succeeded > 0 {
		aggregate.ExactMatch = float64(exactMatches) / float64(aggregate.Succeeded)
		aggregate.F1 /= float64(aggregate.Succeeded)
	}
	if aggregate.EvidenceScored > 0 {
		aggregate.EvidenceHitRate = float64(evidenceHits) / float64(aggregate.EvidenceScored)
	}
	return aggregate
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
