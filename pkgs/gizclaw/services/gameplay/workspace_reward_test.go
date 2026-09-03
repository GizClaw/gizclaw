package gameplay

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/jmoiron/sqlx"
)

type workspaceRewardTestGenerator struct {
	result       string
	results      []string
	err          error
	errs         []error
	beforeInvoke func()
	invokeCount  int
	pattern      string
	context      string
	contexts     []string
	tool         *genx.FuncTool
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
	if g.beforeInvoke != nil {
		g.beforeInvoke()
	}
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
	mu               sync.Mutex
	entries          map[string][]workspace.HistoryEntry
	policy           *WorkspaceRewardPolicySnapshot
	generator        genx.Generator
	notifications    []WorkspaceRewardUpdate
	availability     map[string]error
	availabilityFunc func(context.Context, string) error
	historyCalls     map[string]int
	listCalls        int
}

type workspaceRewardEnvironmentWithoutAvailability struct {
	WorkspaceRewardEnvironment
}

func TestWorkspaceRewardAvailabilityFailsClosedForCompatibleEnvironment(t *testing.T) {
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: workspaceRewardEnvironmentWithoutAvailability{}}
	err := runtime.checkWorkspaceRewardAvailability(t.Context(), "workspace-1")
	if !errors.Is(err, errWorkspaceRewardAvailability) {
		t.Fatalf("checkWorkspaceRewardAvailability() error = %v, want %v", err, errWorkspaceRewardAvailability)
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(t.Context()); !errors.Is(err, errWorkspaceRewardAvailability) {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want %v", err, errWorkspaceRewardAvailability)
	}
}

func (e *workspaceRewardTestEnvironment) EnsureWorkspaceAvailable(ctx context.Context, workspaceID string) error {
	if e.availabilityFunc != nil {
		return e.availabilityFunc(ctx, workspaceID)
	}
	if e.availability == nil {
		return nil
	}
	return e.availability[workspaceID]
}

func (e *workspaceRewardTestEnvironment) ListWorkspaceIDs(context.Context) ([]string, error) {
	e.mu.Lock()
	e.listCalls++
	e.mu.Unlock()
	names := make([]string, 0, len(e.entries))
	for name := range e.entries {
		names = append(names, name)
	}
	return names, nil
}

func (e *workspaceRewardTestEnvironment) workspaceReadCounts() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	historyCalls := 0
	for _, calls := range e.historyCalls {
		historyCalls += calls
	}
	return e.listCalls, historyCalls
}

func (e *workspaceRewardTestEnvironment) LatestHistoryEntry(_ context.Context, name string) (workspace.HistoryEntry, bool, error) {
	e.recordHistoryCall(name)
	entries := e.entries[name]
	if len(entries) == 0 {
		return workspace.HistoryEntry{}, false, nil
	}
	return entries[len(entries)-1], true, nil
}

func (e *workspaceRewardTestEnvironment) LatestHistoryEntryBefore(
	_ context.Context,
	name string,
	before time.Time,
) (workspace.HistoryEntry, bool, error) {
	e.recordHistoryCall(name)
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
	e.recordHistoryCall(name)
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
	e.recordHistoryCall(name)
	for _, entry := range e.entries[name] {
		if entry.ID == id {
			return entry, nil
		}
	}
	return workspace.HistoryEntry{}, fmt.Errorf("History entry %q not found", id)
}

func (e *workspaceRewardTestEnvironment) recordHistoryCall(workspaceID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.historyCalls == nil {
		e.historyCalls = make(map[string]int)
	}
	e.historyCalls[workspaceID]++
}

