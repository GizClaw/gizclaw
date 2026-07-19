package flowcraft

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
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
func (s *Store) Wait(ctx context.Context, operationID string) (memorystore.ObserveResult, error) {
	if err := ctx.Err(); err != nil {
		return observeResult{}, err
	}
	scope, _, err := decodeLocator(operationID)
	if err != nil {
		return observeResult{}, err
	}
	select {
	case <-ctx.Done():
		return observeResult{}, ctx.Err()
	case <-s.waitGate:
	}
	defer func() { s.waitGate <- struct{}{} }()

	s.mu.Lock()
	known, ok := s.operations[operationID]
	s.mu.Unlock()
	if !ok {
		return observeResult{}, fmt.Errorf("%w: flowcraft operation %q", errNotFound, operationID)
	}
	if known.Operation == nil || known.Operation.Status != operationPending {
		return cloneObserveResult(known), nil
	}
	if s.operationMarkedFailed(operationID) {
		if err := s.finalizeFailedOperations(ctx, []string{operationID}); err != nil {
			return observeResult{}, err
		}
		return s.operationResult(operationID), nil
	}
	if s.operationReady(operationID) {
		if err := s.completeReadyOperations(ctx, []string{operationID}); err != nil {
			return observeResult{}, err
		}
		return s.operationResult(operationID), nil
	}
	processor, ok := recall.NewAsyncSemanticProcessor(s.memory)
	if !ok {
		return observeResult{}, fmt.Errorf("%w: flowcraft async processor is unavailable", errUnavailable)
	}
	if s.queue == nil {
		return observeResult{}, fmt.Errorf("%w: flowcraft async queue is unavailable", errUnavailable)
	}
	for {
		s.queue.resetClaims()
		result, err := processor.ProcessAsyncSemantic(ctx, recall.AsyncSemanticProcessOptions{
			Scope: scope, WorkerID: "gizclaw-memory", Limit: 1,
		})
		claimedIDs := s.queue.takeClaims()
		if err != nil {
			return observeResult{}, mapFlowcraftError("wait", err)
		}
		if len(claimedIDs) != result.Claimed {
			return observeResult{}, fmt.Errorf("%w: flowcraft async claim correlation failed", errUnavailable)
		}
		if result.Completed+result.Failed != result.Claimed {
			return observeResult{}, fmt.Errorf("%w: flowcraft returned an invalid async result", errUnavailable)
		}
		if result.Claimed == 0 {
			if err := waitFlowcraftRetry(ctx); err != nil {
				return observeResult{}, err
			}
			continue
		}
		if result.Failed > 0 {
			if result.Failed != 1 || result.Completed != 0 || len(claimedIDs) != 1 {
				return observeResult{}, fmt.Errorf("%w: flowcraft returned an invalid async failure result", errUnavailable)
			}
			if s.operationReady(claimedIDs[0]) {
				if err := s.completeReadyOperations(ctx, claimedIDs); err != nil {
					return observeResult{}, err
				}
			} else {
				if err := s.failOperations(ctx, claimedIDs); err != nil {
					return observeResult{}, err
				}
			}
		}
		if result.Completed > 0 && result.Completed != 1 {
			return observeResult{}, fmt.Errorf("%w: flowcraft returned an invalid async completion result", errUnavailable)
		}
		if result.Completed > 0 {
			completedIDs := claimedIDs[:result.Completed]
			if err := s.markOperationsReady(ctx, completedIDs); err != nil {
				return observeResult{}, err
			}
			if err := s.completeReadyOperations(ctx, completedIDs); err != nil {
				return observeResult{}, err
			}
		}
		current := s.operationResult(operationID)
		if current.Operation != nil && current.Operation.Status != operationPending {
			return current, nil
		}
	}
}

