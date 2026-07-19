package memory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
)

const (
	flowcraftOperationStatusAttribute = "gizclaw.operation_status"
	flowcraftOperationStatusPrepared  = "prepared"
	flowcraftOperationStatusReady     = "ready"
	flowcraftOperationStatusSucceeded = "succeeded"
	flowcraftOperationStatusFailed    = "failed"
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
	if s.operationMarkedFailed(operationID) {
		if err := s.finalizeFailedOperations(ctx, []string{operationID}); err != nil {
			return ObserveResult{}, err
		}
		return s.operationResult(operationID), nil
	}
	if s.operationReady(operationID) {
		if err := s.completeReadyOperations(ctx, []string{operationID}); err != nil {
			return ObserveResult{}, err
		}
		return s.operationResult(operationID), nil
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
			if result.Failed != 1 || result.Completed != 0 || len(claimedIDs) != 1 {
				return ObserveResult{}, fmt.Errorf("%w: flowcraft returned an invalid async failure result", ErrUnavailable)
			}
			if s.operationReady(claimedIDs[0]) {
				if err := s.completeReadyOperations(ctx, claimedIDs); err != nil {
					return ObserveResult{}, err
				}
			} else {
				if err := s.failOperations(ctx, claimedIDs); err != nil {
					return ObserveResult{}, err
				}
			}
		}
		if result.Completed > 0 && result.Completed != 1 {
			return ObserveResult{}, fmt.Errorf("%w: flowcraft returned an invalid async completion result", ErrUnavailable)
		}
		if result.Completed > 0 {
			completedIDs := claimedIDs[:result.Completed]
			if err := s.markOperationsReady(ctx, completedIDs); err != nil {
				return ObserveResult{}, err
			}
			if err := s.completeReadyOperations(ctx, completedIDs); err != nil {
				return ObserveResult{}, err
			}
		}
		current := s.operationResult(operationID)
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
		status     string
	}
	order := make([]string, 0)
	operations := make(map[string]*nativeOperation)
	for _, nativeFact := range nativeFacts {
		if isFlowcraftProvenanceMarker(nativeFact) {
			continue
		}
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
		if status, ok := flowcraftOperationMarker(nativeFact); ok {
			if flowcraftOperationStatusRank(status) >= flowcraftOperationStatusRank(operation.status) {
				operation.status = status
			}
			continue
		}
		if nativeFact.Kind == recall.FactEpisode {
			operation.hasEpisode = true
			continue
		}
		operation.facts = append(operation.facts, nativeFact)
	}
	for _, id := range order {
		operation := operations[id]
		switch operation.status {
		case flowcraftOperationStatusFailed:
			s.failed[id] = struct{}{}
			if err := s.finalizeFailedOperations(ctx, []string{id}); err != nil {
				return err
			}
			continue
		case flowcraftOperationStatusPrepared, flowcraftOperationStatusReady:
			s.operations[id] = ObserveResult{Operation: &Operation{ID: id, Status: OperationPending}}
			s.ready[id] = struct{}{}
			continue
		case flowcraftOperationStatusSucceeded:
			// A completed extraction may intentionally produce no facts.
		case "":
			if !operation.hasEpisode {
				continue
			}
			if len(operation.facts) == 0 {
				s.operations[id] = ObserveResult{Operation: &Operation{ID: id, Status: OperationPending}}
				continue
			}
		default:
			return fmt.Errorf("%w: unknown flowcraft operation status %q", ErrUnavailable, operation.status)
		}
		result, err := s.operationResultFromFacts(ctx, id, operation.facts)
		if err != nil {
			return err
		}
		s.operations[id] = result
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

func (s *FlowcraftStore) markOperationsReady(ctx context.Context, completedIDs []string) error {
	for _, id := range completedIDs {
		if err := s.persistOperationStatus(ctx, id, flowcraftOperationStatusReady); err != nil {
			return err
		}
		s.mu.Lock()
		s.ready[id] = struct{}{}
		s.mu.Unlock()
	}
	return nil
}

func (s *FlowcraftStore) completeReadyOperations(ctx context.Context, completedIDs []string) error {
	if len(completedIDs) == 0 {
		return nil
	}
	if err := s.drainSideEffects(ctx); err != nil {
		return err
	}
	if s.queue == nil {
		return fmt.Errorf("%w: flowcraft async queue is unavailable", ErrUnavailable)
	}
	results := make(map[string]ObserveResult, len(completedIDs))
	for _, id := range completedIDs {
		nativeFacts, err := s.temporal.FindByOriginRequestID(ctx, s.scope, id)
		if err != nil {
			return mapFlowcraftError("load async facts", err)
		}
		result, err := s.operationResultFromFacts(ctx, id, nativeFacts)
		if err != nil {
			return err
		}
		if err := s.queue.Cancel(ctx, id); err != nil {
			return mapFlowcraftError("finalize async operation", err)
		}
		if err := s.persistOperationStatus(ctx, id, flowcraftOperationStatusSucceeded); err != nil {
			return err
		}
		results[id] = result
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range completedIDs {
		s.operations[id] = results[id]
		delete(s.ready, id)
	}
	return nil
}

func (s *FlowcraftStore) failOperations(ctx context.Context, failedIDs []string) error {
	for _, id := range failedIDs {
		if err := s.persistOperationStatus(ctx, id, flowcraftOperationStatusFailed); err != nil {
			return err
		}
		s.mu.Lock()
		s.failed[id] = struct{}{}
		s.mu.Unlock()
	}
	return s.finalizeFailedOperations(ctx, failedIDs)
}

func (s *FlowcraftStore) finalizeFailedOperations(ctx context.Context, failedIDs []string) error {
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
		delete(s.failed, id)
	}
	return nil
}

func (s *FlowcraftStore) operationResultFromFacts(ctx context.Context, id string, nativeFacts []recall.TemporalFact) (ObserveResult, error) {
	facts := make([]Fact, 0, len(nativeFacts))
	for _, nativeFact := range nativeFacts {
		if nativeFact.Kind == recall.FactEpisode {
			continue
		}
		fact, err := s.factFromFlowcraft(ctx, nativeFact)
		if err != nil {
			return ObserveResult{}, err
		}
		facts = append(facts, fact)
	}
	return ObserveResult{Facts: facts, Operation: &Operation{ID: id, Status: OperationSucceeded}}, nil
}

func (s *FlowcraftStore) persistOperationStatus(ctx context.Context, operationID, status string) error {
	nativeFacts, err := s.temporal.FindByOriginRequestID(ctx, s.scope, operationID)
	if err != nil {
		return mapFlowcraftError("load async operation status", err)
	}
	for _, nativeFact := range nativeFacts {
		if existing, ok := flowcraftOperationMarker(nativeFact); ok && existing == status {
			return nil
		}
	}
	marker := recall.TemporalFact{
		ID:         flowcraftOperationMarkerID(operationID, status),
		Scope:      s.scope,
		Kind:       recall.FactEpisode,
		Content:    "flowcraft async operation " + status,
		ObservedAt: time.Now(),
		Origin:     recall.FactOrigin{RequestID: operationID, Kind: recall.OriginKindEpisode},
		Metadata:   map[string]any{flowcraftOperationStatusAttribute: status},
	}
	if err := s.temporal.Append(ctx, []recall.TemporalFact{marker}); err != nil {
		return mapFlowcraftError("persist async operation status", err)
	}
	return nil
}

func (s *FlowcraftStore) recordOperationStatus(ctx context.Context, operationID, status string) error {
	if err := s.persistOperationStatus(ctx, operationID, status); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch status {
	case flowcraftOperationStatusPrepared, flowcraftOperationStatusReady:
		s.ready[operationID] = struct{}{}
	case flowcraftOperationStatusFailed:
		s.failed[operationID] = struct{}{}
	}
	return nil
}

func flowcraftOperationMarker(fact recall.TemporalFact) (string, bool) {
	status, ok := fact.Metadata[flowcraftOperationStatusAttribute].(string)
	if !ok || fact.Kind != recall.FactEpisode || fact.ID != flowcraftOperationMarkerID(fact.Origin.RequestID, status) {
		return "", false
	}
	return status, true
}

func flowcraftOperationMarkerID(operationID, status string) string {
	sum := sha256.Sum256([]byte(operationID + "\x00" + status))
	return fmt.Sprintf("gizclaw-operation-%x", sum)
}

func flowcraftOperationStatusRank(status string) int {
	switch status {
	case flowcraftOperationStatusPrepared:
		return 1
	case flowcraftOperationStatusReady:
		return 2
	case flowcraftOperationStatusSucceeded, flowcraftOperationStatusFailed:
		return 3
	default:
		return 0
	}
}

func (s *FlowcraftStore) operationResult(operationID string) ObserveResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneObserveResult(s.operations[operationID])
}

