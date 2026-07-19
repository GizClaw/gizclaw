package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	flowworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
)

// FlowcraftStore adapts embedded Flowcraft recall memory to Store.
type FlowcraftStore struct {
	config   FlowcraftConfig
	scope    recall.Scope
	memory   recall.Memory
	temporal recall.TemporalStore
	queue    *flowcraftAsyncQueue
	backend  *flowworkspace.Backend

	mu         sync.Mutex
	waitGate   chan struct{}
	operations map[string]ObserveResult
	closeOnce  sync.Once
	closeErr   error
}

func newFlowcraftStore(config FlowcraftConfig, memory recall.Memory, temporal recall.TemporalStore, queue *flowcraftAsyncQueue, backend *flowworkspace.Backend) *FlowcraftStore {
	waitGate := make(chan struct{}, 1)
	waitGate <- struct{}{}
	return &FlowcraftStore{config: config, scope: config.scope(), memory: memory, temporal: temporal, queue: queue, backend: backend, waitGate: waitGate, operations: make(map[string]ObserveResult)}
}

// Observe extracts and persists facts from raw text or turns.
func (s *FlowcraftStore) Observe(ctx context.Context, observation Observation) (ObserveResult, error) {
	if err := validateObservation(observation); err != nil {
		return ObserveResult{}, err
	}
	if err := validateFlowcraftAttributeKeys(observation.Context); err != nil {
		return ObserveResult{}, err
	}
	request := recall.SaveRequest{ObservedAt: observation.ObservedAt}
	if s.config.ExtractionModel == "" {
		parts := make([]string, 0, len(observation.Turns)+1)
		if text := strings.TrimSpace(observation.Text); text != "" {
			parts = append(parts, text)
		}
		for _, turn := range observation.Turns {
			parts = append(parts, turn.Text)
		}
		text := strings.Join(parts, "\n")
		request.Facts = []recall.TemporalFact{{Kind: recall.FactNote, Content: text, ObservedAt: observation.ObservedAt, Metadata: cloneMap(observation.Context)}}
		fact := &request.Facts[0]
		if observation.ID != "" {
			if fact.Metadata == nil {
				fact.Metadata = make(map[string]any)
			}
			fact.Metadata["observation_id"] = observation.ID
		}
		for _, turn := range observation.Turns {
			fact.SourceMessageIDs = append(fact.SourceMessageIDs, turn.ID)
			fact.EvidenceRefs = append(fact.EvidenceRefs, recall.EvidenceRef{ID: turn.ID, MessageID: turn.ID, Role: string(turn.Role), Text: turn.Text, Timestamp: turn.ObservedAt})
		}
	} else {
		request.Turns = flowcraftTurns(observation)
	}
	if s.config.Async.Enabled {
		request.Mode = recall.WriteModeAsyncSemantic
	}
	result, err := s.memory.Save(ctx, s.scope, request)
	if err != nil {
		return ObserveResult{}, mapFlowcraftError("observe", err)
	}
	if result.SemanticPending {
		out := ObserveResult{Operation: &Operation{ID: result.AsyncRequestID, Status: OperationPending}}
		s.mu.Lock()
		s.operations[result.AsyncRequestID] = cloneObserveResult(out)
		s.mu.Unlock()
		return out, nil
	}
	if err := s.drainSideEffects(ctx); err != nil {
		return ObserveResult{}, err
	}
	facts, err := s.loadFacts(ctx, result.FactIDs)
	if err != nil {
		return ObserveResult{}, err
	}
	return ObserveResult{Facts: facts}, nil
}

// Recall returns facts relevant to the query.
func (s *FlowcraftStore) Recall(ctx context.Context, query Query) (RecallResult, error) {
	if err := validateQuery(query); err != nil {
		return RecallResult{}, err
	}
	flowQuery := recall.Query{Text: query.Text, Limit: query.Limit}
	for _, filter := range query.Filters {
		if filter.Operator != FilterEqual {
			return RecallResult{}, fmt.Errorf("%w: flowcraft filter operator %q", ErrUnsupported, filter.Operator)
		}
		switch filter.Field {
		case "subject":
			flowQuery.Subject = fmt.Sprint(filter.Value)
		case "predicate":
			flowQuery.Predicate = fmt.Sprint(filter.Value)
		case "object":
			flowQuery.Object = fmt.Sprint(filter.Value)
		case "entity":
			flowQuery.Entities = append(flowQuery.Entities, fmt.Sprint(filter.Value))
		case "kind":
			flowQuery.Kinds = append(flowQuery.Kinds, recall.FactKind(fmt.Sprint(filter.Value)))
		default:
			return RecallResult{}, fmt.Errorf("%w: flowcraft filter field %q", ErrUnsupported, filter.Field)
		}
	}
	hits, err := s.memory.Recall(ctx, s.scope, flowQuery)
	if err != nil {
		return RecallResult{}, mapFlowcraftError("recall", err)
	}
	out := RecallResult{Matches: make([]Match, len(hits))}
	for i, hit := range hits {
		fact, err := s.factFromFlowcraft(ctx, hit.Fact)
		if err != nil {
			return RecallResult{}, err
		}
		out.Matches[i] = Match{Fact: fact, Score: hit.Score}
	}
	return out, nil
}

