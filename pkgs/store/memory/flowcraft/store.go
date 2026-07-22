package flowcraft

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Store adapts embedded Flowcraft recall memory to Store.
type Store struct {
	config   Config
	memory   recall.Memory
	temporal recall.TemporalStore
	queue    *flowcraftAsyncQueue

	mu         sync.Mutex
	waitGate   chan struct{}
	operations map[string]observeResult
	ready      map[string]struct{}
	failed     map[string]struct{}
	closeOnce  sync.Once
	closeErr   error
}

func newStore(config Config, memory recall.Memory, temporal recall.TemporalStore, queue *flowcraftAsyncQueue) *Store {
	waitGate := make(chan struct{}, 1)
	waitGate <- struct{}{}
	return &Store{
		config: config, memory: memory, temporal: temporal, queue: queue,
		waitGate: waitGate, operations: make(map[string]observeResult), ready: make(map[string]struct{}), failed: make(map[string]struct{}),
	}
}

// Observe extracts and persists facts from raw text or turns.
func (s *Store) Observe(ctx context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	if err := validateObservation(observation); err != nil {
		return observeResult{}, err
	}
	if err := validateFlowcraftAttributeKeys(observation.Context); err != nil {
		return observeResult{}, err
	}
	if err := validateFlowcraftFactCandidates(observation.Facts); err != nil {
		return observeResult{}, err
	}
	scope := nativeScope(observation.Scope)
	if s.config.Extraction.Model != "" && len(observation.Context) > 0 {
		return observeResult{}, fmt.Errorf("%w: flowcraft model extraction does not support observation context", errUnsupported)
	}
	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	request := recall.SaveRequest{
		ObservedAt: observedAt,
		Tier:       s.config.Tier,
		Facts:      flowcraftFactCandidates(observation.Facts, observedAt),
	}
	if s.config.Extraction.Model == "" {
		parts := make([]string, 0, len(observation.Turns)+1)
		if text := strings.TrimSpace(observation.Text); text != "" {
			parts = append(parts, text)
		}
		for _, turn := range observation.Turns {
			parts = append(parts, turn.Text)
		}
		text := strings.Join(parts, "\n")
		if text != "" {
			fact := recall.TemporalFact{Kind: recall.FactNote, Content: text, ObservedAt: observedAt, Metadata: cloneMap(observation.Context)}
			if observation.ID != "" {
				if fact.Metadata == nil {
					fact.Metadata = make(map[string]any)
				}
				fact.Metadata["observation_id"] = observation.ID
			}
			for _, turn := range observation.Turns {
				if turn.ID != "" {
					fact.SourceMessageIDs = append(fact.SourceMessageIDs, turn.ID)
					fact.EvidenceRefs = append(fact.EvidenceRefs, recall.EvidenceRef{ID: turn.ID, MessageID: turn.ID, Role: string(turn.Role), Text: turn.Text, Timestamp: turn.ObservedAt})
				}
			}
			request.Facts = append(request.Facts, fact)
		}
	} else {
		request.Turns = flowcraftTurns(observation)
	}
	if s.queue != nil {
		request.Mode = recall.WriteModeAsyncSemantic
	}
	result, err := s.memory.Save(ctx, scope, request)
	if err != nil {
		return observeResult{}, mapFlowcraftError("observe", err)
	}
	if err := s.persistObservationProvenance(ctx, scope, observation.ID, result); err != nil {
		return observeResult{}, err
	}
	if result.SemanticPending {
		operationID := encodeLocator(scope, result.AsyncRequestID)
		out := observeResult{Operation: &memorystore.Operation{ID: operationID, Status: operationPending}}
		s.mu.Lock()
		s.operations[operationID] = cloneObserveResult(out)
		s.mu.Unlock()
		return out, nil
	}
	if err := s.drainSideEffects(ctx, scope); err != nil {
		return observeResult{}, err
	}
	facts, err := s.loadFacts(ctx, scope, result.FactIDs)
	if err != nil {
		return observeResult{}, err
	}
	return observeResult{Facts: facts}, nil
}

