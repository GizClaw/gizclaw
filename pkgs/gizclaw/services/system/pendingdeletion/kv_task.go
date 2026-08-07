package pendingdeletion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
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
	if err := s.validateTaskSource(); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		return nil, "", fmt.Errorf("pending deletion: KV scan limit must be positive")
	}
	prefix := append(append(kv.Key{}, root...), "by-id")
	var after kv.Key
	if cursor != "" {
		after = append(append(kv.Key{}, prefix...), cursor)
	}
	entries, err := kv.ListAfter(ctx, s.Store, prefix, after, limit)
	if err != nil {
		return nil, "", err
	}
	refs := make([]Reference, 0, limit)
	for _, entry := range entries {
		if len(entry.Key) != len(root)+2 {
			continue
		}
		deletionID := entry.Key[len(entry.Key)-1]
		task, _, err := s.loadTask(ctx, deletionID)
		if err != nil {
			return nil, "", err
		}
		if !s.owns(task.Record.Kind) || !taskDue(task, now) {
			continue
		}
		refs = append(refs, Reference{Source: s.Name(), DeletionID: deletionID, MarkerFingerprint: task.MarkerFingerprint})
	}
	next := ""
	if len(entries) == limit {
		lastKey := entries[len(entries)-1].Key
		next = lastKey[len(lastKey)-1]
	}
	return refs, next, nil
}

func (s KVSource) Claim(ctx context.Context, ref Reference, now time.Time, leaseDuration time.Duration) (Claim, bool, error) {
	if err := s.validateTaskSource(); err != nil {
		return Claim{}, false, err
	}
	if ref.Source != s.Name() {
		return Claim{}, false, fmt.Errorf("pending deletion: KV reference source mismatch")
	}
	task, raw, err := s.loadTaskForMutation(ctx, ref.DeletionID)
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
	if err := s.validateTaskSource(); err != nil {
		return nil, err
	}
	if options.Limit <= 0 {
		return nil, fmt.Errorf("pending deletion: KV list limit must be positive")
	}
	var tasks []Task
	for entry, err := range s.Store.List(ctx, append(append(kv.Key{}, root...), "by-id")) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != len(root)+2 {
			continue
		}
		task, _, err := s.loadTask(ctx, entry.Key[len(entry.Key)-1])
		if err != nil {
			return nil, err
		}
		if !s.owns(task.Record.Kind) || !taskMatches(task, options) {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return taskLess(tasks[i], tasks[j]) })
	if len(tasks) > options.Limit {
		tasks = tasks[:options.Limit]
	}
	return tasks, nil
}

func (s KVSource) Retry(ctx context.Context, deletionID string, now time.Time) (Task, error) {
	task, raw, err := s.loadTaskForMutation(ctx, deletionID)
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

func (s KVSource) transition(ctx context.Context, claim Claim, now time.Time, mutate func(*Task)) error {
	task, raw, err := s.loadTaskForMutation(ctx, claim.Record.DeletionID)
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

func (s KVSource) loadTaskForMutation(ctx context.Context, deletionID string) (Task, []byte, error) {
	task, raw, err := s.loadTask(ctx, deletionID)
	if err != nil {
		return Task{}, nil, err
	}
	if raw != nil {
		return task, raw, nil
	}
	encoded, err := encodeKVTaskState(task)
	if err != nil {
		return Task{}, nil, err
	}
	existing, created, err := kv.CreateIfAbsent(ctx, s.Store, kv.Entry{Key: kvTaskKey(deletionID), Value: encoded}, nil)
	if err != nil {
		return Task{}, nil, err
	}
	if created {
		current, currentErr := Get(ctx, s.Store, deletionID)
		if currentErr == nil {
			currentFingerprint, fingerprintErr := Fingerprint(current)
			if fingerprintErr != nil {
				currentErr = fingerprintErr
			} else if currentFingerprint != task.MarkerFingerprint {
				currentErr = ErrConflict
			}
		}
		if currentErr != nil {
			matched, cleanupErr := kv.CompareAndMutate(ctx, s.Store, kvTaskKey(deletionID), encoded, nil, []kv.Key{kvTaskKey(deletionID)})
			if cleanupErr != nil {
				return Task{}, nil, cleanupErr
			}
			if !matched {
				return Task{}, nil, ErrConflict
			}
			if errors.Is(currentErr, kv.ErrNotFound) {
				return Task{}, nil, ErrNotFound
			}
			return Task{}, nil, currentErr
		}
		return task, encoded, nil
	}
	return s.decodeTask(task.Record, existing)
}

func (s KVSource) loadTask(ctx context.Context, deletionID string) (Task, []byte, error) {
	record, err := Get(ctx, s.Store, deletionID)
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
		fingerprint, fingerprintErr := Fingerprint(record)
		if fingerprintErr != nil {
			return Task{}, nil, fingerprintErr
		}
		task := Task{
			Source: s.Name(), Record: record, MarkerFingerprint: fingerprint,
			Status: StatusQueued, Phase: PhaseValidate,
			NextAttemptAt: record.DeletedAt, UpdatedAt: record.DeletedAt,
		}
		return task, nil, nil
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
	if err := ValidateTask(task); err != nil {
		return Task{}, nil, err
	}
	return task, append([]byte(nil), raw...), nil
}

func (s KVSource) writeTask(ctx context.Context, task Task, expected []byte) (bool, error) {
	encoded, err := encodeKVTaskState(task)
	if err != nil {
		return false, err
	}
	return kv.CompareAndMutate(ctx, s.Store, kvTaskKey(task.Record.DeletionID), expected,
		[]kv.Entry{{Key: kvTaskKey(task.Record.DeletionID), Value: encoded}}, nil)
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
	if !kv.SupportsCreateIfAbsent(s.Store) {
		return kv.ErrCreateIfAbsentUnsupported
	}
	if !kv.SupportsCompareAndMutate(s.Store) {
		return kv.ErrCompareAndMutateUnsupported
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
