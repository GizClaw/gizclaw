package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Mem0Flavor selects the remote Mem0 HTTP protocol variant.
type Mem0Flavor string

const (
	Mem0Platform   Mem0Flavor = "platform"
	Mem0SelfHosted Mem0Flavor = "self_hosted"
)

// Mem0Config configures a Mem0 Platform or self-hosted HTTP client.
// Entity IDs are business memory scopes, not transport tenants.
type Mem0Config struct {
	Endpoint     string        `yaml:"endpoint"`
	APIKey       string        `yaml:"api_key"`
	Flavor       Mem0Flavor    `yaml:"flavor"`
	AppID        string        `yaml:"app_id"`
	UserID       string        `yaml:"user_id"`
	AgentID      string        `yaml:"agent_id"`
	RunID        string        `yaml:"run_id"`
	PollInterval time.Duration `yaml:"poll_interval"`
}

// Mem0Store adapts Mem0's fact-centric remote API to Store.
type Mem0Store struct {
	config Mem0Config
	client *mem0Client
}

const (
	mem0ObservationIDMetadata = "gizclaw.observation_id"
	mem0TurnIDsMetadata       = "gizclaw.turn_ids"
)

// NewMem0Store constructs a remote Mem0 adapter without performing I/O.
func NewMem0Store(config Mem0Config, client HTTPClient) (*Mem0Store, error) {
	if config.Flavor == "" {
		config.Flavor = Mem0Platform
	}
	if config.Flavor != Mem0Platform && config.Flavor != Mem0SelfHosted {
		return nil, fmt.Errorf("%w: unknown mem0 flavor %q", ErrInvalidInput, config.Flavor)
	}
	if config.Flavor == Mem0Platform && !hasMem0EntityScope(config) {
		return nil, fmt.Errorf("%w: mem0 platform requires at least one app_id, user_id, agent_id, or run_id", ErrInvalidInput)
	}
	if config.Flavor == Mem0SelfHosted && !hasMem0SelfHostedScope(config) {
		return nil, fmt.Errorf("%w: self-hosted mem0 requires at least one user_id, agent_id, or run_id", ErrInvalidInput)
	}
	if config.Flavor == Mem0SelfHosted && strings.TrimSpace(config.AppID) != "" {
		return nil, fmt.Errorf("%w: self-hosted mem0 does not support app_id", ErrInvalidInput)
	}
	if config.Flavor == Mem0Platform && strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%w: mem0 platform api_key is required", ErrInvalidInput)
	}
	if config.Endpoint == "" {
		if config.Flavor == Mem0Platform {
			config.Endpoint = "https://api.mem0.ai"
		} else {
			return nil, fmt.Errorf("%w: self-hosted mem0 endpoint is required", ErrInvalidInput)
		}
	}
	if config.PollInterval < 0 {
		return nil, fmt.Errorf("%w: mem0 poll_interval must not be negative", ErrInvalidInput)
	}
	transport, err := newMem0Client(config.Endpoint, config.APIKey, config.Flavor, client)
	if err != nil {
		return nil, err
	}
	return &Mem0Store{config: config, client: transport}, nil
}

func hasMem0SelfHostedScope(config Mem0Config) bool {
	return strings.TrimSpace(config.UserID) != "" ||
		strings.TrimSpace(config.AgentID) != "" ||
		strings.TrimSpace(config.RunID) != ""
}

func hasMem0EntityScope(config Mem0Config) bool {
	return strings.TrimSpace(config.AppID) != "" ||
		strings.TrimSpace(config.UserID) != "" ||
		strings.TrimSpace(config.AgentID) != "" ||
		strings.TrimSpace(config.RunID) != ""
}

