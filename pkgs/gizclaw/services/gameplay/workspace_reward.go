package gameplay

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

const (
	workspaceRewardPending   = "pending"
	workspaceRewardClaimed   = "claimed"
	workspaceRewardRetry     = "retry"
	workspaceRewardCompleted = "completed"
	workspaceRewardBlocked   = "blocked"

	workspaceRewardHistoryPageLimit = 200
	workspaceRewardPollInterval     = time.Second
	workspaceRewardReconcilePeriod  = 30 * time.Second
	workspaceRewardClaimLease       = time.Minute
	workspaceRewardMaxAttempts      = 5
)

var (
	errWorkspaceRewardHistoryUnavailable = errors.New("claimed History high-water is unavailable")
	errWorkspaceRewardHistoryIdentity    = errors.New("first History entry no longer matches frozen beneficiary")
	errWorkspaceRewardTranscriptConflict = errors.New("transcript digest changed across retry")
)

// WorkspaceRewardEnvironment supplies Server-owned Workspace, RuntimeProfile,
// model, and notification capabilities without moving their ownership into
// Gameplay.
type WorkspaceRewardEnvironment interface {
	ListWorkspaceNames(context.Context) ([]string, error)
	LatestHistoryEntry(context.Context, string) (workspace.HistoryEntry, bool, error)
	LatestHistoryEntryBefore(context.Context, string, time.Time) (workspace.HistoryEntry, bool, error)
	ListHistoryEntries(context.Context, string, string, string, int) (workspace.HistoryEntryPage, error)
	GetHistoryEntry(context.Context, string, string) (workspace.HistoryEntry, error)
	ResolveWorkspaceRewardPolicy(context.Context, string, string) (WorkspaceRewardKind, *WorkspaceRewardPolicySnapshot, error)
	WorkspaceRewardGenerator(context.Context, string, WorkspaceRewardPolicySnapshot) (genx.Generator, error)
	NotifyWorkspaceReward(context.Context, string, WorkspaceRewardUpdate) error
}

type WorkspaceRewardKind string

const (
	WorkspaceRewardKindWorkflow       WorkspaceRewardKind = "workflow"
	WorkspaceRewardKindDirectChatroom WorkspaceRewardKind = "direct_chatroom"
	WorkspaceRewardKindGroupChatroom  WorkspaceRewardKind = "group_chatroom"
)

type WorkspaceRewardBadgePolicy struct {
	Alias           string `json:"alias"`
	ResourceID      string `json:"resource_id"`
	DisplayName     string `json:"display_name"`
	RewardPrompt    string `json:"reward_prompt"`
	MaxExpPerWindow int64  `json:"max_exp_per_window"`
}

type WorkspaceRewardPolicySnapshot struct {
	RuntimeProfileName     string                                             `json:"runtime_profile_name"`
	RuntimeProfileRevision string                                             `json:"runtime_profile_revision"`
	ModelAlias             string                                             `json:"model_alias"`
	ModelResourceID        string                                             `json:"model_resource_id"`
	WorkspaceKinds         []WorkspaceRewardKind                              `json:"workspace_kinds"`
	QuietPeriod            time.Duration                                      `json:"quiet_period"`
	MaxWindowAge           time.Duration                                      `json:"max_window_age"`
	MaxEntries             int64                                              `json:"max_entries"`
	MaxTextBytes           int64                                              `json:"max_text_bytes"`
	PointsPrompt           string                                             `json:"points_prompt"`
	ScoreMin               int64                                              `json:"score_min"`
	ScoreMax               int64                                              `json:"score_max"`
	QualifyingScore        int64                                              `json:"qualifying_score"`
	PointsTiers            []apitypes.RuntimeProfileWorkspaceRewardPointsTier `json:"points_tiers"`
	Badges                 []WorkspaceRewardBadgePolicy                       `json:"badges"`
	BudgetPeriod           time.Duration                                      `json:"budget_period"`
	PointsMax              int64                                              `json:"points_max"`
	BadgeExpMax            int64                                              `json:"badge_exp_max"`
	InitialPointsBalance   int64                                              `json:"initial_points_balance"`
	Digest                 string                                             `json:"-"`
}

