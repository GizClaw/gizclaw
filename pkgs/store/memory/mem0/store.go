package mem0

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Flavor selects the remote Mem0 HTTP protocol variant.
type Flavor string

const (
	Platform   Flavor = "platform"
	SelfHosted Flavor = "self_hosted"
)

// Config configures a Mem0 Platform or self-hosted HTTP client.
// Entity IDs are business memory scopes, not transport tenants.
type Config struct {
	Endpoint     string
	APIKey       string
	Flavor       Flavor
	PollInterval time.Duration
	HTTPClient   HTTPClient
}

// Store adapts Mem0's fact-centric remote API to Store.
type Store struct {
	config Config
	client *mem0Client
}

const (
	mem0ObservationIDMetadata = "gizclaw.observation_id"
	mem0TurnIDsMetadata       = "gizclaw.turn_ids"
)

// New constructs a remote Mem0 adapter without performing I/O.
func New(config Config) (*Store, error) {
	if config.Flavor == "" {
		config.Flavor = Platform
	}
	if config.Flavor != Platform && config.Flavor != SelfHosted {
		return nil, fmt.Errorf("%w: unknown mem0 flavor %q", errInvalidInput, config.Flavor)
	}
	if config.Flavor == Platform && strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%w: mem0 platform api_key is required", errInvalidInput)
	}
	if config.Endpoint == "" {
		if config.Flavor == Platform {
			config.Endpoint = "https://api.mem0.ai"
		} else {
			return nil, fmt.Errorf("%w: self-hosted mem0 endpoint is required", errInvalidInput)
		}
	}
	if config.PollInterval < 0 {
		return nil, fmt.Errorf("%w: mem0 poll_interval must not be negative", errInvalidInput)
	}
	transport, err := newMem0Client(config.Endpoint, config.APIKey, config.Flavor, config.HTTPClient)
	if err != nil {
		return nil, err
	}
	return &Store{config: config, client: transport}, nil
}

// Observe submits raw messages for Mem0 extraction.
func (s *Store) Observe(ctx context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	if err := validateObservation(observation); err != nil {
		return observeResult{}, err
	}
	if len(observation.Facts) > 0 {
		return observeResult{}, fmt.Errorf("%w: mem0 does not expose direct structured fact ingestion", errUnsupported)
	}
	if err := validateMem0Metadata(observation.Context); err != nil {
		return observeResult{}, err
	}
	metadata := cloneMap(observation.Context)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if observation.ID != "" {
		metadata[mem0ObservationIDMetadata] = observation.ID
	}
	turnIDs := make([]string, 0, len(observation.Turns))
	for _, turn := range observation.Turns {
		if turn.ID != "" {
			turnIDs = append(turnIDs, turn.ID)
		}
	}
	if len(turnIDs) > 0 {
		metadata[mem0TurnIDsMetadata] = turnIDs
	}
	payload := map[string]any{
		"messages": mem0Messages(observation),
		"metadata": metadata,
		"infer":    true,
	}
	for key, value := range s.entityFields(observation.Scope) {
		payload[key] = value
	}
	path := "/memories"
	if s.config.Flavor == Platform {
		path = "/v3/memories/add/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return observeResult{}, err
	}
	if response.EventID != "" {
		return observeResult{Operation: &memorystore.Operation{ID: response.EventID, Status: operationPending}}, nil
	}
	return observeResult{Facts: response.facts()}, nil
}

func validateMem0Metadata(metadata map[string]any) error {
	for _, key := range []string{mem0ObservationIDMetadata, mem0TurnIDsMetadata} {
		if _, exists := metadata[key]; exists {
			return fmt.Errorf("%w: mem0 metadata %q is provider-owned", errUnsupported, key)
		}
	}
	return nil
}

// Recall performs semantic search with provider-native structured filters.
func (s *Store) Recall(ctx context.Context, query memorystore.Query) (memorystore.RecallResult, error) {
	if err := validateQuery(query); err != nil {
		return recallResult{}, err
	}
	filters, err := s.mem0Filters(query.Scope, query.Filters)
	if err != nil {
		return recallResult{}, err
	}
	payload := map[string]any{"query": query.Text, "top_k": query.Limit, "filters": filters}
	path := "/search"
	if s.config.Flavor == Platform {
		path = "/v3/memories/search/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return recallResult{}, err
	}
	entries := response.entries()
	result := recallResult{Matches: make([]match, len(entries))}
	for index, entry := range entries {
		result.Matches[index] = match{Fact: entry.fact(), Score: entry.Score}
	}
	return result, nil
}