// Update appends a Flowcraft revision that supersedes the current fact.
func (s *FlowcraftStore) Update(ctx context.Context, request UpdateRequest) (Fact, error) {
	if err := validateUpdate(request); err != nil {
		return Fact{}, err
	}
	if err := validateFlowcraftAttributePatch(request.Attributes); err != nil {
		return Fact{}, err
	}
	current, err := s.currentFact(ctx, request.ID)
	if err != nil {
		return Fact{}, err
	}
	if request.ExpectedRevision != "" && request.ExpectedRevision != current.ID {
		return Fact{}, fmt.Errorf("%w: fact %q revision changed", ErrConflict, request.ID)
	}
	next := current.Clone()
	next.ID = ""
	next.CorrectedBy = ""
	next.ValidTo = nil
	next.Supersedes = []string{current.ID}
	next.ObservedAt = time.Now()
	if next.Metadata == nil {
		next.Metadata = make(map[string]any)
	}
	next.Metadata[flowcraftRootIDAttribute] = request.ID
	if request.Text != nil {
		next.Content = *request.Text
	}
	for key, value := range request.Attributes.Set {
		next.Metadata[key] = cloneValue(value)
	}
	for _, key := range request.Attributes.Delete {
		delete(next.Metadata, key)
	}
	result, err := s.memory.Save(ctx, s.scope, recall.SaveRequest{Facts: []recall.TemporalFact{next}, ObservedAt: next.ObservedAt})
	if err != nil {
		return Fact{}, mapFlowcraftError("update", err)
	}
	if err := s.drainSideEffects(ctx); err != nil {
		return Fact{}, err
	}
	if len(result.FactIDs) != 1 {
		return Fact{}, fmt.Errorf("%w: flowcraft update returned %d facts", ErrUnavailable, len(result.FactIDs))
	}
	return s.factByID(ctx, result.FactIDs[0])
}

// Delete soft-retires a Flowcraft fact while preserving its audit history.
func (s *FlowcraftStore) Delete(ctx context.Context, request DeleteRequest) error {
	if err := validateDelete(request); err != nil {
		return err
	}
	current, err := s.currentFact(ctx, request.ID)
	if err != nil {
		return err
	}
	if request.ExpectedRevision != "" && request.ExpectedRevision != current.ID {
		return fmt.Errorf("%w: fact %q revision changed", ErrConflict, request.ID)
	}
	if err := s.memory.Forget(ctx, s.scope, current.ID, recall.ForgetSoft); err != nil {
		return mapFlowcraftError("delete", err)
	}
	return nil
}

func (s *FlowcraftStore) currentFact(ctx context.Context, id string) (recall.TemporalFact, error) {
	lineage, err := s.memory.Lineage(ctx, s.scope, id)
	if err != nil {
		return recall.TemporalFact{}, mapFlowcraftError("lineage", err)
	}
	if len(lineage) == 0 {
		return recall.TemporalFact{}, fmt.Errorf("%w: fact %q", ErrNotFound, id)
	}
	var current *recall.TemporalFact
	for i := range lineage {
		if lineage[i].Fact.CorrectedBy == "" {
			if current != nil {
				return recall.TemporalFact{}, fmt.Errorf("%w: fact %q has multiple current revisions", ErrConflict, id)
			}
			fact := lineage[i].Fact
			current = &fact
		}
	}
	if current != nil {
		return *current, nil
	}
	return recall.TemporalFact{}, fmt.Errorf("%w: fact %q has no current revision", ErrNotFound, id)
}

func (s *FlowcraftStore) factByID(ctx context.Context, id string) (Fact, error) {
	fact, err := s.currentFact(ctx, id)
	if err != nil {
		return Fact{}, err
	}
	return s.factFromFlowcraft(ctx, fact)
}

func (s *FlowcraftStore) loadFacts(ctx context.Context, ids []string) ([]Fact, error) {
	output := make([]Fact, 0, len(ids))
	for _, id := range ids {
		fact, err := s.factByID(ctx, id)
		if err != nil {
			return nil, err
		}
		output = append(output, fact)
	}
	return output, nil
}

