package pendingdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type kvTaskState struct {
	MarkerFingerprint string `json:"marker_fingerprint"`
	Status            Status `json:"status"`
	Phase             Phase  `json:"phase"`
	FailureCount      int    `json:"failure_count"`
	NextAttemptAt     string `json:"next_attempt_at"`
	LeaseToken        string `json:"lease_token"`
	LeaseDeadline     string `json:"lease_deadline"`
	LastErrorCode     string `json:"last_error_code"`
	LastErrorMessage  string `json:"last_error_message"`
	UpdatedAt         string `json:"updated_at"`
}

func (s KVSource) Name() string {
	return s.SourceName
}

func (s KVSource) Kinds() []Kind {
	return append([]Kind(nil), s.OwnedKinds...)
}

// Validate verifies that the KV backend supports guarded task transitions.
func (s KVSource) Validate() error {
	return s.validateTaskSource()
}

func (s KVSource) ScanDue(ctx context.Context, now time.Time, limit int, cursor string) ([]Reference, string, error) {
	return s.scanIndexedDue(ctx, now, limit, cursor)
}

func (s KVSource) Claim(ctx context.Context, ref Reference, now time.Time, leaseDuration time.Duration) (Claim, bool, error) {
	if leaseDuration <= 0 {
		return Claim{}, false, fmt.Errorf("%w: lease duration must be positive", ErrInvalid)
	}
	if err := s.validateTaskSource(); err != nil {
		return Claim{}, false, err
	}
	if ref.Source != s.Name() {
		return Claim{}, false, fmt.Errorf("pending deletion: KV reference source mismatch")
	}
	task, raw, err := s.loadTask(ctx, ref.DeletionID)
	if err != nil {
		return Claim{}, false, err
	}
	if task.MarkerFingerprint != ref.MarkerFingerprint || !taskDue(task, now) {
		return Claim{}, false, nil
	}
	token, err := newKVLeaseToken()
	if err != nil {
		return Claim{}, false, err
	}
	task.Status = StatusRunning
	task.LeaseToken = token
	task.LeaseDeadline = now.Add(leaseDuration)
	task.UpdatedAt = now
	matched, err := s.writeTask(ctx, task, raw)
	if err != nil || !matched {
		return Claim{}, false, err
	}
	return Claim{Task: task}, true, nil
}

func (s KVSource) Renew(ctx context.Context, claim Claim, now time.Time, leaseDuration time.Duration) error {
	if leaseDuration <= 0 {
		return fmt.Errorf("%w: lease duration must be positive", ErrInvalid)
	}
	return s.transition(ctx, claim, now, func(task *Task) {
		task.LeaseDeadline = now.Add(leaseDuration)
	})
}

func (s KVSource) Checkpoint(ctx context.Context, claim Claim, phase Phase, now time.Time) (Claim, error) {
	if err := ValidatePhase(phase); err != nil {
		return Claim{}, err
	}
	err := s.transition(ctx, claim, now, func(task *Task) { task.Phase = phase })
	if err != nil {
		return Claim{}, err
	}
	claim.Phase = phase
	claim.UpdatedAt = now
	return claim, nil
}

func (s KVSource) Defer(ctx context.Context, claim Claim, code, message string, nextAttempt, now time.Time) error {
	return s.transition(ctx, claim, now, func(task *Task) {
		task.Status = StatusRetryWait
		task.NextAttemptAt = nextAttempt
		task.LeaseToken = ""
		task.LeaseDeadline = time.Time{}
		task.LastErrorCode = code
		task.LastErrorMessage = message
	})
}

func (s KVSource) Fail(ctx context.Context, claim Claim, code, message string, terminal bool, nextAttempt, now time.Time, maxAttempts int) error {
	return s.transition(ctx, claim, now, func(task *Task) {
		task.FailureCount++
		task.Status = StatusRetryWait
		if terminal || task.FailureCount >= maxAttempts {
			task.Status = StatusFailed
		}
		task.NextAttemptAt = nextAttempt
		task.LeaseToken = ""
		task.LeaseDeadline = time.Time{}
		task.LastErrorCode = code
		task.LastErrorMessage = message
	})
}

