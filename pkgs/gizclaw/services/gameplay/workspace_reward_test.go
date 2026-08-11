package gameplay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/jmoiron/sqlx"
)

type workspaceRewardTestGenerator struct {
	result      string
	results     []string
	err         error
	errs        []error
	invokeCount int
	pattern     string
	context     string
	contexts    []string
	tool        *genx.FuncTool
}

func (*workspaceRewardTestGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	return nil, errors.New("unexpected GenerateStream")
}

func (g *workspaceRewardTestGenerator) Invoke(
	_ context.Context,
	pattern string,
	modelContext genx.ModelContext,
	tool *genx.FuncTool,
) (genx.Usage, *genx.FuncCall, error) {
	index := g.invokeCount
	g.invokeCount++
	g.pattern = pattern
	g.tool = tool
	g.context, _ = genx.InspectModelContext(modelContext)
	g.contexts = append(g.contexts, g.context)
	if index < len(g.errs) && g.errs[index] != nil {
		return genx.Usage{}, nil, g.errs[index]
	}
	if g.err != nil {
		return genx.Usage{}, nil, g.err
	}
	result := g.result
	if index < len(g.results) {
		result = g.results[index]
	}
	return genx.Usage{}, tool.NewFuncCall(result), nil
}

type workspaceRewardTestEnvironment struct {
	entries       map[string][]workspace.HistoryEntry
	ids           []string
	availability  map[string]error
	historyErrors map[string]error
	policy        *WorkspaceRewardPolicySnapshot
	generator     genx.Generator
	notifications []WorkspaceRewardUpdate
}

type workspaceRewardInvalidWithoutFence struct{}

func (workspaceRewardInvalidWithoutFence) Error() string                     { return "invalid Workspace identity" }
func (workspaceRewardInvalidWithoutFence) Unwrap() error                     { return workspace.ErrWorkspaceInvalid }
func (workspaceRewardInvalidWithoutFence) WorkspaceRewardFenceAllowed() bool { return false }

func (e *workspaceRewardTestEnvironment) ListWorkspaceIDs(context.Context) ([]string, error) {
	if e.ids != nil {
		return append([]string(nil), e.ids...), nil
	}
	names := make([]string, 0, len(e.entries))
	for name := range e.entries {
		names = append(names, name)
	}
	return names, nil
}

func (e *workspaceRewardTestEnvironment) EnsureWorkspaceAvailable(_ context.Context, name string) error {
	return e.availability[name]
}

func (e *workspaceRewardTestEnvironment) LatestHistoryEntry(_ context.Context, name string) (workspace.HistoryEntry, bool, error) {
	if err := e.historyErrors[name]; err != nil {
		return workspace.HistoryEntry{}, false, err
	}
	entries := e.entries[name]
	if len(entries) == 0 {
		return workspace.HistoryEntry{}, false, nil
	}
	return entries[len(entries)-1], true, nil
}

func TestWorkspaceRewardStartupIsolatesUnavailableWorkspace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		ids: []string{"workspace-broken", "workspace-active"},
		availability: map[string]error{
			"workspace-broken": workspace.ErrPeerDeleted,
		},
		historyErrors: map[string]error{
			"workspace-broken": workspace.ErrPeerDeleted,
		},
		entries: map[string][]workspace.HistoryEntry{
			"workspace-active": {{
				ID: "history-active", Type: "gear", GearID: "peer-active",
				Origin: workspace.HistoryOriginAgentHost, Text: "active",
				CreatedAt: now.Add(-time.Minute),
			}},
		},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	brokenSource := workspaceRewardSource{
		WorkspaceID: "workspace-broken", ScheduledCheckpoint: "history-broken",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, brokenSource); err != nil {
		t.Fatalf("insert broken source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-broken", WorkspaceID: brokenSource.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-broken",
		RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "history-broken", HighWaterHistoryID: "history-broken",
		StartHistoryAt: now.Add(-time.Hour), HighWaterHistoryAt: now.Add(-time.Hour),
		OpenedAt: now.Add(-time.Hour), LastActivityAt: now.Add(-time.Hour),
		EvaluateAfter: now.Add(-time.Minute), State: workspaceRewardPending,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, brokenSource); err != nil {
		t.Fatalf("insert broken window: %v", err)
	}
	completed := window
	completed.ID = "window-completed"
	completed.State = workspaceRewardCompleted
	completed.Outcome = "settled"
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, completed, brokenSource); err != nil {
		t.Fatalf("insert completed window: %v", err)
	}

	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
	}
	stop()
	<-done
	if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-broken"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("broken source error = %v, want sql.ErrNoRows", err)
	}
	var pendingCount, completedCount int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE id = 'window-broken'),
		COUNT(*) FILTER (WHERE id = 'window-completed')
		FROM gameplay_workspace_reward_windows`).Scan(&pendingCount, &completedCount); err != nil {
		t.Fatalf("count retained windows: %v", err)
	}
	if pendingCount != 0 || completedCount != 1 {
		t.Fatalf("pending/completed windows = %d/%d, want 0/1", pendingCount, completedCount)
	}
	active, err := runtime.getWorkspaceRewardSource(ctx, "workspace-active")
	if err != nil {
		t.Fatalf("active source error = %v", err)
	}
	if active.CompletedCheckpoint != "history-active" {
		t.Fatalf("active source = %#v, want startup baseline", active)
	}
	if got := len(runtime.workspaceRewardFaults); got != 1 {
		t.Fatalf("fault diagnostics = %d, want one per affected Workspace", got)
	}
	restarted := &Runtime{DB: runtime.DB, WorkspaceRewards: environment, Now: func() time.Time { return now }}
	stop, done, err = restarted.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("restart StartWorkspaceRewardDispatcher() error = %v", err)
	}
	stop()
	<-done
	if got := len(restarted.workspaceRewardFaults); got != 1 {
		t.Fatalf("restart fault diagnostics = %d, want one", got)
	}
	if err := restarted.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE id = 'window-completed'`).Scan(&completedCount); err != nil {
		t.Fatalf("count completed window after restart: %v", err)
	}
	if completedCount != 1 {
		t.Fatalf("completed window count after restart = %d, want 1", completedCount)
	}
}

func TestWorkspaceRewardLocalFaultClassificationUsesTypedIdentity(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		err       error
		wantClass string
		wantLocal bool
	}{
		"Workspace pending": {err: fmt.Errorf("wrapped: %w", workspace.ErrWorkspacePendingDeletion), wantClass: "workspace_pending_deletion", wantLocal: true},
		"owner pending":     {err: fmt.Errorf("wrapped: %w", workspace.ErrPeerPendingDeletion), wantClass: "owner_pending_deletion", wantLocal: true},
		"owner deleted":     {err: fmt.Errorf("wrapped: %w", workspace.ErrPeerDeleted), wantClass: "owner_deleted", wantLocal: true},
		"exact owner missing": {
			err:       fmt.Errorf("wrapped: %w", workspace.NewExactOwnerNotFoundError()),
			wantClass: "owner_missing", wantLocal: true,
		},
		"Workspace invalid":                {err: fmt.Errorf("wrapped: %w", workspace.ErrWorkspaceInvalid), wantClass: "workspace_invalid", wantLocal: true},
		"unrelated owner missing sentinel": {err: fmt.Errorf("unrelated: %w", workspace.ErrPeerNotFound)},
		"matching error text":              {err: errors.New(workspace.ErrPeerDeleted.Error())},
		"provider failure":                 {err: errors.New("provider unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			class, local := workspaceRewardLocalFaultClass(test.err)
			if class != test.wantClass || local != test.wantLocal {
				t.Fatalf("workspaceRewardLocalFaultClass() = %q, %v, want %q, %v", class, local, test.wantClass, test.wantLocal)
			}
		})
	}
}

