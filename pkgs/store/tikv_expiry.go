package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Each registry TiKV root owns one cancellable maintenance worker. Physical
// Storage must outlive Stores, matching all other borrowed backend resources.
type tiKVExpiryWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startTiKVExpiryWorker(root *kv.TiKV) *tiKVExpiryWorker {
	ctx, cancel := context.WithCancel(context.Background())
	worker := &tiKVExpiryWorker{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(worker.done)
		for {
			delay := time.Minute
			run, cancelRun := context.WithTimeout(ctx, 15*time.Second)
			for range 32 {
				processed, err := root.PurgeExpired(run, 128)
				if err != nil {
					if ctx.Err() == nil && run.Err() == nil {
						slog.Error("store: TiKV expiry cleanup failed", "error", err)
					}
					break
				}
				if processed < 128 {
					delay = time.Minute
					break
				}
				delay = time.Second
			}
			cancelRun()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	return worker
}
func (w *tiKVExpiryWorker) Close() error { w.cancel(); <-w.done; return nil }
