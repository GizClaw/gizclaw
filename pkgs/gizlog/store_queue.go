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
const storeQueueWorkers = 4
const storeWriteTimeout = 5 * time.Second

var errStoreQueueClosed = errors.New("system log store queue is closed")
var errStoreQueueWrite = errors.New("system log store write failed")

// ErrStoreQueueFull reports that the bounded system logging queue rejected new
// records. Logger cleanup also returns this error if admission ever overflowed,
// even when every previously accepted record was subsequently persisted.
var ErrStoreQueueFull = errors.New("system log store queue is full")

type storeRejection struct {
	store  string
	reason string
}

type storeJob struct {
	store   logstore.Appender
	name    string
	records []logstore.Record
}

// storeQueue owns a bounded queue and fixed worker pool for configured Store
// sinks. Projection happens on the caller, preserving metadata and values.
// Admission never waits for the worker or fallback: a full queue rejects new
// records, while previously accepted records are never evicted to make room.
type storeQueue struct {
	jobs      chan storeJob
	stopping  chan struct{}
	done      chan struct{}
	fallback  slog.Handler
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	overflow  bool
	rejected  map[storeRejection]uint64
	timeout   time.Duration
	err       error // worker writes hold mu; cleanup reads after done closes
}

func newStoreQueue(fallback slog.Handler) *storeQueue {
	ctx, cancel := context.WithCancel(context.Background())
	return &storeQueue{jobs: make(chan storeJob, storeQueueCapacity), stopping: make(chan struct{}), done: make(chan struct{}), fallback: fallback, ctx: ctx, cancel: cancel, timeout: storeWriteTimeout, rejected: make(map[storeRejection]uint64)}
}

func (q *storeQueue) enqueue(_ context.Context, job storeJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errStoreQueueClosed
	}
	// Context supplies log metadata, not admission lifetime. In particular,
	// cancellation diagnostics may still be accepted after a request ends.
	// The mutex makes this nonblocking send atomic with closing jobs.
	select {
	case q.jobs <- job:
		return nil
	default:
		q.overflow = true
		q.rejected[storeRejection{store: job.name, reason: "full"}] += uint64(len(job.records))
		return ErrStoreQueueFull
	}
}

// rejectProjection records only counts and configured sink names. Reporting
// never runs on a producer, including standard log's globally locked writer.
func (q *storeQueue) rejectProjection(store string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.rejected[storeRejection{store: store, reason: "projection"}]++
	}
}

func (q *storeQueue) reportRejections() {
	q.mu.Lock()
	rejected := q.rejected
	q.rejected = make(map[storeRejection]uint64)
	q.mu.Unlock()
	for key, count := range rejected {
		var pcs [1]uintptr
		runtime.Callers(1, pcs[:])
		record := slog.NewRecord(time.Now(), slog.LevelError, "system log store records rejected", pcs[0])
		record.AddAttrs(slog.String("store", key.store), slog.String("reason", key.reason), slog.Uint64("records", count))
		_ = q.fallback.Handle(q.ctx, record)
	}
}

func (q *storeQueue) run() {
	defer close(q.done)
	defer q.reportRejections()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	batches := make(chan storeJob)
	var workers sync.WaitGroup
	for range storeQueueWorkers {
		workers.Go(func() {
			for job := range batches {
				q.writeBatch(job)
			}
		})
	}
	defer func() { close(batches); workers.Wait() }()
	var pending *storeJob
	for {
		var job storeJob
		if pending != nil {
			job = *pending
			pending = nil
		} else {
			select {
			case <-ticker.C:
				q.reportRejections()
				continue
			case next, ok := <-q.jobs:
				if !ok {
					return
				}
				job = next
			}
		}
		// Collect adjacent jobs before dispatch so each batch has one sink.
		// Batches can complete out of order across the fixed worker pool.
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
	dispatch:
		for {
			select {
			case batches <- job:
				break dispatch
			case <-ticker.C:
				q.reportRejections()
			}
		}
	}
}

func (q *storeQueue) writeBatch(job storeJob) {
	ctx, cancel := context.WithTimeout(q.ctx, q.timeout)
	remaining, err := appendStoreRecords(ctx, job.store, job.records)
	cancel()
	if err == nil {
		return
	}
	q.mu.Lock()
	q.err = errStoreQueueWrite
	q.mu.Unlock()
	// Never copy provider errors or conversation payloads to the fallback.
	var pcs [1]uintptr
	runtime.Callers(1, pcs[:])
	failure := slog.NewRecord(time.Now(), slog.LevelError, "system log store sink failed", pcs[0])
	failure.AddAttrs(slog.String("store", job.name), slog.Int("records", len(remaining)))
	_ = q.fallback.Handle(context.Background(), failure)
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
		close(q.jobs)
		q.mu.Unlock()
		// Store implementations must honor their context. One deadline covers the
		// entire drain, rather than allowing every queued write another five seconds.
		timer := time.AfterFunc(q.timeout, q.cancel)
		<-q.done
		timer.Stop()
		q.cancel()
		if q.overflow {
			q.err = errors.Join(q.err, ErrStoreQueueFull)
		}
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
