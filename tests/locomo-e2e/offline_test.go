//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/flowcraft/eval/locomo"
	"github.com/GizClaw/flowcraft/eval/locomo/runners"
	"github.com/GizClaw/flowcraft/eval/metrics"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestDatasetSyntheticUsesPinnedFlowcraftSchema(t *testing.T) {
	t.Parallel()
	ds, identity, err := loadDataset("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if identity != "flowcraft:synthetic" || len(ds.Conversations) == 0 || len(ds.Questions) == 0 {
		t.Fatalf("dataset identity=%q conversations=%d questions=%d", identity, len(ds.Conversations), len(ds.Questions))
	}
}

func TestScoreDeterministicEMAndF1(t *testing.T) {
	t.Parallel()
	if !metrics.ExactMatch("Alice prefers green tea.", []string{"green tea"}) {
		t.Fatal("ExactMatch should accept a normalized contained answer")
	}
	if got := metrics.F1("green tea", []string{"green tea"}); got != 1 {
		t.Fatalf("F1 = %v, want 1", got)
	}
	if got := metrics.F1("coffee", []string{"green tea"}); got != 0 {
		t.Fatalf("F1 = %v, want 0", got)
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
		DatasetIdentity: "synthetic", Report: &locomo.Report{Runner: "profile"},
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "api_key") || strings.Contains(string(raw), "token") {
		t.Fatalf("report leaks credential material: %s", raw)
	}
}

func TestDatasetScopesAreConversationSpecific(t *testing.T) {
	t.Parallel()
	left := evalScope(runners.Scope{RuntimeID: "locomo", AgentID: "agent", UserID: "run::conversation-a"})
	right := evalScope(runners.Scope{RuntimeID: "locomo", AgentID: "agent", UserID: "run::conversation-b"})
	if left == right || left == "" || right == "" {
		t.Fatalf("scopes left=%q right=%q", left, right)
	}
}

func TestSourceTurnSaverPreservesEvidenceIDs(t *testing.T) {
	t.Parallel()
	recorder := &recordingStore{}
	runner := &storeRunner{name: "recording", store: recorder}
	count, _, err := runner.SaveSourceTurns(context.Background(), runners.Scope{RuntimeID: "locomo", UserID: "conversation"}, []runners.RawTurn{
		{Role: "user", Content: "hello", EvidenceID: "dia-1", SessionID: "session-1"},
		{Role: "assistant", Content: "hi", EvidenceID: "dia-2", SessionID: "session-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || recorder.observation.ID != "session-1" || len(recorder.observation.Turns) != 2 || recorder.observation.Turns[0].ID != "dia-1" || recorder.observation.Turns[1].ID != "dia-2" {
		t.Fatalf("count=%d observation=%+v", count, recorder.observation)
	}
}

type recordingStore struct {
	observation memorystore.Observation
}

func (s *recordingStore) Observe(_ context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	s.observation = observation
	return memorystore.ObserveResult{Facts: []memorystore.Fact{{ID: "fact"}}}, nil
}

func (*recordingStore) Recall(context.Context, memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{}, nil
}

func (*recordingStore) Update(context.Context, memorystore.UpdateRequest) (memorystore.Fact, error) {
	return memorystore.Fact{}, nil
}

func (*recordingStore) Delete(context.Context, memorystore.DeleteRequest) error { return nil }