func (s *FlowcraftStore) operationReady(operationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ready[operationID]
	return ok
}

func (s *FlowcraftStore) operationMarkedFailed(operationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.failed[operationID]
	return ok
}

type flowcraftAsyncQueue struct {
	recall.AsyncSemanticQueue
	mu           sync.Mutex
	claimed      []string
	statusWriter func(context.Context, string, string) error
}

func (q *flowcraftAsyncQueue) Complete(ctx context.Context, requestID, leaseToken string, result recall.AsyncSemanticResult) error {
	if err := q.writeStatus(ctx, requestID, flowcraftOperationStatusPrepared); err != nil {
		return err
	}
	if err := q.AsyncSemanticQueue.Complete(ctx, requestID, leaseToken, result); err != nil {
		return err
	}
	return q.writeStatus(ctx, requestID, flowcraftOperationStatusReady)
}

func (q *flowcraftAsyncQueue) Fail(ctx context.Context, requestID, leaseToken string, failure recall.AsyncSemanticFailure) error {
	if err := q.writeStatus(ctx, requestID, flowcraftOperationStatusFailed); err != nil {
		return err
	}
	return q.AsyncSemanticQueue.Fail(ctx, requestID, leaseToken, failure)
}

func (q *flowcraftAsyncQueue) setStatusWriter(writer func(context.Context, string, string) error) {
	q.mu.Lock()
	q.statusWriter = writer
	q.mu.Unlock()
}

func (q *flowcraftAsyncQueue) writeStatus(ctx context.Context, requestID, status string) error {
	q.mu.Lock()
	writer := q.statusWriter
	q.mu.Unlock()
	if writer == nil {
		return nil
	}
	return writer(ctx, requestID, status)
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