// Update revises one Mem0 memory. Mem0 does not expose conditional writes.
func (s *Store) Update(ctx context.Context, request memorystore.UpdateRequest) (memorystore.Fact, error) {
	if err := validateUpdate(request); err != nil {
		return fact{}, err
	}
	if request.ExpectedRevision != "" {
		return fact{}, fmt.Errorf("%w: mem0 does not expose conditional updates", errUnsupported)
	}
	if len(request.Attributes.Set) > 0 || len(request.Attributes.Delete) > 0 {
		return fact{}, fmt.Errorf("%w: mem0 does not expose attribute patch updates", errUnsupported)
	}
	payload := map[string]any{}
	if request.Text != nil {
		payload["text"] = *request.Text
	}
	var response mem0Envelope
	path := "/v1/memories/" + url.PathEscape(request.ID) + "/"
	if s.config.Flavor == SelfHosted {
		path = "/memories/" + url.PathEscape(request.ID)
	}
	if err := s.client.do(ctx, http.MethodPut, path, payload, &response); err != nil {
		return fact{}, err
	}
	facts := response.facts()
	if len(facts) > 0 {
		return facts[0], nil
	}
	return fact{ID: request.ID, Text: *request.Text}, nil
}

// Delete removes one Mem0 memory. Mem0 does not expose conditional deletes.
func (s *Store) Delete(ctx context.Context, request memorystore.DeleteRequest) error {
	if err := validateDelete(request); err != nil {
		return err
	}
	if request.ExpectedRevision != "" {
		return fmt.Errorf("%w: mem0 does not expose conditional deletes", errUnsupported)
	}
	path := "/v1/memories/" + url.PathEscape(request.ID) + "/"
	if s.config.Flavor == SelfHosted {
		path = "/memories/" + url.PathEscape(request.ID)
	}
	return s.client.do(ctx, http.MethodDelete, path, nil, nil)
}

// Wait polls an asynchronous Mem0 Platform event.
func (s *Store) Wait(ctx context.Context, operationID string) (memorystore.ObserveResult, error) {
	if strings.TrimSpace(operationID) == "" {
		return observeResult{}, fmt.Errorf("%w: mem0 operation id is required", errInvalidInput)
	}
	if s.config.Flavor != Platform {
		return observeResult{}, fmt.Errorf("%w: self-hosted mem0 has no event API", errUnsupported)
	}
	interval := s.config.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		var response mem0Envelope
		if err := s.client.do(ctx, http.MethodGet, "/v1/event/"+url.PathEscape(operationID)+"/", nil, &response); err != nil {
			return observeResult{}, err
		}
		status := strings.ToLower(response.Status)
		switch status {
		case "completed", "complete", "succeeded", "success":
			return observeResult{Facts: response.resultFacts(), Operation: &memorystore.Operation{ID: operationID, Status: operationSucceeded}}, nil
		case "failed", "error":
			return observeResult{Operation: &memorystore.Operation{ID: operationID, Status: operationFailed, Error: "mem0 operation failed"}}, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return observeResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Store) entityFields(scope scope) map[string]string {
	return map[string]string{"user_id": string(scope)}
}

func (s *Store) mem0Filters(scope scope, input []filter) (map[string]any, error) {
	clauses := []any{s.mem0ScopeFilter(scope)}
	for _, filter := range input {
		clause, err := s.mem0FilterClause(filter)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, clause)
	}
	if len(clauses) == 1 {
		return clauses[0].(map[string]any), nil
	}
	return map[string]any{"AND": clauses}, nil
}

func (s *Store) mem0ScopeFilter(scope scope) map[string]any {
	return map[string]any{"user_id": string(scope)}
}

func (s *Store) mem0FilterClause(filter filter) (map[string]any, error) {
	field := strings.TrimSpace(filter.Field)
	if field == mem0ObservationIDMetadata || field == mem0TurnIDsMetadata || isMem0RoutingField(field) {
		return nil, fmt.Errorf("%w: mem0 filter field %q is provider-owned", errUnsupported, field)
	}
	if !isMem0NativeFilterField(field) {
		value := cloneValue(filter.Value)
		if filter.Operator == filterNotEqual {
			value = map[string]any{"ne": value}
		} else if filter.Operator != filterEqual {
			return nil, fmt.Errorf("%w: mem0 metadata filter operator %q", errUnsupported, filter.Operator)
		}
		if s.config.Flavor == SelfHosted {
			return map[string]any{field: value}, nil
		}
		return map[string]any{"metadata": map[string]any{field: value}}, nil
	}

	if filter.Operator == filterNotIn {
		if !mem0FieldSupports(field, filterIn) {
			return nil, fmt.Errorf("%w: mem0 field %q does not support filter operator %q", errUnsupported, field, filter.Operator)
		}
		return map[string]any{"NOT": map[string]any{field: mem0InValue(field, filter.Value)}}, nil
	}
	if !mem0FieldSupports(field, filter.Operator) {
		return nil, fmt.Errorf("%w: mem0 field %q does not support filter operator %q", errUnsupported, field, filter.Operator)
	}
	if filter.Operator == filterEqual {
		return map[string]any{field: cloneValue(filter.Value)}, nil
	}
	if filter.Operator == filterIn {
		return map[string]any{field: mem0InValue(field, filter.Value)}, nil
	}
	operators := map[filterOperator]string{filterNotEqual: "ne", filterGreaterThan: "gt", filterGreaterEqual: "gte", filterLessThan: "lt", filterLessEqual: "lte"}
	return map[string]any{field: map[string]any{operators[filter.Operator]: cloneValue(filter.Value)}}, nil
}