func (e *workspaceRewardTestEnvironment) historyCallCount(workspaceID string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.historyCalls[workspaceID]
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
		WorkspaceRewardKind("group_chatroom"),
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

func TestWorkspaceRewardPendingWorkspaceRetiresWorkAndActiveNeighborContinues(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	generator := &workspaceRewardTestGenerator{result: `{
		"score": 90,
		"reason": "Qualified active conversation.",
		"badges": []
	}`}
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{
			"workspace-pending": nil,
			"workspace-active":  nil,
		},
		policy: &policy, generator: generator,
		availability: make(map[string]error),
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return now },
		NewID: sequentialIDs(
			"window-pending", "claim-pending",
			"window-active", "claim-active", "grant-active", "points-active",
		),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(workspaceID, id, entryType, gearID, text string) workspace.HistoryEntry {
		entry := workspace.HistoryEntry{
			ID: id, Type: entryType, GearID: gearID, Origin: workspace.HistoryOriginAgentHost,
			Text: text, CreatedAt: now,
		}
		environment.entries[workspaceID] = append(environment.entries[workspaceID], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, workspaceID, entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s, %s) error = %v", workspaceID, id, err)
		}
		return entry
	}
	pendingGear := appendEntry("workspace-pending", "001", "gear", "peer-pending", "pending question")
	now = now.Add(time.Second)
	appendEntry("workspace-pending", "002", "agent", "", "pending answer")
	now = now.Add(2 * time.Minute)
	environment.availability["workspace-pending"] = workspace.ErrWorkspacePendingDeletion
	historyCallsBefore := environment.historyCallCount("workspace-pending")
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatch pending Workspace reward = %v, %v", processed, err)
	}
	if got := environment.historyCallCount("workspace-pending"); got != historyCallsBefore {
		t.Fatalf("pending Workspace History calls changed from %d to %d", historyCallsBefore, got)
	}
	if generator.invokeCount != 0 {
		t.Fatalf("pending Workspace evaluator calls = %d, want 0", generator.invokeCount)
	}
	var pendingRows int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE workspace_id = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?)`,
		"workspace-pending", "workspace-pending").Scan(&pendingRows); err != nil {
		t.Fatal(err)
	}
	if pendingRows != 0 {
		t.Fatalf("pending Workspace reward rows = %d, want 0", pendingRows)
	}
	if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workspace-pending", pendingGear); err != nil {
		t.Fatalf("queued pending activity error = %v", err)
	}
	if got := environment.historyCallCount("workspace-pending"); got != historyCallsBefore {
		t.Fatalf("queued pending activity made History calls: before=%d after=%d", historyCallsBefore, got)
	}
	restarted := &Runtime{
		DB: runtime.DB, WorkspaceRewards: environment, Now: runtime.Now,
		NewID: sequentialIDs("restart-claim"),
	}
	if processed, err := restarted.dispatchWorkspaceReward(ctx); err != nil || processed {
		t.Fatalf("restart dispatch = %v, %v; want no recreated pending work", processed, err)
	}

	now = now.Add(time.Second)
	appendEntry("workspace-active", "001", "gear", "peer-active", "active question")
	now = now.Add(time.Second)
	appendEntry("workspace-active", "002", "agent", "", "active answer")
	now = now.Add(2 * time.Minute)
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatch active Workspace reward = %v, %v", processed, err)
	}
	if generator.invokeCount != 1 {
		t.Fatalf("active Workspace evaluator calls = %d, want 1", generator.invokeCount)
	}
	now = now.Add(time.Second)
	appendEntry("workspace-active", "003", "gear", "peer-active", "second question")
	now = now.Add(time.Second)
	appendEntry("workspace-active", "004", "agent", "", "second answer")
	now = now.Add(2 * time.Minute)
	environment.availability["workspace-active"] = workspace.ErrWorkspacePendingDeletion
	if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
		t.Fatalf("dispatch active Workspace after PendingDeletion = %v, %v", processed, err)
	}
	var completed, unsettled, sources int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		COUNT(*) FILTER (WHERE state = ?),
		COUNT(*) FILTER (WHERE state <> ?),
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?)
		FROM gameplay_workspace_reward_windows WHERE workspace_id = ?`,
		workspaceRewardCompleted, workspaceRewardCompleted,
		"workspace-active", "workspace-active").Scan(&completed, &unsettled, &sources); err != nil {
		t.Fatal(err)
	}
	if completed != 1 || unsettled != 0 || sources != 0 {
		t.Fatalf("terminal Workspace completed/unsettled/sources = %d/%d/%d, want 1/0/0", completed, unsettled, sources)
	}
	if generator.invokeCount != 1 {
		t.Fatalf("terminal Workspace evaluator calls = %d, want completed-call count 1", generator.invokeCount)
	}
}