func validateFlowcraftFactCandidates(candidates []memorystore.FactCandidate) error {
	for index, candidate := range candidates {
		for _, key := range []string{"kind", "subject", "predicate", "object"} {
			if value, exists := candidate.Attributes[key]; exists {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("%w: fact candidate %d attribute %q must be a string", errInvalidInput, index, key)
				}
			}
		}
		if value, exists := candidate.Attributes["entities"]; exists {
			switch typed := value.(type) {
			case []string:
			case []any:
				for _, item := range typed {
					if _, ok := item.(string); !ok {
						return fmt.Errorf("%w: fact candidate %d attribute %q must contain only strings", errInvalidInput, index, "entities")
					}
				}
			default:
				return fmt.Errorf("%w: fact candidate %d attribute %q must be a string array", errInvalidInput, index, "entities")
			}
		}
	}
	return nil
}

func flowcraftFactCandidates(candidates []memorystore.FactCandidate, observedAt time.Time) []recall.TemporalFact {
	facts := make([]recall.TemporalFact, 0, len(candidates))
	for _, candidate := range candidates {
		attributes := cloneMap(candidate.Attributes)
		kind := recall.FactKind(stringAttribute(attributes, "kind"))
		if !kind.IsValid() {
			kind = recall.FactNote
		}
		fact := recall.TemporalFact{
			Kind:       kind,
			Content:    strings.TrimSpace(candidate.Text),
			Subject:    stringAttribute(attributes, "subject"),
			Predicate:  stringAttribute(attributes, "predicate"),
			Object:     stringAttribute(attributes, "object"),
			Entities:   stringSliceAttribute(attributes, "entities"),
			ObservedAt: observedAt,
			ValidFrom:  &observedAt,
			Polarity:   recall.PolarityAffirmed,
			Modality:   recall.ModalityActual,
			Certainty:  recall.CertaintyExplicit,
			Confidence: 0.9,
			Metadata:   attributes,
		}
		facts = append(facts, fact)
	}
	return facts
}

func stringAttribute(attributes map[string]any, key string) string {
	value, _ := attributes[key].(string)
	delete(attributes, key)
	return strings.TrimSpace(value)
}

func stringSliceAttribute(attributes map[string]any, key string) []string {
	value := attributes[key]
	delete(attributes, key)
	var result []string
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				result = append(result, item)
			}
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if text = strings.TrimSpace(text); text != "" {
					result = append(result, text)
				}
			}
		}
	}
	return result
}

// Recall returns facts relevant to the query.
func (s *Store) Recall(ctx context.Context, query memorystore.Query) (memorystore.RecallResult, error) {
	if err := validateQuery(query); err != nil {
		return recallResult{}, err
	}
	flowQuery := recall.Query{Text: query.Text, Limit: query.Limit}
	for _, filter := range query.Filters {
		if filter.Operator != filterEqual {
			return recallResult{}, fmt.Errorf("%w: flowcraft filter operator %q", errUnsupported, filter.Operator)
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
		case "include_retired":
			include, ok := filter.Value.(bool)
			if !ok {
				return recallResult{}, fmt.Errorf("%w: flowcraft include_retired filter requires a boolean", errUnsupported)
			}
			flowQuery.IncludeRetired = include
		default:
			return recallResult{}, fmt.Errorf("%w: flowcraft filter field %q", errUnsupported, filter.Field)
		}
	}
	scope := nativeScope(query.Scope)
	hits, err := s.memory.Recall(ctx, scope, flowQuery)
	if err != nil {
		return recallResult{}, mapFlowcraftError("recall", err)
	}
	out := recallResult{Matches: make([]match, len(hits))}
	for i, hit := range hits {
		fact, err := s.factFromFlowcraft(ctx, scope, hit.Fact)
		if err != nil {
			return recallResult{}, err
		}
		out.Matches[i] = match{Fact: fact, Score: hit.Score}
	}
	sort.SliceStable(out.Matches, func(i, j int) bool {
		return out.Matches[i].Score > out.Matches[j].Score
	})
	return out, nil
}

