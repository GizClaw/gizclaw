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
	matched bool
	err     error
}

type transitionMutation struct {
	ctx      context.Context
	mutation kv.Mutation
	result   chan transitionResult
}

func (s *Service) applyTransition(ctx context.Context, mutation kv.Mutation) (bool, error) {
	request := transitionMutation{ctx: ctx, mutation: mutation, result: make(chan transitionResult, 1)}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case s.mutations <- request:
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case result := <-request.result:
		return result.matched, result.err
	}
}

// Two workers bound the number of competing generation-guard transactions,
// while allowing another batch to progress when one storage call stalls.
// The per-Peer writer still owns claim state and orders connection transitions.
func (s *Service) writeBatches() {
	defer s.workers.Done()
	for s.ctx.Err() == nil {
		var first transitionMutation
		select {
		case <-s.ctx.Done():
			return
		case first = <-s.mutations:
		}
		batch := []transitionMutation{first}
		timer := time.NewTimer(time.Millisecond)
	collect:
		for len(batch) < transitionBatchSize {
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case request := <-s.mutations:
				batch = append(batch, request)
			case <-timer.C:
				break collect
			}
		}
		timer.Stop()
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
		s.commitTransitions(ctx, active)
		cancel()
	}
}

func (s *Service) commitTransitions(ctx context.Context, batch []transitionMutation) {
	if len(batch) == 0 {
		return
	}
	mutation := kv.Mutation{Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}}}
	for _, request := range batch {
		// Only connection transitions enter this queue: the first condition is
		// the common generation guard, the second is this Peer's owner token.
		mutation.Conditions = append(mutation.Conditions, request.mutation.Conditions[1:]...)
		mutation.Entries = append(mutation.Entries, request.mutation.Entries...)
	}
	matched, err := s.store.ApplyMutation(ctx, mutation)
	if err == nil && !matched && len(batch) > 1 {
		generation, readErr := s.store.Get(kv.WithPrimaryReads(ctx), generationKey(s.server))
		if readErr != nil && !errors.Is(readErr, kv.ErrNotFound) {
			err = readErr
		} else if bytes.Equal(generation, []byte(s.generation)) {
			// A failed owner condition must not fence innocent publishers. Split
			// only definitively rejected transactions; never replay an uncertain
			// commit here. Each caller retains its original claim across errors.
			middle := len(batch) / 2
			s.commitTransitions(ctx, batch[:middle])
			s.commitTransitions(ctx, batch[middle:])
			return
		}
	}
	for _, request := range batch {
		request.result <- transitionResult{matched: matched, err: err}
	}
}
