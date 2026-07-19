package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
)

// Wait drains caller-owned Flowcraft async work until the requested operation
// reaches a terminal state or the context ends.
func (s *FlowcraftStore) Wait(ctx context.Context, operationID string) (ObserveResult, error) {
	select {
	case <-ctx.Done():
		return ObserveResult{}, ctx.Err()
	case <-s.waitGate:
	}
	defer func() { s.waitGate <- struct{}{} }()

	s.mu.Lock()
	known, ok := s.operations[operationID]
	s.mu.Unlock()
	if !ok {
		return ObserveResult{}, fmt.Errorf("%w: flowcraft operation %q", ErrNotFound, operationID)
	}
	if known.Operation == nil || known.Operation.Status != OperationPending {
		return cloneObserveResult(known), nil
	}
	processor, ok := recall.NewAsyncSemanticProcessor(s.memory)
	if !ok {
		return ObserveResult{}, fmt.Errorf("%w: flowcraft async processor is unavailable", ErrUnavailable)
	}
	for {
		result, err := processor.ProcessAsyncSemantic(ctx, recall.AsyncSemanticProcessOptions{
			Scope: s.scope, WorkerID: s.config.Async.WorkerID, Limit: 1,
		})
		if err != nil {
			return ObserveResult{}, mapFlowcraftError("wait", err)
		}
		if result.Claimed == 0 {
			if err := waitFlowcraftRetry(ctx); err != nil {
				return ObserveResult{}, err
			}
			continue
		}
		if result.Failed > 0 {
			if err := waitFlowcraftRetry(ctx); err != nil {
				return ObserveResult{}, err
			}
			continue
		}
		if err := s.finishPending(ctx, result.Completed); err != nil {
			return ObserveResult{}, err
		}
		if err := s.drainSideEffects(ctx); err != nil {
			return ObserveResult{}, err
		}
		s.mu.Lock()
		current := cloneObserveResult(s.operations[operationID])
		s.mu.Unlock()
		if current.Operation != nil && current.Operation.Status != OperationPending {
			return current, nil
		}
	}
}

func waitFlowcraftRetry(ctx context.Context) error {
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *FlowcraftStore) finishPending(ctx context.Context, completed int) error {
	s.mu.Lock()
	if completed > len(s.pending) {
		s.mu.Unlock()
		return fmt.Errorf("%w: flowcraft completed unknown async operations", ErrUnavailable)
	}
	completedIDs := append([]string(nil), s.pending[:completed]...)
	s.mu.Unlock()

	results := make(map[string]ObserveResult, len(completedIDs))
	for _, id := range completedIDs {
		nativeFacts, err := s.temporal.FindByOriginRequestID(ctx, s.scope, id)
		if err != nil {
			return mapFlowcraftError("load async facts", err)
		}
		facts := make([]Fact, 0, len(nativeFacts))
		for _, nativeFact := range nativeFacts {
			if nativeFact.Kind == recall.FactEpisode {
				continue
			}
			fact, err := s.factFromFlowcraft(ctx, nativeFact)
			if err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		results[id] = ObserveResult{Facts: facts, Operation: &Operation{ID: id, Status: OperationSucceeded}}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) < len(completedIDs) {
		return fmt.Errorf("%w: flowcraft async operation order changed", ErrUnavailable)
	}
	for index, id := range completedIDs {
		if s.pending[index] != id {
			return fmt.Errorf("%w: flowcraft async operation order changed", ErrUnavailable)
		}
		s.operations[id] = results[id]
	}
	s.pending = append([]string(nil), s.pending[len(completedIDs):]...)
	return nil
}

func (s *FlowcraftStore) drainSideEffects(ctx context.Context) error {
	processor, ok := recall.NewSideEffectProcessor(s.memory)
	if !ok {
		return nil
	}
	for {
		result, err := processor.ProcessSideEffects(ctx, recall.SideEffectProcessOptions{Scope: s.scope, Limit: 100})
		if err != nil {
			return mapFlowcraftError("process side effects", err)
		}
		if result.Failed > 0 || result.DeadLetter > 0 {
			return fmt.Errorf("%w: flowcraft side-effect processing failed", ErrUnavailable)
		}
		if result.Claimed == 0 {
			return nil
		}
	}
}

// Close releases the embedded Flowcraft memory and durable backend.
func (s *FlowcraftStore) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.memory.Close()
		if s.backend != nil {
			s.closeErr = errors.Join(s.closeErr, s.backend.Close())
		}
	})
	return s.closeErr
}

func cloneObserveResult(input ObserveResult) ObserveResult {
	output := ObserveResult{Facts: make([]Fact, len(input.Facts))}
	for i := range input.Facts {
		output.Facts[i] = cloneFact(input.Facts[i])
	}
	if input.Operation != nil {
		operation := *input.Operation
		output.Operation = &operation
	}
	return output
}