// Update appends a Flowcraft revision that supersedes the current fact.
func (s *Store) Update(ctx context.Context, request memorystore.UpdateRequest) (memorystore.Fact, error) {
	if err := validateUpdate(request); err != nil {
		return fact{}, err
	}
	if err := validateFlowcraftAttributePatch(request.Attributes); err != nil {
		return fact{}, err
	}
	scope, nativeID, err := decodeLocator(request.ID)
	if err != nil {
		return fact{}, err
	}
	current, err := s.currentFact(ctx, scope, nativeID)
	if err != nil {
		return fact{}, err
	}
	if request.ExpectedRevision != "" {
		revisionScope, revisionID, decodeErr := decodeLocator(request.ExpectedRevision)
		if decodeErr != nil || !sameScope(revisionScope, scope) || revisionID != current.ID {
			return fact{}, fmt.Errorf("%w: fact revision changed", errConflict)
		}
	}
	next := current.Clone()
	next.ID = ""
	next.CorrectedBy = ""
	next.ValidTo = nil
	next.Origin = recall.FactOrigin{}
	next.Supersedes = []string{current.ID}
	next.ObservedAt = time.Now()
	if next.Metadata == nil {
		next.Metadata = make(map[string]any)
	}
	next.Metadata[flowcraftRootIDAttribute] = nativeID
	if request.Text != nil {
		next.Content = *request.Text
	}
	for key, value := range request.Attributes.Set {
		next.Metadata[key] = cloneValue(value)
	}
	for _, key := range request.Attributes.Delete {
		delete(next.Metadata, key)
	}
	result, err := s.memory.Save(ctx, scope, recall.SaveRequest{Facts: []recall.TemporalFact{next}, ObservedAt: next.ObservedAt})
	if err != nil {
		return fact{}, mapFlowcraftError("update", err)
	}
	if err := s.drainSideEffects(ctx, scope); err != nil {
		return fact{}, err
	}
	if len(result.FactIDs) != 1 {
		return fact{}, fmt.Errorf("%w: flowcraft update returned %d facts", errUnavailable, len(result.FactIDs))
	}
	return s.factByID(ctx, scope, result.FactIDs[0])
}

// Delete soft-retires a Flowcraft fact while preserving its audit history.
func (s *Store) Delete(ctx context.Context, request memorystore.DeleteRequest) error {
	if err := validateDelete(request); err != nil {
		return err
	}
	scope, nativeID, err := decodeLocator(request.ID)
	if err != nil {
		return err
	}
	current, err := s.currentFact(ctx, scope, nativeID)
	if err != nil {
		return err
	}
	if request.ExpectedRevision != "" {
		revisionScope, revisionID, decodeErr := decodeLocator(request.ExpectedRevision)
		if decodeErr != nil || !sameScope(revisionScope, scope) || revisionID != current.ID {
			return fmt.Errorf("%w: fact revision changed", errConflict)
		}
	}
	if err := s.memory.Forget(ctx, scope, current.ID, recall.ForgetSoft); err != nil {
		return mapFlowcraftError("delete", err)
	}
	return nil
}

func (s *Store) currentFact(ctx context.Context, scope recall.Scope, id string) (recall.TemporalFact, error) {
	lineage, err := s.memory.Lineage(ctx, scope, id)
	if err != nil {
		return recall.TemporalFact{}, mapFlowcraftError("lineage", err)
	}
	if len(lineage) == 0 {
		return recall.TemporalFact{}, fmt.Errorf("%w: fact %q", errNotFound, id)
	}
	var current *recall.TemporalFact
	for i := range lineage {
		if lineage[i].Fact.CorrectedBy == "" && !lineage[i].Fact.Closed {
			if current != nil {
				return recall.TemporalFact{}, fmt.Errorf("%w: fact %q has multiple current revisions", errConflict, id)
			}
			fact := lineage[i].Fact
			current = &fact
		}
	}
	if current != nil {
		return *current, nil
	}
	return recall.TemporalFact{}, fmt.Errorf("%w: fact %q has no current revision", errNotFound, id)
}