func (s *Store) rehydrateOperations(ctx context.Context) error {
	enumerator, ok := s.temporal.(recall.ScopeEnumerator)
	if !ok {
		return nil
	}
	scopes, err := enumerator.ListScopes(ctx, recall.ScopeListQuery{RuntimeID: "gizclaw"})
	if err != nil {
		return mapFlowcraftError("rehydrate async operations", err)
	}
	for _, scope := range scopes {
		if err := s.rehydrateScopeOperations(ctx, scope); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) rehydrateScopeOperations(ctx context.Context, scope recall.Scope) error {
	nativeFacts, err := s.temporal.List(ctx, scope, recall.ListQuery{IncludeSuperseded: true})
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
		locator := encodeLocator(scope, id)
		operation := operations[id]
		switch operation.status {
		case flowcraftOperationStatusFailed:
			s.failed[locator] = struct{}{}
			if err := s.finalizeFailedOperations(ctx, []string{locator}); err != nil {
				return err
			}
			continue
		case flowcraftOperationStatusPrepared, flowcraftOperationStatusReady:
			s.operations[locator] = observeResult{Operation: &memorystore.Operation{ID: locator, Status: operationPending}}
			s.ready[locator] = struct{}{}
			continue
		case flowcraftOperationStatusSucceeded:
			// A completed extraction may intentionally produce no facts.
		case "":
			if !operation.hasEpisode {
				continue
			}
			if len(operation.facts) == 0 {
				s.operations[locator] = observeResult{Operation: &memorystore.Operation{ID: locator, Status: operationPending}}
				continue
			}
		default:
			return fmt.Errorf("%w: unknown flowcraft operation status %q", errUnavailable, operation.status)
		}
		result, err := s.operationResultFromFacts(ctx, scope, locator, operation.facts)
		if err != nil {
			return err
		}
		s.operations[locator] = result
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

func (s *Store) markOperationsReady(ctx context.Context, completedIDs []string) error {
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

func (s *Store) completeReadyOperations(ctx context.Context, completedIDs []string) error {
	if len(completedIDs) == 0 {
		return nil
	}
	if s.queue == nil {
		return fmt.Errorf("%w: flowcraft async queue is unavailable", errUnavailable)
	}
	results := make(map[string]observeResult, len(completedIDs))
	for _, id := range completedIDs {
		scope, nativeID, err := decodeLocator(id)
		if err != nil {
			return err
		}
		if err := s.drainSideEffects(ctx, scope); err != nil {
			return err
		}
		nativeFacts, err := s.temporal.FindByOriginRequestID(ctx, scope, nativeID)
		if err != nil {
			return mapFlowcraftError("load async facts", err)
		}
		result, err := s.operationResultFromFacts(ctx, scope, id, nativeFacts)
		if err != nil {
			return err
		}
		if err := s.queue.Cancel(ctx, nativeID); err != nil {
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

func (s *Store) failOperations(ctx context.Context, failedIDs []string) error {
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

func (s *Store) finalizeFailedOperations(ctx context.Context, failedIDs []string) error {
	if s.queue == nil {
		return fmt.Errorf("%w: flowcraft async queue is unavailable", errUnavailable)
	}
	for _, id := range failedIDs {
		_, nativeID, err := decodeLocator(id)
		if err != nil {
			return err
		}
		if err := s.queue.Cancel(ctx, nativeID); err != nil {
			return mapFlowcraftError("cancel failed async operation", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range failedIDs {
		s.operations[id] = observeResult{Operation: &memorystore.Operation{ID: id, Status: operationFailed, Error: "flowcraft async extraction failed"}}
		delete(s.failed, id)
	}
	return nil
}

func (s *Store) operationResultFromFacts(ctx context.Context, scope recall.Scope, id string, nativeFacts []recall.TemporalFact) (observeResult, error) {
	facts := make([]fact, 0, len(nativeFacts))
	for _, nativeFact := range nativeFacts {
		if nativeFact.Kind == recall.FactEpisode {
			continue
		}
		fact, err := s.factFromFlowcraft(ctx, scope, nativeFact)
		if err != nil {
			return observeResult{}, err
		}
		facts = append(facts, fact)
	}
	return observeResult{Facts: facts, Operation: &memorystore.Operation{ID: id, Status: operationSucceeded}}, nil
}

func (s *Store) persistOperationStatus(ctx context.Context, operationID, status string) error {
	scope, nativeID, err := decodeLocator(operationID)
	if err != nil {
		return err
	}
	nativeFacts, err := s.temporal.FindByOriginRequestID(ctx, scope, nativeID)
	if err != nil {
		return mapFlowcraftError("load async operation status", err)
	}
	for _, nativeFact := range nativeFacts {
		if existing, ok := flowcraftOperationMarker(nativeFact); ok && existing == status {
			return nil
		}
	}
	marker := recall.TemporalFact{
		ID:         flowcraftOperationMarkerID(nativeID, status),
		Scope:      scope,
		Kind:       recall.FactEpisode,
		Content:    "flowcraft async operation " + status,
		ObservedAt: time.Now(),
		Origin:     recall.FactOrigin{RequestID: nativeID, Kind: recall.OriginKindEpisode},
		Metadata:   map[string]any{flowcraftOperationStatusAttribute: status},
	}
	if err := s.temporal.Append(ctx, []recall.TemporalFact{marker}); err != nil {
		return mapFlowcraftError("persist async operation status", err)
	}
	return nil
}

func (s *Store) recordOperationStatus(ctx context.Context, scope recall.Scope, nativeID, status string) error {
	operationID := encodeLocator(scope, nativeID)
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

func (s *Store) operationResult(operationID string) observeResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneObserveResult(s.operations[operationID])
}

func (s *Store) operationReady(operationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ready[operationID]
	return ok
}

func (s *Store) operationMarkedFailed(operationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.failed[operationID]
	return ok
}

type flowcraftAsyncQueue struct {
	recall.AsyncSemanticQueue
	mu           sync.Mutex
	claimed      []string
	claimScopes  map[string]recall.Scope
	statusWriter func(context.Context, recall.Scope, string, string) error
}

// Close leaves the injected queue caller-owned.
func (*flowcraftAsyncQueue) Close() error { return nil }

func (q *flowcraftAsyncQueue) Complete(ctx context.Context, requestID, leaseToken string, result recall.AsyncSemanticResult) error {
	if err := q.writeStatus(ctx, requestID, flowcraftOperationStatusPrepared); err != nil {
		return err
	}
	if err := q.AsyncSemanticQueue.Complete(ctx, requestID, leaseToken, result); err != nil {
		return err
	}
	if err := q.writeStatus(ctx, requestID, flowcraftOperationStatusReady); err != nil {
		return err
	}
	q.forgetScope(requestID)
	return nil
}

func (q *flowcraftAsyncQueue) Fail(ctx context.Context, requestID, leaseToken string, failure recall.AsyncSemanticFailure) error {
	if err := q.writeStatus(ctx, requestID, flowcraftOperationStatusFailed); err != nil {
		return err
	}
	if err := q.AsyncSemanticQueue.Fail(ctx, requestID, leaseToken, failure); err != nil {
		return err
	}
	q.forgetScope(requestID)
	return nil
}

func (q *flowcraftAsyncQueue) setStatusWriter(writer func(context.Context, recall.Scope, string, string) error) {
	q.mu.Lock()
	q.statusWriter = writer
	q.mu.Unlock()
}

func (q *flowcraftAsyncQueue) writeStatus(ctx context.Context, requestID, status string) error {
	q.mu.Lock()
	writer := q.statusWriter
	scope := q.claimScopes[requestID]
	q.mu.Unlock()
	if writer == nil {
		return nil
	}
	return writer(ctx, scope, requestID, status)
}

func (q *flowcraftAsyncQueue) forgetScope(requestID string) {
	q.mu.Lock()
	delete(q.claimScopes, requestID)
	q.mu.Unlock()
}

func newFlowcraftAsyncQueue(queue recall.AsyncSemanticQueue) *flowcraftAsyncQueue {
	if queue == nil {
		return nil
	}
	return &flowcraftAsyncQueue{AsyncSemanticQueue: queue, claimScopes: make(map[string]recall.Scope)}
}

func (q *flowcraftAsyncQueue) Claim(ctx context.Context, options recall.AsyncSemanticClaimOptions) ([]recall.AsyncSemanticJob, error) {
	jobs, err := q.AsyncSemanticQueue.Claim(ctx, options)
	if err != nil {
		return nil, err
	}
	q.mu.Lock()
	for _, job := range jobs {
		locator := encodeLocator(job.Scope, job.RequestID)
		q.claimed = append(q.claimed, locator)
		q.claimScopes[job.RequestID] = job.Scope
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

func (s *Store) drainSideEffects(ctx context.Context, scope recall.Scope) error {
	processor, ok := recall.NewSideEffectProcessor(s.memory)
	if !ok {
		return nil
	}
	for {
		result, err := processor.ProcessSideEffects(ctx, recall.SideEffectProcessOptions{Scope: scope, Limit: 100})
		if err != nil {
			return mapFlowcraftError("process side effects", err)
		}
		if result.Failed > 0 || result.DeadLetter > 0 {
			return fmt.Errorf("%w: flowcraft side-effect processing failed", errUnavailable)
		}
		if result.Claimed == 0 {
			return nil
		}
	}
}

// Close releases only the recall memory constructed by this adapter.
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.memory.Close()
	})
	return s.closeErr
}

func cloneObserveResult(input observeResult) observeResult {
	output := observeResult{Facts: make([]fact, len(input.Facts))}
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