func TestWorkspaceRewardCancellationRechecksPendingDeletionBeforeRetry(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 3, 30, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	dispatchCtx, cancelDispatch := context.WithCancel(ctx)
	terminal := false
	generator := &workspaceRewardTestGenerator{
		result: `{
			"score": 90,
			"reason": "Qualified active conversation.",
			"badges": []
		}`,
		beforeInvoke: func() {
			terminal = true
			cancelDispatch()
		},
	}
	environment := &workspaceRewardTestEnvironment{
		entries:   map[string][]workspace.HistoryEntry{"workspace-race": nil},
		policy:    &policy,
		generator: generator,
		availabilityFunc: func(ctx context.Context, workspaceID string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if workspaceID == "workspace-race" && terminal {
				return workspace.ErrWorkspacePendingDeletion
			}
			return nil
		},
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now:   func() time.Time { return now },
		NewID: sequentialIDs("window-race", "claim-race"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	appendEntry := func(id, entryType, gearID, text string) {
		entry := workspace.HistoryEntry{
			ID: id, Type: entryType, GearID: gearID, Origin: workspace.HistoryOriginAgentHost,
			Text: text, CreatedAt: now,
		}
		environment.entries["workspace-race"] = append(environment.entries["workspace-race"], entry)
		if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workspace-race", entry); err != nil {
			t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", id, err)
		}
	}
	appendEntry("001", "gear", "peer-race", "question")
	now = now.Add(time.Second)
	appendEntry("002", "agent", "", "answer")
	now = now.Add(2 * time.Minute)

	processed, err := runtime.dispatchWorkspaceReward(dispatchCtx)
	if err != nil || !processed {
		t.Fatalf("dispatchWorkspaceReward() = %v, %v", processed, err)
	}
	if generator.invokeCount != 1 {
		t.Fatalf("evaluator calls = %d, want 1", generator.invokeCount)
	}
	var rows int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE workspace_id = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?)`,
		"workspace-race", "workspace-race").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("terminal Workspace reward rows = %d, want 0", rows)
	}
}

func TestWorkspaceRewardActivationIsolatesPendingWorkspace(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 4, 0, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{
			"workspace-pending": {{ID: "001", CreatedAt: now.Add(-time.Minute)}},
			"workspace-active":  {{ID: "001", CreatedAt: now.Add(-time.Minute)}},
		},
		availability: map[string]error{"workspace-pending": workspace.ErrWorkspacePendingDeletion},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ActivateWorkspaceReward(ctx, "workspace-pending"); err != nil {
		t.Fatalf("ActivateWorkspaceReward(pending) error = %v", err)
	}
	if got := environment.historyCallCount("workspace-pending"); got != 0 {
		t.Fatalf("pending Workspace History calls = %d, want 0", got)
	}
	if got := environment.historyCallCount("workspace-active"); got != 0 {
		t.Fatalf("unrelated active Workspace History calls = %d, want 0", got)
	}
	if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-pending"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("pending Workspace source error = %v, want no row", err)
	}
	if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-active"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unactivated Workspace source error = %v, want no row", err)
	}
	if err := runtime.ActivateWorkspaceReward(ctx, "workspace-active"); err != nil {
		t.Fatalf("ActivateWorkspaceReward(active) error = %v", err)
	}
	if source, err := runtime.getWorkspaceRewardSource(ctx, "workspace-active"); err != nil || source.CompletedCheckpoint != "001" {
		t.Fatalf("active Workspace source = %#v, %v", source, err)
	}
}

func TestWorkspaceRewardActivationRetiresPhysicallyDeletedWorkspaceIdempotently(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{
			"workspace-deleted": {{ID: "001", CreatedAt: now.Add(-time.Minute)}},
		},
		availability: map[string]error{"workspace-deleted": workspace.ErrWorkspaceDeleted},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_workspace_reward_sources
		(workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
		VALUES (?, '', '', ?, ?)`, "workspace-deleted", formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		if err := runtime.ActivateWorkspaceReward(ctx, "workspace-deleted"); err != nil {
			t.Fatalf("ActivateWorkspaceReward(deleted) attempt %d error = %v", attempt+1, err)
		}
	}
	if got := environment.historyCallCount("workspace-deleted"); got != 0 {
		t.Fatalf("deleted Workspace History calls = %d, want 0", got)
	}
	var rows int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM gameplay_workspace_reward_windows WHERE workspace_id = ?) +
		(SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?)`,
		"workspace-deleted", "workspace-deleted").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("deleted Workspace unsettled reward rows = %d, want 0", rows)
	}
}

func TestWorkspaceRewardTerminalLifecycleStatesRetireUnsettledData(t *testing.T) {
	for _, lifecycleErr := range []error{
		workspace.ErrWorkspacePendingDeletion,
		workspace.ErrWorkspaceDeleted,
		workspace.ErrPeerPendingDeletion,
		workspace.ErrPeerDeleted,
	} {
		t.Run(lifecycleErr.Error(), func(t *testing.T) {
			ctx := t.Context()
			now := time.Date(2026, 8, 13, 1, 15, 0, 0, time.UTC)
			environment := &workspaceRewardTestEnvironment{
				entries:      map[string][]workspace.HistoryEntry{"workspace-terminal": {{ID: "001", CreatedAt: now}}},
				availability: map[string]error{"workspace-terminal": lifecycleErr},
			}
			runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
			if err := runtime.Migration(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_workspace_reward_sources
				(workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
				VALUES (?, '', '', ?, ?)`, "workspace-terminal", formatTime(now), formatTime(now)); err != nil {
				t.Fatal(err)
			}
			if err := runtime.ActivateWorkspaceReward(ctx, "workspace-terminal"); err != nil {
				t.Fatalf("ActivateWorkspaceReward() error = %v", err)
			}
			if _, err := runtime.getWorkspaceRewardSource(ctx, "workspace-terminal"); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("terminal Workspace source error = %v, want no row", err)
			}
			if got := environment.historyCallCount("workspace-terminal"); got != 0 {
				t.Fatalf("terminal Workspace History calls = %d, want 0", got)
			}
		})
	}
}

func TestWorkspaceRewardDispatcherConsumesQueuedDeletedActivationsIdempotently(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 13, 1, 30, 0, 0, time.UTC)
	availabilityCalls := make(chan struct{}, 3)
	releaseFirst := make(chan struct{})
	var firstCall atomic.Bool
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{
			"workspace-queued-deleted": {{ID: "001", CreatedAt: now.Add(-time.Minute)}},
		},
		availabilityFunc: func(context.Context, string) error {
			availabilityCalls <- struct{}{}
			if firstCall.CompareAndSwap(false, true) {
				<-releaseFirst
			}
			return workspace.ErrWorkspaceDeleted
		},
	}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DB.ExecContext(ctx, `INSERT INTO gameplay_workspace_reward_sources
		(workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at)
		VALUES (?, '', '', ?, ?)`, "workspace-queued-deleted", formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	logs := &lockedLogBuffer{}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop()
		<-done
	})
	if err := runtime.EnqueueWorkspaceRewardActivation(ctx, "workspace-queued-deleted"); err != nil {
		t.Fatal(err)
	}
	<-availabilityCalls
	close(releaseFirst)
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := runtime.getWorkspaceRewardSource(ctx, "workspace-queued-deleted")
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("queued deleted Workspace source was not retired")
		}
		time.Sleep(time.Millisecond)
	}
	for range 2 {
		if err := runtime.EnqueueWorkspaceRewardActivation(ctx, "workspace-queued-deleted"); err != nil {
			t.Fatal(err)
		}
	}
	<-availabilityCalls
	<-availabilityCalls
	stop()
	<-done
	if got := environment.historyCallCount("workspace-queued-deleted"); got != 0 {
		t.Fatalf("deleted Workspace History calls = %d, want 0", got)
	}
	if output := logs.String(); strings.Contains(output, "activate Workspace reward") {
		t.Fatalf("stale activation emitted an operational error: %s", output)
	}
}

type lockedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedLogBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestWorkspaceRewardActivationLazilyBaselinesAndRecoversExactWorkspace(t *testing.T) {
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
	if err := runtime.ActivateWorkspaceReward(ctx, "old"); err != nil {
		t.Fatalf("ActivateWorkspaceReward(old) error = %v", err)
	}
	oldSource, err := runtime.getWorkspaceRewardSource(ctx, "old")
	if err != nil {
		t.Fatalf("get old source: %v", err)
	}
	if oldSource.ScheduledCheckpoint != "001" || oldSource.CompletedCheckpoint != "001" {
		t.Fatalf("old source = %#v, want startup baseline", oldSource)
	}
	if _, err := runtime.getWorkspaceRewardSource(ctx, "new"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unactivated new source error = %v, want no row", err)
	}
	if got := environment.historyCallCount("new"); got != 0 {
		t.Fatalf("unactivated new Workspace History calls = %d, want 0", got)
	}
	if err := runtime.ActivateWorkspaceReward(ctx, "new"); err != nil {
		t.Fatalf("ActivateWorkspaceReward(new) error = %v", err)
	}
	newSource, err := runtime.getWorkspaceRewardSource(ctx, "new")
	if err != nil {
		t.Fatalf("get activated new source: %v", err)
	}
	if newSource.ScheduledCheckpoint != "002" || newSource.CompletedCheckpoint != "" {
		t.Fatalf("new source = %#v, want recovered pending History", newSource)
	}
	window, err := runtime.activeWorkspaceRewardWindow(ctx, "new")
	if err != nil || window.StartHistoryID != "002" || window.BeneficiaryPublicKey != "peer-new" {
		t.Fatalf("new active window = %#v, %v", window, err)
	}
}

