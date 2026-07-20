//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestDatasetBundledSmokeSubset(t *testing.T) {
	t.Parallel()
	dataset, identity, err := loadDataset(defaultDatasetPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Conversations) != 1 || len(dataset.Conversations[0].Turns) != 58 || len(dataset.Questions) != 6 {
		t.Fatalf("conversations=%d turns=%d questions=%d, want 1, 58, and 6",
			len(dataset.Conversations), len(dataset.Conversations[0].Turns), len(dataset.Questions))
	}
	if dataset.Conversations[0].ID != "conv-30" || !strings.HasPrefix(identity, "locomo10_smoke.jsonl:sha256:") {
		t.Fatalf("conversation=%q identity=%q", dataset.Conversations[0].ID, identity)
	}
	if dataset.Conversations[0].MinimumFactsPerSession != 1 {
		t.Fatalf("minimum facts per session=%d, want 1", dataset.Conversations[0].MinimumFactsPerSession)
	}
	first := dataset.Conversations[0].Turns[0]
	if first.Speaker != "Gina" || first.ObservedAt.Format(time.RFC3339) != "2023-01-20T16:04:00Z" {
		t.Fatalf("first turn speaker=%q observed_at=%s", first.Speaker, first.ObservedAt.Format(time.RFC3339))
	}
}

func TestDatasetRejectsInvalidReferencesAndContent(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*benchmarkDataset){
		"duplicate conversation": func(dataset *benchmarkDataset) {
			dataset.Conversations = append(dataset.Conversations, dataset.Conversations[0])
		},
		"duplicate evidence": func(dataset *benchmarkDataset) {
			dataset.Conversations[0].Turns = append(dataset.Conversations[0].Turns, dataset.Conversations[0].Turns[0])
		},
		"unknown conversation": func(dataset *benchmarkDataset) {
			dataset.Questions[0].ConversationID = "missing"
		},
		"unknown evidence": func(dataset *benchmarkDataset) {
			dataset.Questions[0].EvidenceIDs = []string{"missing"}
		},
		"empty question": func(dataset *benchmarkDataset) {
			dataset.Questions[0].Query = " "
		},
		"empty answer": func(dataset *benchmarkDataset) {
			dataset.Questions[0].GoldAnswers = []string{" "}
		},
		"missing speaker": func(dataset *benchmarkDataset) {
			dataset.Conversations[0].Turns[0].Speaker = " "
		},
		"missing timestamp": func(dataset *benchmarkDataset) {
			dataset.Conversations[0].Turns[0].ObservedAt = time.Time{}
		},
		"inconsistent session timestamp": func(dataset *benchmarkDataset) {
			turn := dataset.Conversations[0].Turns[0]
			turn.EvidenceID = "turn-2"
			turn.ObservedAt = turn.ObservedAt.Add(time.Minute)
			dataset.Conversations[0].Turns = append(dataset.Conversations[0].Turns, turn)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dataset := validOfflineDataset()
			mutate(dataset)
			if err := validateDataset(dataset); err == nil {
				t.Fatal("validateDataset should reject invalid data")
			}
		})
	}
}

func TestDatasetRejectsUnresolvedLFSPointer(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "locomo10_smoke.jsonl")
	if err := os.WriteFile(path, []byte("version https://git-lfs.github.com/spec/v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := loadDataset(path)
	if err == nil || !strings.Contains(err.Error(), "git lfs pull") {
		t.Fatalf("error=%v", err)
	}
}

func TestDatasetRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	var line datasetLine
	if err := decodeStrict([]byte(`{"type":"question"} {}`), &line); err == nil {
		t.Fatal("decodeStrict should reject trailing JSON")
	}
}

func TestScoreDeterministicEMAndF1(t *testing.T) {
	t.Parallel()
	if !exactMatch("The green tea.", []string{"green tea"}) {
		t.Fatal("exactMatch should accept normalized equality")
	}
	if exactMatch("not green tea", []string{"green tea"}) {
		t.Fatal("exactMatch should reject a containing but non-equal answer")
	}
	if got := tokenF1("green tea", []string{"green tea"}); got != 1 {
		t.Fatalf("F1 = %v, want 1", got)
	}
	if got := tokenF1("coffee", []string{"green tea"}); got != 0 {
		t.Fatalf("F1 = %v, want 0", got)
	}
}

