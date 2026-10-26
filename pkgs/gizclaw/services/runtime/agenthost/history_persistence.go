package agenthost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const (
	historyPersistenceMaxEntries = 64
	historyPersistenceMaxBytes   = 16 << 20
)

var errHistoryPersistenceBacklog = errors.New("agenthost: History persistence backlog full")

type historyAsyncPersistenceKey struct{}

type historyPersistJob struct {
	entry   *historyPendingEntry
	forward func() error
	key     string
	bytes   int64
	done    chan struct{}
	err     error // published by closing done
}

// historyPersistence has one ordered writer per Transform. Recorder.mu owns
// admission, counters and channel closure; storage runs without that mutex.
type historyPersistence struct {
	recorder *historyRecorder
	ctx      context.Context
	cancel   context.CancelFunc
	jobs     chan *historyPersistJob
	done     chan struct{}
	closed   bool
	sequence uint64
	entries  int
	bytes    int64
}

func (r *historyRecorder) startPersistence(ctx context.Context) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p := &historyPersistence{recorder: r, ctx: ctx, cancel: cancel, jobs: make(chan *historyPersistJob, historyPersistenceMaxEntries), done: make(chan struct{})}
	r.persistence = p
	go p.run()
}

func (r *historyRecorder) outputPersistenceContext(ctx context.Context) context.Context {
	if r == nil || r.persistence == nil {
		return ctx
	}
	return context.WithValue(ctx, historyAsyncPersistenceKey{}, true)
}

// enqueueLocked transfers a completed entry, including its read barrier, to
// the writer before another producer can enqueue a later entry.
func (p *historyPersistence) enqueueLocked(key string, entry *historyPendingEntry) (*historyPersistJob, error) {
	p.sequence++
	job := &historyPersistJob{entry: entry, key: key + "\x00persist:" + strconv.FormatUint(p.sequence, 10), bytes: int64(entry.text.Len()) + entry.audioBytes, done: make(chan struct{})}
	if excluded, ok := p.recorder.pendingText[key]; ok {
		delete(p.recorder.pendingText, key)
		p.recorder.pendingText[job.key] = excluded
	}
	return job, p.admitLocked(job)
}

func (p *historyPersistence) admitLocked(job *historyPersistJob) error {
	if p.closed {
		return io.ErrClosedPipe
	}
	// One entry that was already buffered may exceed the backlog budget. This
	// bounds queued work, without introducing a maximum reply/record length.
	if p.entries >= historyPersistenceMaxEntries || job.bytes > 0 && p.entries > 0 && p.bytes+job.bytes > historyPersistenceMaxBytes {
		return errHistoryPersistenceBacklog
	}
	p.entries++
	p.bytes += job.bytes
	select {
	case p.jobs <- job:
		return nil
	default:
		p.entries--
		p.bytes -= job.bytes
		return errHistoryPersistenceBacklog
	}
}

func (p *historyPersistence) forwardAfterWrites(forward func() error) error {
	job := &historyPersistJob{forward: forward, done: make(chan struct{})}
	p.recorder.mu.Lock()
	err := p.admitLocked(job)
	p.recorder.mu.Unlock()
	return err
}

func (p *historyPersistence) run() {
	defer close(p.done)
	for job := range p.jobs {
		err := p.ctx.Err()
		if job.forward != nil {
			if err == nil {
				err = job.forward()
			}
		} else if err == nil {
			err = p.recorder.persistEntry(p.ctx, job.entry)
		} else {
			job.entry.closeAudio()
		}
		if err != nil && job.entry != nil {
			logHistoryRecordError(p.ctx, "persist output", err)
		}
		p.recorder.mu.Lock()
		p.entries--
		p.bytes -= job.bytes
		p.recorder.mu.Unlock()
		if job.entry != nil {
			p.recorder.finishText(job.key, err)
		}
		job.err = err
		close(job.done)
	}
}

func (p *historyPersistence) abort() {
	p.recorder.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.jobs)
	}
	p.recorder.mu.Unlock()
	p.cancel()
}

func (p *historyPersistence) close(ctx context.Context) {
	p.recorder.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.jobs)
	}
	p.recorder.mu.Unlock()
	select {
	case <-p.done:
	case <-ctx.Done():
		p.cancel()
		<-p.done
	}
	p.cancel()
}

func (r *historyRecorder) persistCompleted(ctx context.Context, entry *historyPendingEntry, job *historyPersistJob, enqueueErr error) error {
	if enqueueErr != nil {
		entry.closeAudio()
		r.finishText(job.key, enqueueErr)
		return enqueueErr
	}
	if async, _ := ctx.Value(historyAsyncPersistenceKey{}).(bool); async {
		return nil
	}
	select {
	case <-job.done:
		return job.err
	case <-ctx.Done():
		return fmt.Errorf("agenthost: wait for History persistence: %w", context.Cause(ctx))
	}
}
