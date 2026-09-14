package gizlog

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"runtime"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

const storeQueueCapacity = 1024
const storeWriteTimeout = 5 * time.Second

var errStoreQueueClosed = errors.New("system log store queue is closed")
var errStoreQueueWrite = errors.New("system log store write failed")

type storeJob struct {
	store   logstore.Appender
	name    string
	records []logstore.Record
}

// storeQueue owns one ordered worker for all configured Store sinks. Projection
// happens on the caller before enqueue, preserving caller metadata and values.
// A full queue applies backpressure; accepted records are never silently evicted.
type storeQueue struct {
	jobs      chan storeJob
	stopping  chan struct{}
	done      chan struct{}
	fallback  slog.Handler
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	producers sync.WaitGroup
	closeOnce sync.Once
	timeout   time.Duration
	err       error // worker-owned until done closes
}

func newStoreQueue(fallback slog.Handler) *storeQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &storeQueue{jobs: make(chan storeJob, storeQueueCapacity), stopping: make(chan struct{}), done: make(chan struct{}), fallback: fallback, ctx: ctx, cancel: cancel, timeout: storeWriteTimeout}
}

func (q *storeQueue) enqueue(ctx context.Context, job storeJob) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errStoreQueueClosed
	}
	q.producers.Add(1)
	q.mu.Unlock()
	defer q.producers.Done()
	select {
	case q.jobs <- job:
		return nil
	case <-q.stopping:
		return errStoreQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *storeQueue) run() {
	defer close(q.done)
	var pending *storeJob
	for {
		var job storeJob
		if pending != nil {
			job = *pending
			pending = nil
		} else {
			next, ok := <-q.jobs
			if !ok {
				return
			}
			job = next
		}
		// Batch only adjacent writes for the same configured sink. Never wait to
		// fill a batch or reorder writes across differently configured sinks.
	collect:
		for len(job.records) < 64 {
			select {
			case next, ok := <-q.jobs:
				if !ok {
					break collect
				}
				if next.name != job.name {
					pending = &next
					break collect
				}
				job.records = append(job.records, next.records...)
			default:
				break collect
			}
		}
		ctx, cancel := context.WithTimeout(q.ctx, q.timeout)
		remaining, err := appendStoreRecords(ctx, job.store, job.records)
		cancel()
		if err != nil {
			q.err = errStoreQueueWrite
			// Never copy provider errors or conversation payloads to the fallback.
			var pcs [1]uintptr
			runtime.Callers(1, pcs[:])
			failure := slog.NewRecord(time.Now(), slog.LevelError, "system log store sink failed", pcs[0])
			failure.AddAttrs(slog.String("store", job.name), slog.Int("records", len(remaining)))
			_ = q.fallback.Handle(context.Background(), failure)
		}
	}
}

// appendStoreRecords retries only the unaccepted suffix under the batch's
// original deadline. A provider error never exposes payloads to the fallback.
func appendStoreRecords(ctx context.Context, store logstore.Appender, records []logstore.Record) ([]logstore.Record, error) {
	for len(records) > 0 {
		if err := ctx.Err(); err != nil {
			return records, err
		}
		keys, err := store.Append(ctx, records)
		if len(keys) > len(records) {
			return records, errStoreQueueWrite
		}
		for i, key := range keys {
			if key != records[i].Key() {
				return records, errStoreQueueWrite
			}
		}
		records = records[len(keys):]
		if len(records) == 0 {
			return nil, err
		}
		if err != nil || len(keys) == 0 {
			// Bound retry pressure when the provider fails or makes no progress.
			timer := time.NewTimer(10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return records, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, nil
}

func (q *storeQueue) close() error {
	q.closeOnce.Do(func() {
		q.mu.Lock()
		q.closed = true
		close(q.stopping)
		q.mu.Unlock()
		q.producers.Wait()
		close(q.jobs)
		// Store implementations must honor their context. One deadline covers the
		// entire drain, rather than allowing every queued write another five seconds.
		timer := time.AfterFunc(q.timeout, q.cancel)
		<-q.done
		timer.Stop()
		q.cancel()
	})
	return q.err
}

type queuedStoreAppender struct {
	queue *storeQueue
	store logstore.Appender
	name  string
}

func (a queuedStoreAppender) Append(ctx context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	owned := make([]logstore.Record, len(records))
	keys := make([]logstore.RecordKey, len(records))
	for i, record := range records {
		owned[i] = record
		owned[i].Attributes = maps.Clone(record.Attributes)
		keys[i] = record.Key()
	}
	if err := a.queue.enqueue(ctx, storeJob{store: a.store, name: a.name, records: owned}); err != nil {
		return nil, err
	}
	return keys, nil
}
