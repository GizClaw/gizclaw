package metrics

import (
	"context"
	"errors"
	"maps"
	"sync/atomic"
	"time"
)

const (
	prometheusBatchDelay      = 5 * time.Millisecond
	prometheusBatchSamples    = 4096
	prometheusPendingSamples  = 16384
	prometheusPendingBytes    = 8 << 20
	prometheusPendingRequests = 256
)

var errPrometheusClosed = errors.New("metrics: prometheus store is closed")

type prometheusAppend struct {
	ctx     context.Context
	samples []Sample
	bytes   int
	done    chan error
}

// Append waits until the remote-write backend accepts its samples. Concurrent
// calls are combined into bounded batches; admission applies backpressure and
// respects cancellation. A canceled call may already have reached the backend,
// just as with an individual HTTP write. Writes are never automatically retried.
func (s *PrometheusStore) Append(ctx context.Context, samples []Sample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}
	size := 0
	for _, sample := range samples {
		if err := validateSample(sample); err != nil {
			return err
		}
		size += len(sample.Name) + 32
		for key, value := range sample.Labels {
			size += len(key) + len(value) + 32
		}
	}
	request := &prometheusAppend{ctx: ctx, bytes: size, done: make(chan error, 1)}
	for {
		s.batchMu.Lock()
		select {
		case <-s.closed:
			s.batchMu.Unlock()
			return errPrometheusClosed
		default:
		}
		// A single oversized caller batch is admitted alone to preserve the public
		// Append contract; it cannot share the pending budget with other requests.
		fits := s.pendingSamples == 0 || (s.pendingSamples+len(samples) <= prometheusPendingSamples && s.pendingBytes+size <= prometheusPendingBytes && len(s.pending) < prometheusPendingRequests)
		if fits {
			request.samples = make([]Sample, len(samples))
			for i, sample := range samples {
				request.samples[i] = sample
				request.samples[i].Labels = maps.Clone(sample.Labels)
			}
			s.pending = append(s.pending, request)
			s.pendingSamples += len(samples)
			s.pendingBytes += size
			if s.workerDone == nil {
				s.workerDone = make(chan struct{})
				go s.runAppendBatches(s.workerDone)
			}
			s.batchMu.Unlock()
			break
		}
		changed := s.changed
		s.batchMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closed:
			return errPrometheusClosed
		case <-changed:
		}
	}
	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return errPrometheusClosed
	}
}

// The worker starts only on demand and exits when no requests remain. There is
// no constructor-owned or permanently idle background goroutine.
func (s *PrometheusStore) runAppendBatches(done chan struct{}) {
	defer close(done)
	for {
		timer := time.NewTimer(prometheusBatchDelay)
		select {
		case <-timer.C:
		case <-s.closed:
		}
		timer.Stop()
		s.batchMu.Lock()
		count, samples := 0, 0
		for _, request := range s.pending {
			if count > 0 && (samples+len(request.samples) > prometheusBatchSamples || count >= 128) {
				break
			}
			count++
			samples += len(request.samples)
		}
		batch := append([]*prometheusAppend(nil), s.pending[:count]...)
		clear(s.pending[:count])
		s.pending = s.pending[count:]
		s.batchMu.Unlock()
		s.writeAppendBatch(batch)
		s.batchMu.Lock()
		for _, request := range batch {
			s.pendingSamples -= len(request.samples)
			s.pendingBytes -= request.bytes
		}
		close(s.changed)
		s.changed = make(chan struct{})
		if len(s.pending) == 0 {
			s.workerDone = nil
			s.batchMu.Unlock()
			return
		}
		s.batchMu.Unlock()
	}
}

func (s *PrometheusStore) writeAppendBatch(batch []*prometheusAppend) {
	active := make([]*prometheusAppend, 0, len(batch))
	var samples []Sample
	for _, request := range batch {
		select {
		case <-s.closed:
			request.done <- errPrometheusClosed
			continue
		default:
		}
		if err := request.ctx.Err(); err != nil {
			request.done <- err
			continue
		}
		active = append(active, request)
		samples = append(samples, request.samples...)
	}
	if len(active) == 0 {
		return
	}
	// One caller's deadline must not abort another caller's write. Propagate
	// context values from the first caller; cancel when all callers have left,
	// when the store closes, or when the bounded backend operation times out.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(active[0].ctx), 30*time.Second)
	defer cancel()
	var remaining atomic.Int32
	remaining.Store(int32(len(active)))
	stops := make([]func() bool, 0, len(active))
	for _, request := range active {
		stops = append(stops, context.AfterFunc(request.ctx, func() {
			if remaining.Add(-1) == 0 {
				cancel()
			}
		}))
	}
	finished := make(chan struct{})
	go func() {
		select {
		case <-s.closed:
			cancel()
		case <-finished:
		}
	}()
	err := s.appendRemote(ctx, samples)
	close(finished)
	for _, stop := range stops {
		stop()
	}
	for _, request := range active {
		result := err
		if canceled := request.ctx.Err(); canceled != nil {
			result = canceled
		}
		select {
		case <-s.closed:
			result = errPrometheusClosed
		default:
		}
		request.done <- result
	}
}