func TestWorkspaceRewardRestartWaitsForExactActivationToRecoverDroppedNotification(t *testing.T) {
	ctx := t.Context()
	activation := time.Date(2026, 7, 29, 2, 30, 0, 0, time.UTC)
	clock := activation
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": nil},
		policy:  &policy,
	}
	db := testDB(t)
	first := &Runtime{DB: db, WorkspaceRewards: environment, Now: func() time.Time { return clock }}
	stop, done, err := first.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("first StartWorkspaceRewardDispatcher() error = %v", err)
	}
	stop()
	<-done

	entry := workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "persisted without a reward notification", CreatedAt: activation.Add(time.Minute),
	}
	environment.entries["workflow-a"] = append(environment.entries["workflow-a"], entry)
	clock = activation.Add(2 * time.Minute)
	restarted := &Runtime{
		DB: db, WorkspaceRewards: environment, Now: func() time.Time { return clock },
		NewID: sequentialIDs("window-recovered"),
	}
	stop, done, err = restarted.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("restarted StartWorkspaceRewardDispatcher() error = %v", err)
	}
	t.Cleanup(func() {
		stop()
		<-done
	})
	if _, err := restarted.getWorkspaceRewardSource(ctx, "workflow-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cold Workspace source before activation error = %v, want no row", err)
	}
	if got := environment.historyCallCount("workflow-a"); got != 0 {
		t.Fatalf("cold Workspace History calls before activation = %d, want 0", got)
	}

	if err := restarted.ActivateWorkspaceReward(ctx, "workflow-a"); err != nil {
		t.Fatalf("ActivateWorkspaceReward() error = %v", err)
	}
	source, err := restarted.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil || source.ScheduledCheckpoint != entry.ID {
		t.Fatalf("recovered source = %#v, %v", source, err)
	}
	window, err := restarted.activeWorkspaceRewardWindow(ctx, "workflow-a")
	if err != nil || window.StartHistoryID != entry.ID {
		t.Fatalf("recovered window = %#v, %v", window, err)
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

func TestWorkspaceRewardDispatchBlocksCorruptPolicyAndContinues(t *testing.T) {
	for name, corrupt := range map[string]func(*testing.T, *Runtime, string){
		"malformed json": func(t *testing.T, runtime *Runtime, windowID string) {
			t.Helper()
			if _, err := runtime.DB.ExecContext(t.Context(), `UPDATE gameplay_workspace_reward_windows SET policy_json = '{' WHERE id = ?`, windowID); err != nil {
				t.Fatalf("corrupt policy JSON: %v", err)
			}
		},
		"digest mismatch": func(t *testing.T, runtime *Runtime, windowID string) {
			t.Helper()
			if _, err := runtime.DB.ExecContext(t.Context(), `UPDATE gameplay_workspace_reward_windows SET policy_digest = 'mismatch' WHERE id = ?`, windowID); err != nil {
				t.Fatalf("corrupt policy digest: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
			clock := now.Add(-3 * time.Hour)
			policy := workspaceRewardTestPolicy(t)
			generator := &workspaceRewardTestGenerator{result: `{"score":90,"reason":"Qualified.","badges":[]}`}
			environment := &workspaceRewardTestEnvironment{
				entries: map[string][]workspace.HistoryEntry{"workspace-healthy": nil, "workspace-corrupt": nil},
				policy:  &policy, generator: generator,
			}
			runtime := &Runtime{
				DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return clock },
				NewID: sequentialIDs("window-healthy", "claim-corrupt", "claim-healthy", "grant-healthy", "points-healthy"),
			}
			if err := runtime.Migration(ctx); err != nil {
				t.Fatalf("Migration() error = %v", err)
			}
			if _, err := runtime.ensureWorkspaceRewardActivation(ctx); err != nil {
				t.Fatalf("ensureWorkspaceRewardActivation() error = %v", err)
			}
			clock = now
			for _, entry := range []workspace.HistoryEntry{
				{ID: "001", Type: "gear", GearID: "peer-healthy", Origin: workspace.HistoryOriginAgentHost, Text: "question", CreatedAt: now.Add(-time.Hour)},
				{ID: "002", Type: "agent", Origin: workspace.HistoryOriginAgentHost, Text: "answer", CreatedAt: now.Add(-time.Hour + time.Second)},
			} {
				environment.entries["workspace-healthy"] = append(environment.entries["workspace-healthy"], entry)
				if err := runtime.ScheduleWorkspaceRewardActivity(ctx, "workspace-healthy", entry); err != nil {
					t.Fatalf("ScheduleWorkspaceRewardActivity(%s) error = %v", entry.ID, err)
				}
			}
			corruptSource := workspaceRewardSource{
				WorkspaceID: "workspace-corrupt", ScheduledCheckpoint: "history-corrupt",
				CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
			}
			if err := runtime.insertWorkspaceRewardSource(ctx, corruptSource); err != nil {
				t.Fatalf("insert corrupt source: %v", err)
			}
			corruptWindow := workspaceRewardWindow{
				ID: "window-corrupt", WorkspaceID: corruptSource.WorkspaceID,
				WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-corrupt",
				RuntimeProfileId: policy.RuntimeProfileId, RuntimeProfileRevision: policy.RuntimeProfileRevision,
				Policy: policy, PolicyDigest: policy.Digest,
				StartHistoryID: "history-corrupt", HighWaterHistoryID: "history-corrupt",
				StartHistoryAt: now.Add(-2 * time.Hour), HighWaterHistoryAt: now.Add(-2 * time.Hour),
				OpenedAt: now.Add(-2 * time.Hour), LastActivityAt: now.Add(-2 * time.Hour),
				EvaluateAfter: now.Add(-90 * time.Minute), State: workspaceRewardPending,
				CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
			}
			if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, corruptWindow, corruptSource); err != nil {
				t.Fatalf("insert corrupt window: %v", err)
			}
			corrupt(t, runtime, corruptWindow.ID)

			if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
				t.Fatalf("dispatch corrupt Workspace reward = %v, %v", processed, err)
			}
			var state, lastError string
			if err := runtime.DB.QueryRowContext(ctx, `SELECT state, last_error FROM gameplay_workspace_reward_windows WHERE id = ?`, corruptWindow.ID).Scan(&state, &lastError); err != nil {
				t.Fatalf("read corrupt window: %v", err)
			}
			wantError := "reward_policy_invalid"
			if name == "digest mismatch" {
				wantError = "reward_policy_digest_mismatch"
			}
			if state != workspaceRewardBlocked || lastError != wantError {
				t.Fatalf("corrupt window state/error = %q/%q, want %q/%q", state, lastError, workspaceRewardBlocked, wantError)
			}
			if processed, err := runtime.dispatchWorkspaceReward(ctx); err != nil || !processed {
				t.Fatalf("dispatch healthy Workspace reward = %v, %v", processed, err)
			}
			if generator.invokeCount != 1 {
				t.Fatalf("healthy evaluator invokes = %d, want 1", generator.invokeCount)
			}
		})
	}
}

