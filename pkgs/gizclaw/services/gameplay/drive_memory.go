package gameplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const (
	driveFactPending   = "pending"
	driveFactSubmitted = "submitted"
	driveFactDelivered = "delivered"
	driveFactBlocked   = "blocked"
)

// DriveFactTarget is the immutable Workspace Memory binding snapshot attached
// to an accepted Drive. BindingIdentity identifies the physical backend
// without persisting credentials.
type DriveFactTarget struct {
	WorkspaceName   string
	ProfileName     string
	ProfileRevision string
	BindingName     string
	BindingIdentity string
}

// DriveFactMemory resolves and leases the existing Workspace-bound Memory.
// Gameplay never receives provider credentials or provider-specific clients.
type DriveFactMemory interface {
	Snapshot(context.Context, string) (DriveFactTarget, error)
	Observe(context.Context, DriveFactTarget, memory.Observation) (memory.ObserveResult, error)
	Wait(context.Context, DriveFactTarget, memory.OperationRequest) (memory.ObserveResult, error)
}

type driveFactPayload struct {
	ID         string         `json:"id"`
	Text       string         `json:"text"`
	Attributes map[string]any `json:"attributes"`
	ObservedAt time.Time      `json:"observed_at"`
}

func (payload driveFactPayload) observation(workspaceName string) memory.Observation {
	return memory.Observation{
		Scope:      memory.Scope{AppID: workspaceName},
		ID:         payload.ID,
		Facts:      []memory.FactCandidate{{Text: payload.Text, Attributes: cloneDriveFactAttributes(payload.Attributes)}},
		ObservedAt: payload.ObservedAt,
	}
}