func TestWorkspaceRewardStartupFailsWhenLocalFenceRollsBack(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 8, 20, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		ids:          []string{"workspace-broken"},
		availability: map[string]error{"workspace-broken": workspace.ErrPeerDeleted},
		entries:      map[string][]workspace.HistoryEntry{"workspace-broken": nil},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	source := workspaceRewardSource{WorkspaceID: "workspace-broken", CreatedAt: now, UpdatedAt: now}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-broken", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-broken",
		RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "history-broken", HighWaterHistoryID: "history-broken",
		StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
		EvaluateAfter: now, State: workspaceRewardPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert window: %v", err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `CREATE TRIGGER fail_workspace_reward_source_fence
		BEFORE DELETE ON gameplay_workspace_reward_sources
		WHEN OLD.workspace_id = 'workspace-broken'
		BEGIN SELECT RAISE(ABORT, 'forced Workspace reward fence failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(ctx); err == nil || !strings.Contains(err.Error(), "forced Workspace reward fence failure") {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want fence failure", err)
	}
	if _, err := runtime.getWorkspaceRewardSource(ctx, source.WorkspaceID); err != nil {
		t.Fatalf("source changed after rollback: %v", err)
	}
	var count int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE id = ?`, window.ID).Scan(&count); err != nil {
		t.Fatalf("count window after rollback: %v", err)
	}
	if count != 1 {
		t.Fatalf("window count after rollback = %d, want 1", count)
	}
}

func TestWorkspaceRewardStartupPreservesUntrustedMismatchState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 8, 30, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		ids: []string{"workspace-key"},
		availability: map[string]error{
			"workspace-key": fmt.Errorf("exact lookup: %w", workspaceRewardInvalidWithoutFence{}),
		},
		entries: map[string][]workspace.HistoryEntry{"workspace-key": nil},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	source := workspaceRewardSource{
		WorkspaceID: "workspace-key", ScheduledCheckpoint: "history-key",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-key", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-key",
		RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "history-key", HighWaterHistoryID: "history-key",
		StartHistoryAt: now.Add(-time.Hour), HighWaterHistoryAt: now.Add(-time.Hour),
		OpenedAt: now.Add(-time.Hour), LastActivityAt: now.Add(-time.Hour),
		EvaluateAfter: now.Add(-time.Minute), State: workspaceRewardPending,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert window: %v", err)
	}

	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
	}
	stop()
	<-done
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || processed {
		t.Fatalf("dispatchWorkspaceReward(isolated mismatch) = %v, %v, want false, nil", processed, err)
	}
	if _, err := runtime.getWorkspaceRewardSource(ctx, source.WorkspaceID); err != nil {
		t.Fatalf("source was changed: %v", err)
	}
	var state string
	var attempts int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, attempt_count FROM gameplay_workspace_reward_windows WHERE id = ?`, window.ID).Scan(&state, &attempts); err != nil {
		t.Fatalf("read preserved window: %v", err)
	}
	if state != workspaceRewardPending || attempts != 0 {
		t.Fatalf("preserved window state/attempts = %q/%d, want pending/0", state, attempts)
	}
}

func TestWorkspaceRewardStartupKeepsAvailabilityBackendFailuresGlobal(t *testing.T) {
	t.Parallel()
	backendErr := errors.New("workspace backend unavailable")
	environment := &workspaceRewardTestEnvironment{
		ids:          []string{"workspace-active"},
		availability: map[string]error{"workspace-active": backendErr},
		entries:      map[string][]workspace.HistoryEntry{"workspace-active": nil},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(t.Context()); !errors.Is(err, backendErr) {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want backend error", err)
	}
}

func TestWorkspaceRewardStartupKeepsUnreadableSQLIdentityGlobal(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 8, 45, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		ids:     []string{"workspace-active"},
		entries: map[string][]workspace.HistoryEntry{"workspace-active": nil},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	source := workspaceRewardSource{
		WorkspaceID: "workspace-active", ScheduledCheckpoint: "history-active",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-active",
		RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "history-active", HighWaterHistoryID: "history-active",
		StartHistoryAt: now.Add(-time.Hour), HighWaterHistoryAt: now.Add(-time.Hour),
		OpenedAt: now.Add(-time.Hour), LastActivityAt: now.Add(-time.Hour),
		EvaluateAfter: now.Add(-time.Minute), State: workspaceRewardPending,
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert invalid-identity window: %v", err)
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(ctx); err == nil ||
		!strings.Contains(err.Error(), "invalid workspace reward window identity") {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want global row identity error", err)
	}
	var state string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state FROM gameplay_workspace_reward_windows WHERE id = ''`).Scan(&state); err != nil {
		t.Fatalf("read invalid-identity window: %v", err)
	}
	if state != workspaceRewardPending {
		t.Fatalf("invalid-identity window state = %q, want unchanged pending", state)
	}
}

func TestWorkspaceRewardDispatcherRequiresAvailabilityCapability(t *testing.T) {
	t.Parallel()
	environment := &workspaceRewardTestEnvironment{entries: map[string][]workspace.HistoryEntry{}}
	db := testDB(t)
	runtime := &Runtime{
		DB: db,
		WorkspaceRewards: &struct{ WorkspaceRewardEnvironment }{
			WorkspaceRewardEnvironment: environment,
		},
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(t.Context()); !errors.Is(err, errWorkspaceRewardAvailability) {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want availability configuration error", err)
	}
	var migrationTables int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'gameplay_workspace_reward_activation'`).Scan(&migrationTables); err != nil {
		t.Fatalf("inspect migration state: %v", err)
	}
	if migrationTables != 0 {
		t.Fatalf("availability capability was checked after migration")
	}
}

func TestWorkspaceRewardStartupBlocksCorruptPersistedPolicy(t *testing.T) {
	for name, corrupt := range map[string]func(*testing.T, *Runtime, context.Context, workspaceRewardWindow){
		"malformed json": func(t *testing.T, runtime *Runtime, ctx context.Context, window workspaceRewardWindow) {
			t.Helper()
			if _, err := runtime.DB.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows SET policy_json = '{' WHERE id = ?`, window.ID); err != nil {
				t.Fatalf("corrupt policy JSON: %v", err)
			}
		},
		"digest mismatch": func(t *testing.T, runtime *Runtime, ctx context.Context, window workspaceRewardWindow) {
			t.Helper()
			if _, err := runtime.DB.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows SET policy_digest = 'mismatch' WHERE id = ?`, window.ID); err != nil {
				t.Fatalf("corrupt policy digest: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
			environment := &workspaceRewardTestEnvironment{
				ids: []string{"workspace-corrupt", "workspace-active"},
				entries: map[string][]workspace.HistoryEntry{
					"workspace-corrupt": nil,
					"workspace-active": {{
						ID: "history-active", Type: "gear", GearID: "peer-active",
						Origin: workspace.HistoryOriginAgentHost, Text: "active",
						CreatedAt: now.Add(-time.Minute),
					}},
				},
			}
			runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
			if err := runtime.Migration(ctx); err != nil {
				t.Fatalf("Migration() error = %v", err)
			}
			source := workspaceRewardSource{
				WorkspaceID: "workspace-corrupt", ScheduledCheckpoint: "history-corrupt",
				CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
			}
			if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
				t.Fatalf("insert source: %v", err)
			}
			policy := workspaceRewardTestPolicy(t)
			window := workspaceRewardWindow{
				ID: "window-corrupt", WorkspaceID: source.WorkspaceID,
				WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-corrupt",
				RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
				Policy: policy, PolicyDigest: policy.Digest,
				StartHistoryID: "history-corrupt", HighWaterHistoryID: "history-corrupt",
				StartHistoryAt: now.Add(-time.Hour), HighWaterHistoryAt: now.Add(-time.Hour),
				OpenedAt: now.Add(-time.Hour), LastActivityAt: now.Add(-time.Hour),
				EvaluateAfter: now.Add(-time.Minute), State: workspaceRewardPending,
				CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
			}
			if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
				t.Fatalf("insert window: %v", err)
			}
			corrupt(t, runtime, ctx, window)

			stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
			if err != nil {
				t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
			}
			stop()
			<-done
			var state, lastError string
			if err := runtime.DB.QueryRowContext(ctx, `SELECT state, last_error FROM gameplay_workspace_reward_windows WHERE id = ?`, window.ID).Scan(&state, &lastError); err != nil {
				t.Fatalf("read corrupt window: %v", err)
			}
			wantError := "reward_policy_invalid"
			if name == "digest mismatch" {
				wantError = "reward_policy_digest_mismatch"
			}
			if state != workspaceRewardBlocked || lastError != wantError {
				t.Fatalf("corrupt window state/error = %q/%q, want %q/%q", state, lastError, workspaceRewardBlocked, wantError)
			}
			if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-active"); err != nil {
				t.Fatalf("active source error = %v", err)
			}
			corruptSource, err := runtime.getWorkspaceRewardSource(ctx, "workspace-corrupt")
			if err != nil {
				t.Fatalf("corrupt source error = %v", err)
			}
			if corruptSource.ScheduledCheckpoint != source.ScheduledCheckpoint {
				t.Fatalf("corrupt source checkpoint = %q, want %q", corruptSource.ScheduledCheckpoint, source.ScheduledCheckpoint)
			}
		})
	}
}