func flowcraftTurns(observation Observation) []recall.TurnContext {
	turns := make([]recall.TurnContext, 0, len(observation.Turns)+1)
	if strings.TrimSpace(observation.Text) != "" {
		turns = append(turns, recall.TurnContext{ID: observation.ID, EvidenceID: observation.ID, SessionID: observation.ID, Role: string(RoleUser), Time: observation.ObservedAt, Text: observation.Text})
	}
	for _, turn := range observation.Turns {
		turns = append(turns, recall.TurnContext{ID: turn.ID, EvidenceID: turn.ID, SessionID: observation.ID, Role: string(turn.Role), Speaker: turn.Speaker, Time: turn.ObservedAt, Text: turn.Text})
	}
	return turns
}

const flowcraftRootIDAttribute = "gizclaw.root_id"

var flowcraftReservedAttributes = map[string]struct{}{
	flowcraftRootIDAttribute: {},
	"observation_id":         {},
	"kind":                   {},
	"subject":                {},
	"predicate":              {},
	"object":                 {},
	"entities":               {},
}

func validateFlowcraftAttributeKeys(attributes map[string]any) error {
	for key := range attributes {
		if _, reserved := flowcraftReservedAttributes[key]; reserved {
			return fmt.Errorf("%w: flowcraft attribute %q is provider-owned", ErrUnsupported, key)
		}
	}
	return nil
}

func validateFlowcraftAttributePatch(patch AttributePatch) error {
	if err := validateFlowcraftAttributeKeys(patch.Set); err != nil {
		return err
	}
	for _, key := range patch.Delete {
		if _, reserved := flowcraftReservedAttributes[key]; reserved {
			return fmt.Errorf("%w: flowcraft attribute %q is provider-owned", ErrUnsupported, key)
		}
	}
	return nil
}

func (s *FlowcraftStore) factFromFlowcraft(ctx context.Context, input recall.TemporalFact) (Fact, error) {
	turnIDs := append([]string(nil), input.SourceMessageIDs...)
	if len(turnIDs) == 0 {
		for _, evidence := range input.EvidenceRefs {
			if evidence.MessageID != "" {
				turnIDs = append(turnIDs, evidence.MessageID)
			} else if evidence.ID != "" {
				turnIDs = append(turnIDs, evidence.ID)
			}
		}
	}
	attributes := cloneMap(input.Metadata)
	if attributes == nil {
		attributes = make(map[string]any)
	}
	attributes["kind"] = string(input.Kind)
	if input.Subject != "" {
		attributes["subject"] = input.Subject
	}
	if input.Predicate != "" {
		attributes["predicate"] = input.Predicate
	}
	if input.Object != "" {
		attributes["object"] = input.Object
	}
	if len(input.Entities) > 0 {
		attributes["entities"] = append([]string(nil), input.Entities...)
	}
	rootID := ""
	if len(input.Supersedes) > 0 {
		rootID, _ = attributes[flowcraftRootIDAttribute].(string)
	}
	delete(attributes, flowcraftRootIDAttribute)
	createdAt := input.ObservedAt
	if input.ValidFrom != nil {
		createdAt = *input.ValidFrom
	}
	if len(input.Supersedes) > 0 {
		lineage, err := s.memory.Lineage(ctx, s.scope, input.ID)
		if err != nil {
			return Fact{}, mapFlowcraftError("resolve root revision", err)
		}
		for _, node := range lineage {
			if (rootID != "" && node.Fact.ID == rootID) || (rootID == "" && len(node.Fact.Supersedes) == 0) {
				rootID = node.Fact.ID
				createdAt = node.Fact.ObservedAt
				if node.Fact.ValidFrom != nil {
					createdAt = *node.Fact.ValidFrom
				}
				break
			}
		}
	}
	if rootID == "" {
		rootID = input.ID
	}
	observationID, _ := attributes["observation_id"].(string)
	delete(attributes, "observation_id")
	var sources []SourceRef
	if observationID != "" || len(turnIDs) > 0 {
		sources = []SourceRef{{ObservationID: observationID, TurnIDs: turnIDs}}
	}
	return Fact{ID: rootID, Revision: input.ID, Text: input.Content, Attributes: attributes, Sources: sources, CreatedAt: createdAt, UpdatedAt: input.ObservedAt}, nil
}

func mapFlowcraftError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case errdefs.IsValidation(err):
		return fmt.Errorf("%w: flowcraft %s: %v", ErrInvalidInput, operation, err)
	case errdefs.IsNotFound(err):
		return fmt.Errorf("%w: flowcraft %s", ErrNotFound, operation)
	case errdefs.IsConflict(err):
		return fmt.Errorf("%w: flowcraft %s", ErrConflict, operation)
	case errdefs.IsNotAvailable(err):
		return fmt.Errorf("%w: flowcraft %s", ErrUnavailable, operation)
	default:
		return fmt.Errorf("flowcraft %s: %w", operation, err)
	}
}

var _ Store = (*FlowcraftStore)(nil)
var _ OperationWaiter = (*FlowcraftStore)(nil)
