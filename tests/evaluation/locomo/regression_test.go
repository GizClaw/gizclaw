//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func compareBaseline(path string, candidate reportEnvelope, maxF1Drop, maxEvidenceDrop float64) error {
	raw, err := os.ReadFile(repoPath(path))
	if err != nil {
		return fmt.Errorf("read LoCoMo baseline: %w", err)
	}
	var baseline reportEnvelope
	if err := json.Unmarshal(raw, &baseline); err != nil {
		return fmt.Errorf("decode LoCoMo baseline: %w", err)
	}
	return compareReports(baseline, candidate, maxF1Drop, maxEvidenceDrop)
}

func compareReports(baseline, candidate reportEnvelope, maxF1Drop, maxEvidenceDrop float64) error {
	left, right := baseline.Models, candidate.Models
	// SDK changes are deliberately comparable; changes to the dataset, model
	// stack, prompt, ingestion unit, or retrieval budget require a new baseline.
	left.SDKVersion, right.SDKVersion = "", ""
	// Service tier selects inference QoS, not the model or extraction policy.
	left.ExtractionServiceTier, right.ExtractionServiceTier = "", ""
	if baseline.Profile != candidate.Profile || baseline.DatasetIdentity != candidate.DatasetIdentity ||
		baseline.Protocol.Version == "" || baseline.Protocol != candidate.Protocol || left != right {
		return fmt.Errorf("LoCoMo baseline is incompatible: require the same profile, dataset, models, prompt, ingestion unit, and top-K")
	}
	if baseline.Aggregate.Failed != 0 || candidate.Aggregate.Failed != 0 || baseline.Aggregate.Questions == 0 ||
		baseline.Aggregate.Questions != candidate.Aggregate.Questions || len(baseline.Questions) != len(candidate.Questions) {
		return fmt.Errorf("LoCoMo baseline and candidate must be complete error-free runs")
	}
	for index, before := range baseline.Questions {
		after := candidate.Questions[index]
		if before.ID != after.ID || before.Category != after.Category || before.Answerable != after.Answerable {
			return fmt.Errorf("LoCoMo question accounting changed at index %d", index)
		}
	}
	if baseline.Aggregate.F1-candidate.Aggregate.F1 > maxF1Drop {
		return fmt.Errorf("LoCoMo F1 regression: baseline %.4f candidate %.4f exceeds allowed drop %.4f", baseline.Aggregate.F1, candidate.Aggregate.F1, maxF1Drop)
	}
	if baseline.Aggregate.EvidenceHitRate-candidate.Aggregate.EvidenceHitRate > maxEvidenceDrop {
		return fmt.Errorf("LoCoMo evidence regression: baseline %.4f candidate %.4f exceeds allowed drop %.4f", baseline.Aggregate.EvidenceHitRate, candidate.Aggregate.EvidenceHitRate, maxEvidenceDrop)
	}
	if candidate.Aggregate.AdversarialRate < baseline.Aggregate.AdversarialRate {
		return fmt.Errorf("LoCoMo adversarial rejection regressed: baseline %.4f candidate %.4f", baseline.Aggregate.AdversarialRate, candidate.Aggregate.AdversarialRate)
	}
	for category, before := range baseline.ByCategory {
		after, exists := candidate.ByCategory[category]
		if !exists || before.Questions != after.Questions {
			return fmt.Errorf("LoCoMo category %d accounting changed", category)
		}
		if before.F1-after.F1 > max(0.10, maxF1Drop) {
			return fmt.Errorf("LoCoMo category %d F1 regression: baseline %.4f candidate %.4f", category, before.F1, after.F1)
		}
	}
	return nil
}

func TestTurnIngestionAllowsEmptyTurnsButRequiresFactsPerSession(t *testing.T) {
	t.Parallel()
	store := &turnRecordingStore{}
	conversation := validOfflineDataset().Conversations[0]
	conversation.MinimumFactsPerSession = 1
	turn := conversation.Turns[0]
	turn.EvidenceID = "second"
	conversation.Turns = append(conversation.Turns, turn)
	result, err := ingestConversationMode(t.Context(), store, memorystore.Scope{AppID: "test"}, conversation, time.Second, nil, "turn")
	if err != nil {
		t.Fatal(err)
	}
	if result.Observations != 2 || result.Facts != 1 || len(store.inputs) != 2 ||
		len(store.inputs[0].Turns) != 1 || store.inputs[0].ID != conversation.Turns[0].EvidenceID || store.inputs[1].ID != "second" {
		t.Fatalf("per-turn ingestion lost accounting or provenance: %+v", result)
	}
}

type turnRecordingStore struct {
	recordingStore
	inputs []memorystore.Observation
}

func (s *turnRecordingStore) Observe(_ context.Context, input memorystore.Observation) (memorystore.ObserveResult, error) {
	s.inputs = append(s.inputs, input)
	if len(s.inputs) == 1 {
		return memorystore.ObserveResult{}, nil
	}
	return memorystore.ObserveResult{Facts: []memorystore.Fact{{ID: "fact"}}}, nil
}

func TestRegressionRejectsMetricDropsAndProtocolChanges(t *testing.T) {
	t.Parallel()
	base := reportEnvelope{Profile: "mem0", DatasetIdentity: "fixed", Protocol: reportProtocol{Version: "v2", TopK: 50},
		Models:    reportModels{Extraction: "extract", Answer: "answer"},
		Aggregate: aggregateResult{Questions: 1, Succeeded: 1, F1: .5, EvidenceHitRate: .8, AdversarialRate: 1},
		Questions: []questionResult{{ID: "q1", Category: 4, Answerable: true}},
	}
	for name, mutate := range map[string]func(*reportEnvelope){
		"F1":          func(r *reportEnvelope) { r.Aggregate.F1 = .1 },
		"evidence":    func(r *reportEnvelope) { r.Aggregate.EvidenceHitRate = .1 },
		"adversarial": func(r *reportEnvelope) { r.Aggregate.AdversarialRate = 0 },
		"budget":      func(r *reportEnvelope) { r.Protocol.TopK = 10 },
		"model":       func(r *reportEnvelope) { r.Models.Answer = "changed" },
		"dataset":     func(r *reportEnvelope) { r.DatasetIdentity = "changed" },
		"partial":     func(r *reportEnvelope) { r.Aggregate.Failed = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if err := compareReports(base, candidate, .02, .05); err == nil {
				t.Fatal("regression was accepted")
			}
		})
	}
	candidate := base
	candidate.Models.SDKVersion = "upgraded"
	candidate.Models.ExtractionServiceTier = "fast"
	if err := compareReports(base, candidate, .02, .05); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidate.Questions, base.Questions) {
		t.Fatal("comparison mutated question records")
	}
}