type WorkspaceRewardUpdate struct {
	WorkspaceName string
	RewardGrantID string
	Revision      time.Time
}

type WorkspaceRewardTranscriptEntry struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type workspaceRewardSource struct {
	WorkspaceName       string
	ScheduledCheckpoint string
	CompletedCheckpoint string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type workspaceRewardWindow struct {
	ID                     string
	WorkspaceName          string
	WorkspaceKind          WorkspaceRewardKind
	BeneficiaryPublicKey   string
	RuntimeProfileName     string
	RuntimeProfileRevision string
	Policy                 WorkspaceRewardPolicySnapshot
	PolicyDigest           string
	StartHistoryID         string
	HighWaterHistoryID     string
	StartHistoryAt         time.Time
	HighWaterHistoryAt     time.Time
	OpenedAt               time.Time
	LastActivityAt         time.Time
	EvaluateAfter          time.Time
	State                  string
	AttemptCount           int
	NextAttemptAt          time.Time
	ClaimToken             string
	ClaimUntil             time.Time
	TranscriptDigest       string
	Outcome                string
	LastError              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// SnapshotWorkspaceRewardPolicy freezes the profile-authored policy and
// resolved Badge resources used by one conversation window.
func (r *Runtime) SnapshotWorkspaceRewardPolicy(
	ctx context.Context,
	profile apitypes.RuntimeProfile,
	kind WorkspaceRewardKind,
) (*WorkspaceRewardPolicySnapshot, error) {
	if profile.Spec.Gameplay == nil || profile.Spec.Gameplay.WorkspaceReward == nil ||
		!profile.Spec.Gameplay.WorkspaceReward.Enabled {
		return nil, nil
	}
	reward := profile.Spec.Gameplay.WorkspaceReward
	if reward.WorkspaceKinds == nil || reward.Debounce == nil || reward.Transcript == nil ||
		reward.Evaluation == nil || reward.Points == nil || reward.Badges == nil ||
		reward.RollingBudget == nil {
		return nil, errors.New("gameplay: enabled workspace reward policy is incomplete")
	}
	allowedKinds := make([]WorkspaceRewardKind, 0, len(*reward.WorkspaceKinds))
	kindAllowed := false
	for _, configured := range *reward.WorkspaceKinds {
		value := WorkspaceRewardKind(configured)
		allowedKinds = append(allowedKinds, value)
		kindAllowed = kindAllowed || value == kind
	}
	if !kindAllowed {
		return nil, nil
	}
	modelBinding, ok := bindingValue(profile.Spec.Resources.Models, reward.Evaluation.Model)
	if !ok {
		return nil, fmt.Errorf("gameplay: workspace reward model alias %q is missing", reward.Evaluation.Model)
	}
	quietPeriod, err := time.ParseDuration(reward.Debounce.QuietPeriod)
	if err != nil {
		return nil, fmt.Errorf("gameplay: parse workspace reward quiet period: %w", err)
	}
	maxWindowAge, err := time.ParseDuration(reward.Debounce.MaxWindowAge)
	if err != nil {
		return nil, fmt.Errorf("gameplay: parse workspace reward max window age: %w", err)
	}
	budgetPeriod, err := time.ParseDuration(reward.RollingBudget.Period)
	if err != nil {
		return nil, fmt.Errorf("gameplay: parse workspace reward budget period: %w", err)
	}
	aliases := make([]string, 0, len(*reward.Badges))
	for alias := range *reward.Badges {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	badges := make([]WorkspaceRewardBadgePolicy, 0, len(aliases))
	for _, alias := range aliases {
		binding, ok := bindingValue(profile.Spec.Resources.BadgeDefs, alias)
		if !ok {
			return nil, fmt.Errorf("gameplay: workspace reward badge alias %q is missing", alias)
		}
		if r == nil || r.Catalog == nil {
			return nil, errors.New("gameplay: catalog is not configured")
		}
		badgeDef, err := r.Catalog.GetBadgeDefByID(ctx, binding.ResourceId)
		if err != nil {
			return nil, err
		}
		if badgeDef.Spec.RewardPrompt == nil || strings.TrimSpace(*badgeDef.Spec.RewardPrompt) == "" {
			return nil, fmt.Errorf("gameplay: workspace reward badge alias %q has no reward_prompt", alias)
		}
		badges = append(badges, WorkspaceRewardBadgePolicy{
			Alias: alias, ResourceID: binding.ResourceId,
			DisplayName:     strings.TrimSpace(badgeDef.Spec.DisplayName),
			RewardPrompt:    strings.TrimSpace(*badgeDef.Spec.RewardPrompt),
			MaxExpPerWindow: (*reward.Badges)[alias].MaxExpPerWindow,
		})
	}
	snapshot := WorkspaceRewardPolicySnapshot{
		RuntimeProfileName: profile.Name, RuntimeProfileRevision: profile.Revision,
		ModelAlias: reward.Evaluation.Model, ModelResourceID: modelBinding.ResourceId,
		WorkspaceKinds: allowedKinds, QuietPeriod: quietPeriod, MaxWindowAge: maxWindowAge,
		MaxEntries: reward.Transcript.MaxEntries, MaxTextBytes: reward.Transcript.MaxTextBytes,
		PointsPrompt: reward.Evaluation.PointsPrompt, ScoreMin: reward.Evaluation.ScoreMin,
		ScoreMax: reward.Evaluation.ScoreMax, QualifyingScore: reward.Evaluation.QualifyingScore,
		PointsTiers: append([]apitypes.RuntimeProfileWorkspaceRewardPointsTier(nil), reward.Points.Tiers...),
		Badges:      badges, BudgetPeriod: budgetPeriod, PointsMax: reward.RollingBudget.PointsMax,
		BadgeExpMax: reward.RollingBudget.BadgeExpMax,
	}
	if profile.Spec.Gameplay.Points != nil && profile.Spec.Gameplay.Points.InitialBalance != nil {
		snapshot.InitialPointsBalance = *profile.Spec.Gameplay.Points.InitialBalance
	}
	digest, err := workspaceRewardPolicyDigest(snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.Digest = digest
	return &snapshot, nil
}

func bindingValue(values *map[string]apitypes.RuntimeProfileBinding, alias string) (apitypes.RuntimeProfileBinding, bool) {
	if values == nil {
		return apitypes.RuntimeProfileBinding{}, false
	}
	value, ok := (*values)[strings.TrimSpace(alias)]
	return value, ok
}

func workspaceRewardPolicyDigest(snapshot WorkspaceRewardPolicySnapshot) (string, error) {
	snapshot.Digest = ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("gameplay: encode workspace reward policy: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// StartWorkspaceRewardDispatcher initializes durable source boundaries and
// starts one poller for all Workspace reward windows.
func (r *Runtime) StartWorkspaceRewardDispatcher(parent context.Context) (context.CancelFunc, <-chan struct{}, error) {
	if r == nil || r.WorkspaceRewards == nil {
		return nil, nil, errors.New("gameplay: workspace reward environment is not configured")
	}
	if err := r.Migration(parent); err != nil {
		return nil, nil, err
	}
	activation, err := r.ensureWorkspaceRewardActivation(parent)
	if err != nil {
		return nil, nil, err
	}
	if err := r.initializeWorkspaceRewardSources(parent, activation); err != nil {
		return nil, nil, err
	}
	if err := r.reconcileWorkspaceRewardSources(parent); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	wake := r.workspaceRewardWakeChannel()
	go func() {
		defer close(done)
		poll := time.NewTicker(workspaceRewardPollInterval)
		defer poll.Stop()
		reconcile := time.NewTicker(workspaceRewardReconcilePeriod)
		defer reconcile.Stop()
		for {
			select {
			case <-ctx.Done():
				r.releaseWorkspaceRewardClaims(context.WithoutCancel(ctx))
				return
			case <-wake:
			case <-poll.C:
			case <-reconcile.C:
				if err := r.initializeWorkspaceRewardSources(ctx, activation); err != nil && ctx.Err() == nil {
					slog.Error("reconcile workspace reward sources", "error_class", "source_init", "error", err)
				}
				if err := r.reconcileWorkspaceRewardSources(ctx); err != nil && ctx.Err() == nil {
					slog.Error("reconcile workspace reward History", "error_class", "history_reconcile", "error", err)
				}
			}
			for {
				processed, err := r.dispatchWorkspaceReward(ctx)
				if err != nil {
					if ctx.Err() == nil {
						slog.Error("dispatch workspace reward", "error_class", "dispatch", "error", err)
					}
					break
				}
				if !processed {
					break
				}
			}
		}
	}()
	return cancel, done, nil
}

// ScheduleWorkspaceRewardActivity durably records the exact History high-water
// after AgentHost append succeeds. It performs no model invocation.
func (r *Runtime) ScheduleWorkspaceRewardActivity(ctx context.Context, workspaceName string, entry workspace.HistoryEntry) error {
	if r == nil || r.WorkspaceRewards == nil {
		return nil
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" || strings.TrimSpace(entry.ID) == "" {
		return errors.New("gameplay: workspace reward activity requires Workspace and History IDs")
	}
	lock := r.workspaceRewardMutex(workspaceName)
	lock.Lock()
	defer lock.Unlock()
	source, err := r.getWorkspaceRewardSource(ctx, workspaceName)
	if errors.Is(err, sql.ErrNoRows) {
		now := r.now()
		activation, activationErr := r.ensureWorkspaceRewardActivation(ctx)
		if activationErr != nil {
			return activationErr
		}
		checkpoint := ""
		latest, ok, latestErr := r.WorkspaceRewards.LatestHistoryEntryBefore(ctx, workspaceName, activation)
		if latestErr != nil {
			return latestErr
		}
		if ok {
			checkpoint = latest.ID
		}
		source = workspaceRewardSource{
			WorkspaceName: workspaceName, ScheduledCheckpoint: checkpoint,
			CompletedCheckpoint: checkpoint, CreatedAt: now, UpdatedAt: now,
		}
		if err := r.insertWorkspaceRewardSource(ctx, source); err != nil {
			return err
		}
		source, err = r.getWorkspaceRewardSource(ctx, workspaceName)
		if err != nil {
			return err
		}
		if err := r.reconcileWorkspaceRewardSourceLocked(ctx, &source, entry.ID); err != nil {
			return err
		}
		r.wakeWorkspaceRewardDispatcher()
		return nil
	}
	if err != nil {
		return err
	}
	if entry.ID <= source.ScheduledCheckpoint {
		return nil
	}
	if err := r.reconcileWorkspaceRewardSourceLocked(ctx, &source, entry.ID); err != nil {
		return err
	}
	r.wakeWorkspaceRewardDispatcher()
	return nil
}

func (r *Runtime) initializeWorkspaceRewardSources(ctx context.Context, activation time.Time) error {
	names, err := r.WorkspaceRewards.ListWorkspaceNames(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		lock := r.workspaceRewardMutex(name)
		lock.Lock()
		_, err := r.getWorkspaceRewardSource(ctx, name)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
			checkpoint := ""
			latest, ok, latestErr := r.WorkspaceRewards.LatestHistoryEntryBefore(ctx, name, activation)
			if latestErr != nil {
				err = latestErr
			} else if ok {
				checkpoint = latest.ID
			}
			if err == nil {
				now := r.now()
				err = r.insertWorkspaceRewardSource(ctx, workspaceRewardSource{
					WorkspaceName: name, ScheduledCheckpoint: checkpoint,
					CompletedCheckpoint: checkpoint, CreatedAt: now, UpdatedAt: now,
				})
			}
		}
		lock.Unlock()
		if err != nil {
			return fmt.Errorf("initialize workspace reward source %q: %w", name, err)
		}
	}
	return nil
}

func (r *Runtime) reconcileWorkspaceRewardSources(ctx context.Context) error {
	names, err := r.WorkspaceRewards.ListWorkspaceNames(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := r.reconcileWorkspaceRewardSource(ctx, name, ""); err != nil {
			return fmt.Errorf("reconcile workspace reward source %q: %w", name, err)
		}
	}
	return nil
}

func (r *Runtime) reconcileWorkspaceRewardSource(ctx context.Context, workspaceName, through string) error {
	lock := r.workspaceRewardMutex(workspaceName)
	lock.Lock()
	defer lock.Unlock()
	source, err := r.getWorkspaceRewardSource(ctx, workspaceName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.reconcileWorkspaceRewardSourceLocked(ctx, &source, through)
}

func (r *Runtime) reconcileWorkspaceRewardSourceLocked(ctx context.Context, source *workspaceRewardSource, through string) error {
	active, err := r.activeWorkspaceRewardWindow(ctx, source.WorkspaceName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && active.State != workspaceRewardPending {
		return nil
	}
	cursor := source.ScheduledCheckpoint
	for {
		page, err := r.WorkspaceRewards.ListHistoryEntries(ctx, source.WorkspaceName, cursor, through, workspaceRewardHistoryPageLimit)
		if err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if err := r.applyWorkspaceRewardEntry(ctx, source, entry); err != nil {
				return err
			}
			cursor = entry.ID
			if through != "" && cursor >= through {
				return nil
			}
			active, err = r.activeWorkspaceRewardWindow(ctx, source.WorkspaceName)
			if err == nil && active.State != workspaceRewardPending {
				return nil
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if !page.HasNext || len(page.Entries) == 0 {
			return nil
		}
		cursor = page.NextCursor
	}
}

func (r *Runtime) applyWorkspaceRewardEntry(ctx context.Context, source *workspaceRewardSource, entry workspace.HistoryEntry) error {
	active, err := r.activeWorkspaceRewardWindow(ctx, source.WorkspaceName)
	if err == nil {
		if active.State != workspaceRewardPending {
			return nil
		}
		if entry.CreatedAt.After(active.EvaluateAfter) {
			r.wakeWorkspaceRewardDispatcher()
			return nil
		}
		return r.advanceWorkspaceRewardWindow(ctx, source, active, entry)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	source.ScheduledCheckpoint = entry.ID
	source.UpdatedAt = r.now()
	if entry.Origin != workspace.HistoryOriginAgentHost || entry.Type != "gear" ||
		strings.TrimSpace(entry.GearID) == "" {
		source.CompletedCheckpoint = entry.ID
		return r.updateWorkspaceRewardSource(ctx, *source)
	}
	kind, policy, err := r.WorkspaceRewards.ResolveWorkspaceRewardPolicy(ctx, source.WorkspaceName, entry.GearID)
	if err != nil {
		return err
	}
	if policy == nil {
		source.CompletedCheckpoint = entry.ID
		return r.updateWorkspaceRewardSource(ctx, *source)
	}
	now := r.now()
	window := workspaceRewardWindow{
		ID: r.newID(), WorkspaceName: source.WorkspaceName, WorkspaceKind: kind,
		BeneficiaryPublicKey: entry.GearID, RuntimeProfileName: policy.RuntimeProfileName,
		RuntimeProfileRevision: policy.RuntimeProfileRevision, Policy: *policy,
		PolicyDigest: policy.Digest, StartHistoryID: entry.ID, HighWaterHistoryID: entry.ID,
		StartHistoryAt: entry.CreatedAt, HighWaterHistoryAt: entry.CreatedAt,
		OpenedAt: entry.CreatedAt, LastActivityAt: entry.CreatedAt,
		EvaluateAfter: minWorkspaceRewardTime(entry.CreatedAt.Add(policy.QuietPeriod), entry.CreatedAt.Add(policy.MaxWindowAge)),
		State:         workspaceRewardPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return r.insertWorkspaceRewardWindowAndUpdateSource(ctx, window, *source)
}

func (r *Runtime) advanceWorkspaceRewardWindow(
	ctx context.Context,
	source *workspaceRewardSource,
	window workspaceRewardWindow,
	entry workspace.HistoryEntry,
) error {
	source.ScheduledCheckpoint = entry.ID
	source.UpdatedAt = r.now()
	window.HighWaterHistoryID = entry.ID
	window.HighWaterHistoryAt = entry.CreatedAt
	if entry.Origin == workspace.HistoryOriginAgentHost {
		window.LastActivityAt = entry.CreatedAt
		window.EvaluateAfter = minWorkspaceRewardTime(
			entry.CreatedAt.Add(window.Policy.QuietPeriod),
			window.OpenedAt.Add(window.Policy.MaxWindowAge),
		)
	}
	window.UpdatedAt = r.now()
	return r.updateWorkspaceRewardWindowAndSource(ctx, window, *source)
}

func minWorkspaceRewardTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (r *Runtime) dispatchWorkspaceReward(ctx context.Context) (bool, error) {
	window, ok, err := r.claimWorkspaceRewardWindow(ctx)
	if err != nil || !ok {
		return ok, err
	}
	err = r.processWorkspaceRewardClaim(ctx, window)
	if err == nil {
		_ = r.reconcileWorkspaceRewardSource(ctx, window.WorkspaceName, "")
		return true, nil
	}
	if ctx.Err() != nil {
		return true, r.retryWorkspaceRewardWindow(context.WithoutCancel(ctx), window, ctx.Err())
	}
	var invalid *invalidWorkspaceRewardError
	if errors.As(err, &invalid) {
		return true, r.blockWorkspaceRewardWindow(ctx, window, invalid)
	}
	if window.AttemptCount >= workspaceRewardMaxAttempts {
		return true, r.blockWorkspaceRewardWindow(ctx, window, err)
	}
	return true, r.retryWorkspaceRewardWindow(ctx, window, err)
}

func (r *Runtime) processWorkspaceRewardClaim(ctx context.Context, window workspaceRewardWindow) error {
	transcript, digest, outcome, err := r.workspaceRewardTranscript(ctx, window)
	if err != nil {
		return &invalidWorkspaceRewardError{cause: err}
	}
	if outcome != "" {
		if outcome == "skipped_over_limit" {
			slog.Warn("workspace reward skipped",
				"workspace", window.WorkspaceName,
				"beneficiary", window.BeneficiaryPublicKey,
				"profile", window.RuntimeProfileName,
				"policy_digest", window.PolicyDigest,
				"window", window.ID,
				"state", workspaceRewardCompleted,
				"outcome", outcome,
				"error_class", "transcript_limit",
			)
		}
		return r.completeWorkspaceRewardWithoutGrant(ctx, window, digest, outcome)
	}
	if window.TranscriptDigest != "" && window.TranscriptDigest != digest {
		return &invalidWorkspaceRewardError{cause: errWorkspaceRewardTranscriptConflict}
	}
	if err := r.setWorkspaceRewardTranscriptDigest(ctx, window, digest); err != nil {
		return err
	}
	generator, err := r.WorkspaceRewards.WorkspaceRewardGenerator(ctx, window.BeneficiaryPublicKey, window.Policy)
	if err != nil {
		return fmt.Errorf("resolve evaluator: %w", err)
	}
	result, err := evaluateWorkspaceReward(ctx, generator, window.Policy, transcript)
	if err != nil {
		return err
	}
	grant, changed, err := r.settleWorkspaceReward(ctx, window, digest, result)
	if err != nil {
		return err
	}
	if changed {
		if err := r.WorkspaceRewards.NotifyWorkspaceReward(context.WithoutCancel(ctx), window.BeneficiaryPublicKey, WorkspaceRewardUpdate{
			WorkspaceName: window.WorkspaceName, RewardGrantID: grant.Id, Revision: grant.CreatedAt,
		}); err != nil {
			slog.Warn("notify workspace reward", "workspace", window.WorkspaceName, "beneficiary", window.BeneficiaryPublicKey, "window", window.ID, "error_class", "event_delivery", "error", err)
		}
	}
	return nil
}

func (r *Runtime) workspaceRewardTranscript(
	ctx context.Context,
	window workspaceRewardWindow,
) ([]WorkspaceRewardTranscriptEntry, string, string, error) {
	first, err := r.WorkspaceRewards.GetHistoryEntry(ctx, window.WorkspaceName, window.StartHistoryID)
	if err != nil {
		return nil, "", "", fmt.Errorf("read first History entry: %w", err)
	}
	if first.Origin != workspace.HistoryOriginAgentHost || first.Type != "gear" ||
		first.GearID != window.BeneficiaryPublicKey {
		return nil, "", "", errWorkspaceRewardHistoryIdentity
	}
	entries := []workspace.HistoryEntry{first}
	cursor := first.ID
	lastSeenID := first.ID
	for cursor < window.HighWaterHistoryID {
		page, err := r.WorkspaceRewards.ListHistoryEntries(ctx, window.WorkspaceName, cursor, window.HighWaterHistoryID, workspaceRewardHistoryPageLimit)
		if err != nil {
			return nil, "", "", err
		}
		entries = append(entries, page.Entries...)
		if len(page.Entries) > 0 {
			lastSeenID = page.Entries[len(page.Entries)-1].ID
		}
		if len(page.Entries) == 0 || !page.HasNext {
			break
		}
		cursor = page.NextCursor
	}
	if lastSeenID != window.HighWaterHistoryID {
		return nil, "", "", errWorkspaceRewardHistoryUnavailable
	}
	transcript := make([]WorkspaceRewardTranscriptEntry, 0, len(entries))
	var textBytes int64
	hasGear := false
	hasAgentAfterGear := false
	for _, entry := range entries {
		if entry.Origin != workspace.HistoryOriginAgentHost {
			continue
		}
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			continue
		}
		role := ""
		switch entry.Type {
		case "gear":
			role = "user"
			hasGear = true
		case "agent":
			role = "assistant"
			hasAgentAfterGear = hasGear
		default:
			continue
		}
		textBytes += int64(len([]byte(text)))
		transcript = append(transcript, WorkspaceRewardTranscriptEntry{Role: role, Text: text})
		if int64(len(transcript)) > window.Policy.MaxEntries || textBytes > window.Policy.MaxTextBytes {
			return nil, "", "skipped_over_limit", nil
		}
	}
	data, err := json.Marshal(transcript)
	if err != nil {
		return nil, "", "", err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if !hasGear || !hasAgentAfterGear {
		return transcript, digest, "skipped_incomplete", nil
	}
	return transcript, digest, "", nil
}

func (r *Runtime) workspaceRewardWakeChannel() <-chan struct{} {
	r.workspaceRewardMu.Lock()
	defer r.workspaceRewardMu.Unlock()
	if r.workspaceRewardWake == nil {
		r.workspaceRewardWake = make(chan struct{}, 1)
	}
	return r.workspaceRewardWake
}

func (r *Runtime) wakeWorkspaceRewardDispatcher() {
	r.workspaceRewardMu.Lock()
	if r.workspaceRewardWake == nil {
		r.workspaceRewardWake = make(chan struct{}, 1)
	}
	wake := r.workspaceRewardWake
	r.workspaceRewardMu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (r *Runtime) workspaceRewardMutex(key string) *sync.Mutex {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return &r.workspaceRewardLocks[hash.Sum32()%uint32(len(r.workspaceRewardLocks))]
}

func workspaceRewardRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > workspaceRewardMaxAttempts {
		attempt = workspaceRewardMaxAttempts
	}
	return time.Second << (attempt - 1)
}

func safeWorkspaceRewardErrorClass(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "evaluation_timeout"
	case errors.Is(err, context.Canceled):
		return "evaluation_canceled"
	case errors.Is(err, errWorkspaceRewardHistoryUnavailable):
		return "history_unavailable"
	case errors.Is(err, errWorkspaceRewardHistoryIdentity):
		return "history_identity_mismatch"
	case errors.Is(err, errWorkspaceRewardTranscriptConflict):
		return "transcript_digest_conflict"
	}
	var invalid *invalidWorkspaceRewardError
	if errors.As(err, &invalid) {
		return "invalid_result"
	}
	return "dependency_failure"
}

type invalidWorkspaceRewardError struct {
	cause error
}

func (e *invalidWorkspaceRewardError) Error() string {
	if e == nil || e.cause == nil {
		return "invalid workspace reward result"
	}
	return "invalid workspace reward result: " + e.cause.Error()
}

func (e *invalidWorkspaceRewardError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (r *Runtime) workspaceRewardMutexForSettlement(owner, profile string) *sync.Mutex {
	return r.driveMutex(owner + "\x00" + profile)
}

func sumBadgeExp(values map[string]int64) int64 {
	var total int64
	for _, value := range values {
		total += value
	}
	return total
}

func sortedWorkspaceRewardBadges(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