// Observe submits raw messages for Mem0 extraction.
func (s *Mem0Store) Observe(ctx context.Context, observation Observation) (ObserveResult, error) {
	if err := validateObservation(observation); err != nil {
		return ObserveResult{}, err
	}
	if err := validateMem0Metadata(observation.Context); err != nil {
		return ObserveResult{}, err
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
	for key, value := range s.entityFields() {
		payload[key] = value
	}
	path := "/memories"
	if s.config.Flavor == Mem0Platform {
		path = "/v3/memories/add/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return ObserveResult{}, err
	}
	if response.EventID != "" {
		return ObserveResult{Operation: &Operation{ID: response.EventID, Status: OperationPending}}, nil
	}
	return ObserveResult{Facts: response.facts()}, nil
}

func validateMem0Metadata(metadata map[string]any) error {
	for _, key := range []string{mem0ObservationIDMetadata, mem0TurnIDsMetadata} {
		if _, exists := metadata[key]; exists {
			return fmt.Errorf("%w: mem0 metadata %q is provider-owned", ErrUnsupported, key)
		}
	}
	return nil
}

// Recall performs semantic search with provider-native structured filters.
func (s *Mem0Store) Recall(ctx context.Context, query Query) (RecallResult, error) {
	if err := validateQuery(query); err != nil {
		return RecallResult{}, err
	}
	filters, err := s.mem0Filters(query.Filters)
	if err != nil {
		return RecallResult{}, err
	}
	payload := map[string]any{"query": query.Text, "top_k": query.Limit, "filters": filters}
	path := "/search"
	if s.config.Flavor == Mem0Platform {
		path = "/v3/memories/search/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return RecallResult{}, err
	}
	entries := response.entries()
	result := RecallResult{Matches: make([]Match, len(entries))}
	for index, entry := range entries {
		result.Matches[index] = Match{Fact: entry.fact(), Score: entry.Score}
	}
	return result, nil
}

// Update revises one Mem0 memory. Mem0 does not expose conditional writes.
func (s *Mem0Store) Update(ctx context.Context, request UpdateRequest) (Fact, error) {
	if err := validateUpdate(request); err != nil {
		return Fact{}, err
	}
	if request.ExpectedRevision != "" {
		return Fact{}, fmt.Errorf("%w: mem0 does not expose conditional updates", ErrUnsupported)
	}
	if len(request.Attributes.Set) > 0 || len(request.Attributes.Delete) > 0 {
		return Fact{}, fmt.Errorf("%w: mem0 does not expose attribute patch updates", ErrUnsupported)
	}
	payload := map[string]any{}
	if request.Text != nil {
		payload["text"] = *request.Text
	}
	var response mem0Envelope
	path := "/v1/memories/" + url.PathEscape(request.ID) + "/"
	if s.config.Flavor == Mem0SelfHosted {
		path = "/memories/" + url.PathEscape(request.ID)
	}
	if err := s.client.do(ctx, http.MethodPut, path, payload, &response); err != nil {
		return Fact{}, err
	}
	facts := response.facts()
	if len(facts) > 0 {
		return facts[0], nil
	}
	return Fact{ID: request.ID, Text: *request.Text}, nil
}

// Delete removes one Mem0 memory. Mem0 does not expose conditional deletes.
func (s *Mem0Store) Delete(ctx context.Context, request DeleteRequest) error {
	if err := validateDelete(request); err != nil {
		return err
	}
	if request.ExpectedRevision != "" {
		return fmt.Errorf("%w: mem0 does not expose conditional deletes", ErrUnsupported)
	}
	path := "/v1/memories/" + url.PathEscape(request.ID) + "/"
	if s.config.Flavor == Mem0SelfHosted {
		path = "/memories/" + url.PathEscape(request.ID)
	}
	return s.client.do(ctx, http.MethodDelete, path, nil, nil)
}

// Wait polls an asynchronous Mem0 Platform event.
func (s *Mem0Store) Wait(ctx context.Context, operationID string) (ObserveResult, error) {
	if strings.TrimSpace(operationID) == "" {
		return ObserveResult{}, fmt.Errorf("%w: mem0 operation id is required", ErrInvalidInput)
	}
	if s.config.Flavor != Mem0Platform {
		return ObserveResult{}, fmt.Errorf("%w: self-hosted mem0 has no event API", ErrUnsupported)
	}
	interval := s.config.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		var response mem0Envelope
		if err := s.client.do(ctx, http.MethodGet, "/v1/event/"+url.PathEscape(operationID)+"/", nil, &response); err != nil {
			return ObserveResult{}, err
		}
		status := strings.ToLower(response.Status)
		switch status {
		case "completed", "complete", "succeeded", "success":
			return ObserveResult{Facts: response.resultFacts(), Operation: &Operation{ID: operationID, Status: OperationSucceeded}}, nil
		case "failed", "error":
			return ObserveResult{Operation: &Operation{ID: operationID, Status: OperationFailed, Error: "mem0 operation failed"}}, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ObserveResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Mem0Store) entityFields() map[string]string {
	fields := make(map[string]string)
	entities := map[string]string{"user_id": s.config.UserID, "agent_id": s.config.AgentID, "run_id": s.config.RunID}
	if s.config.Flavor == Mem0Platform {
		entities["app_id"] = s.config.AppID
	}
	for key, value := range entities {
		if value = strings.TrimSpace(value); value != "" {
			fields[key] = value
		}
	}
	return fields
}

func (s *Mem0Store) mem0Filters(input []Filter) (map[string]any, error) {
	clauses := []any{s.mem0ScopeFilter()}
	for _, filter := range input {
		clause, err := mem0FilterClause(filter)
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

func (s *Mem0Store) mem0ScopeFilter() map[string]any {
	fields := []struct {
		name  string
		value string
	}{
		{name: "app_id", value: s.config.AppID},
		{name: "user_id", value: s.config.UserID},
		{name: "agent_id", value: s.config.AgentID},
		{name: "run_id", value: s.config.RunID},
	}
	clauses := make([]any, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field.value)
		if value != "" && (field.name != "app_id" || s.config.Flavor == Mem0Platform) {
			clauses = append(clauses, map[string]any{field.name: value})
		}
	}
	if len(clauses) == 1 {
		return clauses[0].(map[string]any)
	}
	return map[string]any{"OR": clauses}
}

func mem0FilterClause(filter Filter) (map[string]any, error) {
	field := strings.TrimSpace(filter.Field)
	if field == mem0ObservationIDMetadata || field == mem0TurnIDsMetadata {
		return nil, fmt.Errorf("%w: mem0 filter field %q is provider-owned", ErrUnsupported, field)
	}
	if !isMem0NativeFilterField(field) {
		switch filter.Operator {
		case FilterEqual:
			return map[string]any{"metadata": map[string]any{field: cloneValue(filter.Value)}}, nil
		case FilterNotEqual:
			return map[string]any{"metadata": map[string]any{field: map[string]any{"ne": cloneValue(filter.Value)}}}, nil
		default:
			return nil, fmt.Errorf("%w: mem0 metadata filter operator %q", ErrUnsupported, filter.Operator)
		}
	}

	if filter.Operator == FilterNotIn {
		if !mem0FieldSupports(field, FilterIn) {
			return nil, fmt.Errorf("%w: mem0 field %q does not support filter operator %q", ErrUnsupported, field, filter.Operator)
		}
		return map[string]any{"NOT": map[string]any{field: mem0InValue(field, filter.Value)}}, nil
	}
	if !mem0FieldSupports(field, filter.Operator) {
		return nil, fmt.Errorf("%w: mem0 field %q does not support filter operator %q", ErrUnsupported, field, filter.Operator)
	}
	if filter.Operator == FilterEqual {
		return map[string]any{field: cloneValue(filter.Value)}, nil
	}
	if filter.Operator == FilterIn {
		return map[string]any{field: mem0InValue(field, filter.Value)}, nil
	}
	operators := map[FilterOperator]string{FilterNotEqual: "ne", FilterGreaterThan: "gt", FilterGreaterEqual: "gte", FilterLessThan: "lt", FilterLessEqual: "lte"}
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
	case "user_id", "agent_id", "app_id", "run_id", "created_at", "updated_at", "timestamp", "categories", "metadata", "keywords", "memory_ids":
		return true
	default:
		return false
	}
}

func mem0FieldSupports(field string, operator FilterOperator) bool {
	switch field {
	case "user_id", "agent_id", "app_id", "run_id":
		return operator == FilterEqual || operator == FilterNotEqual || operator == FilterIn
	case "created_at", "updated_at", "timestamp":
		return operator == FilterEqual || operator == FilterNotEqual || operator == FilterGreaterThan || operator == FilterGreaterEqual || operator == FilterLessThan || operator == FilterLessEqual
	case "categories":
		return operator == FilterEqual || operator == FilterNotEqual || operator == FilterIn
	case "memory_ids":
		return operator == FilterIn
	default:
		return false
	}
}

type mem0Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

func mem0Messages(observation Observation) []mem0Message {
	messages := make([]mem0Message, 0, len(observation.Turns)+1)
	if strings.TrimSpace(observation.Text) != "" {
		messages = append(messages, mem0Message{Role: RoleUser, Content: observation.Text})
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

func (e mem0Envelope) facts() []Fact {
	entries := e.entries()
	facts := make([]Fact, len(entries))
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

func (e mem0Envelope) resultFacts() []Fact {
	entries := e.resultEntries()
	facts := make([]Fact, len(entries))
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

func (e mem0Envelope) fact() Fact {
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
	var sources []SourceRef
	if observationID != "" || len(turnIDs) > 0 {
		sources = []SourceRef{{ObservationID: observationID, TurnIDs: turnIDs}}
	}
	return Fact{ID: e.ID, Revision: revision, Text: text, Attributes: attributes, Sources: sources, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

var _ Store = (*Mem0Store)(nil)
var _ OperationWaiter = (*Mem0Store)(nil)