func (s *Store) factByID(ctx context.Context, scope recall.Scope, id string) (fact, error) {
	nativeFact, err := s.currentFact(ctx, scope, id)
	if err != nil {
		return fact{}, err
	}
	return s.factFromFlowcraft(ctx, scope, nativeFact)
}

func (s *Store) loadFacts(ctx context.Context, scope recall.Scope, ids []string) ([]fact, error) {
	output := make([]fact, 0, len(ids))
	for _, id := range ids {
		fact, err := s.factByID(ctx, scope, id)
		if err != nil {
			return nil, err
		}
		output = append(output, fact)
	}
	return output, nil
}

func flowcraftTurns(observation observation) []recall.TurnContext {
	turns := make([]recall.TurnContext, 0, len(observation.Turns)+1)
	if strings.TrimSpace(observation.Text) != "" {
		turns = append(turns, recall.TurnContext{ID: observation.ID, EvidenceID: observation.ID, SessionID: observation.ID, Role: string(roleUser), Time: observation.ObservedAt, Text: observation.Text})
	}
	for _, turn := range observation.Turns {
		turns = append(turns, recall.TurnContext{ID: turn.ID, EvidenceID: turn.ID, SessionID: observation.ID, Role: string(turn.Role), Speaker: turn.Speaker, Time: turn.ObservedAt, Text: turn.Text})
	}
	return turns
}

const (
	flowcraftRootIDAttribute           = "gizclaw.root_id"
	flowcraftProvenanceMarkerAttribute = "gizclaw.provenance_marker"
)

var flowcraftReservedAttributes = map[string]struct{}{
	flowcraftRootIDAttribute:           {},
	flowcraftOperationStatusAttribute:  {},
	flowcraftProvenanceMarkerAttribute: {},
	"observation_id":                   {},
	"kind":                             {},
	"subject":                          {},
	"predicate":                        {},
	"object":                           {},
	"entities":                         {},
}

func validateFlowcraftAttributeKeys(attributes map[string]any) error {
	for key := range attributes {
		if _, reserved := flowcraftReservedAttributes[key]; reserved {
			return fmt.Errorf("%w: flowcraft attribute %q is provider-owned", errUnsupported, key)
		}
	}
	return nil
}

func validateFlowcraftAttributePatch(patch attributePatch) error {
	if err := validateFlowcraftAttributeKeys(patch.Set); err != nil {
		return err
	}
	for _, key := range patch.Delete {
		if _, reserved := flowcraftReservedAttributes[key]; reserved {
			return fmt.Errorf("%w: flowcraft attribute %q is provider-owned", errUnsupported, key)
		}
	}
	return nil
}