type driveFactOutbox struct {
	ObservationID  string
	PayloadDigest  string
	OwnerPublicKey string
	RuntimeProfile string
	PetID          string
	Target         DriveFactTarget
	Payload        driveFactPayload
	State          string
	OperationID    string
	AttemptCount   int
	NextAttemptAt  time.Time
	LastError      string
	ClaimToken     string
	ClaimUntil     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func canonicalDriveFact(pet apitypes.Pet, behavior apitypes.PetBehavior, result *apitypes.GameResult, grant apitypes.RewardGrant, observedAt time.Time) (driveFactPayload, string, error) {
	sourceType := "reward_grant"
	sourceID := grant.Id
	attributes := map[string]any{
		"kind":                 "event",
		"source_type":          sourceType,
		"source_id":            sourceID,
		"pet_id":               pet.Id,
		"petdef_id":            pet.PetdefId,
		"workspace_name":       pet.WorkspaceName,
		"runtime_profile_name": pet.RuntimeProfileName,
		"pet_exp_delta":        grant.PetExpDelta,
		"points_delta":         grant.PointsDelta,
		"reward_grant_id":      grant.Id,
		"badge_exp_delta":      sortedBadgeDeltas(grant.BadgeExpDelta),
		"pet_lifecycle":        string(pet.Lifecycle),
		"pet_level":            pet.Progression.Level,
		"pet_experience":       pet.Progression.Experience,
		"pet_life":             pet.Stats.Life,
		"pet_energy":           pet.Stats.Energy,
		"pet_health":           pet.Stats.Health,
		"pet_hygiene":          pet.Stats.Hygiene,
		"pet_mood":             pet.Stats.Mood,
		"pet_satiety":          pet.Stats.Satiety,
	}
	text := fmt.Sprintf("Pet %s completed care behavior %s.", strconv.Quote(pet.Id), strconv.Quote(string(behavior)))
	attributes["behavior"] = string(behavior)
	if result != nil {
		sourceType = "game_result"
		sourceID = result.Id
		attributes["source_type"] = sourceType
		attributes["source_id"] = sourceID
		attributes["game_def_id"] = result.GameDefId
		attributes["game_result_id"] = result.Id
		attributes["result_occurred_at"] = result.OccurredAt.UTC().Format(time.RFC3339Nano)
		setOptionalDriveFactAttribute(attributes, "score", result.Score)
		setOptionalDriveFactAttribute(attributes, "max_score", result.MaxScore)
		setOptionalDriveFactAttribute(attributes, "difficulty", result.Difficulty)
		setOptionalDriveFactAttribute(attributes, "outcome", result.Outcome)
		setOptionalDriveFactAttribute(attributes, "duration_ms", result.DurationMs)
		text = fmt.Sprintf("Pet %s completed game %s.", strconv.Quote(pet.Id), strconv.Quote(result.GameDefId))
	}
	payload := driveFactPayload{
		ID:         "gameplay/drive/" + sourceType + "/" + sourceID,
		Text:       text,
		Attributes: attributes,
		ObservedAt: observedAt.UTC(),
	}
	digest, err := memory.ObservationPayloadDigest(payload.observation(pet.WorkspaceName))
	if err != nil {
		return driveFactPayload{}, "", err
	}
	return payload, digest, nil
}

func setOptionalDriveFactAttribute[T any](attributes map[string]any, key string, value *T) {
	if value != nil {
		attributes[key] = *value
	}
}

func sortedBadgeDeltas(input map[string]int64) []string {
	result := make([]string, 0, len(input))
	for id, delta := range input {
		result = append(result, id+"="+strconv.FormatInt(delta, 10))
	}
	sort.Strings(result)
	return result
}

func cloneDriveFactAttributes(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	data, _ := json.Marshal(input)
	var output map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	_ = decoder.Decode(&output)
	return output
}

func (r *Runtime) snapshotDriveFactTarget(ctx context.Context, workspaceName string) (DriveFactTarget, string) {
	if r.DriveFacts == nil {
		return DriveFactTarget{WorkspaceName: workspaceName}, "Workspace Memory delivery is not configured"
	}
	target, err := r.DriveFacts.Snapshot(ctx, workspaceName)
	if err != nil {
		return DriveFactTarget{WorkspaceName: workspaceName}, sanitizeDriveFactError(err)
	}
	if strings.TrimSpace(target.WorkspaceName) == "" {
		target.WorkspaceName = workspaceName
	}
	if target.WorkspaceName != workspaceName || strings.TrimSpace(target.BindingIdentity) == "" {
		return DriveFactTarget{WorkspaceName: workspaceName}, "Workspace Memory binding snapshot is invalid"
	}
	return target, ""
}

// DispatchDriveFactsOnce claims and advances at most one due outbox item.
// It is exported for deterministic service and restart tests.
func (r *Runtime) DispatchDriveFactsOnce(ctx context.Context) (bool, error) {
	if _, err := r.db(); err != nil {
		return false, err
	}
	item, found, err := r.claimDriveFact(ctx)
	if err != nil || !found {
		return found, err
	}
	observation := item.Payload.observation(item.Target.WorkspaceName)
	digest, err := memory.ObservationPayloadDigest(observation)
	if err != nil || digest != item.PayloadDigest {
		if err == nil {
			err = fmt.Errorf("%w: persisted Drive Fact payload digest changed", memory.ErrConflict)
		}
		return true, r.failDriveFact(ctx, item, err)
	}
	if r.DriveFacts == nil {
		return true, r.failDriveFact(ctx, item, fmt.Errorf("%w: Workspace Memory delivery is not configured", memory.ErrInvalidInput))
	}
	if strings.TrimSpace(item.Target.BindingIdentity) == "" {
		target, err := r.DriveFacts.Snapshot(ctx, item.Target.WorkspaceName)
		if err != nil {
			return true, r.failDriveFact(ctx, item, err)
		}
		if err := r.setDriveFactClaimTarget(ctx, item, target); err != nil {
			return true, err
		}
		item.Target = target
	}
	callCtx, stopHeartbeat := r.driveFactClaimContext(ctx, item)
	var result memory.ObserveResult
	switch item.State {
	case driveFactSubmitted:
		result, err = r.DriveFacts.Wait(callCtx, item.Target, memory.OperationRequest{
			Scope: observation.Scope,
			ID:    item.OperationID,
		})
	case driveFactPending, driveFactBlocked:
		result, err = r.DriveFacts.Observe(callCtx, item.Target, observation)
	default:
		err = fmt.Errorf("%w: invalid Drive Fact outbox state %q", memory.ErrInvalidInput, item.State)
	}
	if heartbeatErr := stopHeartbeat(); heartbeatErr != nil {
		return true, heartbeatErr
	}
	if err != nil {
		return true, r.failDriveFact(ctx, item, err)
	}
	if result.Operation != nil && result.Operation.Status == memory.OperationPending {
		if strings.TrimSpace(result.Operation.ID) == "" {
			return true, r.failDriveFact(ctx, item, fmt.Errorf("%w: Memory returned an empty operation locator", memory.ErrUnavailable))
		}
		return true, r.finishDriveFactClaim(ctx, item, driveFactSubmitted, result.Operation.ID, "", r.now())
	}
	if result.Operation != nil && result.Operation.Status == memory.OperationFailed {
		return true, r.failDriveFact(ctx, item, fmt.Errorf("%w: Memory operation failed", memory.ErrUnavailable))
	}
	if len(result.Facts) != 1 {
		cause := fmt.Errorf("%w: Memory materialized %d Facts for one Drive observation", memory.ErrUnavailable, len(result.Facts))
		if item.State == driveFactSubmitted {
			return true, r.finishDriveFactClaim(
				ctx, item, driveFactPending, "", sanitizeDriveFactError(cause),
				r.now().Add(driveFactRetryDelay(item.AttemptCount, false)),
			)
		}
		return true, r.failDriveFact(ctx, item, cause)
	}
	return true, r.finishDriveFactClaim(ctx, item, driveFactDelivered, "", "", r.now())
}

func (r *Runtime) driveFactClaimContext(parent context.Context, item driveFactOutbox) (context.Context, func() error) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				if err := r.extendDriveFactClaim(ctx, item, r.now().Add(30*time.Second)); err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	var once sync.Once
	var heartbeatErr error
	return ctx, func() error {
		once.Do(func() {
			cancel()
			heartbeatErr = <-done
		})
		return heartbeatErr
	}
}

