package memorystore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowsqlite "github.com/GizClaw/flowcraft/memory/recall/store/sqlite"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
)

func TestMigrateLegacyFlowcraftStateReadsPinnedWorkspaceEncoding(t *testing.T) {
	dir := t.TempDir()
	legacy, err := flowworkspace.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	scope := flowrecall.Scope{RuntimeID: "workspace-native"}
	fact := flowrecall.TemporalFact{
		ID: "native-fact", Scope: scope, Kind: flowrecall.FactNote,
		Content: "written by the pinned workspace backend", ObservedAt: time.Now().UTC(),
	}
	if err := legacy.TemporalStore().Append(t.Context(), []flowrecall.TemporalFact{fact}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.EvidenceStore().Append(t.Context(), scope, fact.ID, []flowrecall.EvidenceRef{{ID: "native-evidence", Text: "source"}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	populateNativeLegacySideEffects(t, legacy.SideEffectOutbox(), scope, fact, now)
	populateNativeLegacyAsyncQueue(t, legacy.AsyncSemanticQueue(), scope, fact, now)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	legacyRaw, err := os.ReadFile(filepath.Join(dir, legacyFlowcraftStateName))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
		t.Fatal(err)
	}
	retainedRaw, err := os.ReadFile(filepath.Join(dir, legacyFlowcraftStateName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyRaw, retainedRaw) {
		t.Fatal("migration modified workspace-backend state.json")
	}

	databasePath := filepath.Join(dir, flowcraftSQLiteName)
	for range 2 {
		migrated, err := flowsqlite.Open(t.Context(), databasePath)
		if err != nil {
			t.Fatal(err)
		}
		got, err := migrated.TemporalStore().Get(t.Context(), scope, fact.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, got, fact)
		evidence, err := migrated.EvidenceStore().Get(t.Context(), scope, "native-evidence")
		if err != nil {
			t.Fatal(err)
		}
		if evidence.Text != "source" {
			t.Fatalf("evidence text = %q", evidence.Text)
		}
		assertNativeMigrationStats(t, migrated, scope, now)
		if err := migrated.Close(); err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
			t.Fatalf("restart migration verification: %v", err)
		}
	}
	assertNativeMigrationRows(t, databasePath, now)
}

func populateNativeLegacySideEffects(t *testing.T, outbox flowrecall.SideEffectOutbox, scope flowrecall.Scope, fact flowrecall.TemporalFact, now time.Time) {
	t.Helper()
	ctx := t.Context()
	enqueue := func(requestID string) {
		t.Helper()
		if err := outbox.Enqueue(ctx, flowrecall.SideEffectJob{
			RequestID: requestID,
			Scope:     scope,
			Kind:      flowrecall.SideEffectProjectRequired,
			Facts:     []flowrecall.TemporalFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(requestID string) flowrecall.SideEffectJob {
		t.Helper()
		jobs, err := outbox.Claim(ctx, flowrecall.SideEffectClaimOptions{WorkerID: "native-worker", Scope: scope, Now: now, Max: 1})
		if err != nil || len(jobs) != 1 || jobs[0].RequestID != requestID {
			t.Fatalf("claim side effect %q = %#v, %v", requestID, jobs, err)
		}
		return jobs[0]
	}

	enqueue("native-side-failed")
	failed := claim("native-side-failed")
	if err := outbox.Fail(ctx, failed.ID, failed.LeaseToken, flowrecall.SideEffectFailure{ErrClass: flowrecall.ErrClassPermanent, Err: "native permanent failure"}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-side-complete")
	complete := claim("native-side-complete")
	if err := outbox.Complete(ctx, complete.ID, complete.LeaseToken, flowrecall.SideEffectResult{CompletedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-side-retry")
	retry := claim("native-side-retry")
	if err := outbox.Fail(ctx, retry.ID, retry.LeaseToken, flowrecall.SideEffectFailure{ErrClass: flowrecall.ErrClassTransient, Err: "native transient failure", RetryAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-side-leased")
	_ = claim("native-side-leased")
	enqueue("native-side-pending")
	enqueue("native-side-cancelled")
	if err := outbox.Cancel(ctx, "native-side-cancelled"); err != nil {
		t.Fatal(err)
	}
}

func populateNativeLegacyAsyncQueue(t *testing.T, queue flowrecall.AsyncSemanticQueue, scope flowrecall.Scope, fact flowrecall.TemporalFact, now time.Time) {
	t.Helper()
	ctx := t.Context()
	enqueue := func(requestID string) {
		t.Helper()
		if _, err := queue.Enqueue(ctx, flowrecall.AsyncSemanticJob{
			RequestID: requestID, Scope: scope, EpisodeFactIDs: []string{fact.ID},
			ObservedAt: now, Tier: "native", ExistingFactsAnchor: []flowrecall.TemporalFact{fact},
		}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(requestID string) flowrecall.AsyncSemanticJob {
		t.Helper()
		jobs, err := queue.Claim(ctx, flowrecall.AsyncSemanticClaimOptions{WorkerID: "native-worker", Scope: &scope, Now: now, Max: 1})
		if err != nil || len(jobs) != 1 || jobs[0].RequestID != requestID {
			t.Fatalf("claim async job %q = %#v, %v", requestID, jobs, err)
		}
		return jobs[0]
	}

	enqueue("native-async-failed")
	failed := claim("native-async-failed")
	if err := queue.Fail(ctx, failed.RequestID, failed.LeaseToken, flowrecall.AsyncSemanticFailure{ErrClass: flowrecall.ErrClassPermanent, Err: "native permanent failure"}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-async-complete")
	complete := claim("native-async-complete")
	if err := queue.Complete(ctx, complete.RequestID, complete.LeaseToken, flowrecall.AsyncSemanticResult{SemanticFactIDs: []string{"native-semantic"}, RecoveredFromPriorAttempt: true}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-async-retry")
	retry := claim("native-async-retry")
	if err := queue.Fail(ctx, retry.RequestID, retry.LeaseToken, flowrecall.AsyncSemanticFailure{ErrClass: flowrecall.ErrClassTransient, Err: "native transient failure", RetryAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	enqueue("native-async-leased")
	_ = claim("native-async-leased")
	enqueue("native-async-pending")
	enqueue("native-async-cancelled")
	if err := queue.Cancel(ctx, "native-async-cancelled"); err != nil {
		t.Fatal(err)
	}
}

func assertNativeMigrationStats(t *testing.T, backend interface {
	SideEffectOutbox() flowrecall.SideEffectOutbox
	AsyncSemanticQueue() flowrecall.AsyncSemanticQueue
}, scope flowrecall.Scope, now time.Time) {
	t.Helper()
	side, err := backend.SideEffectOutbox().Stats(t.Context(), scope, now)
	if err != nil {
		t.Fatal(err)
	}
	if side.Pending != 2 || side.Leased != 1 || side.Failed != 1 || side.DeadLetter != 1 || side.Completed != 1 || side.CancelledTotal != 1 {
		t.Fatalf("native side-effect stats = %#v", side)
	}
	async, err := backend.AsyncSemanticQueue().Stats(t.Context(), flowrecall.AsyncSemanticStatsFilter{Scope: scope, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if async.Pending != 2 || async.Leased != 1 || async.Failed != 1 || async.DeadLetter != 1 || async.Completed != 1 || async.CancelledTotal != 1 {
		t.Fatalf("native async stats = %#v", async)
	}
}

func assertNativeMigrationRows(t *testing.T, databasePath string, now time.Time) {
	t.Helper()
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	assertRow := func(query, id, wantStatus, wantFailure string, wantResult bool) {
		t.Helper()
		var status, leaseToken, failureClass, result string
		var attempt int
		var retryAt, leaseUntil int64
		if err := database.QueryRow(query, id).Scan(&status, &leaseToken, &attempt, &failureClass, &result, &retryAt, &leaseUntil); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || failureClass != wantFailure || (result != "") != wantResult {
			t.Fatalf("native row %q = status=%q failure=%q result=%q", id, status, failureClass, result)
		}
		switch wantStatus {
		case "leased":
			if leaseToken == "" || attempt != 1 || leaseUntil <= now.UnixNano() {
				t.Fatalf("native leased row %q = token=%q attempt=%d lease=%d", id, leaseToken, attempt, leaseUntil)
			}
		case "pending":
			if wantFailure == string(flowrecall.ErrClassTransient) && (attempt != 1 || retryAt != now.Add(time.Hour).UnixNano()) {
				t.Fatalf("native retry row %q = attempt=%d retry=%d", id, attempt, retryAt)
			}
		}
	}
	const sideQuery = `SELECT status,lease_token,attempt,failure_class,result_json,COALESCE(retry_at_ns, 0),COALESCE(lease_until_ns, 0) FROM recall_side_effect_jobs WHERE request_id = ?`
	assertRow(sideQuery, "native-side-retry", "pending", string(flowrecall.ErrClassTransient), false)
	assertRow(sideQuery, "native-side-leased", "leased", "", false)
	assertRow(sideQuery, "native-side-failed", "failed", string(flowrecall.ErrClassPermanent), false)
	assertRow(sideQuery, "native-side-complete", "complete", "", true)
	const asyncQuery = `SELECT status,lease_token,attempt,failure_class,result_json,CASE WHEN status = 'pending' THEN COALESCE(lease_until_ns, 0) ELSE 0 END,COALESCE(lease_until_ns, 0) FROM recall_async_semantic_jobs WHERE request_id = ?`
	assertRow(asyncQuery, "native-async-retry", "pending", string(flowrecall.ErrClassTransient), false)
	assertRow(asyncQuery, "native-async-leased", "leased", "", false)
	assertRow(asyncQuery, "native-async-failed", "failed", string(flowrecall.ErrClassPermanent), false)
	assertRow(asyncQuery, "native-async-complete", "complete", "", true)
}

func TestMigrateLegacyFlowcraftStatePreservesCanonicalRecords(t *testing.T) {
	dir := t.TempDir()
	state := legacyMigrationFixture()
	raw := writeLegacyMigrationFixture(t, dir, state)
	if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
		t.Fatal(err)
	}
	legacyAfter, err := os.ReadFile(filepath.Join(dir, legacyFlowcraftStateName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, legacyAfter) {
		t.Fatal("migration modified legacy state.json")
	}
	databasePath := filepath.Join(dir, flowcraftSQLiteName)
	backend, err := flowsqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range state.Facts {
		got, err := backend.TemporalStore().Get(t.Context(), fact.Scope, fact.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, got, fact)
	}
	for _, evidence := range state.Evidence {
		got, err := backend.EvidenceStore().Get(t.Context(), evidence.Scope, evidence.EvidenceID)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, got, evidence.Ref)
	}
	scope := state.Facts[0].Scope
	sideStats, err := backend.SideEffectOutbox().Stats(t.Context(), scope, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if sideStats.Pending != 1 || sideStats.Leased != 1 || sideStats.Failed != 1 || sideStats.Completed != 1 || sideStats.CancelledTotal != 3 {
		t.Fatalf("side-effect stats = %#v", sideStats)
	}
	asyncStats, err := backend.AsyncSemanticQueue().Stats(t.Context(), flowrecall.AsyncSemanticStatsFilter{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if asyncStats.Pending != 1 || asyncStats.Leased != 1 || asyncStats.Failed != 1 || asyncStats.Completed != 1 || asyncStats.CancelledTotal != 5 {
		t.Fatalf("async stats = %#v", asyncStats)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var status, leaseToken, failureClass, result string
	var attempt int
	if err := database.QueryRow(`SELECT status,lease_token,attempt,failure_class,result_json FROM recall_side_effect_jobs WHERE id = ?`, "side-leased").Scan(
		&status, &leaseToken, &attempt, &failureClass, &result,
	); err != nil {
		t.Fatal(err)
	}
	if status != "leased" || leaseToken != "side-token" || attempt != 2 || failureClass != "" || result != "" {
		t.Fatalf("leased side-effect row = status=%q token=%q attempt=%d failure=%q result=%q", status, leaseToken, attempt, failureClass, result)
	}
	if err := database.QueryRow(`SELECT status,failure_class,result_json FROM recall_async_semantic_jobs WHERE request_id = ?`, "async-complete").Scan(
		&status, &failureClass, &result,
	); err != nil {
		t.Fatal(err)
	}
	if status != "complete" || result == "" {
		t.Fatalf("complete async row = status=%q result=%q", status, result)
	}
	var episodeRows int
	if err := database.QueryRow(`SELECT COUNT(*) FROM recall_async_semantic_job_episodes`).Scan(&episodeRows); err != nil {
		t.Fatal(err)
	}
	if episodeRows != len(state.Async) {
		t.Fatalf("async episode rows = %d, want %d", episodeRows, len(state.Async))
	}

	if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
		t.Fatalf("restart migration verification: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacyFlowcraftStateName), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("digest mismatch error = %v", err)
	}
}

func TestMigrateLegacyFlowcraftStateRetriesWithoutPublishingPartialDatabase(t *testing.T) {
	dir := t.TempDir()
	raw := writeLegacyMigrationFixture(t, dir, legacyMigrationFixture())
	if err := os.WriteFile(filepath.Join(dir, flowcraftMigrationTmpName), []byte("interrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected rename failure")
	err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{
		rename: func(string, string) error { return injected },
	})
	if !errors.Is(err, injected) {
		t.Fatalf("rename failure = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, flowcraftSQLiteName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial memory.db published: %v", err)
	}
	legacyAfter, err := os.ReadFile(filepath.Join(dir, legacyFlowcraftStateName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, legacyAfter) {
		t.Fatal("failed migration modified legacy state.json")
	}
	if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
		t.Fatalf("retry migration: %v", err)
	}
}

func TestMigrateLegacyFlowcraftStateStageFailuresRemainRetryable(t *testing.T) {
	injected := errors.New("injected migration stage failure")
	tests := map[string]func() sqliteMigrationOps{
		"import": func() sqliteMigrationOps {
			return sqliteMigrationOps{beforeImport: func() error { return injected }}
		},
		"verification": func() sqliteMigrationOps {
			return sqliteMigrationOps{verify: func(context.Context, *sql.DB, legacyFlowcraftState, string) error { return injected }}
		},
		"checkpoint": func() sqliteMigrationOps {
			return sqliteMigrationOps{checkpoint: func(context.Context, *sql.DB) error { return injected }}
		},
	}
	for name, testOps := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			raw := writeLegacyMigrationFixture(t, dir, legacyMigrationFixture())
			if err := migrateLegacyFlowcraftState(t.Context(), dir, testOps()); !errors.Is(err, injected) {
				t.Fatalf("stage failure = %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, flowcraftSQLiteName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stage failure published memory.db: %v", err)
			}
			legacyAfter, err := os.ReadFile(filepath.Join(dir, legacyFlowcraftStateName))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(raw, legacyAfter) {
				t.Fatal("stage failure modified legacy state.json")
			}
			if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err != nil {
				t.Fatalf("retry after stage failure: %v", err)
			}
		})
	}
}

func TestMigrateLegacyFlowcraftStateFailsClosed(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		dir := t.TempDir()
		writeLegacyMigrationFixture(t, dir, legacyMigrationFixture())
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := migrateLegacyFlowcraftState(ctx, dir, sqliteMigrationOps{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled migration error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, flowcraftSQLiteName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancelled migration published memory.db: %v", err)
		}
	})
	t.Run("unsupported version", func(t *testing.T) {
		dir := t.TempDir()
		state := legacyMigrationFixture()
		state.Version = 2
		writeLegacyMigrationFixture(t, dir, state)
		if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("unsupported version error = %v", err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, legacyFlowcraftStateName), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Fatalf("malformed state error = %v", err)
		}
	})
	t.Run("ambiguous database", func(t *testing.T) {
		dir := t.TempDir()
		writeLegacyMigrationFixture(t, dir, legacyMigrationFixture())
		backend, err := flowsqlite.Open(t.Context(), filepath.Join(dir, flowcraftSQLiteName))
		if err != nil {
			t.Fatal(err)
		}
		if err := backend.Close(); err != nil {
			t.Fatal(err)
		}
		if err := migrateLegacyFlowcraftState(t.Context(), dir, sqliteMigrationOps{}); err == nil || !strings.Contains(err.Error(), "unmarked") {
			t.Fatalf("ambiguous database error = %v", err)
		}
	})
}

func legacyMigrationFixture() legacyFlowcraftState {
	now := time.Date(2026, time.August, 23, 10, 0, 0, 0, time.UTC)
	scope := flowrecall.Scope{RuntimeID: "workspace-a", UserID: "user-a", AgentID: "agent-a"}
	secondScope := flowrecall.Scope{RuntimeID: "workspace-b"}
	priorValidTo := now.Add(time.Minute)
	facts := []flowrecall.TemporalFact{
		{
			ID: "fact-prior", Scope: scope, Kind: flowrecall.FactPreference, Content: "likes fish",
			ObservedAt: now, ValidTo: &priorValidTo, CorrectedBy: "fact-current", Entities: []string{"Mochi"},
			Metadata: map[string]any{"nested": map[string]any{"value": "kept"}},
		},
		{
			ID: "fact-current", Scope: scope, Kind: flowrecall.FactPreference, Content: "likes salmon",
			ObservedAt: now.Add(time.Minute), Supersedes: []string{"fact-prior"}, Entities: []string{"Mochi", "salmon"},
			Origin: flowrecall.FactOrigin{RequestID: "origin-request"},
		},
		{
			ID: "fact-workspace-b", Scope: secondScope, Kind: flowrecall.FactState, Content: "workspace-b state",
			ObservedAt: now.Add(2 * time.Minute), Metadata: map[string]any{"workspace": "b"},
		},
	}
	statuses := []string{"pending", "leased", "failed", "complete"}
	sideEffects := make([]legacySideEffectRecord, 0, len(statuses))
	async := make([]legacyAsyncSemanticRecord, 0, len(statuses))
	for index, status := range statuses {
		sideJob := flowrecall.SideEffectJob{
			ID: "side-" + status, RequestID: "side-request-" + status, Scope: scope,
			Kind: flowrecall.SideEffectProjectRequired, Facts: facts[:1], Attempt: index,
		}
		asyncJob := flowrecall.AsyncSemanticJob{
			RequestID: "async-" + status, Scope: scope, EpisodeFactIDs: []string{"fact-prior"},
			ObservedAt: now, Attempt: index,
		}
		if status == "leased" {
			sideJob.LeaseToken = "side-token"
			sideJob.Attempt = 2
			sideJob.LeaseUntil = now.Add(time.Hour)
			asyncJob.LeaseToken = "async-token"
			asyncJob.LeaseUntil = now.Add(time.Hour)
		}
		sideRecord := legacySideEffectRecord{Job: sideJob, Status: status, EnqueuedAt: now.Add(time.Duration(index) * time.Second)}
		asyncRecord := legacyAsyncSemanticRecord{Job: asyncJob, Status: status, EnqueuedAt: now.Add(time.Duration(index) * time.Second)}
		if status == "failed" {
			sideRecord.Failure = flowrecall.SideEffectFailure{ErrClass: flowrecall.ErrClassPermanent, Err: "side failed"}
			asyncRecord.Failure = flowrecall.AsyncSemanticFailure{ErrClass: flowrecall.ErrClassPermanent, Err: "async failed"}
		}
		if status == "complete" {
			sideRecord.Result = flowrecall.SideEffectResult{CompletedAt: now.Add(time.Hour)}
			asyncRecord.Result = flowrecall.AsyncSemanticResult{SemanticFactIDs: []string{"fact-current"}}
		}
		sideEffects = append(sideEffects, sideRecord)
		async = append(async, asyncRecord)
	}
	return legacyFlowcraftState{
		Version: legacyFlowcraftVersion,
		Facts:   facts,
		Evidence: []legacyEvidenceRecord{
			{
				Scope: scope, FactID: "fact-current", EvidenceID: "evidence-1", Ordinal: 3,
				Ref: flowrecall.EvidenceRef{ID: "evidence-1", MessageID: "message-1", Text: "source text", Timestamp: now},
			},
			{
				Scope: secondScope, FactID: "fact-workspace-b", EvidenceID: "evidence-2", Ordinal: 0,
				Ref: flowrecall.EvidenceRef{ID: "evidence-2", MessageID: "message-2", Text: "workspace-b source", Timestamp: now},
			},
		},
		SideEffects: sideEffects,
		Async:       async,
		Counters: map[string]legacyCounterPair{
			scope.PartitionKey():       {SideEffect: 3, AsyncSemantic: 5},
			secondScope.PartitionKey(): {SideEffect: 1, AsyncSemantic: 2},
		},
	}
}

func writeLegacyMigrationFixture(t *testing.T, dir string, state legacyFlowcraftState) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacyFlowcraftStateName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertJSONEqual(t *testing.T, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}
