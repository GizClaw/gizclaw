package peerpresence

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const transitionBatchSize = 128

type transitionResult struct {
	matched  bool
	err      error
	observed []kv.GetResult // Authority observations in request-condition order.
}

type transitionMutation struct {
	ctx      context.Context
	mutation kv.Mutation
	result   chan transitionResult
}

func (s *Service) applyTransition(ctx context.Context, mutation kv.Mutation) transitionResult {
	request := transitionMutation{ctx: ctx, mutation: mutation, result: make(chan transitionResult, 1)}
	select {
	case <-ctx.Done():
		return transitionResult{err: ctx.Err()}
	case s.mutations <- request:
	}
	select {
	case <-ctx.Done():
		return transitionResult{err: ctx.Err()}
	case result := <-request.result:
		return result
	}
}

// One ordinary commit at a time avoids contention on the Server generation
// guard. The second worker can bypass a stalled commit after a quarter of the
// existing operation budget; a blocked storage call cannot consume that entire
// budget for unrelated Peers. The per-Peer writer retains connection ordering.
func (s *Service) writeBatches() {
	defer s.workers.Done()
	for s.ctx.Err() == nil {
		var first transitionMutation
		select {
		case <-s.ctx.Done():
			return
		case first = <-s.mutations:
		}
		s.writeBatch(first)
	}
}

func (s *Service) writeBatch(first transitionMutation) {
	timer := time.NewTimer(writeTimeout / 4)
	defer timer.Stop()
	select {
	case s.commitSlot <- struct{}{}:
		defer func() { <-s.commitSlot }()
	case <-first.ctx.Done():
		first.result <- transitionResult{err: first.ctx.Err()}
		return
	case <-timer.C:
		// At most two workers exist, even when storage stalls. This fallback
		// does not change deadlines, guards or uncertain-commit handling.
	}
	batch := []transitionMutation{first}
collect:
	for len(batch) < transitionBatchSize {
		select {
		case <-s.ctx.Done():
			return
		case request := <-s.mutations:
			batch = append(batch, request)
		default:
			break collect
		}
	}
	active := batch[:0]
	deadline := time.Now().Add(writeTimeout)
	for _, request := range batch {
		if err := request.ctx.Err(); err != nil {
			request.result <- transitionResult{err: err}
			continue
		}
		if limit, ok := request.ctx.Deadline(); ok && limit.Before(deadline) {
			deadline = limit
		}
		active = append(active, request)
	}
	ctx, cancel := context.WithDeadline(s.ctx, deadline)
	defer cancel()
	s.commitTransitions(ctx, active)
}

func (s *Service) commitTransitions(ctx context.Context, batch []transitionMutation) {
	for len(batch) > 0 {
		if err := ctx.Err(); err != nil {
			completeTransitions(batch, false, err)
			return
		}
		mutation := kv.Mutation{Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}}}
		for _, request := range batch {
			// Only the Server generation guard is shared. First claims also
			// guard their reservation; retain every per-request condition.
			mutation.Conditions = append(mutation.Conditions, request.mutation.Conditions[1:]...)
			mutation.Entries = append(mutation.Entries, request.mutation.Entries...)
			mutation.DeleteKeys = append(mutation.DeleteKeys, request.mutation.DeleteKeys...)
		}
		matched, err := s.store.ApplyMutation(ctx, mutation)
		if matched || err != nil || len(batch) == 1 {
			// Never replay an uncertain commit. The connection writer retains
			// its original claim condition when it retries a storage error.
			completeTransitions(batch, matched, err)
			return
		}
		// The rejected transaction wrote nothing. Identify changed conditions
		// in one authority read rather than one transaction per stale owner.
		// These reads only select retries; every retry keeps all atomic guards.
		keys := []kv.Key{generationKey(s.server)}
		for _, request := range batch {
			for _, condition := range request.mutation.Conditions[1:] {
				keys = append(keys, condition.Key)
			}
		}
		values := s.store.BatchGet(kv.WithPrimaryReads(ctx), keys)
		generation := values[0]
		if generation.Err != nil && !errors.Is(generation.Err, kv.ErrNotFound) {
			completeTransitions(batch, false, generation.Err)
			return
		}
		if generation.Err != nil || !bytes.Equal(generation.Value, []byte(s.generation)) {
			for _, request := range batch {
				request.result <- transitionResult{observed: []kv.GetResult{generation}}
			}
			return
		}
		active := make([]transitionMutation, 0, len(batch))
		offset := 1
		for _, request := range batch {
			end := offset + len(request.mutation.Conditions) - 1
			observed := append([]kv.GetResult{generation}, values[offset:end]...)
			offset = end
			matched := true
			var err error
			for i, value := range observed[1:] {
				absent := errors.Is(value.Err, kv.ErrNotFound)
				if value.Err != nil && !absent {
					err = value.Err
					break
				}
				expected := request.mutation.Conditions[i+1].Expected
				if !(expected == nil && absent || expected != nil && value.Err == nil && bytes.Equal(value.Value, expected)) {
					matched = false
				}
			}
			if err != nil {
				request.result <- transitionResult{err: err}
			} else if matched {
				active = append(active, request)
			} else {
				request.result <- transitionResult{observed: observed}
			}
		}
		batch = active
	}
}

func completeTransitions(batch []transitionMutation, matched bool, err error) {
	for _, request := range batch {
		request.result <- transitionResult{matched: matched, err: err}
	}
}