func TestWorkspaceRewardCorruptPolicyBlockingHonorsClaimFence(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 9, 15, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	runtime := &Runtime{DB: testDB(t), Now: func() time.Time { return now }, NewID: sequentialIDs("claim-observed")}
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
	if _, err := runtime.DB.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows
		SET policy_json = '{' WHERE id = ?`, window.ID); err != nil {
		t.Fatalf("corrupt policy JSON: %v", err)
	}

	_, _, err := runtime.claimWorkspaceRewardWindow(ctx)
	var corrupt *workspaceRewardPolicyCorruptionError
	if !errors.As(err, &corrupt) {
		t.Fatalf("claimWorkspaceRewardWindow() error = %v, want policy corruption", err)
	}
	if corrupt.State != workspaceRewardClaimed || corrupt.ClaimToken != "claim-observed" {
		t.Fatalf("observed corruption fence = %q/%q", corrupt.State, corrupt.ClaimToken)
	}
	if _, err := runtime.DB.ExecContext(ctx, `UPDATE gameplay_workspace_reward_windows
		SET claim_token = 'claim-renewed' WHERE id = ?`, window.ID); err != nil {
		t.Fatalf("simulate renewed claim: %v", err)
	}
	if err := runtime.blockCorruptWorkspaceRewardWindow(ctx, corrupt); err == nil {
		t.Fatal("stale corrupt-row observer blocked a renewed claim")
	}
	var state, claimToken string
	if err := runtime.DB.QueryRowContext(ctx, `SELECT state, claim_token
		FROM gameplay_workspace_reward_windows WHERE id = ?`, window.ID).Scan(&state, &claimToken); err != nil {
		t.Fatalf("read renewed row: %v", err)
	}
	if state != workspaceRewardClaimed || claimToken != "claim-renewed" {
		t.Fatalf("renewed row = %q/%q, want claimed/claim-renewed", state, claimToken)
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

func TestWorkspaceRewardActivationQueueAppliesBackpressure(t *testing.T) {
	runtime := &Runtime{WorkspaceRewards: &workspaceRewardTestEnvironment{}}
	for range workspaceRewardActivityCapacity {
		if err := runtime.EnqueueWorkspaceRewardActivation(t.Context(), "workflow-a"); err != nil {
			t.Fatalf("EnqueueWorkspaceRewardActivation() error = %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := runtime.EnqueueWorkspaceRewardActivation(ctx, "workflow-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("full activation queue error = %v, want context cancellation", err)
	}
	if got := len(runtime.workspaceRewardActivationChannel()); got != workspaceRewardActivityCapacity {
		t.Fatalf("queued activations = %d, want %d", got, workspaceRewardActivityCapacity)
	}
}

func TestWorkspaceRewardDispatcherStartsWithoutReadingWorkspaces(t *testing.T) {
	ctx := context.Background()
	entries := make(map[string][]workspace.HistoryEntry, 1000)
	for index := range 1000 {
		entries[fmt.Sprintf("cold-workspace-%04d", index)] = []workspace.HistoryEntry{{
			ID: "history-cold", Type: "gear", GearID: "peer-cold",
			Origin: workspace.HistoryOriginAgentHost, CreatedAt: time.Unix(1, 0),
		}}
	}
	environment := &workspaceRewardTestEnvironment{entries: entries}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment}
	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
	}
	t.Cleanup(func() {
		stop()
		<-done
	})

	// Let the normal due-work poller run without relying on a timing assertion.
	time.Sleep(2 * workspaceRewardPollInterval)
	listCalls, historyCalls := environment.workspaceReadCounts()
	if listCalls != 0 || historyCalls != 0 {
		t.Fatalf("dispatcher startup Workspace reads = list:%d history:%d, want 0/0", listCalls, historyCalls)
	}
}

func TestWorkspaceRewardDispatcherRejectsInvalidActivationBoundary(t *testing.T) {
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: &workspaceRewardTestEnvironment{}}
	if err := runtime.Migration(t.Context()); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	if _, err := runtime.DB.ExecContext(t.Context(), `INSERT INTO gameplay_workspace_reward_activation
		(singleton, activated_at) VALUES (1, 'invalid')`); err != nil {
		t.Fatalf("seed invalid activation boundary: %v", err)
	}
	if _, _, err := runtime.StartWorkspaceRewardDispatcher(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "invalid workspace reward activation boundary") {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v, want invalid activation boundary", err)
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

func TestWorkspaceRewardDispatcherReconcilesQueuedActivation(t *testing.T) {
	ctx := t.Context()
	activation := time.Date(2026, 7, 29, 5, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": {
			{
				ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
				Text: "before activation", CreatedAt: activation.Add(-time.Minute),
			},
			{
				ID: "002", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
				Text: "after activation", CreatedAt: activation.Add(time.Minute),
			},
		}},
		policy: &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return activation }, NewID: sequentialIDs("window-activated"),
	}
	stop, done, err := runtime.StartWorkspaceRewardDispatcher(ctx)
	if err != nil {
		t.Fatalf("StartWorkspaceRewardDispatcher() error = %v", err)
	}
	t.Cleanup(func() {
		stop()
		<-done
	})
	if err := runtime.EnqueueWorkspaceRewardActivation(ctx, "workflow-a"); err != nil {
		t.Fatalf("EnqueueWorkspaceRewardActivation() error = %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		source, sourceErr := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
		if sourceErr == nil && source.ScheduledCheckpoint == "002" {
			if source.CompletedCheckpoint != "001" {
				t.Fatalf("activated source = %#v, want pre-activation checkpoint 001", source)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("queued activation was not reconciled: source=%#v error=%v", source, sourceErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkspaceRewardConcurrentActivationAndActivityAreIdempotent(t *testing.T) {
	ctx := t.Context()
	activation := time.Date(2026, 7, 29, 5, 30, 0, 0, time.UTC)
	entry := workspace.HistoryEntry{
		ID: "001", Type: "gear", GearID: "peer-a", Origin: workspace.HistoryOriginAgentHost,
		Text: "new conversation", CreatedAt: activation.Add(time.Minute),
	}
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		entries: map[string][]workspace.HistoryEntry{"workflow-a": {entry}}, policy: &policy,
	}
	runtime := &Runtime{
		DB: testDB(t), WorkspaceRewards: environment,
		Now: func() time.Time { return activation }, NewID: sequentialIDs("window-concurrent"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 32)
	var wg sync.WaitGroup
	for index := range 32 {
		wg.Go(func() {
			<-start
			if index%2 == 0 {
				errs <- runtime.ActivateWorkspaceReward(ctx, "workflow-a")
				return
			}
			errs <- runtime.ScheduleWorkspaceRewardActivity(ctx, "workflow-a", entry)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent activation/activity error = %v", err)
		}
	}

	source, err := runtime.getWorkspaceRewardSource(ctx, "workflow-a")
	if err != nil || source.ScheduledCheckpoint != entry.ID {
		t.Fatalf("source = %#v, %v; want checkpoint %q", source, err, entry.ID)
	}
	var windows int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_windows
		WHERE workspace_id = ? AND state IN (?, ?, ?)`, "workflow-a",
		workspaceRewardPending, workspaceRewardClaimed, workspaceRewardRetry).Scan(&windows); err != nil {
		t.Fatalf("count active windows: %v", err)
	}
	if windows != 1 {
		t.Fatalf("active windows = %d, want 1", windows)
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

func TestWorkspaceRewardSettlementRejectsDeletionAfterFinalAvailabilityCheck(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 13, 2, 0, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	environment := &workspaceRewardTestEnvironment{
		availability: map[string]error{"workflow-deleting": workspace.ErrWorkspacePendingDeletion},
	}
	runtime := &Runtime{
		DB:               testDB(t),
		WorkspaceRewards: environment,
		Now:              func() time.Time { return now },
		NewID:            sequentialIDs("grant-deleting", "points-deleting"),
	}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	source := workspaceRewardSource{
		WorkspaceID: "workflow-deleting", ScheduledCheckpoint: "002",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	window := workspaceRewardWindow{
		ID: "window-deleting", WorkspaceID: source.WorkspaceID,
		WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: "peer-a",
		RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
		Policy: policy, PolicyDigest: policy.Digest, StartHistoryID: "001", HighWaterHistoryID: "002",
		StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
		EvaluateAfter: now, State: workspaceRewardClaimed, AttemptCount: 1,
		ClaimToken: "claim-deleting", ClaimUntil: now.Add(time.Minute),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := runtime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
		t.Fatal(err)
	}

	_, changed, err := runtime.settleWorkspaceReward(ctx, window, "transcript", workspaceRewardEvaluation{
		Score: 90, Reason: "Qualified.",
	})
	if !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
		t.Fatalf("settleWorkspaceReward() error = %v, want %v", err, workspace.ErrWorkspacePendingDeletion)
	}
	if changed {
		t.Fatal("settleWorkspaceReward() changed = true for deleting Workspace")
	}
	var grants int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_reward_grants WHERE source_id = ?`, window.ID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("deleting Workspace grants = %d, want 0", grants)
	}
}

func TestWorkspaceRewardSQLiteDeletionFenceOrdersMarkerAndSettlement(t *testing.T) {
	testWorkspaceRewardDeletionFenceOrdersMarkerAndSettlement(t, testDB(t))
}

func TestWorkspaceRewardDeletionFenceFailsClosedAfterCanceledCommit(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 13, 2, 15, 0, 0, time.UTC)
	deleting := false
	environment := &workspaceRewardTestEnvironment{availabilityFunc: func(context.Context, string) error {
		if deleting {
			return workspace.ErrWorkspacePendingDeletion
		}
		return nil
	}}
	runtime := &Runtime{DB: testDB(t), WorkspaceRewards: environment, Now: func() time.Time { return now }}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	deleteCtx, cancelDelete := context.WithCancel(ctx)
	err := runtime.WithWorkspaceDeletionFence(deleteCtx, "workspace-canceled-delete", func(context.Context) error {
		deleting = true
		cancelDelete()
		return nil
	})
	if err == nil {
		t.Fatal("WithWorkspaceDeletionFence() error = nil after cancellation")
	}
	if err := runtime.checkWorkspaceRewardAvailability(ctx, "workspace-canceled-delete"); !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
		t.Fatalf("availability after canceled fence commit = %v", err)
	}
	var rows int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_sources WHERE workspace_id = ?`, "workspace-canceled-delete").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("canceled deletion fence rows = %d, want rolled back", rows)
	}
}