func TestRunBenchmarkRejectsZeroQuality(t *testing.T) {
	t.Parallel()
	dataset := &benchmarkDataset{
		Conversations: []benchmarkConversation{{
			ID: "conversation", Turns: []benchmarkTurn{{Role: "user", Content: "green tea", EvidenceID: "turn-1", SessionID: "session-1"}},
		}},
		Questions: []benchmarkQuestion{{
			ID: "question", ConversationID: "conversation", Query: "drink?", GoldAnswers: []string{"green tea"}, EvidenceIDs: []string{"turn-1"},
		}},
	}
	report, err := runBenchmark(context.Background(), benchmarkOptions{
		Profile: "quality", DatasetIdentity: "offline", Models: reportModels{Answer: "fake"},
		TopK: 1, IngestTimeout: time.Second, QATimeout: time.Second, MinimumF1: 0.05,
	}, &recordingStore{}, dataset, staticAnswerer("coffee"))
	if err == nil || !strings.Contains(err.Error(), "below minimum") || report.Aggregate.F1 != 0 {
		t.Fatalf("report=%+v error=%v", report.Aggregate, err)
	}
}

func TestRunBenchmarkRejectsSuccessfulZeroFactSession(t *testing.T) {
	t.Parallel()
	dataset := &benchmarkDataset{
		Conversations: []benchmarkConversation{{
			ID: "conversation", MinimumFactsPerSession: 1,
			Turns: []benchmarkTurn{{Role: "user", Content: "green tea", EvidenceID: "turn-1", SessionID: "session-1"}},
		}},
		Questions: []benchmarkQuestion{{
			ID: "question", ConversationID: "conversation", Query: "drink?", GoldAnswers: []string{"green tea"}, EvidenceIDs: []string{"turn-1"},
		}},
	}
	report, err := runBenchmark(context.Background(), benchmarkOptions{
		Profile: "zero-facts", DatasetIdentity: "offline", Models: reportModels{Answer: "fake"},
		TopK: 1, IngestTimeout: time.Second, QATimeout: time.Second,
	}, &emptyObservationStore{}, dataset, staticAnswerer("green tea"))
	if err == nil || !strings.Contains(err.Error(), "materialized 0 facts") || len(report.Ingest) != 1 || report.Ingest[0].Observations != 1 || report.Ingest[0].Turns != 1 || report.Ingest[0].Duration <= 0 || len(report.Questions) != 0 {
		t.Fatalf("report=%+v error=%v", report, err)
	}
}

func TestMatchesDescending(t *testing.T) {
	t.Parallel()
	if !matchesDescending([]memorystore.Match{{Score: 1}, {Score: 1}, {Score: 0.5}}) {
		t.Fatal("equal and descending scores should pass")
	}
	if matchesDescending([]memorystore.Match{{Score: 0.5}, {Score: 0.75}}) {
		t.Fatal("ascending score should fail")
	}
}

func TestAwaitObservationWaitsForTerminalResult(t *testing.T) {
	t.Parallel()
	store := &waitingStore{wait: func(context.Context, string) (memorystore.ObserveResult, error) {
		return memorystore.ObserveResult{
			Facts:     []memorystore.Fact{{ID: "fact"}},
			Operation: &memorystore.Operation{ID: "operation", Status: memorystore.OperationSucceeded},
		}, nil
	}}
	result, err := awaitObservation(context.Background(), store, memorystore.ObserveResult{
		Operation: &memorystore.Operation{ID: "operation", Status: memorystore.OperationPending},
	})
	if err != nil || len(result.Facts) != 1 || store.waitCalls != 1 {
		t.Fatalf("result=%+v wait_calls=%d error=%v", result, store.waitCalls, err)
	}
}

func TestAwaitObservationReturnsOperationFailure(t *testing.T) {
	t.Parallel()
	store := &waitingStore{wait: func(context.Context, string) (memorystore.ObserveResult, error) {
		return memorystore.ObserveResult{
			Operation: &memorystore.Operation{ID: "operation", Status: memorystore.OperationFailed, Error: "extract failed"},
		}, nil
	}}
	_, err := awaitObservation(context.Background(), store, memorystore.ObserveResult{
		Operation: &memorystore.Operation{ID: "operation", Status: memorystore.OperationPending},
	})
	if err == nil || !strings.Contains(err.Error(), "extract failed") {
		t.Fatalf("error=%v", err)
	}
}

func TestAwaitObservationHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &waitingStore{wait: func(ctx context.Context, _ string) (memorystore.ObserveResult, error) {
		return memorystore.ObserveResult{}, ctx.Err()
	}}
	_, err := awaitObservation(ctx, store, memorystore.ObserveResult{
		Operation: &memorystore.Operation{ID: "operation", Status: memorystore.OperationPending},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestRunQuestionReportsRecallAndAnswerFailures(t *testing.T) {
	t.Parallel()
	question := benchmarkQuestion{ID: "question", ConversationID: "conversation", Query: "drink?", GoldAnswers: []string{"tea"}}
	recallFailure := runQuestion(context.Background(), &failingRecallStore{}, staticAnswerer("tea"), "scope", question, 1, time.Second)
	if !strings.Contains(recallFailure.Error, "recall failed") {
		t.Fatalf("recall result=%+v", recallFailure)
	}
	answerFailure := runQuestion(context.Background(), &recordingStore{}, errorAnswerer{}, "scope", question, 1, time.Second)
	if !strings.Contains(answerFailure.Error, "answer failed") {
		t.Fatalf("answer result=%+v", answerFailure)
	}
}

func TestRunQuestionHandlesEmptyMatches(t *testing.T) {
	t.Parallel()
	question := benchmarkQuestion{ID: "question", ConversationID: "conversation", Query: "drink?", GoldAnswers: []string{"tea"}}
	result := runQuestion(context.Background(), &emptyRecallStore{}, staticAnswerer("unknown"), "scope", question, 1, time.Second)
	if result.Error != "" || result.EvidenceHit != nil || len(result.Recalled) != 0 || result.F1 != 0 {
		t.Fatalf("result=%+v", result)
	}
}

func TestEvidenceHitUsesFactSources(t *testing.T) {
	t.Parallel()
	matches := []memorystore.Match{{Fact: memorystore.Fact{Sources: []memorystore.SourceRef{{TurnIDs: []string{"turn-1"}}}}}}
	if hit := evidenceHit(matches, []string{"turn-1"}); hit == nil || !*hit {
		t.Fatalf("hit=%v", hit)
	}
	if hit := evidenceHit(matches, []string{"turn-2"}); hit == nil || *hit {
		t.Fatalf("hit=%v", hit)
	}
	if hit := evidenceHit(matches, nil); hit != nil {
		t.Fatalf("unscored question returned evidence result: %v", *hit)
	}
	if hit := evidenceHit(nil, []string{"turn-1"}); hit == nil || *hit {
		t.Fatalf("missing provenance should be an evidence miss: %v", hit)
	}
}

func TestCloseStoreUsesOwnedCloser(t *testing.T) {
	t.Parallel()
	store := &closableStore{}
	explicit := &countCloser{}
	if err := closeStore(store, explicit); err != nil {
		t.Fatal(err)
	}
	if explicit.calls != 1 || store.calls != 0 {
		t.Fatalf("explicit calls=%d store calls=%d", explicit.calls, store.calls)
	}
	if err := closeStore(store, nil); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("store calls=%d", store.calls)
	}
}

func TestPreflightReportsAllMissingInputs(t *testing.T) {
	t.Parallel()
	err := validateRequired(map[string]string{"dataset": "", "token": " "}, "dataset", "token")
	if err == nil || !strings.Contains(err.Error(), "dataset, token") {
		t.Fatalf("error = %v", err)
	}
}

func TestRedactionReportContainsOnlyFingerprint(t *testing.T) {
	t.Parallel()
	secret := "never-print-this-token"
	envelope := reportEnvelope{
		Profile: "profile", ConfigFingerprint: configFingerprint("profile", "endpoint", "deployment", secret),
		DatasetIdentity: "dataset", Models: reportModels{Answer: defaultDoubaoModel},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "api_key") || strings.Contains(string(raw), "token") {
		t.Fatalf("report leaks credential material: %s", raw)
	}
}

func TestSessionObservationsPreserveEvidenceIDs(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2023, time.January, 20, 16, 4, 0, 0, time.UTC)
	conversation := benchmarkConversation{ID: "conversation", Turns: []benchmarkTurn{
		{Role: "user", Speaker: "Jon", Content: "hello", EvidenceID: "dia-1", SessionID: "session-1", ObservedAt: observedAt},
		{Role: "assistant", Speaker: "Gina", Content: "hi", EvidenceID: "dia-2", SessionID: "session-1", ObservedAt: observedAt},
		{Role: "user", Speaker: "Jon", Content: "later", EvidenceID: "dia-3", SessionID: "session-2", ObservedAt: observedAt.Add(time.Hour)},
	}}
	observations := sessionObservations("scope", conversation)
	if len(observations) != 2 || observations[0].ID != "session-1" || !observations[0].ObservedAt.Equal(observedAt) || len(observations[0].Turns) != 2 || observations[0].Turns[0].ID != "dia-1" || observations[0].Turns[0].Speaker != "Jon" || !observations[0].Turns[0].ObservedAt.Equal(observedAt) || observations[1].Turns[0].ID != "dia-3" {
		t.Fatalf("observations=%+v", observations)
	}
}