func (r *Runtime) failDriveFact(ctx context.Context, item driveFactOutbox, cause error) error {
	sanitized := sanitizeDriveFactError(cause)
	slog.WarnContext(ctx, "gameplay Drive Fact delivery deferred",
		"observation_id", item.ObservationID,
		"workspace_name", item.Target.WorkspaceName,
		"state", item.State,
		"attempt_count", item.AttemptCount,
		"error", sanitized,
	)
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		return r.finishDriveFactClaim(persistCtx, item, item.State, item.OperationID, sanitized, r.now().Add(time.Second))
	}
	state := item.State
	delay := driveFactRetryDelay(item.AttemptCount, false)
	if errors.Is(cause, memory.ErrInvalidInput) ||
		errors.Is(cause, memory.ErrUnsupported) ||
		errors.Is(cause, memory.ErrConflict) {
		state = driveFactBlocked
		delay = driveFactRetryDelay(item.AttemptCount, true)
	}
	if state == driveFactBlocked && !errors.Is(cause, memory.ErrUnavailable) {
		delay = driveFactRetryDelay(item.AttemptCount, true)
	}
	return r.finishDriveFactClaim(ctx, item, state, item.OperationID, sanitized, r.now().Add(delay))
}

func driveFactRetryDelay(attempt int, blocked bool) time.Duration {
	base, limit := time.Second, 5*time.Minute
	if blocked {
		base, limit = time.Minute, 6*time.Hour
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for range min(attempt-1, 10) {
		delay *= 2
		if delay >= limit {
			return limit
		}
	}
	return delay
}

func sanitizeDriveFactError(err error) string {
	if err == nil {
		return ""
	}
	fields := strings.Fields(err.Error())
	for index, field := range fields {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "secret") ||
			strings.Contains(lower, "credential") ||
			strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "token") {
			fields[index] = "[redacted]"
		}
	}
	value := strings.Join(fields, " ")
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

// StartDriveFactDispatcher starts the single Gameplay-service dispatcher
// owned by one Server process.
func (r *Runtime) StartDriveFactDispatcher(parent context.Context) (context.CancelFunc, <-chan struct{}) {
	r.driveFactMu.Lock()
	if r.driveFactWake == nil {
		r.driveFactWake = make(chan struct{}, 1)
	}
	wake := r.driveFactWake
	r.driveFactMu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := r.Migration(ctx); err != nil {
			slog.ErrorContext(ctx, "gameplay Drive Fact dispatcher migration failed", "error", sanitizeDriveFactError(err))
			return
		}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			for {
				processed, err := r.DispatchDriveFactsOnce(ctx)
				if err != nil || !processed {
					break
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-wake:
			}
		}
	}()
	return cancel, done
}

func (r *Runtime) wakeDriveFactDispatcher() {
	r.driveFactMu.Lock()
	wake := r.driveFactWake
	r.driveFactMu.Unlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}