func testWorkspaceRewardDeletionFenceOrdersMarkerAndSettlement(t *testing.T, db *sqlx.DB) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 8, 13, 2, 30, 0, 0, time.UTC)
	policy := workspaceRewardTestPolicy(t)
	var availabilityMu sync.Mutex
	deleting := map[string]bool{}
	settlementAtFence := make(chan struct{})
	releaseSettlement := make(chan struct{})
	var settlementBarrier sync.Once
	environment := &workspaceRewardTestEnvironment{availabilityFunc: func(_ context.Context, workspaceID string) error {
		if workspaceID == "workflow-settlement-first" {
			settlementBarrier.Do(func() {
				close(settlementAtFence)
				<-releaseSettlement
			})
		}
		availabilityMu.Lock()
		defer availabilityMu.Unlock()
		if deleting[workspaceID] {
			return workspace.ErrWorkspacePendingDeletion
		}
		return nil
	}}
	settlementRuntime := &Runtime{
		DB: db, WorkspaceRewards: environment, Now: func() time.Time { return now },
		NewID: sequentialIDs("grant-settlement-first", "points-settlement-first"),
	}
	deletionRuntime := &Runtime{DB: db, Now: func() time.Time { return now }}
	if err := settlementRuntime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	seedWindow := func(workspaceID, owner, windowID, claimToken string) workspaceRewardWindow {
		t.Helper()
		source := workspaceRewardSource{
			WorkspaceID: workspaceID, ScheduledCheckpoint: "002", CreatedAt: now, UpdatedAt: now,
		}
		if err := settlementRuntime.insertWorkspaceRewardSource(ctx, source); err != nil {
			t.Fatal(err)
		}
		window := workspaceRewardWindow{
			ID: windowID, WorkspaceID: workspaceID,
			WorkspaceKind: WorkspaceRewardKindWorkflow, BeneficiaryPublicKey: owner,
			RuntimeProfileId: "runtime-profile-a", RuntimeProfileRevision: "revision-a",
			Policy: policy, PolicyDigest: policy.Digest, StartHistoryID: "001", HighWaterHistoryID: "002",
			StartHistoryAt: now, HighWaterHistoryAt: now, OpenedAt: now, LastActivityAt: now,
			EvaluateAfter: now, State: workspaceRewardClaimed, AttemptCount: 1,
			ClaimToken: claimToken, ClaimUntil: now.Add(time.Minute), CreatedAt: now, UpdatedAt: now,
		}
		if err := settlementRuntime.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, source); err != nil {
			t.Fatal(err)
		}
		return window
	}
	first := seedWindow("workflow-settlement-first", "peer-settlement-first", "window-settlement-first", "claim-settlement-first")
	type settlementResult struct {
		changed bool
		err     error
	}
	settled := make(chan settlementResult, 1)
	go func() {
		_, changed, err := settlementRuntime.settleWorkspaceReward(ctx, first, "transcript", workspaceRewardEvaluation{
			Score: 90, Reason: "Qualified.",
		})
		settled <- settlementResult{changed: changed, err: err}
	}()
	<-settlementAtFence
	deletionAttempted := make(chan struct{})
	markerCreated := make(chan struct{})
	deletionDone := make(chan error, 1)
	go func() {
		close(deletionAttempted)
		deletionDone <- deletionRuntime.WithWorkspaceDeletionFence(ctx, first.WorkspaceID, func(context.Context) error {
			availabilityMu.Lock()
			deleting[first.WorkspaceID] = true
			availabilityMu.Unlock()
			close(markerCreated)
			return nil
		})
	}()
	<-deletionAttempted
	select {
	case <-markerCreated:
		t.Fatal("deletion marker overtook an in-flight settlement holding the Workspace fence")
	default:
	}
	close(releaseSettlement)
	if result := <-settled; result.err != nil || !result.changed {
		t.Fatalf("settlement-first result = changed %v, error %v", result.changed, result.err)
	}
	if err := <-deletionDone; err != nil {
		t.Fatalf("settlement-first deletion fence error = %v", err)
	}
	<-markerCreated
	if err := deletionRuntime.DeleteWorkspaceData(ctx, first.WorkspaceID); err != nil {
		t.Fatalf("settlement-first cleanup error = %v", err)
	}

	second := seedWindow("workflow-marker-first", "peer-marker-first", "window-marker-first", "claim-marker-first")
	if err := deletionRuntime.WithWorkspaceDeletionFence(ctx, second.WorkspaceID, func(context.Context) error {
		availabilityMu.Lock()
		deleting[second.WorkspaceID] = true
		availabilityMu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("marker-first deletion fence error = %v", err)
	}
	_, changed, err := settlementRuntime.settleWorkspaceReward(ctx, second, "transcript", workspaceRewardEvaluation{
		Score: 90, Reason: "Qualified.",
	})
	if !errors.Is(err, workspace.ErrWorkspacePendingDeletion) || changed {
		t.Fatalf("marker-first settlement = changed %v, error %v", changed, err)
	}
	if err := deletionRuntime.DeleteWorkspaceData(ctx, second.WorkspaceID); err != nil {
		t.Fatalf("marker-first cleanup error = %v", err)
	}
	for _, workspaceID := range []string{first.WorkspaceID, second.WorkspaceID} {
		absent, err := deletionRuntime.WorkspaceDataAbsent(ctx, workspaceID)
		if err != nil || !absent {
			t.Fatalf("WorkspaceDataAbsent(%q) = %v, %v", workspaceID, absent, err)
		}
	}
	var grants int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM gameplay_reward_grants
		WHERE source_id IN (?, ?)`), first.ID, second.ID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 1 {
		t.Fatalf("ordered Workspace grants = %d, want 1", grants)
	}
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
	environment := &workspaceRewardTestEnvironment{}
	runtimes := []*Runtime{
		{DB: db, Catalog: catalog, WorkspaceRewards: environment, Now: func() time.Time { return now }, NewID: sequentialIDs("grant-a", "points-a")},
		{DB: db, Catalog: catalog, WorkspaceRewards: environment, Now: func() time.Time { return now }, NewID: sequentialIDs("grant-b", "points-b")},
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