func TestRunBenchmarkUsesConversationSpecificScopes(t *testing.T) {
	t.Parallel()
	store := &recordingStore{}
	dataset := &benchmarkDataset{
		Conversations: []benchmarkConversation{
			{ID: "left", Turns: []benchmarkTurn{{Role: "user", Content: "left", EvidenceID: "left-1", SessionID: "s1"}}},
			{ID: "right", Turns: []benchmarkTurn{{Role: "user", Content: "right", EvidenceID: "right-1", SessionID: "s1"}}},
		},
		Questions: []benchmarkQuestion{
			{ID: "q-left", ConversationID: "left", Query: "left?", GoldAnswers: []string{"left"}},
			{ID: "q-right", ConversationID: "right", Query: "right?", GoldAnswers: []string{"right"}},
		},
	}
	report, err := runBenchmark(context.Background(), benchmarkOptions{
		Profile: "offline", DatasetIdentity: "offline", Models: reportModels{Answer: "fake"},
		TopK: 1, IngestTimeout: time.Second, QATimeout: time.Second,
	}, store, dataset, staticAnswerer("left"))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.scopes) != 2 || store.scopes[0] == store.scopes[1] || report.Aggregate.Questions != 2 {
		t.Fatalf("scopes=%v aggregate=%+v", store.scopes, report.Aggregate)
	}
}

func TestAwaitObservationRequiresWaiter(t *testing.T) {
	t.Parallel()
	_, err := awaitObservation(context.Background(), &recordingStore{}, memorystore.ObserveResult{
		Operation: &memorystore.Operation{ID: "pending", Status: memorystore.OperationPending},
	})
	if err == nil || !strings.Contains(err.Error(), "OperationWaiter") {
		t.Fatalf("error=%v", err)
	}
}

type staticAnswerer string

func (a staticAnswerer) Answer(context.Context, string, []memorystore.Match) (string, error) {
	if a == "" {
		return "", errors.New("empty static answer")
	}
	return string(a), nil
}

type recordingStore struct {
	scopes []memorystore.Scope
}

type emptyObservationStore struct {
	recordingStore
}

type waitingStore struct {
	recordingStore
	wait      func(context.Context, string) (memorystore.ObserveResult, error)
	waitCalls int
}

func (s *waitingStore) Wait(ctx context.Context, operationID string) (memorystore.ObserveResult, error) {
	s.waitCalls++
	return s.wait(ctx, operationID)
}

type failingRecallStore struct {
	recordingStore
}

func (*failingRecallStore) Recall(context.Context, memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{}, errors.New("recall failed")
}

type emptyRecallStore struct {
	recordingStore
}

func (*emptyRecallStore) Recall(context.Context, memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{}, nil
}

type errorAnswerer struct{}

func (errorAnswerer) Answer(context.Context, string, []memorystore.Match) (string, error) {
	return "", errors.New("answer failed")
}

type closableStore struct {
	recordingStore
	calls int
}

func (s *closableStore) Close() error {
	s.calls++
	return nil
}

type countCloser struct {
	calls int
}

func (c *countCloser) Close() error {
	c.calls++
	return nil
}

func (*emptyObservationStore) Observe(context.Context, memorystore.Observation) (memorystore.ObserveResult, error) {
	return memorystore.ObserveResult{}, nil
}

func (s *recordingStore) Observe(_ context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	s.scopes = append(s.scopes, observation.Scope)
	return memorystore.ObserveResult{Facts: []memorystore.Fact{{ID: "fact"}}}, nil
}

func (*recordingStore) Recall(_ context.Context, query memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{Matches: []memorystore.Match{{Fact: memorystore.Fact{Text: query.Text}}}}, nil
}

func (*recordingStore) Update(context.Context, memorystore.UpdateRequest) (memorystore.Fact, error) {
	return memorystore.Fact{}, nil
}

func (*recordingStore) Delete(context.Context, memorystore.DeleteRequest) error { return nil }

func validOfflineDataset() *benchmarkDataset {
	observedAt := time.Date(2023, time.January, 20, 16, 4, 0, 0, time.UTC)
	return &benchmarkDataset{
		Conversations: []benchmarkConversation{{
			ID: "conversation", Turns: []benchmarkTurn{{Role: "user", Speaker: "Jon", Content: "tea", EvidenceID: "turn-1", SessionID: "session-1", ObservedAt: observedAt}},
		}},
		Questions: []benchmarkQuestion{{
			ID: "question", ConversationID: "conversation", Query: "drink?", GoldAnswers: []string{"tea"}, EvidenceIDs: []string{"turn-1"},
		}},
	}
}