func TestWorkspaceRewardStartupFailsWhenCorruptWindowCannotBeBlocked(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 9, 15, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		ids:     []string{"workspace-corrupt"},
		entries: map[string][]workspace.HistoryEntry{"workspace-corrupt": nil},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	source := workspaceRewardSource{WorkspaceID: "workspace-corrupt", CreatedAt: now, UpdatedAt: now}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-corrupt", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-corrupt",
		RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "history-corrupt", HighWaterHistoryID: "history-corrupt",
		StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
		EvaluateAfter: now, State: workspaceRewardPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert window: %v", err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows SET policy_json = '{' WHERE id = ?`, window.ID); err != nil {
		t.Fatalf("corrupt policy JSON: %v", err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `CREATE TRIGGER fail_workspace_reward_window_block
		BEFORE UPDATE ON gameplay_workspace_reward_windows
		WHEN OLD.id = 'window-corrupt' AND NEW.state = 'blocked'
		BEGIN SELECT RAISE(ABORT, 'forced Workspace reward block failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(ctx); err == nil || !strings.Contains(err.Error(), "forced Workspace reward block failure") {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want block failure", err)
	}
	var state, lastError string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, last_error FROM gameplay_workspace_reward_windows WHERE id = ?`, window.ID).Scan(&state, &lastError); err != nil {
		t.Fatalf("read corrupt window: %v", err)
	}
	if state != workspaceRewardPending || lastError != "" {
		t.Fatalf("corrupt window state/error = %q/%q, want unchanged pending/empty", state, lastError)
	}
}

func (e *workspaceRewardTestEnvironment) LatestHistoryEntryBefore(
	_ context.Context,
	name string,
	before time.Time,
) (workspace.HistoryEntry, bool, error) {
	for i := len(e.entries[name]) - 1; i >= 0; i-- {
		if e.entries[name][i].CreatedAt.Before(before) {
			return e.entries[name][i], true, nil
		}
	}
	return workspace.HistoryEntry{}, false, nil
}

func (e *workspaceRewardTestEnvironment) ListHistoryEntries(
	_ context.Context,
	name, after, through string,
	limit int,
) (workspace.HistoryEntryPage, error) {
	var values []workspace.HistoryEntry
	for _, entry := range e.entries[name] {
		if entry.ID <= after || through != "" && entry.ID > through {
			continue
		}
		values = append(values, entry)
	}
	hasNext := len(values) > limit
	if hasNext {
		values = values[:limit]
	}
	next := ""
	if hasNext {
		next = values[len(values)-1].ID
	}
	return workspace.HistoryEntryPage{Entries: values, HasNext: hasNext, NextCursor: next}, nil
}

func (e *workspaceRewardTestEnvironment) GetHistoryEntry(_ context.Context, name, id string) (workspace.HistoryEntry, error) {
	for _, entry := range e.entries[name] {
		if entry.ID == id {
			return entry, nil
		}
	}
	return workspace.HistoryEntry{}, fmt.Errorf("History entry %q not found", id)
}

func (e *workspaceRewardTestEnvironment) ResolveWorkspaceRewardPolicy(
	context.Context,
	string,
	string,
) (WorkspaceRewardKind, *WorkspaceRewardPolicySnapshot, error) {
	return WorkspaceRewardKindWorkflow, e.policy, nil
}

func (e *workspaceRewardTestEnvironment) WorkspaceRewardGenerator(
	context.Context,
	string,
	WorkspaceRewardPolicySnapshot,
) (genx.Generator, error) {
	return e.generator, nil
}

func (e *workspaceRewardTestEnvironment) NotifyWorkspaceReward(
	_ context.Context,
	_ string,
	update WorkspaceRewardUpdate,
) error {
	e.notifications = append(e.notifications, update)
	return nil
}

func TestEvaluateWorkspaceRewardUsesOneBoundedStructuredInvoke(t *testing.T) {
	t.Parallel()
	generator := &workspaceRewardTestGenerator{result: `{
		"score": 90,
		"reason": "Strong scientific reasoning.",
		"badges": [{"alias": "science", "exp": 4}]
	}`}
	policy := workspaceRewardTestPolicy(t)
	result, err := evaluateWorkspaceReward(context.Background(), generator, policy, []WorkspaceRewardTranscriptEntry{
		{Role: "user", Text: "Why do objects fall?"},
		{Role: "assistant", Text: "Gravity attracts masses."},
	})
	if err != nil {
		t.Fatalf("evaluateWorkspaceReward() error = %v", err)
	}
	if result.Score != 90 || len(result.Badges) != 1 || result.Badges[0].Alias != "science" {
		t.Fatalf("evaluateWorkspaceReward() = %#v", result)
	}
	if generator.invokeCount != 1 || generator.pattern != "model/reward-evaluator" {
		t.Fatalf("Invoke count/pattern = %d, %q", generator.invokeCount, generator.pattern)
	}
	if generator.tool == nil || generator.tool.Name != "submit_workspace_reward" {
		t.Fatalf("tool = %#v", generator.tool)
	}
	aliases := generator.tool.Argument.Properties["badges"].Items.Properties["alias"].Enum
	if len(aliases) != 1 || aliases[0] != "science" {
		t.Fatalf("badge alias enum = %#v", aliases)
	}
	if score := generator.tool.Argument.Properties["score"]; score.Minimum == nil ||
		*score.Minimum != 0 || score.Maximum == nil || *score.Maximum != 100 {
		t.Fatalf("score schema = %#v", score)
	}
	if badges := generator.tool.Argument.Properties["badges"]; badges.MaxItems == nil ||
		*badges.MaxItems != 1 || badges.Items.Properties["exp"].Maximum == nil ||
		*badges.Items.Properties["exp"].Maximum != 5 {
		t.Fatalf("badges schema = %#v", badges)
	}
	for _, required := range []string{"untrusted conversation data", "Reward scientific curiosity.", "science", "Why do objects fall?"} {
		if !strings.Contains(generator.context, required) {
			t.Fatalf("model context does not contain %q:\n%s", required, generator.context)
		}
	}
	for _, forbidden := range []string{"badge-science", "model-reward"} {
		if strings.Contains(generator.context, forbidden) {
			t.Fatalf("model context exposes %q:\n%s", forbidden, generator.context)
		}
	}
}