func (s KVSource) GetTask(ctx context.Context, deletionID string) (Task, error) {
	task, _, err := s.loadTask(ctx, deletionID)
	return task, err
}

func (s KVSource) ListTasks(ctx context.Context, options SourceListOptions) ([]Task, error) {
	return s.listIndexedTasks(ctx, options)
}

func (s KVSource) Retry(ctx context.Context, deletionID string, now time.Time) (Task, error) {
	task, raw, err := s.loadTask(ctx, deletionID)
	if err != nil {
		return Task{}, err
	}
	if task.Status != StatusFailed {
		return Task{}, ErrConflict
	}
	task.Status = StatusQueued
	task.FailureCount = 0
	task.NextAttemptAt = now
	task.LeaseToken = ""
	task.LeaseDeadline = time.Time{}
	task.LastErrorCode = ""
	task.LastErrorMessage = ""
	task.UpdatedAt = now
	matched, err := s.writeTask(ctx, task, raw)
	if err != nil {
		return Task{}, err
	}
	if !matched {
		return Task{}, ErrConflict
	}
	return task, nil
}

// Finalize atomically removes caller-owned domain keys together with the exact
// marker, locator, and mutable task state for a live KV-backed claim.
func (s KVSource) Finalize(ctx context.Context, claim Claim, now time.Time, deleteKeys []kv.Key) error {
	return s.FinalizeWithEntries(ctx, claim, now, nil, deleteKeys)
}

// FinalizeWithEntries atomically replaces caller-owned domain records while
// consuming the exact marker and task. It is used for permanent tombstones.
func (s KVSource) FinalizeWithEntries(ctx context.Context, claim Claim, now time.Time, entries []kv.Entry, deleteKeys []kv.Key, removeMembers ...kv.SetMembers) error {
	if err := s.validateTaskSource(); err != nil {
		return err
	}
	if err := ValidateTask(claim.Task); err != nil {
		return err
	}
	if claim.Source != s.Name() || !s.owns(claim.Record.Kind) {
		return ErrConflict
	}
	task, raw, err := s.loadTask(ctx, claim.Record.DeletionID)
	if errors.Is(err, ErrNotFound) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if task.MarkerFingerprint != claim.MarkerFingerprint || task.Status != StatusRunning ||
		task.LeaseToken != claim.LeaseToken || !task.LeaseDeadline.After(now) {
		return ErrConflict
	}
	locatorRecord, err := GetByLocator(ctx, s.Store, claim.Record.Kind, claim.Record.ResourceID)
	if errors.Is(err, kv.ErrNotFound) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if locatorRecord.DeletionID != claim.Record.DeletionID {
		return ErrConflict
	}
	fingerprint, err := StoredFingerprint(locatorRecord)
	if err != nil {
		return err
	}
	if fingerprint != claim.MarkerFingerprint {
		return ErrConflict
	}

	keys, err := kvFinalizeDeleteKeys(claim.Record, deleteKeys)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(keys)+len(entries))
	for _, key := range keys {
		seen[kvKeyIdentity(key)] = struct{}{}
	}
	for _, entry := range entries {
		if err := validateFinalizeDomainKey(entry.Key); err != nil {
			return err
		}
		identity := kvKeyIdentity(entry.Key)
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("pending deletion: duplicate KV finalization key %q", entry.Key.String())
		}
		seen[identity] = struct{}{}
	}
	for _, group := range removeMembers {
		if err := validateFinalizeDomainKey(group.Key); err != nil {
			return err
		}
		if _, exists := seen[kvKeyIdentity(group.Key)]; exists {
			return fmt.Errorf("pending deletion: overlapping collection key")
		}
	}
	_, removeIndexes := kvTaskIndexChanges(&task, nil)
	matched, err := s.Store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: kvTaskKey(claim.Record.DeletionID), Expected: raw}}, Entries: entries, DeleteKeys: keys, RemoveMembers: removeMembers, RemoveOrderedMembers: removeIndexes})
	if err != nil {
		return err
	}
	if !matched {
		return ErrConflict
	}
	return nil
}

