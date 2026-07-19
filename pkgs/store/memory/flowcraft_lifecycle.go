package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	if s.queue == nil {
		return ObserveResult{}, fmt.Errorf("%w: flowcraft async queue is unavailable", ErrUnavailable)
	}
	for {
		s.queue.resetClaims()
		result, err := processor.ProcessAsyncSemantic(ctx, recall.AsyncSemanticProcessOptions{
			Scope: s.scope, WorkerID: s.config.Async.WorkerID, Limit: 1,
		})
		claimedIDs := s.queue.takeClaims()
		if err != nil {
			return ObserveResult{}, mapFlowcraftError("wait", err)
		}
		if len(claimedIDs) != result.Claimed {
			return ObserveResult{}, fmt.Errorf("%w: flowcraft async claim correlation failed", ErrUnavailable)
		}
		if result.Completed+result.Failed != result.Claimed {
			return ObserveResult{}, fmt.Errorf("%w: flowcraft returned an invalid async result", ErrUnavailable)
		}
		if result.Claimed == 0 {
			if err := waitFlowcraftRetry(ctx); err != nil {
				return ObserveResult{}, err
			}
			continue
		}
		if result.Failed > 0 {
			if result.Failed != 1 || result.Completed != 0 {
				return ObserveResult{}, fmt.Errorf("%w: flowcraft returned an invalid async failure result", ErrUnavailable)
			}
			if err := s.failOperations(ctx, claimedIDs); err != nil {
				return ObserveResult{}, err
			}
		}
		if result.Completed > 0 && result.Completed != 1 {
			return ObserveResult{}, fmt.Errorf("%w: flowcraft returned an invalid async completion result", ErrUnavailable)
		}
		if err := s.finishOperations(ctx, claimedIDs[:result.Completed]); err != nil {
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

func (s *FlowcraftStore) rehydrateOperations(ctx context.Context) error {
	nativeFacts, err := s.temporal.List(ctx, s.scope, recall.ListQuery{IncludeSuperseded: true})
	if err != nil {
		return mapFlowcraftError("rehydrate async operations", err)
	}
	type nativeOperation struct {
		hasEpisode bool
		facts      []recall.TemporalFact
	}
	order := make([]string, 0)
	operations := make(map[string]*nativeOperation)
	for _, nativeFact := range nativeFacts {
		id := nativeFact.Origin.RequestID
		if id == "" {
			continue
		}
		operation := operations[id]
		if operation == nil {
			operation = &nativeOperation{}
			operations[id] = operation
			order = append(order, id)
		}
		if nativeFact.Kind == recall.FactEpisode {
			operation.hasEpisode = true
			continue
		}
		operation.facts = append(operation.facts, nativeFact)
	}
	for _, id := range order {
		operation := operations[id]
		if !operation.hasEpisode {
			continue
		}
		if len(operation.facts) == 0 {
			s.operations[id] = ObserveResult{Operation: &Operation{ID: id, Status: OperationPending}}
			continue
		}
		facts := make([]Fact, 0, len(operation.facts))
		for _, nativeFact := range operation.facts {
			fact, err := s.factFromFlowcraft(ctx, nativeFact)
			if err != nil {
				return err
			}
			facts = append(facts, fact)
		}
		s.operations[id] = ObserveResult{Facts: facts, Operation: &Operation{ID: id, Status: OperationSucceeded}}
	}
	return nil
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

func (s *FlowcraftStore) finishOperations(ctx context.Context, completedIDs []string) error {
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
	for _, id := range completedIDs {
		s.operations[id] = results[id]
	}
	return nil
}

func (s *FlowcraftStore) failOperations(ctx context.Context, failedIDs []string) error {
	if s.queue == nil {
		return fmt.Errorf("%w: flowcraft async queue is unavailable", ErrUnavailable)
	}
	for _, id := range failedIDs {
		if err := s.queue.Cancel(ctx, id); err != nil {
			return mapFlowcraftError("cancel failed async operation", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range failedIDs {
		s.operations[id] = ObserveResult{Operation: &Operation{ID: id, Status: OperationFailed, Error: "flowcraft async extraction failed"}}
	}
	return nil
}

type flowcraftAsyncQueue struct {
	recall.AsyncSemanticQueue
	mu      sync.Mutex
	claimed []string
}

func newFlowcraftAsyncQueue(queue recall.AsyncSemanticQueue) *flowcraftAsyncQueue {
	if queue == nil {
		return nil
	}
	return &flowcraftAsyncQueue{AsyncSemanticQueue: queue}
}

func (q *flowcraftAsyncQueue) Claim(ctx context.Context, options recall.AsyncSemanticClaimOptions) ([]recall.AsyncSemanticJob, error) {
	jobs, err := q.AsyncSemanticQueue.Claim(ctx, options)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	for _, job := range jobs {
		q.claimed = append(q.claimed, job.RequestID)
	}
	q.mu.Unlock()
	return jobs, nil
}

func (q *flowcraftAsyncQueue) resetClaims() {
	q.mu.Lock()
	q.claimed = nil
	q.mu.Unlock()
}

func (q *flowcraftAsyncQueue) takeClaims() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	claimed := append([]string(nil), q.claimed...)
	q.claimed = nil
	return claimed
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

var _ recall.AsyncSemanticQueue = (*flowcraftAsyncQueue)(nil)