func TestSnapshotWorkspaceRewardPolicyResolvesFrozenResources(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 1, 30, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	rewardPrompt := "Reward evidence of scientific reasoning."
	response, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{
		Body: &adminhttp.BadgeDefUpsert{Id: "badge-science", Spec: apitypes.BadgeDefSpec{
			DisplayName: " Science ", RewardPrompt: &rewardPrompt,
		}},
	})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, response)
	models := map[string]apitypes.RuntimeProfileBinding{
		"reward-evaluator": {ResourceId: "model-reward"},
	}
	badgeDefs := map[string]apitypes.RuntimeProfileBinding{
		"science": {ResourceId: "badge-science"},
	}
	kinds := []apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKinds{
		apitypes.RuntimeProfileWorkspaceRewardSpecWorkspaceKindsWorkflow,
	}
	badges := map[string]apitypes.RuntimeProfileWorkspaceRewardBadgeSpec{
		"science": {MaxExpPerWindow: 5},
	}
	initialBalance := int64(7)
	profile := apitypes.RuntimeProfile{
		Id: "profile-a", Revision: "revision-a",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{
				Models: &models, BadgeDefs: &badgeDefs,
			},
			Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Points: &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
				WorkspaceReward: &apitypes.RuntimeProfileWorkspaceRewardSpec{
					Enabled: true, WorkspaceKinds: &kinds,
					Debounce: &apitypes.RuntimeProfileWorkspaceRewardDebounceSpec{
						QuietPeriod: "1m", MaxWindowAge: "10m",
					},
					Transcript: &apitypes.RuntimeProfileWorkspaceRewardTranscriptSpec{
						MaxEntries: 20, MaxTextBytes: 4096,
					},
					Evaluation: &apitypes.RuntimeProfileWorkspaceRewardEvaluationSpec{
						Model: "reward-evaluator", PointsPrompt: "Reward quality.",
						ScoreMin: 0, ScoreMax: 100, QualifyingScore: 80,
					},
					Points: &apitypes.RuntimeProfileWorkspaceRewardPointsSpec{
						Tiers: []apitypes.RuntimeProfileWorkspaceRewardPointsTier{{MinScore: 80, Delta: 10}},
					},
					Badges: &badges,
					RollingBudget: &apitypes.RuntimeProfileWorkspaceRewardRollingBudgetSpec{
						Period: "24h", PointsMax: 10, BadgeExpMax: 5,
					},
				},
			},
		},
	}
	runtime := &Runtime{Catalog: catalog}
	snapshot, err := runtime.SnapshotWorkspaceRewardPolicy(ctx, profile, WorkspaceRewardKindWorkflow)
	if err != nil {
		t.Fatalf("SnapshotWorkspaceRewardPolicy() error = %v", err)
	}
	if snapshot == nil || snapshot.ModelResourceID != "model-reward" ||
		snapshot.InitialPointsBalance != initialBalance || len(snapshot.Badges) != 1 ||
		snapshot.Badges[0].ResourceID != "badge-science" ||
		snapshot.Badges[0].DisplayName != "Science" ||
		snapshot.Badges[0].RewardPrompt != rewardPrompt || snapshot.Digest == "" {
		t.Fatalf("SnapshotWorkspaceRewardPolicy() = %#v", snapshot)
	}
	if disallowed, err := runtime.SnapshotWorkspaceRewardPolicy(
		ctx,
		profile,
		WorkspaceRewardKindGroupChatroom,
	); err != nil || disallowed != nil {
		t.Fatalf("SnapshotWorkspaceRewardPolicy(disallowed) = %#v, %v", disallowed, err)
	}
}

func TestEvaluateWorkspaceRewardSupportsPointsOnlyPolicy(t *testing.T) {
	t.Parallel()
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"Qualified.","badges":[]}`,
	}
	policy := workspaceRewardTestPolicy(t)
	policy.Badges = nil
	result, err := evaluateWorkspaceReward(
		context.Background(),
		generator,
		policy,
		[]WorkspaceRewardTranscriptEntry{
			{Role: "user", Text: "I learned something."},
			{Role: "assistant", Text: "Show your reasoning."},
		},
	)
	if err != nil {
		t.Fatalf("evaluateWorkspaceReward() error = %v", err)
	}
	if result.Score != 90 || len(result.Badges) != 0 {
		t.Fatalf("evaluateWorkspaceReward() = %#v", result)
	}
	badges := generator.tool.Argument.Properties["badges"]
	alias := badges.Items.Properties["alias"]
	if badges.MaxItems == nil || *badges.MaxItems != 0 || len(alias.Enum) != 0 {
		t.Fatalf("points-only Badge schema = %#v", badges)
	}
}

func TestValidateWorkspaceRewardEvaluationRejectsModelEscalation(t *testing.T) {
	t.Parallel()
	policy := workspaceRewardTestPolicy(t)
	for name, value := range map[string]workspaceRewardEvaluation{
		"score":         {Score: 101, Reason: "invalid"},
		"empty reason":  {Score: 90},
		"unknown badge": {Score: 90, Reason: "invalid", Badges: []workspaceRewardBadgeRecommendation{{Alias: "admin", Exp: 1}}},
		"duplicate":     {Score: 90, Reason: "invalid", Badges: []workspaceRewardBadgeRecommendation{{Alias: "science", Exp: 1}, {Alias: "science", Exp: 1}}},
		"excess exp":    {Score: 90, Reason: "invalid", Badges: []workspaceRewardBadgeRecommendation{{Alias: "science", Exp: 6}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateWorkspaceRewardEvaluation(value, policy); err == nil {
				t.Fatalf("validateWorkspaceRewardEvaluation(%#v) succeeded", value)
			}
		})
	}
}

func TestWorkspaceRewardWindowSettlesOnceAndEnforcesRollingBudget(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 3, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	generator := &workspaceRewardTestGenerator{result: `{
		"score": 90,
		"reason": "High-quality learning conversation.",
		"badges": [{"alias": "science", "exp": 5}]
	}`}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy, generator: generator,
	}
	catalog := testCatalog(t, now)
	rewardPrompt := "Award sound scientific reasoning."
	response, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{
		Body: &adminhttp.BadgeDefUpsert{Id: "badge-science", Spec: apitypes.BadgeDefSpec{
			DisplayName: "Science", RewardPrompt: &rewardPrompt,
		}},
	})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, response)
	runtime := &Runtime{
		DB:               testDB(t),
		Catalog:          catalog,
		WorkspaceRewards: environment,
		Now:              func() time.Time { return now },
		NewID:            sequentialIDs("window-1", "claim-1", "grant-1", "points-1", "window-2", "claim-2"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(id, entryType, gearID, text string) workspace.HistoryEntry {
		entry := workspace.HistoryEntry{
			ID: id, Type: entryType, GearID: gearID, Origin: workspace.HistoryOriginAgentHost,
			Text: text, CreatedAt: now,
		}
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", id, err)
		}
		return entry
	}
	appendEntry("001", "gear", "peer-a", "I formed a hypothesis.")
	now = now.Add(time.Second)
	appendEntry("002", "agent", "", "Let us test it.")
	now = now.Add(2 * time.Minute)
	processed, err := runtime.dispatchWorkspaceReward(ctx)
	if err != nil || !processed {
		t.Fatalf("dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	initialBalance := int64(0)
	account, err := runtime.GetPoints(WithRuntimeProfile(ctx, apitypes.RuntimeProfile{
		Id: "runtime-profile-a",
		Spec: apitypes.RuntimeProfileSpec{Gameplay: &apitypes.RuntimeProfileGameplaySpec{
			Points: &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
		}},
	}), "peer-a", "runtime-profile-a")
	if err != nil {
		t.Fatalf("GetPoints() error = %v", err)
	}
	if account.Balance != 10 {
		t.Fatalf("points balance = %d, want 10", account.Balance)
	}
	badge, err := runtime.GetBadge(ctx, "peer-a", "badge-science")
	if err != nil {
		t.Fatalf("GetBadge() error = %v", err)
	}
	if badge.Exp != 5 {
		t.Fatalf("badge EXP = %d, want 5", badge.Exp)
	}
	if generator.invokeCount != 1 || len(environment.notifications) != 1 ||
		environment.notifications[0].RewardGrantID != "grant-1" {
		t.Fatalf("invoke/notifications = %d, %#v", generator.invokeCount, environment.notifications)
	}

	now = now.Add(time.Second)
	appendEntry("003", "gear", "peer-a", "I improved the experiment.")
	now = now.Add(time.Second)
	appendEntry("004", "agent", "", "The evidence is stronger.")
	now = now.Add(2 * time.Minute)
	processed, err = runtime.dispatchWorkspaceReward(ctx)
	if err != nil || !processed {
		t.Fatalf("second dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	if generator.invokeCount != 2 {
		t.Fatalf("evaluator Invoke count = %d, want 2", generator.invokeCount)
	}
	var grantCount int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_reward_grants`).Scan(&grantCount); err != nil {
		t.Fatalf("count RewardGrants: %v", err)
	}
	if grantCount != 1 || len(environment.notifications) != 1 {
		t.Fatalf("grant/notification counts = %d/%d, want 1/1", grantCount, len(environment.notifications))
	}
	source, err := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("getWorkspaceRewardSource() error = %v", err)
	}
	if source.CompletedCheckpoint != "004" {
		t.Fatalf("completed checkpoint = %q, want 004", source.CompletedCheckpoint)
	}
}