func mem0InValue(field string, value any) any {
	if field == "memory_ids" {
		return cloneValue(value)
	}
	return map[string]any{"in": cloneValue(value)}
}

func isMem0NativeFilterField(field string) bool {
	switch field {
	case "created_at", "updated_at", "timestamp", "categories", "metadata", "keywords", "memory_ids":
		return true
	default:
		return false
	}
}

func isMem0RoutingField(field string) bool {
	switch field {
	case "user_id", "agent_id", "app_id", "run_id":
		return true
	default:
		return false
	}
}

func mem0FieldSupports(field string, operator filterOperator) bool {
	switch field {
	case "created_at", "updated_at", "timestamp":
		return operator == filterEqual || operator == filterNotEqual || operator == filterGreaterThan || operator == filterGreaterEqual || operator == filterLessThan || operator == filterLessEqual
	case "categories":
		return operator == filterEqual || operator == filterNotEqual || operator == filterIn
	case "memory_ids":
		return operator == filterIn
	default:
		return false
	}
}

type mem0Message struct {
	Role    role   `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

func mem0Messages(observation observation) []mem0Message {
	messages := make([]mem0Message, 0, len(observation.Turns)+1)
	if strings.TrimSpace(observation.Text) != "" {
		messages = append(messages, mem0Message{Role: roleUser, Content: observation.Text})
	}
	for _, turn := range observation.Turns {
		messages = append(messages, mem0Message{Role: turn.Role, Content: turn.Text, Name: turn.Speaker})
	}
	return messages
}

type mem0Envelope struct {
	EventID    string          `json:"event_id"`
	Status     string          `json:"status"`
	Error      string          `json:"error"`
	ID         string          `json:"id"`
	Hash       string          `json:"hash"`
	Memory     string          `json:"memory"`
	Text       string          `json:"text"`
	Score      float64         `json:"score"`
	Categories []string        `json:"categories"`
	Metadata   map[string]any  `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Results    json.RawMessage `json:"results"`
	Data       json.RawMessage `json:"data"`
}

func (e mem0Envelope) facts() []fact {
	entries := e.entries()
	facts := make([]fact, len(entries))
	for index, entry := range entries {
		facts[index] = entry.fact()
	}
	return facts
}

func (e mem0Envelope) entries() []mem0Envelope {
	if e.ID != "" {
		return []mem0Envelope{e}
	}
	return e.resultEntries()
}

func (e mem0Envelope) resultFacts() []fact {
	entries := e.resultEntries()
	facts := make([]fact, len(entries))
	for index, entry := range entries {
		facts[index] = entry.fact()
	}
	return facts
}

func (e mem0Envelope) resultEntries() []mem0Envelope {
	for _, raw := range []json.RawMessage{e.Results, e.Data} {
		if len(raw) == 0 {
			continue
		}
		var items []mem0Envelope
		if json.Unmarshal(raw, &items) == nil {
			entries := make([]mem0Envelope, 0, len(items))
			for _, item := range items {
				if item.ID != "" {
					entries = append(entries, item)
				}
			}
			return entries
		}
		var nested mem0Envelope
		if json.Unmarshal(raw, &nested) == nil {
			if entries := nested.resultEntries(); len(entries) > 0 {
				return entries
			}
			if nested.ID != "" {
				return []mem0Envelope{nested}
			}
		}
	}
	return nil
}

func (e mem0Envelope) fact() fact {
	text := e.Memory
	if text == "" {
		text = e.Text
	}
	attributes := cloneMap(e.Metadata)
	if attributes == nil {
		attributes = make(map[string]any)
	}
	if len(e.Categories) > 0 {
		attributes["categories"] = append([]string(nil), e.Categories...)
	}
	revision := e.Hash
	if revision == "" {
		hash, _ := attributes["hash"].(string)
		revision = hash
	}
	observationID, _ := attributes[mem0ObservationIDMetadata].(string)
	delete(attributes, mem0ObservationIDMetadata)
	var turnIDs []string
	switch values := attributes[mem0TurnIDsMetadata].(type) {
	case []string:
		turnIDs = append([]string(nil), values...)
	case []any:
		for _, value := range values {
			if id, ok := value.(string); ok {
				turnIDs = append(turnIDs, id)
			}
		}
	}
	delete(attributes, mem0TurnIDsMetadata)
	var sources []sourceRef
	if observationID != "" || len(turnIDs) > 0 {
		sources = []sourceRef{{ObservationID: observationID, TurnIDs: turnIDs}}
	}
	return fact{ID: e.ID, Revision: revision, Text: text, Attributes: attributes, Sources: sources, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

var _ storeContract = (*Store)(nil)
var _ operationWaiterContract = (*Store)(nil)
