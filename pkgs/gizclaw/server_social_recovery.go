package gizclaw

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type socialRecoveryTask struct {
	kind      string
	reconcile func(context.Context) error
}

// socialRecovery owns one worker per Social intent kind. Persistent indexes
// remain the source of truth; a failed attempt is retried on the next pass.
type socialRecovery struct {
	interval time.Duration
	tasks    []socialRecoveryTask

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func (r *socialRecovery) start(parent context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.done = make(chan struct{})
	var workers sync.WaitGroup
	for _, task := range r.tasks {
		workers.Go(func() { r.run(ctx, task) })
	}
	go func(done chan struct{}) {
		workers.Wait()
		close(done)
	}(r.done)
}

func (r *socialRecovery) close() {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	r.mu.Lock()
	if r.done == done {
		r.cancel = nil
		r.done = nil
	}
	r.mu.Unlock()
}

func (r *socialRecovery) run(ctx context.Context, task socialRecoveryTask) {
	for ctx.Err() == nil {
		if err := task.reconcile(ctx); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "gizclaw: Social recovery pass failed", "work_kind", task.kind, "error", err)
		}
		// Wait after each pass, including a slow or failed one, so retries do
		// not accumulate ticker events or spin on a persistent bad record.
		timer := time.NewTimer(r.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