func TestWorkspaceRewardActivationBaselinesOldHistoryAndRecoversNewWorkspace(t *testing.T) {
	ctx := context.Background()
	activation := time.Date(2026, 7, 29, 2, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{
			"old": {{
				ID: "001", Type: "gear", GearID: "peer-old", Origin: workspace.HistoryOriginAgentHost,
				Text: "old", CreatedAt: activation.Add(-time.Hour),
			}},
			"new": {{
				ID: "002", Type: "gear", GearID: "peer-new", Origin: workspace.HistoryOriginAgentHost,
				Text: "new", CreatedAt: activation.Add(time.Minute),
			}},
		},
		policy: &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return activation }, NewID: sequentialIDs("window-new"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if err := runtime.initializeWorkspaceRewardSources(ctx, activation); err != nil {
		t.Fatalf("initializeWorkspaceRewardSources() error = %v", err)
	}
	if err := runtime.reconcileWorkspaceRewardSources(ctx); err != nil {
		t.Fatalf("reconcileWorkspaceRewardSources() error = %v", err)
	}
	oldSource, err := runtime.getWorkspaceRewardSource(ctx, "old")
	if err != nil {
		t.Fatalf("get old source: %v", err)
	}
	if oldSource.ScheduledCheckpoint != "001" || oldSource.CompletedCheckpoint != "001" {
		t.Fatalf("old source = %#v, want startup baseline", oldSource)
	}
	newSource, err := runtime.getWorkspaceRewardSource(ctx, "new")
	if err != nil {
		t.Fatalf("get new source: %v", err)
	}
	if newSource.ScheduledCheckpoint != "002" || newSource.CompletedCheckpoint != "" {
		t.Fatalf("new source = %#v, want recovered pending History", newSource)
	}
	window, err := runtime.activeWorkspaceRewardWindow(ctx, "new")
	if err != nil || window.StartHistoryID != "002" || window.BeneficiaryPublicKey != "peer-new" {
		t.Fatalf("new active window = %#v, %v", window, err)
	}
}

func TestWorkspaceRewardCallbackSourceCreationUsesActivationBoundary(t *testing.T) {
	ctx := context.Background()
	activation := time.Date(2026, 7, 29, 3, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	oldEntry := workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-old", Origin: workspace.HistoryOriginAgentHost,
		Text: "old", CreatedAt: activation.Add(-time.Minute),
	}
	newEntry := workspace.HistoryEntry{
		ID: "002", Type: "gear", GearID: "peer-new", Origin: workspace.HistoryOriginAgentHost,
		Text: "new", CreatedAt: activation.Add(time.Minute),
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": {oldEntry, newEntry}},
		policy:  &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return activation }, NewID: sequentialIDs("window-new"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if _, err := runtime.ensureWorkspaceRewardActivation(ctx); err != nil {
		t.Fatalf("ensureWorkspaceRewardActivation() error = %v", err)
	}

	if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", newEntry); err != nil {
		t.Fatalf("ScheduleWorkspaceRewardActivity() error = %v", err)
	}

	source, err := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("getWorkspaceRewardSource() error = %v", err)
	}
	if source.CompletedCheckpoint != oldEntry.ID || source.ScheduledCheckpoint != newEntry.ID {
		t.Fatalf("source = %#v, want old entry baselined and new entry scheduled", source)
	}
	window, err := runtime.activeWorkspaceRewardWindow(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("activeWorkspaceRewardWindow() error = %v", err)
	}
	if window.StartHistoryID != newEntry.ID || window.BeneficiaryPublicKey != newEntry.GearID {
		t.Fatalf("window = %#v, want only post-activation initiation", window)
	}
}

