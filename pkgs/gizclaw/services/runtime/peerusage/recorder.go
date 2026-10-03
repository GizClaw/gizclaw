package peerusage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type snapshotWriter interface {
	Write(context.Context, []Snapshot) error
	Maintain(context.Context) error
}

type bucketKey struct {
	peer  giznet.PublicKey
	model string
	hour  time.Time
}
type bucketCounter struct {
	writer                 string
	quantity, acknowledged int64
}

// Recorder coalesces provider quantities in memory and flushes cumulative
// snapshots. Reports are durable only after Flush succeeds; process crashes
// can lose reports in the one-second volatile window or during an outage.
type Recorder struct {
	store           snapshotWriter
	reader          *Store
	identity        string
	Now             func() time.Time
	mu              sync.Mutex
	buckets         map[bucketKey]*bucketCounter
	sequence        uint64
	closed, started bool
	cancel          context.CancelFunc
	done            chan struct{}
	gate            chan struct{}
	rejected        atomic.Uint64
}

// NewRecorder constructs an idle recorder. Start explicitly owns its worker.
func NewRecorder(store *Store) *Recorder {
	r := newRecorder(store)
	r.reader = store
	return r
}

// Hourly flushes pending reports and returns this Peer's retained hourly usage.
func (r *Recorder) Hourly(ctx context.Context, peer giznet.PublicKey) ([]HourlyUsage, error) {
	if r == nil || r.reader == nil {
		return nil, errors.New("peerusage: SQL reader is not configured")
	}
	if err := r.Flush(ctx); err != nil {
		return nil, err
	}
	now := r.now().Truncate(time.Hour)
	return r.reader.Query(ctx, peer, "", now.Add(-Retention), now.Add(time.Hour))
}
func newRecorder(store snapshotWriter) *Recorder {
	return &Recorder{store: store, identity: rand.Text(), buckets: make(map[bucketKey]*bucketCounter), done: make(chan struct{}), gate: make(chan struct{}, 1)}
}
func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Handler binds a Peer to the provider's actual billing model/resource ID.
// Rejected records are counted and reported by the worker without blocking
// provider goroutines on logging or database I/O.
func (r *Recorder) Handler(peer giznet.PublicKey) genx.UsageRecorder {
	return func(record genx.UsageRecord) {
		if err := r.Record(peer, record.Model, record); err != nil {
			r.rejected.Add(1)
		}
	}
}

// Record performs no database or network I/O. Counts and their sum must be
// nonnegative int64 quantities; modality is deliberately not retained.
func (r *Recorder) Record(peer giznet.PublicKey, modelID string, record genx.UsageRecord) error {
	if r == nil || r.store == nil {
		return errors.New("peerusage: recorder is not configured")
	}
	if peer.IsZero() || modelID == "" || len(modelID) > 256 {
		return errors.New("peerusage: invalid recorder identity")
	}
	var total int64
	for _, count := range []int64{record.Input, record.CachedInput, record.Output} {
		if count < 0 || count > math.MaxInt64-total {
			return errors.New("peerusage: invalid or overflowing provider quantity")
		}
		total += count
	}
	if total == 0 {
		return nil
	}
	hour := r.now().Truncate(time.Hour)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("peerusage: recorder is closed")
	}
	key := bucketKey{peer: peer, model: modelID, hour: hour}
	counter := r.buckets[key]
	if counter == nil {
		r.sequence++
		counter = &bucketCounter{writer: fmt.Sprintf("%s-%d", r.identity, r.sequence)}
		r.buckets[key] = counter
	}
	if total > math.MaxInt64-counter.quantity {
		return errors.New("peerusage: hourly quantity overflow")
	}
	counter.quantity += total
	return nil
}

// Start launches one owned worker; repeated starts are harmless.
func (r *Recorder) Start(parent context.Context) {
	r.mu.Lock()
	if r.started || r.closed {
		r.mu.Unlock()
		return
	}
	r.started = true
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.mu.Unlock()
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		maintenance := time.NewTicker(time.Hour)
		defer maintenance.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if rejected := r.rejected.Swap(0); rejected > 0 {
					slog.ErrorContext(ctx, "peerusage: rejected invalid or closed usage reports", "count", rejected)
				}
				attempt, done := context.WithTimeout(ctx, 10*time.Second)
				err := r.Flush(attempt)
				done()
				if err != nil && ctx.Err() == nil {
					slog.ErrorContext(ctx, "peerusage: hourly flush failed", "error", err)
				}
			case <-maintenance.C:
				attempt, done := context.WithTimeout(ctx, 10*time.Second)
				err := r.store.Maintain(attempt)
				done()
				if err != nil && ctx.Err() == nil {
					slog.ErrorContext(ctx, "peerusage: retention maintenance failed", "error", err)
				}
			}
		}
	}()
}

// Flush retries the same cumulative snapshots after any database error. The
// operation gate coordinates drains without holding the state mutex over I/O.
func (r *Recorder) Flush(ctx context.Context) error {
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	cutoff := r.now().Truncate(time.Hour).Add(-Retention)
	r.mu.Lock()
	var items []Snapshot
	for key, counter := range r.buckets {
		if key.hour.Before(cutoff) {
			delete(r.buckets, key)
			continue
		}
		if counter.quantity != counter.acknowledged {
			items = append(items, Snapshot{Peer: key.peer, ModelID: key.model, Hour: key.hour, WriterID: counter.writer, Quantity: counter.quantity})
		}
	}
	r.mu.Unlock()
	if len(items) == 0 {
		return nil
	}
	if err := r.store.Write(ctx, items); err != nil {
		return err
	}
	hour := r.now().Truncate(time.Hour)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range items {
		key := bucketKey{peer: item.Peer, model: item.ModelID, hour: item.Hour}
		counter := r.buckets[key]
		if counter == nil || counter.writer != item.WriterID {
			continue
		}
		counter.acknowledged = max(counter.acknowledged, item.Quantity)
	}
	for key, counter := range r.buckets {
		if key.hour.Before(hour) && counter.quantity == counter.acknowledged {
			delete(r.buckets, key)
		}
	}
	return nil
}

// Close stops reports, cancels and joins the worker, and flushes outstanding
// snapshots with the caller's bounded context. Failure is returned, not hidden.
func (r *Recorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closed = true
	cancel, started := r.cancel, r.started
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		select {
		case <-r.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.Flush(ctx)
}