func (s *Store) factFromFlowcraft(ctx context.Context, scope recall.Scope, input recall.TemporalFact) (fact, error) {
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
		lineage, err := s.memory.Lineage(ctx, scope, input.ID)
		if err != nil {
			return fact{}, mapFlowcraftError("resolve root revision", err)
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
	if observationID == "" && s.config.Extraction.Model != "" {
		resolvedObservationID, err := s.observationIDForFact(ctx, scope, input)
		if err != nil {
			return fact{}, err
		}
		observationID = resolvedObservationID
	}
	var sources []sourceRef
	if observationID != "" || len(turnIDs) > 0 {
		sources = []sourceRef{{ObservationID: observationID, TurnIDs: turnIDs}}
	}
	return fact{ID: encodeLocator(scope, rootID), Revision: encodeLocator(scope, input.ID), Text: input.Content, Attributes: attributes, Sources: sources, CreatedAt: createdAt, UpdatedAt: input.ObservedAt}, nil
}

func (s *Store) persistObservationProvenance(ctx context.Context, scope recall.Scope, observationID string, result recall.SaveResult) error {
	if observationID == "" || s.config.Extraction.Model == "" {
		return nil
	}
	markers := make([]recall.TemporalFact, 0, len(result.FactIDs)+1)
	if result.SemanticPending {
		markers = append(markers, flowcraftProvenanceMarker(scope, "operation", result.AsyncRequestID, observationID, result.AsyncRequestID))
	} else {
		for _, factID := range result.FactIDs {
			markers = append(markers, flowcraftProvenanceMarker(scope, "fact", factID, observationID, ""))
		}
	}
	if err := s.temporal.Append(ctx, markers); err != nil {
		return mapFlowcraftError("persist observation provenance", err)
	}
	return nil
}

func flowcraftProvenanceMarker(scope recall.Scope, kind, key, observationID, requestID string) recall.TemporalFact {
	return recall.TemporalFact{
		ID:         flowcraftProvenanceMarkerID(kind, key),
		Scope:      scope,
		Kind:       recall.FactEpisode,
		ObservedAt: time.Now(),
		Origin:     recall.FactOrigin{RequestID: requestID, Kind: recall.OriginKindEpisode},
		Metadata: map[string]any{
			flowcraftProvenanceMarkerAttribute: true,
			"observation_id":                   observationID,
		},
	}
}

func flowcraftProvenanceMarkerID(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + key))
	return fmt.Sprintf("gizclaw-provenance-%x", sum)
}

func isFlowcraftProvenanceMarker(fact recall.TemporalFact) bool {
	marker, _ := fact.Metadata[flowcraftProvenanceMarkerAttribute].(bool)
	return marker && fact.Kind == recall.FactEpisode
}

func (s *Store) observationIDForFact(ctx context.Context, scope recall.Scope, fact recall.TemporalFact) (string, error) {
	if fact.Origin.RequestID != "" {
		return s.observationIDFromMarker(ctx, scope, "operation", fact.Origin.RequestID)
	}
	facts := []recall.TemporalFact{fact}
	if len(fact.Supersedes) > 0 {
		lineage, err := s.memory.Lineage(ctx, scope, fact.ID)
		if err != nil {
			return "", mapFlowcraftError("load observation provenance lineage", err)
		}
		for _, node := range lineage {
			if node.Fact.ID != fact.ID {
				facts = append(facts, node.Fact)
			}
		}
	}
	for _, candidate := range facts {
		kind, key := "fact", candidate.ID
		if candidate.Origin.RequestID != "" {
			kind, key = "operation", candidate.Origin.RequestID
		}
		observationID, err := s.observationIDFromMarker(ctx, scope, kind, key)
		if err != nil {
			return "", err
		}
		if observationID != "" {
			return observationID, nil
		}
	}
	return "", nil
}

func (s *Store) observationIDFromMarker(ctx context.Context, scope recall.Scope, kind, key string) (string, error) {
	marker, err := s.temporal.Get(ctx, scope, flowcraftProvenanceMarkerID(kind, key))
	if err != nil {
		mapped := mapFlowcraftError("load observation provenance", err)
		if errors.Is(mapped, errNotFound) {
			return "", nil
		}
		return "", mapped
	}
	if !isFlowcraftProvenanceMarker(marker) {
		return "", fmt.Errorf("%w: invalid flowcraft observation provenance", errUnavailable)
	}
	observationID, _ := marker.Metadata["observation_id"].(string)
	return observationID, nil
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
		return fmt.Errorf("%w: flowcraft %s: %v", errInvalidInput, operation, err)
	case errdefs.IsNotFound(err):
		return fmt.Errorf("%w: flowcraft %s", errNotFound, operation)
	case errdefs.IsConflict(err):
		return fmt.Errorf("%w: flowcraft %s", errConflict, operation)
	case errdefs.IsNotAvailable(err):
		return fmt.Errorf("%w: flowcraft %s", errUnavailable, operation)
	default:
		return fmt.Errorf("flowcraft %s: %w", operation, err)
	}
}

var _ storeContract = (*Store)(nil)
var _ operationWaiterContract = (*Store)(nil)
