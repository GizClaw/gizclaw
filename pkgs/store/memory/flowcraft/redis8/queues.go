package redis8

import (
	"context"
	"fmt"
	"sort"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
)

const (
	queuePending  = "pending"
	queueLeased   = "leased"
	queueComplete = "complete"
	queueFailed   = "failed"
)

type asyncQueue struct{ state *stateStore }

func (q *asyncQueue) Enqueue(ctx context.Context, job flowrecall.AsyncSemanticJob) (receipt flowrecall.AsyncSemanticReceipt, err error) {
	if job.RequestID == "" {
		return receipt, errdefs.Validationf("flowcraft redis8 async queue: request id is required")
	}
	err = q.state.update(ctx, func(state *durableState) error {
		for _, row := range state.Async {
			if row.Job.RequestID == job.RequestID {
				receipt = flowrecall.AsyncSemanticReceipt{RequestID: job.RequestID, EnqueuedAt: row.EnqueuedAt, QueueDepth: asyncPending(state)}
				return nil
			}
		}
		now := time.Now()
		state.Async = append(state.Async, asyncRecord{Job: flowrecall.CloneAsyncSemanticJob(job), Status: queuePending, EnqueuedAt: now, RetryAt: job.LeaseUntil})
		receipt = flowrecall.AsyncSemanticReceipt{RequestID: job.RequestID, EnqueuedAt: now, QueueDepth: asyncPending(state)}
		return nil
	})
	return receipt, err
}
func asyncPending(state *durableState) int {
	n := 0
	for _, row := range state.Async {
		if row.Status == queuePending {
			n++
		}
	}
	return n
}
func (q *asyncQueue) Claim(ctx context.Context, opts flowrecall.AsyncSemanticClaimOptions) (jobs []flowrecall.AsyncSemanticJob, err error) {
	if opts.Max <= 0 {
		return nil, nil
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	err = q.state.update(ctx, func(state *durableState) error {
		for i := range state.Async {
			row := &state.Async[i]
			if row.Status == queueLeased && !row.Job.LeaseUntil.IsZero() && !now.Before(row.Job.LeaseUntil) {
				row.Status = queuePending
				row.Job.LeaseToken = ""
				row.Job.LeaseUntil = time.Time{}
			}
		}
		sort.SliceStable(state.Async, func(i, j int) bool {
			if state.Async[i].EnqueuedAt.Equal(state.Async[j].EnqueuedAt) {
				return state.Async[i].Job.RequestID < state.Async[j].Job.RequestID
			}
			return state.Async[i].EnqueuedAt.Before(state.Async[j].EnqueuedAt)
		})
		for i := range state.Async {
			if len(jobs) >= opts.Max {
				break
			}
			row := &state.Async[i]
			if row.Status != queuePending || (!row.RetryAt.IsZero() && now.Before(row.RetryAt)) {
				continue
			}
			if opts.Scope != nil && !sameScope(row.Job.Scope, *opts.Scope) {
				continue
			}
			if opts.Scope == nil && opts.RuntimeID != "" && row.Job.Scope.RuntimeID != opts.RuntimeID {
				continue
			}
			row.Status = queueLeased
			row.Job.Attempt++
			row.Job.LeaseToken = leaseToken()
			row.Job.LeaseUntil = now.Add(5 * time.Minute)
			jobs = append(jobs, flowrecall.CloneAsyncSemanticJob(row.Job))
		}
		return nil
	})
	return jobs, err
}
func (q *asyncQueue) Complete(ctx context.Context, id, token string, result flowrecall.AsyncSemanticResult) error {
	return q.state.update(ctx, func(state *durableState) error {
		for i := range state.Async {
			row := &state.Async[i]
			if row.Job.RequestID != id {
				continue
			}
			if row.Status == queueComplete {
				return nil
			}
			if row.Status != queueLeased || token == "" || row.Job.LeaseToken != token {
				return nil
			}
			row.Status = queueComplete
			row.Result = result
			row.Job.LeaseToken = ""
			row.Job.LeaseUntil = time.Time{}
			flowrecall.ScrubAsyncSemanticJobPII(&row.Job)
			return nil
		}
		return nil
	})
}
func (q *asyncQueue) Fail(ctx context.Context, id, token string, failure flowrecall.AsyncSemanticFailure) error {
	return q.state.update(ctx, func(state *durableState) error {
		for i := range state.Async {
			row := &state.Async[i]
			if row.Job.RequestID != id || row.Status != queueLeased || token == "" || row.Job.LeaseToken != token {
				continue
			}
			row.Failure = failure
			row.Job.LeaseToken = ""
			row.Job.LeaseUntil = time.Time{}
			if failure.ErrClass == flowrecall.ErrClassPermanent {
				row.Status = queueFailed
				flowrecall.ScrubAsyncSemanticJobPII(&row.Job)
			} else {
				row.Status = queuePending
				row.RetryAt = failure.RetryAt
				if row.RetryAt.IsZero() {
					row.RetryAt = time.Now().Add(30 * time.Second)
				}
			}
			return nil
		}
		return nil
	})
}
func (q *asyncQueue) Cancel(ctx context.Context, id string) error {
	_, err := q.cancel(ctx, func(row asyncRecord) bool { return row.Job.RequestID == id && row.Status != queueComplete }, false)
	return err
}
func (q *asyncQueue) CancelScope(ctx context.Context, scope flowrecall.Scope) (int, error) {
	return q.cancel(ctx, func(row asyncRecord) bool { return sameScope(row.Job.Scope, scope) && row.Status != queueComplete }, false)
}
func (q *asyncQueue) PurgeScope(ctx context.Context, scope flowrecall.Scope) (int, error) {
	return q.cancel(ctx, func(row asyncRecord) bool { return sameScope(row.Job.Scope, scope) }, true)
}
func (q *asyncQueue) CancelMatchingEpisodes(ctx context.Context, scope flowrecall.Scope, ids []string) (int, error) {
	targets := map[string]bool{}
	for _, id := range ids {
		targets[id] = true
	}
	return q.cancel(ctx, func(row asyncRecord) bool {
		if !sameScope(row.Job.Scope, scope) || row.Status == queueComplete {
			return false
		}
		for _, id := range row.Job.EpisodeFactIDs {
			if targets[id] {
				return true
			}
		}
		return false
	}, false)
}
func (q *asyncQueue) cancel(ctx context.Context, match func(asyncRecord) bool, purge bool) (count int, err error) {
	err = q.state.update(ctx, func(state *durableState) error {
		out := state.Async[:0]
		for _, row := range state.Async {
			if match(row) {
				count++
				if !purge {
					counter := state.Counters[row.Job.Scope.PartitionKey()]
					counter.Async++
					state.Counters[row.Job.Scope.PartitionKey()] = counter
				}
				continue
			}
			out = append(out, row)
		}
		state.Async = out
		return nil
	})
	return count, err
}
func (q *asyncQueue) Stats(ctx context.Context, filter flowrecall.AsyncSemanticStatsFilter) (flowrecall.AsyncSemanticQueueStats, error) {
	if filter.Scope.PartitionKey() == "" {
		return flowrecall.AsyncSemanticQueueStats{}, errdefs.Validationf("flowcraft redis8 async queue: scope partition is required")
	}
	state, err := q.state.read(ctx)
	if err != nil {
		return flowrecall.AsyncSemanticQueueStats{}, err
	}
	now := filter.Now
	if now.IsZero() {
		now = time.Now()
	}
	out := flowrecall.AsyncSemanticQueueStats{CancelledTotal: state.Counters[filter.Scope.PartitionKey()].Async}
	for _, row := range state.Async {
		if !sameScope(row.Job.Scope, filter.Scope) {
			continue
		}
		switch row.Status {
		case queuePending:
			out.Pending++
		case queueLeased:
			out.Leased++
			if !row.Job.LeaseUntil.IsZero() && !now.Before(row.Job.LeaseUntil) {
				out.ExpiredLeases++
			}
		case queueFailed:
			out.Failed++
			if row.Failure.ErrClass == flowrecall.ErrClassPermanent {
				out.DeadLetter++
			}
		case queueComplete:
			out.Completed++
		}
	}
	return out, nil
}

type sideEffectOutbox struct{ state *stateStore }

func (q *sideEffectOutbox) Enqueue(ctx context.Context, job flowrecall.SideEffectJob) error {
	if job.Scope.PartitionKey() == "" || job.RequestID == "" {
		return nil
	}
	if job.ID == "" {
		job.ID = fmt.Sprintf("%s|%s", job.RequestID, job.Kind)
	}
	return q.state.update(ctx, func(state *durableState) error {
		for _, row := range state.SideEffects {
			if row.Job.ID == job.ID {
				return nil
			}
		}
		state.SideEffects = append(state.SideEffects, sideEffectRecord{Job: cloneSideJob(job), Status: queuePending, EnqueuedAt: time.Now()})
		return nil
	})
}
func cloneSideJob(job flowrecall.SideEffectJob) flowrecall.SideEffectJob {
	out := job
	if len(job.Facts) > 0 {
		out.Facts = make([]flowrecall.TemporalFact, len(job.Facts))
		for i, fact := range job.Facts {
			out.Facts[i] = fact.Clone()
		}
	}
	return out
}
func (q *sideEffectOutbox) Claim(ctx context.Context, opts flowrecall.SideEffectClaimOptions) (jobs []flowrecall.SideEffectJob, err error) {
	if opts.Max <= 0 || opts.Scope.PartitionKey() == "" {
		return nil, nil
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	err = q.state.update(ctx, func(state *durableState) error {
		for i := range state.SideEffects {
			row := &state.SideEffects[i]
			if row.Status == queueLeased && !row.Job.LeaseUntil.IsZero() && !now.Before(row.Job.LeaseUntil) {
				row.Status = queuePending
				row.Job.LeaseUntil = time.Time{}
				row.Job.LeaseToken = ""
			}
		}
		sort.SliceStable(state.SideEffects, func(i, j int) bool { return state.SideEffects[i].EnqueuedAt.Before(state.SideEffects[j].EnqueuedAt) })
		for i := range state.SideEffects {
			if len(jobs) >= opts.Max {
				break
			}
			row := &state.SideEffects[i]
			if row.Status != queuePending || !sameScope(row.Job.Scope, opts.Scope) || (!row.RetryAt.IsZero() && now.Before(row.RetryAt)) {
				continue
			}
			row.Status = queueLeased
			row.Job.Attempt++
			row.Job.LeaseToken = leaseToken()
			row.Job.LeaseUntil = now.Add(30 * time.Second)
			jobs = append(jobs, cloneSideJob(row.Job))
		}
		return nil
	})
	return jobs, err
}
func (q *sideEffectOutbox) Complete(ctx context.Context, id, token string, result flowrecall.SideEffectResult) error {
	return q.ack(ctx, id, token, func(row *sideEffectRecord) { row.Status = queueComplete; row.Result = result; scrubSideJob(&row.Job) })
}
func (q *sideEffectOutbox) Fail(ctx context.Context, id, token string, failure flowrecall.SideEffectFailure) error {
	return q.ack(ctx, id, token, func(row *sideEffectRecord) {
		row.Failure = failure
		if failure.ErrClass == flowrecall.ErrClassPermanent {
			row.Status = queueFailed
			scrubSideJob(&row.Job)
		} else {
			row.Status = queuePending
			row.RetryAt = failure.RetryAt
		}
	})
}
func (q *sideEffectOutbox) ack(ctx context.Context, id, token string, apply func(*sideEffectRecord)) error {
	return q.state.update(ctx, func(state *durableState) error {
		for i := range state.SideEffects {
			row := &state.SideEffects[i]
			if row.Job.ID == id && row.Status == queueLeased && token != "" && row.Job.LeaseToken == token {
				row.Job.LeaseToken = ""
				row.Job.LeaseUntil = time.Time{}
				apply(row)
				return nil
			}
		}
		return nil
	})
}
func scrubSideJob(job *flowrecall.SideEffectJob) {
	facts := make([]flowrecall.TemporalFact, 0, len(job.Facts))
	for _, fact := range job.Facts {
		facts = append(facts, flowrecall.TemporalFact{ID: fact.ID, Scope: fact.Scope, Kind: fact.Kind})
	}
	job.Facts = facts
}
func (q *sideEffectOutbox) Cancel(ctx context.Context, requestID string) error {
	_, err := q.remove(ctx, func(row sideEffectRecord) bool { return row.Job.RequestID == requestID && row.Status != queueComplete }, false)
	return err
}
func (q *sideEffectOutbox) CancelScope(ctx context.Context, scope flowrecall.Scope) (int, error) {
	return q.remove(ctx, func(row sideEffectRecord) bool { return sameScope(row.Job.Scope, scope) && row.Status != queueComplete }, false)
}
func (q *sideEffectOutbox) PurgeScope(ctx context.Context, scope flowrecall.Scope) (int, error) {
	return q.remove(ctx, func(row sideEffectRecord) bool { return sameScope(row.Job.Scope, scope) }, true)
}
func (q *sideEffectOutbox) remove(ctx context.Context, match func(sideEffectRecord) bool, purge bool) (count int, err error) {
	err = q.state.update(ctx, func(state *durableState) error {
		out := state.SideEffects[:0]
		for _, row := range state.SideEffects {
			if match(row) {
				count++
				if !purge {
					c := state.Counters[row.Job.Scope.PartitionKey()]
					c.SideEffect++
					state.Counters[row.Job.Scope.PartitionKey()] = c
				}
				continue
			}
			out = append(out, row)
		}
		state.SideEffects = out
		return nil
	})
	return count, err
}
func (q *sideEffectOutbox) Stats(ctx context.Context, scope flowrecall.Scope, now time.Time) (flowrecall.SideEffectOutboxStats, error) {
	if scope.PartitionKey() == "" {
		return flowrecall.SideEffectOutboxStats{}, errdefs.Validationf("flowcraft redis8 side effects: scope partition is required")
	}
	if now.IsZero() {
		now = time.Now()
	}
	state, err := q.state.read(ctx)
	if err != nil {
		return flowrecall.SideEffectOutboxStats{}, err
	}
	out := flowrecall.SideEffectOutboxStats{CancelledTotal: state.Counters[scope.PartitionKey()].SideEffect}
	for _, row := range state.SideEffects {
		if !sameScope(row.Job.Scope, scope) {
			continue
		}
		switch row.Status {
		case queuePending:
			out.Pending++
		case queueLeased:
			out.Leased++
			if !row.Job.LeaseUntil.IsZero() && !now.Before(row.Job.LeaseUntil) {
				out.ExpiredLeases++
			}
		case queueFailed:
			out.Failed++
			if row.Failure.ErrClass == flowrecall.ErrClassPermanent {
				out.DeadLetter++
			}
		case queueComplete:
			out.Completed++
		}
	}
	return out, nil
}

var _ flowrecall.AsyncSemanticQueue = (*asyncQueue)(nil)
var _ flowrecall.SideEffectOutbox = (*sideEffectOutbox)(nil)
