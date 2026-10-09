package gizclaw

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type historyMaintainer interface {
	Maintain(context.Context) error
}

// The Server owns this worker and stops it before the host closes borrowed
// stores/pools. Store constructors never start maintenance goroutines.
type historyRetention struct {
	interval time.Duration
	stores   []historyMaintainer
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
}

func newHistoryRetention(stores ...logstore.ImmutableStore) *historyRetention {
	r := &historyRetention{interval: time.Hour}
	for _, store := range stores {
		if maintenance, ok := store.(historyMaintainer); ok {
			r.stores = append(r.stores, maintenance)
		}
	}
	if len(r.stores) == 0 {
		return nil
	}
	return r
}

func (r *historyRetention) start(parent context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.done = make(chan struct{})
	go r.run(ctx, r.done)
}

func (r *historyRetention) close() {
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

func (r *historyRetention) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	for ctx.Err() == nil {
		for _, store := range r.stores {
			attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := store.Maintain(attempt)
			cancel()
			if err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "gizclaw: history retention failed", "error", err)
			}
		}
		timer := time.NewTimer(r.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