func validateFinalizeDomainKey(key kv.Key) error {
	if len(key) == 0 {
		return fmt.Errorf("pending deletion: empty KV finalization key")
	}
	if slices.Contains(key, "") {
		return fmt.Errorf("pending deletion: empty KV finalization key segment")
	}
	if len(key) >= len(root) && slices.Equal(key[:len(root)], root) {
		return fmt.Errorf("pending deletion: domain finalization key enters the pending-deletion namespace")
	}
	return nil
}

func kvKeyIdentity(key kv.Key) string {
	encoded, _ := json.Marshal([]string(key))
	return string(encoded)
}

func kvFinalizeDeleteKeys(record Record, domainKeys []kv.Key) ([]kv.Key, error) {
	seen := make(map[string]struct{}, len(domainKeys)+4)
	keys := make([]kv.Key, 0, len(domainKeys)+4)
	appendKey := func(key kv.Key, domain bool) error {
		if domain {
			if err := validateFinalizeDomainKey(key); err != nil {
				return err
			}
		} else if len(key) == 0 {
			return fmt.Errorf("pending deletion: empty KV finalization key")
		}
		identity := kvKeyIdentity(key)
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("pending deletion: duplicate KV finalization key %q", identity)
		}
		seen[identity] = struct{}{}
		keys = append(keys, append(kv.Key(nil), key...))
		return nil
	}
	for _, key := range domainKeys {
		if err := appendKey(key, true); err != nil {
			return nil, err
		}
	}
	commonKeys := []kv.Key{
		byIDKey(record.DeletionID),
		byLocatorKey(record.Kind, record.ResourceID),
		kvTaskKey(record.DeletionID),
	}
	for _, key := range commonKeys {
		if err := appendKey(key, false); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (s KVSource) transition(ctx context.Context, claim Claim, now time.Time, mutate func(*Task)) error {
	task, raw, err := s.loadTask(ctx, claim.Record.DeletionID)
	if err != nil {
		return err
	}
	if task.MarkerFingerprint != claim.MarkerFingerprint || task.Status != StatusRunning ||
		task.LeaseToken != claim.LeaseToken || !task.LeaseDeadline.After(now) {
		return ErrConflict
	}
	mutate(&task)
	task.UpdatedAt = now
	matched, err := s.writeTask(ctx, task, raw)
	if err != nil {
		return err
	}
	if !matched {
		return ErrConflict
	}
	return nil
}

func (s KVSource) loadTask(ctx context.Context, deletionID string) (Task, []byte, error) {
	record, err := getStoredRecord(ctx, s.Store, deletionID)
	if errors.Is(err, kv.ErrNotFound) {
		return Task{}, nil, ErrNotFound
	}
	if err != nil {
		return Task{}, nil, err
	}
	if !s.owns(record.Kind) {
		return Task{}, nil, ErrNotFound
	}
	raw, err := s.Store.Get(ctx, kvTaskKey(deletionID))
	if errors.Is(err, kv.ErrNotFound) {
		if _, err := getStoredRecord(ctx, s.Store, deletionID); errors.Is(err, kv.ErrNotFound) {
			return Task{}, nil, ErrNotFound
		} else if err != nil {
			return Task{}, nil, err
		}
		return Task{}, nil, fmt.Errorf("%w: persisted KV task state is missing", ErrInvalid)
	}
	if err != nil {
		return Task{}, nil, err
	}
	return s.decodeTask(record, raw)
}

func (s KVSource) decodeTask(record Record, raw []byte) (Task, []byte, error) {
	var state kvTaskState
	if err := json.Unmarshal(raw, &state); err != nil {
		return Task{}, nil, fmt.Errorf("pending deletion: decode KV task %q: %w", record.DeletionID, err)
	}
	task := Task{
		Source: s.Name(), Record: record, MarkerFingerprint: state.MarkerFingerprint,
		Status: state.Status, Phase: state.Phase, FailureCount: state.FailureCount,
		NextAttemptAt: parseKVTaskTime(state.NextAttemptAt), LeaseToken: state.LeaseToken,
		LeaseDeadline: parseKVTaskTime(state.LeaseDeadline), LastErrorCode: state.LastErrorCode,
		LastErrorMessage: state.LastErrorMessage, UpdatedAt: parseKVTaskTime(state.UpdatedAt),
	}
	if err := validateStoredTask(task); err != nil {
		return Task{}, nil, err
	}
	return task, append([]byte(nil), raw...), nil
}

func (s KVSource) writeTask(ctx context.Context, task Task, expected []byte) (bool, error) {
	previous, _, err := s.decodeTask(task.Record, expected)
	if err != nil {
		return false, err
	}
	if err := validateStoredTask(task); err != nil {
		return false, err
	}
	encoded, err := encodeKVTaskState(task)
	if err != nil {
		return false, err
	}
	add, remove := kvTaskIndexChanges(&previous, &task)
	return s.Store.ApplyMutation(ctx, kv.Mutation{
		Conditions:        []kv.Condition{{Key: kvTaskKey(task.Record.DeletionID), Expected: expected}},
		Entries:           []kv.Entry{{Key: kvTaskKey(task.Record.DeletionID), Value: encoded}},
		AddOrderedMembers: add, RemoveOrderedMembers: remove,
	})
}

func encodeKVTaskState(task Task) ([]byte, error) {
	return json.Marshal(kvTaskState{
		MarkerFingerprint: task.MarkerFingerprint, Status: task.Status, Phase: task.Phase,
		FailureCount: task.FailureCount, NextAttemptAt: formatKVTaskTime(task.NextAttemptAt),
		LeaseToken: task.LeaseToken, LeaseDeadline: formatKVTaskTime(task.LeaseDeadline),
		LastErrorCode: task.LastErrorCode, LastErrorMessage: task.LastErrorMessage,
		UpdatedAt: formatKVTaskTime(task.UpdatedAt),
	})
}

func (s KVSource) validateTaskSource() error {
	if s.Store == nil {
		return errors.New("pending deletion: KV store not configured")
	}
	if !sourceNamePattern.MatchString(s.Name()) || len(s.OwnedKinds) == 0 {
		return fmt.Errorf("pending deletion: invalid KV task source")
	}

	return nil
}

func (s KVSource) owns(kind Kind) bool {
	return slices.Contains(s.OwnedKinds, kind)
}

func taskDue(task Task, now time.Time) bool {
	switch task.Status {
	case StatusQueued, StatusRetryWait:
		return !task.NextAttemptAt.After(now)
	case StatusRunning:
		return !task.LeaseDeadline.After(now)
	default:
		return false
	}
}

func taskMatches(task Task, options SourceListOptions) bool {
	if len(options.Kinds) > 0 && !options.Kinds[task.Record.Kind] {
		return false
	}
	if len(options.Statuses) > 0 && !options.Statuses[task.Status] {
		return false
	}
	if options.StartTime != nil && task.Record.DeletedAt.Before(*options.StartTime) {
		return false
	}
	if options.EndTime != nil && !task.Record.DeletedAt.Before(*options.EndTime) {
		return false
	}
	if options.AfterCreatedAt == nil {
		return true
	}
	if task.Record.DeletedAt.After(*options.AfterCreatedAt) {
		return true
	}
	if !task.Record.DeletedAt.Equal(*options.AfterCreatedAt) {
		return false
	}
	if task.Source != options.AfterSource {
		return task.Source > options.AfterSource
	}
	return task.Record.DeletionID > options.AfterDeletionID
}

func kvTaskKey(deletionID string) kv.Key {
	return append(append(kv.Key{}, root...), "task", deletionID)
}

func formatKVTaskTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseKVTaskTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func newKVLeaseToken() (string, error) {
	return newPendingDeletionToken()
}