func TestWorkspaceRewardMigrationReplacesBlockedActiveIndex(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 3, 30, 0, 0, time.UTC)
	runtime := &Runtime{DB: testDB(t), Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("initial Migration() error = %v", err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `DROP INDEX gameplay_workspace_reward_windows_active_v2_idx`); err != nil {
		t.Fatalf("drop v2 active index: %v", err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `CREATE UNIQUE INDEX gameplay_workspace_reward_windows_active_idx
		ON gameplay_workspace_reward_windows(workspace_id)
		WHERE state IN ('pending', 'claimed', 'retry', 'blocked')`); err != nil {
		t.Fatalf("create legacy active index: %v", err)
	}
	source := workspaceRewardSource{
		WorkspaceID: "workflow-a", ScheduledCheckpoint: "001",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatalf("insertWorkspaceRewardSource() error = %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	window := workspaceRewardWindow{
		ID: "window-blocked", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-a",
		RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
		Policy: policy, PolicyDigest: policy.Digest,
		StartHistoryID: "001", HighWaterHistoryID: "001",
		StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now,
		LastActivityAt: now, EvaluateAfter: now, State: workspaceRewardBlocked,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert legacy blocked window: %v", err)
	}

	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("upgrade Migration() error = %v", err)
	}
	window.ID = "window-pending"
	window.BeneficiaryPublicKey = "peer-b"
	window.StartHistoryID = "002"
	window.HighWaterHistoryID = "002"
	window.State = workspaceRewardPending
	source.ScheduledCheckpoint = "002"
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatalf("insert pending window after upgrade: %v", err)
	}
	var legacyIndexes, currentIndexes int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE name = 'gameplay_workspace_reward_windows_active_idx'),
		COUNT(*) FILTER (WHERE name = 'gameplay_workspace_reward_windows_active_v2_idx')
		FROM sqlite_master WHERE type = 'index'`).Scan(&legacyIndexes, &currentIndexes); err != nil {
		t.Fatalf("read active indexes: %v", err)
	}
	if legacyIndexes != 0 || currentIndexes != 1 {
		t.Fatalf("active index counts legacy/current = %d/%d", legacyIndexes, currentIndexes)
	}
}

func TestWorkspaceRewardDisabledHistoryCannotBecomeRetroactivelyEligible(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 0, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now }, NewID: sequentialIDs("window-enabled"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(entry workspace.HistoryEntry) {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	appendEntry(workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "disabled", CreatedAt: now,
	})
	appendEntry(workspace.HistoryEntry{
		ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
		Text: "disabled response", CreatedAt: now.Add(time.Second),
	})
	policy := workspaceRewardTestPolicy(t)
	environment.policy = &policy
	appendEntry(workspace.HistoryEntry{
		ID: "003", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
		Text: "still not an initiation", CreatedAt: now.Add(2 * time.Second),
	})
	source, err := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("getWorkspaceRewardSource() error = %v", err)
	}
	if source.ScheduledCheckpoint != "003" || source.CompletedCheckpoint != "003" {
		t.Fatalf("disabled source = %#v", source)
	}
	if _, err := runtime.activeWorkspaceRewardWindow(ctx, "workflow-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("active window before enabled initiation error = %v", err)
	}
	appendEntry(workspace.HistoryEntry{
		ID: "004", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "enabled initiation", CreatedAt: now.Add(3 * time.Second),
	})
	window, err := runtime.activeWorkspaceRewardWindow(ctx, "workflow-a")
	if err != nil || window.StartHistoryID != "004" {
		t.Fatalf("enabled window = %#v, %v", window, err)
	}
}

func TestWorkspaceRewardIncompleteAndOverLimitWindowsSkipEvaluator(t *testing.T) {
	for name, entries := range map[string][]workspace.HistoryEntry{
		"user only": {{
			ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "no response", CreatedAt: time.Date(2026, 7, 29, 4, 30, 0, 0, time.UTC),
		}},
		"over limit": {
			{
				ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
				Text: "question", CreatedAt: time.Date(2026, 7, 29, 4, 30, 0, 0, time.UTC),
			},
			{
				ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
				Text: "answer", CreatedAt: time.Date(2026, 7, 29, 4, 30, 1, 0, time.UTC),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			now := entries[0].CreatedAt
			policy := workspaceRewardTestPolicy(t)
			if name == "over limit" {
				policy.MaxEntries = 1
				digest, err := workspaceRewardPolicyDigest(policy)
				if err != nil {
					t.Fatalf("workspaceRewardPolicyDigest() error = %v", err)
				}
				policy.Digest = digest
			}
			generator := &workspaceRewardTestGenerator{result: `{"score":90,"reason":"unexpected","badges":[]}`}
			environment := &workspaceRewardTestEnvironment{
				entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
				policy:  &policy, generator: generator,
			}
			runtime := &Runtime{
				DB: testDB(t), WorkspaceRewards: environment,
				Now: func() time.Time { return now }, NewID: sequentialIDs("window-skip", "claim-skip"),
			}
			if err := runtime.Migration(ctx); err != nil {
				t.Fatalf("Migration() error = %v", err)
			}
			for _, entry := range entries {
				environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
				if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
					t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
				}
			}
			if name == "user only" {
				now = now.Add(policy.MaxWindowAge)
			} else {
				now = now.Add(2 * time.Minute)
			}
			if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
				t.Fatalf("dispatchWorkspaceReward() = %v, %v", processed, err)
			}
			var state, outcome string
			if err := runtime.DB.QueryRowContext(ctx, `SELECT state, outcome
				FROM gameplay_workspace_reward_windows WHERE id = 'window-skip'`).Scan(&state, &outcome); err != nil {
				t.Fatalf("read skipped window: %v", err)
			}
			if state != workspaceRewardCompleted || !strings.HasPrefix(outcome, "skipped_") ||
				generator.invokeCount != 0 {
				t.Fatalf("skipped window state=%q outcome=%q invokes=%d", state, outcome, generator.invokeCount)
			}
		})
	}
}

func TestWorkspaceRewardIncompleteWindowWaitsForAgentResponse(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 40, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"Qualified.","badges":[]}`,
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy, generator: generator,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-wait", "claim-incomplete", "claim-complete", "grant-wait", "points-wait",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(entry workspace.HistoryEntry) {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	appendEntry(workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "I tested a hypothesis.", CreatedAt: now,
	})
	now = now.Add(policy.QuietPeriod)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatch incomplete Workspace reward = %v, %v", processed, err)
	}
	var state, outcome string
	var attempts int
	var evaluateAfterRaw string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, outcome, attempt_count, evaluate_after
		FROM gameplay_workspace_reward_windows WHERE id = ?`, "window-wait").Scan(
		&state, &outcome, &attempts, &evaluateAfterRaw,
	); err != nil {
		t.Fatalf("read deferred incomplete window: %v", err)
	}
	evaluateAfter := parseTime(evaluateAfterRaw)
	if state != workspaceRewardPending || outcome != "" || attempts != 0 ||
		!evaluateAfter.Equal(now.Add(policy.QuietPeriod)) || generator.invokeCount != 0 {
		t.Fatalf(
			"deferred incomplete window state=%q outcome=%q attempts=%d evaluate_after=%s invokes=%d",
			state,
			outcome,
			attempts,
			evaluateAfter,
			generator.invokeCount,
		)
	}

	now = now.Add(time.Second)
	appendEntry(workspace.HistoryEntry{
		ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
		Text: "The evidence supports your hypothesis.", CreatedAt: now,
	})
	now = now.Add(policy.QuietPeriod)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatch completed Workspace reward = %v, %v", processed, err)
	}
	var grantCount int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_reward_grants`).Scan(&grantCount); err != nil {
		t.Fatalf("count deferred Workspace RewardGrant: %v", err)
	}
	if generator.invokeCount != 1 || grantCount != 1 {
		t.Fatalf("completed deferred window invokes=%d grants=%d, want 1/1", generator.invokeCount, grantCount)
	}
}

func TestWorkspaceRewardMaxWindowAgeBoundsContinuousActivity(t *testing.T) {
	ctx := context.Background()
	openedAt := time.Date(2026, 7, 29, 4, 45, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	policy.QuietPeriod = 5 * time.Minute
	policy.MaxWindowAge = 10 * time.Minute
	digest, err := workspaceRewardPolicyDigest(policy)
	if err != nil {
		t.Fatalf("workspaceRewardPolicyDigest() error = %v", err)
	}
	policy.Digest = digest
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return openedAt }, NewID: sequentialIDs("window-max-age"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	for _, entry := range []workspace.HistoryEntry{
		{
			ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "start", CreatedAt: openedAt,
		},
		{
			ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
			Text: "continue", CreatedAt: openedAt.Add(4 * time.Minute),
		},
		{
			ID: "003", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "continue again", CreatedAt: openedAt.Add(8 * time.Minute),
		},
	} {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	window, err := runtime.activeWorkspaceRewardWindow(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("activeWorkspaceRewardWindow() error = %v", err)
	}
	if !window.EvaluateAfter.Equal(openedAt.Add(10*time.Minute)) ||
		window.HighWaterHistoryID != "003" {
		t.Fatalf("continuous window = %#v", window)
	}
}

func TestWorkspaceRewardClaimFreezesFirstBeneficiaryAndDefersLaterHistory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 50, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	policy.Badges = nil
	policy.BadgeExpMax = 0
	digest, err := workspaceRewardPolicyDigest(policy)
	if err != nil {
		t.Fatalf("workspaceRewardPolicyDigest() error = %v", err)
	}
	policy.Digest = digest
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"Qualified.","badges":[]}`,
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"group-a": nil},
		policy:  &policy, generator: generator,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-first", "claim-first", "grant-first", "points-first", "window-second",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(entry workspace.HistoryEntry) {
		environment.entries["group-a"] = append(environment.entries["group-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "group-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	appendEntry(workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-first", Origin: workspace.HistoryOriginAgentHost,
		Text: "I started the discussion.", CreatedAt: now,
	})
	appendEntry(workspace.HistoryEntry{
		ID: "002", Type: "gear", GearID: "peer-later", Origin: workspace.HistoryOriginAgentHost,
		Text: "I joined later.", CreatedAt: now.Add(time.Second),
	})
	appendEntry(workspace.HistoryEntry{
		ID: "003", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
		Text: "Here is a response.", CreatedAt: now.Add(2 * time.Second),
	})
	now = now.Add(2 * time.Minute)
	claimed, ok, err := runtime.claimWorkspaceRewardWindow(ctx)
	if err != nil || !ok || claimed.BeneficiaryPublicKey != "peer-first" ||
		claimed.HighWaterHistoryID != "003" {
		t.Fatalf("claimWorkspaceRewardWindow() = %#v, %v, %v", claimed, ok, err)
	}
	appendEntry(workspace.HistoryEntry{
		ID: "004", Type: "gear", GearID: "peer-later", Origin: workspace.HistoryOriginAgentHost,
		Text: "I start the next discussion.", CreatedAt: now,
	})
	source, err := runtime.getWorkspaceRewardSource(ctx, "group-a")
	if err != nil || source.ScheduledCheckpoint != "003" {
		t.Fatalf("source while claimed = %#v, %v", source, err)
	}
	if err := runtime.processWorkspaceRewardClaim(ctx, claimed); err != nil {
		t.Fatalf("processWorkspaceRewardClaim() error = %v", err)
	}
	if err := runtime.reconcileWorkspaceRewardSource(ctx, "group-a", ""); err != nil {
		t.Fatalf("reconcileWorkspaceRewardSource() error = %v", err)
	}
	next, err := runtime.activeWorkspaceRewardWindow(ctx, "group-a")
	if err != nil || next.StartHistoryID != "004" ||
		next.BeneficiaryPublicKey != "peer-later" {
		t.Fatalf("next window = %#v, %v", next, err)
	}
	var owner string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT owner_public_key
		FROM gameplay_reward_grants WHERE source_id = 'window-first'`).Scan(&owner); err != nil {
		t.Fatalf("read first RewardGrant owner: %v", err)
	}
	if owner != "peer-first" || generator.invokeCount != 1 {
		t.Fatalf("first RewardGrant owner/invokes = %q/%d", owner, generator.invokeCount)
	}
}

