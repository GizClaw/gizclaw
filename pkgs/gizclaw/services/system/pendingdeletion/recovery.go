package pendingdeletion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type failedRecoveryCursor struct {
	createdAt  time.Time
	deletionID string
}

// scanFailedRecovery reads one bounded failed page only for domains that opt
// into recovery. Independent cursors advance past retained unsafe records.
// Probes use the same dispatch queue and workers as normal deletion attempts.
func (p *Processor) scanFailedRecovery(ctx context.Context, source Source, dispatch chan<- dispatchItem) bool {
	if _, ok := source.(FailedRecoverySource); !ok {
		return false
	}
	kinds := map[Kind]bool{}
	for _, kind := range source.Kinds() {
		_, handler, ok := p.registry.lookup(source.Name(), kind)
		if !ok {
			continue
		}
		if _, ok := handler.(FailedRecovery); ok {
			kinds[kind] = true
		}
	}
	if len(kinds) == 0 {
		return false
	}
	if p.failedCursors == nil {
		p.failedCursors = make(map[string]failedRecoveryCursor)
	}
	options := SourceListOptions{Kinds: kinds, Statuses: map[Status]bool{StatusFailed: true}, Limit: p.config.PageSize}
	if cursor, ok := p.failedCursors[source.Name()]; ok {
		options.AfterCreatedAt = &cursor.createdAt
		options.AfterDeletionID = cursor.deletionID
		options.AfterSource = source.Name()
	}
	tasks, err := source.ListTasks(ctx, options)
	if err != nil {
		slog.WarnContext(ctx, "pending deletion: failed recovery scan failed", "source_name", source.Name(), "error", err)
		p.observe(source.Name(), "", "", "", "recovery_scan_error")
		return false
	}
	if len(tasks) > p.config.PageSize {
		p.observe(source.Name(), "", "", "", "recovery_scan_error")
		return false
	}
	for _, task := range tasks {
		if err := ValidateTask(task); err != nil || task.Source != source.Name() || task.Status != StatusFailed || !kinds[task.Record.Kind] {
			p.observe(source.Name(), "", "", "", "recovery_scan_error")
			return false
		}
		observed := task
		item := dispatchItem{source: source, ref: Reference{Source: task.Source, DeletionID: task.Record.DeletionID, MarkerFingerprint: task.MarkerFingerprint}, failed: &observed}
		select {
		case dispatch <- item:
			p.failedCursors[source.Name()] = failedRecoveryCursor{createdAt: task.Record.DeletedAt, deletionID: task.Record.DeletionID}
		case <-ctx.Done():
			return true
		default:
			return true
		}
	}
	if len(tasks) < p.config.PageSize {
		delete(p.failedCursors, source.Name())
	}
	return false
}

func (p *Processor) recoverFailed(ctx context.Context, item dispatchItem) (Task, bool) {
	source, ok := item.source.(FailedRecoverySource)
	if !ok {
		return Task{}, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, p.config.AttemptTimeout)
	defer cancel()
	p.active.Add(1)
	p.observeValue("gizclaw_pending_deletion_active_workers", map[string]string{}, float64(p.active.Load()))
	defer func() {
		active := p.active.Add(-1)
		p.observeValue("gizclaw_pending_deletion_active_workers", map[string]string{}, float64(active))
	}()
	current, err := item.source.GetTask(probeCtx, item.ref.DeletionID)
	if err != nil || !sameFailedObservation(current, *item.failed) {
		return Task{}, false
	}
	_, handler, ok := p.registry.lookup(item.source.Name(), current.Record.Kind)
	if !ok {
		return Task{}, false
	}
	policy, ok := handler.(FailedRecovery)
	if !ok {
		return Task{}, false
	}
	p.observe(current.Source, current.Record.Kind, current.Status, current.Phase, "recovery_probe")
	eligible, err := callFailedRecovery(probeCtx, policy, current)
	if err != nil {
		if !errors.Is(err, ErrNotLocal) {
			p.observe(current.Source, current.Record.Kind, current.Status, current.Phase, "recovery_rejected")
			code := "recovery_probe_failed"
			if outcome, ok := errors.AsType[*OutcomeError](err); ok {
				code = normalizeOutcome(outcome).Code
			}
			slog.WarnContext(probeCtx, "pending deletion: automatic recovery rejected",
				"source_name", current.Source, "kind", current.Record.Kind,
				"deletion_id", current.Record.DeletionID, "error_code", code)
		}
		return Task{}, false
	}
	if !eligible {
		return Task{}, false
	}
	retried, err := source.RetryFailed(probeCtx, current, p.now().UTC())
	if err != nil {
		if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrNotFound) {
			p.observe(current.Source, current.Record.Kind, current.Status, current.Phase, "recovery_transition_error")
		}
		return Task{}, false
	}
	p.observe(current.Source, current.Record.Kind, current.Status, current.Phase, "recovery_queued")
	return retried, true
}

func sameFailedObservation(current, observed Task) bool {
	return current.Status == StatusFailed && observed.Status == StatusFailed &&
		current.Source == observed.Source && current.Record.DeletionID == observed.Record.DeletionID &&
		current.MarkerFingerprint == observed.MarkerFingerprint && current.Phase == observed.Phase &&
		current.FailureCount == observed.FailureCount && current.UpdatedAt.Equal(observed.UpdatedAt) &&
		current.LastErrorCode == observed.LastErrorCode && current.LastErrorMessage == observed.LastErrorMessage &&
		current.NextAttemptAt.Equal(observed.NextAttemptAt) && current.LeaseToken == observed.LeaseToken &&
		current.LeaseDeadline.Equal(observed.LeaseDeadline)
}

func callFailedRecovery(ctx context.Context, policy FailedRecovery, task Task) (eligible bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			eligible = false
			err = Terminal("recovery_probe_panic", "Deletion recovery probe panicked", fmt.Errorf("panic: %v", recovered))
		}
	}()
	return policy.CanRecoverFailed(ctx, task)
}
