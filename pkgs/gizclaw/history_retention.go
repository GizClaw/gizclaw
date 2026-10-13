package gizclaw

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
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

func newHistoryRetention(assets objectstore.ObjectStore, stores ...memory.HistoryStore) *historyRetention {
	r := &historyRetention{interval: time.Hour}
	for _, store := range stores {
		if maintenance, ok := store.(historyMaintainer); ok {
			r.stores = append(r.stores, historyAssetMaintenance{historyMaintainer: maintenance, store: store, assets: assets})
		}
	}
	if len(r.stores) == 0 {
		return nil
	}
	return r
}

type historyAssetMaintenance struct {
	historyMaintainer
	store  memory.HistoryStore
	assets objectstore.ObjectStore
}

func (m historyAssetMaintenance) Maintain(ctx context.Context) error {
	if err := m.historyMaintainer.Maintain(ctx); err != nil {
		return err
	}
	var deleteAsset func(memory.HistoryKey, memory.HistoryAsset) error
	if m.assets != nil {
		deleteAsset = func(_ memory.HistoryKey, asset memory.HistoryAsset) error {
			err := m.assets.Delete(asset.Name)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
	}
	return memory.CleanupHistory(ctx, m.store, memory.HistoryCleanupQuery{Before: time.Now().UTC()}, deleteAsset)
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