func TestWorkspaceRewardExpiredLeaseCannotSettleStaleClaim(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 55, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	policy.Badges = nil
	policy.BadgeExpMax = 0
	digest, err := workspaceRewardPolicyDigest(policy)
	if err != nil {
		t.Fatalf("workspaceRewardPolicyDigest() error = %v", err)
	}
	policy.Digest = digest
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"Qualified.","badges":[]}`,
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy, generator: generator,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-lease", "claim-stale", "claim-current", "grant-lease", "points-lease",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	for _, entry := range []workspace.HistoryEntry{
		{
			ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "question", CreatedAt: now,
		},
		{
			ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
			Text: "answer", CreatedAt: now.Add(time.Second),
		},
	} {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	now = now.Add(2 * time.Minute)
	stale, ok, err := runtime.claimWorkspaceRewardWindow(ctx)
	if err != nil || !ok {
		t.Fatalf("first claim = %#v, %v, %v", stale, ok, err)
	}
	now = now.Add(workspaceRewardClaimLease + time.Second)
	current, ok, err := runtime.claimWorkspaceRewardWindow(ctx)
	if err != nil || !ok || current.ClaimToken == stale.ClaimToken || current.AttemptCount != 2 {
		t.Fatalf("reclaimed window = %#v, %v, %v", current, ok, err)
	}
	if err := runtime.processWorkspaceRewardClaim(ctx, stale); err == nil {
		t.Fatal("stale processWorkspaceRewardClaim() succeeded")
	}
	if generator.invokeCount != 0 {
		t.Fatalf("stale claim evaluator invokes = %d, want 0", generator.invokeCount)
	}
	if err := runtime.processWorkspaceRewardClaim(ctx, current); err != nil {
		t.Fatalf("current processWorkspaceRewardClaim() error = %v", err)
	}
	var grants int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_reward_grants
		WHERE source_id = 'window-lease'`).Scan(&grants); err != nil {
		t.Fatalf("count lease RewardGrants: %v", err)
	}
	if grants != 1 || generator.invokeCount != 1 {
		t.Fatalf("lease grants/invokes = %d/%d, want 1/1", grants, generator.invokeCount)
	}
}

func TestWorkspaceRewardMissingClaimedHistoryBlocksWithoutCheckpoint(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 58, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"unexpected","badges":[]}`,
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy, generator: generator,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-blocked", "claim-blocked", "window-after-blocked",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	for _, entry := range []workspace.HistoryEntry{
		{
			ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "question", CreatedAt: now,
		},
		{
			ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
			Text: "answer", CreatedAt: now.Add(time.Second),
		},
	} {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	environment.entries["workflow-a"] = environment.entries["workflow-a"][:1]
	now = now.Add(2 * time.Minute)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	var state, lastError string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, last_error
		FROM gameplay_workspace_reward_windows WHERE id = 'window-blocked'`).Scan(
		&state,
		&lastError,
	); err != nil {
		t.Fatalf("read blocked window: %v", err)
	}
	source, err := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("getWorkspaceRewardSource() error = %v", err)
	}
	if state != workspaceRewardBlocked || lastError != "history_unavailable" ||
		source.CompletedCheckpoint != "" || generator.invokeCount != 0 {
		t.Fatalf(
			"blocked state=%q error=%q checkpoint=%q invokes=%d",
			state,
			lastError,
			source.CompletedCheckpoint,
			generator.invokeCount,
		)
	}
	nextEntry := workspace.HistoryEntry{
		ID: "003", Type: "gear", GearID: "peer-next", Origin: workspace.HistoryOriginAgentHost,
		Text: "new conversation", CreatedAt: now.Add(time.Second),
	}
	environment.entries["workflow-a"] = append(environment.entries["workflow-a"], nextEntry)
	if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", nextEntry); err != nil {
		t.Fatalf("ScheduleWorkspaceRewardActivity(after blocked) error = %v", err)
	}
	next, err := runtime.activeWorkspaceRewardWindow(ctx, "workflow-a")
	if err != nil {
		t.Fatalf("activeWorkspaceRewardWindow(after blocked) error = %v", err)
	}
	if next.ID != "window-after-blocked" || next.StartHistoryID != nextEntry.ID ||
		next.BeneficiaryPublicKey != nextEntry.GearID {
		t.Fatalf("window after blocked = %#v", next)
	}
}

func TestWorkspaceRewardActivityQueueIsBoundedAndDefersIO(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{WorkspaceRewards: &workspaceRewardTestEnvironment{}}
	entry := workspace.HistoryEntry{ID: "history-a"}
	for range workspaceRewardActivityCapacity + 10 {
		if err := runtime.EnqueueWorkspaceRewardActivity("workflow-a", entry); err != nil {
			t.Fatalf("EnqueueWorkspaceRewardActivity() error = %v", err)
		}
	}
	if got := len(runtime.workspaceRewardActivityChannel()); got != workspaceRewardActivityCapacity {
		t.Fatalf("queued activities = %d, want %d", got, workspaceRewardActivityCapacity)
	}
}

