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
	if s.config.Flavor == Mem0SelfHosted {
		for key, value := range s.entityFields() {
			payload[key] = value
		}
	}
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
	path := "/v1/memories/" + url.PathEscape(request.ID)
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
	path := "/v1/memories/" + url.PathEscape(request.ID)
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
	for key, value := range map[string]string{"app_id": s.config.AppID, "user_id": s.config.UserID, "agent_id": s.config.AgentID, "run_id": s.config.RunID} {
		if value != "" {
			fields[key] = value
		}
	}
	return fields
}

func (s *Mem0Store) mem0Filters(input []Filter) (map[string]any, error) {
	clauses := make([]map[string]any, 0, len(input)+4)
	for key, value := range s.entityFields() {
		clauses = append(clauses, map[string]any{key: value})
	}
	operators := map[FilterOperator]string{FilterNotEqual: "ne", FilterIn: "in", FilterGreaterThan: "gt", FilterGreaterEqual: "gte", FilterLessThan: "lt", FilterLessEqual: "lte"}
	for _, filter := range input {
		if filter.Operator == FilterEqual {
			clauses = append(clauses, map[string]any{filter.Field: cloneValue(filter.Value)})
			continue
		}
		op, ok := operators[filter.Operator]
		if !ok {
			return nil, fmt.Errorf("%w: mem0 filter operator %q", ErrUnsupported, filter.Operator)
		}
		clauses = append(clauses, map[string]any{filter.Field: map[string]any{op: cloneValue(filter.Value)}})
	}
	if len(clauses) == 0 {
		return map[string]any{}, nil
	}
	if len(clauses) == 1 {
		return clauses[0], nil
	}
	items := make([]any, len(clauses))
	for i := range clauses {
		items[i] = clauses[i]
	}
	return map[string]any{"AND": items}, nil
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
	EventID   string          `json:"event_id"`
	Status    string          `json:"status"`
	Error     string          `json:"error"`
	ID        string          `json:"id"`
	Hash      string          `json:"hash"`
	Memory    string          `json:"memory"`
	Text      string          `json:"text"`
	Score     float64         `json:"score"`
	Metadata  map[string]any  `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Results   json.RawMessage `json:"results"`
	Data      json.RawMessage `json:"data"`
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