func TestWorkspaceRewardDispatcherConsumesQueuedActivity(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 4, 59, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now }, NewID: sequentialIDs("window-queued"),
	}
	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
	}
	defer func() {
		stop()
		<-done
	}()
	entry := workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "queued conversation", CreatedAt: now,
	}
	environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
	if err := runtime.EnqueueWorkspaceRewardActivity("workflow-a", entry); err != nil {
		t.Fatalf("EnqueueWorkspaceRewardActivity() error = %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		source, sourceErr := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
		if sourceErr == nil && source.ScheduledCheckpoint == entry.ID {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("queued activity was not scheduled: source=%#v error=%v", source, sourceErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkspaceRewardTranscriptRequiresClaimedHighWater(t *testing.T) {
	t.Parallel()
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{entries: map[string][]workspace.HistoryEntry{
		"workflow-a": {{
			ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
			Text: "hello", CreatedAt: time.Unix(1, 0),
		}},
	}}
	runtime := &Runtime{WorkspaceRewards: environment}
	_, _, _, err := runtime.workspaceRewardTranscript(context.Background(), workspaceRewardWindow{
		WorkspaceID: "workflow-a", BeneficiaryPublicKey: "peer-a",
		StartHistoryID: "001", HighWaterHistoryID: "002", Policy: policy,
	})
	if err == nil || !strings.Contains(err.Error(), "high-water") {
		t.Fatalf("workspaceRewardTranscript() error = %v, want missing high-water", err)
	}
}

func TestWorkspaceRewardRetryUsesFrozenTranscriptAndPolicy(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 5, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	generator := &workspaceRewardTestGenerator{
		result: `{"score":90,"reason":"Qualified.","badges":[]}`,
		errs:   []error{errors.New("temporary evaluator outage"), nil},
	}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy, generator: generator,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-retry", "claim-first", "claim-second", "grant-retry", "points-retry",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(entry workspace.HistoryEntry) {
		environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
		}
	}
	appendEntry(workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "I tested a hypothesis.", CreatedAt: now,
	})
	now = now.Add(time.Second)
	appendEntry(workspace.HistoryEntry{
		ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost,
		Text: "Your evidence supports it.", CreatedAt: now,
	})
	if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", environment.entries["workflow-a"][1]); err != nil {
		t.Fatalf("duplicate callback error = %v", err)
	}
	now = now.Add(2 * time.Minute)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("first dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	var state, transcriptDigest, lastError string
	var attempts int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, attempt_count, transcript_digest, last_error
		FROM gameplay_workspace_reward_windows WHERE id = ?`, "window-retry").Scan(
		&state, &attempts, &transcriptDigest, &lastError,
	); err != nil {
		t.Fatalf("read retry window: %v", err)
	}
	if state != workspaceRewardRetry || attempts != 1 || transcriptDigest == "" ||
		lastError != "dependency_failure" {
		t.Fatalf(
			"retry window = state %q attempts %d digest %q error %q",
			state,
			attempts,
			transcriptDigest,
			lastError,
		)
	}

	mutated := policy
	mutated.PointsPrompt = "A later profile prompt must not be used."
	mutated.PointsTiers = []apitypes.RuntimeProfileWorkspaceRewardPointsTier{{MinScore: 80, Delta: 999}}
	mutatedDigest, err := workspaceRewardPolicyDigest(mutated)
	if err != nil {
		t.Fatalf("workspaceRewardPolicyDigest(mutated) error = %v", err)
	}
	mutated.Digest = mutatedDigest
	environment.policy = &mutated
	now = now.Add(2 * time.Second)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("second dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	if generator.invokeCount != 2 || len(generator.contexts) != 2 ||
		generator.contexts[0] != generator.contexts[1] ||
		strings.Contains(generator.contexts[1], mutated.PointsPrompt) {
		t.Fatalf("retry contexts = %#v", generator.contexts)
	}
	var pointsDelta int64
	if err := runtime.DB.QueryRowContext(ctx, `SELECT points_delta FROM gameplay_reward_grants
		WHERE source_id = ?`, "window-retry").Scan(&pointsDelta); err != nil {
		t.Fatalf("read retry RewardGrant: %v", err)
	}
	if pointsDelta != 10 {
		t.Fatalf("retry points delta = %d, want frozen 10", pointsDelta)
	}
}

func TestWorkspaceRewardConcurrentSQLiteSettlementHonorsBudget(t *testing.T) {
	testConcurrentWorkspaceRewardSettlement(t, testDB(t))
}

func testConcurrentWorkspaceRewardSettlement(t *testing.T, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)
	catalog := testCatalog(t, now)
	rewardPrompt := "Reward scientific reasoning."
	response, err := catalog.CreateBadgeDef(ctx, adminhttp.CreateBadgeDefRequestObject{
		Body: &adminhttp.BadgeDefUpsert{Id: "badge-science", Spec: apitypes.BadgeDefSpec{
			DisplayName: "Science", RewardPrompt: &rewardPrompt,
		}},
	})
	if err != nil {
		t.Fatalf("CreateBadgeDef() error = %v", err)
	}
	requireResponse[adminhttp.CreateBadgeDef200JSONResponse](t, response)
	runtimes := []*Runtime{
		{DB: db, Catalog: catalog, Now: func() time.Time { return now }, NewID: sequentialIDs("grant-a", "points-a")},
		{DB: db, Catalog: catalog, Now: func() time.Time { return now }, NewID: sequentialIDs("grant-b", "points-b")},
	}
	if err := runtimes[0].Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	initialBalance := int64(0)
	if _, err := runtimes[0].GetPoints(
		WithRuntimeProfile(ctx, apitypes.RuntimeProfile{
			Id: "runtime-profile-a",
			Spec: apitypes.RuntimeProfileSpec{Gameplay: &apitypes.RuntimeProfileGameplaySpec{
				Points: &apitypes.RuntimeProfilePointsSpec{InitialBalance: &initialBalance},
			}},
		}),
		"peer-a",
		"runtime-profile-a",
	); err != nil {
		t.Fatalf("GetPoints() error = %v", err)
	}
	policy := workspaceRewardTestPolicy(t)
	windows := make([]workspaceRewardWindow, 2)
	for i := range windows {
		suffix := string(rune('a' + i))
		source := workspaceRewardSource{
			WorkspaceID: "workflow-" + suffix, ScheduledCheckpoint: "002",
			CreatedAt: now, UpdatedAt: now,
		}
		if err := runtimes[i].insertWorkspaceRewardSource(ctx, source); err != nil {
			t.Fatalf("insert source %s: %v", suffix, err)
		}
		window := workspaceRewardWindow{
			ID: "window-" + suffix, WorkspaceID: source.WorkspaceID,
			WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-a",
			RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
			Policy: policy, PolicyDigest: policy.Digest, StartHistoryID: "001", HighWaterHistoryID: "002",
			StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
			EvaluateAfter: now, State: workspaceRewardClaimed, AttemptCount: 1,
			ClaimToken: "claim-" + suffix, ClaimUntil: now.Add(time.Minute),
			CreatedAt: now, UpdatedAt: now,
		}
		source.ScheduledCheckpoint = window.HighWaterHistoryID
		if err := runtimes[i].insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
			t.Fatalf("insert window %s: %v", suffix, err)
		}
		windows[i] = window
	}
	evaluation := workspaceRewardEvaluation{
		Score: 90, Reason: "Qualified.",
		Badges: []workspaceRewardBadgeRecommendation{{Alias: "science", Exp: 5}},
	}
	type result struct {
		changed bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, len(runtimes))
	var wg sync.WaitGroup
	for i := range runtimes {
		wg.Go(func() {
			<-start
			_, changed, err := runtimes[i].settleWorkspaceReward(ctx, windows[i], "transcript", evaluation)
			results <- result{changed: changed, err: err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	changedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("settleWorkspaceReward() error = %v", result.err)
		}
		if result.changed {
			changedCount++
		}
	}
	var grants, completed, checkpoints int
	if err := db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gameplay_reward_grants),
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE state = 'completed'),
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE completed_checkpoint = '002')`,
	).Scan(&grants, &completed, &checkpoints); err != nil {
		t.Fatalf("count settlement rows: %v", err)
	}
	var balance, badgeExp int64
	if err := db.QueryRowContext(ctx, `SELECT balance FROM gameplay_points_accounts
		WHERE owner_public_key = 'peer-a' AND runtime_profile_id = 'runtime-profile-a'`).Scan(&balance); err != nil {
		t.Fatalf("read Points balance: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT exp FROM gameplay_badges
		WHERE owner_public_key = 'peer-a' AND id = 'badge-science'`).Scan(&badgeExp); err != nil {
		t.Fatalf("read Badge EXP: %v", err)
	}
	if changedCount != 1 || grants != 1 || completed != 2 || checkpoints != 2 ||
		balance != 10 || badgeExp != 5 {
		t.Fatalf(
			"settlement changed=%d grants=%d completed=%d checkpoints=%d balance=%d badge_exp=%d",
			changedCount, grants, completed, checkpoints, balance, badgeExp,
		)
	}
}

func workspaceRewardTestPolicy(t *testing.T) WorkspaceRewardPolicySnapshot {
	t.Helper()
	policy := WorkspaceRewardPolicySnapshot{
		RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
		ModelAlias: "reward-evaluator", ModelResourceID: "model-reward",
		WorkspaceKinds: []WorkspaceRewardKind{WorkspaceRewardKindWorkflow},
		QuietPeriod:    time.Minute, MaxWindowAge: 10 * time.Minute,
		MaxEntries: 20, MaxTextBytes: 4096,
		PointsPrompt: "Reward scientific curiosity.",
		ScoreMin:     0, ScoreMax: 100, QualifyingScore: 80,
		PointsTiers: []apitypes.RuntimeProfileWorkspaceRewardPointsTier{{MinScore: 80, Delta: 10}},
		Badges: []WorkspaceRewardBadgePolicy{{
			Alias: "science", ResourceID: "badge-science", DisplayName: "Science",
			RewardPrompt: "Award for sound scientific reasoning.", MaxExpPerWindow: 5,
		}},
		BudgetPeriod: 24 * time.Hour, PointsMax: 10, BadgeExpMax: 5,
	}
	digest, err := workspaceRewardPolicyDigest(policy)
	if err != nil {
		t.Fatalf("workspaceRewardPolicyDigest() error = %v", err)
	}
	policy.Digest = digest
	return policy
}
