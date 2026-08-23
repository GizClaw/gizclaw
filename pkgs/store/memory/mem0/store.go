package mem0

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Flavor selects the remote Mem0 HTTP protocol variant.
type Flavor string

const (
	Platform     Flavor = "platform"
	SelfHosted   Flavor = "self_hosted"
	VolcPlatform Flavor = "volc_platform"
)

// Config configures a Mem0 Platform, Volc-compatible, or self-hosted HTTP client.
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
	config   Config
	client   *mem0Client
	directMu sync.Mutex
}

const (
	mem0ObservationIDMetadata     = "gizclaw.observation_id"
	mem0TurnIDsMetadata           = "gizclaw.turn_ids"
	mem0ObservationDigestMetadata = "gizclaw.observation_digest"
	mem0EntityScopeMetadata       = "gizclaw.entity_scope"
	mem0OperationMarkerMetadata   = "gizclaw.operation_marker"
	volcScopeUserPrefix           = "gizclaw-volc-scope-v1:"
	volcOperationNativePrefix     = "volc-job-v1:"
)

// New constructs a remote Mem0 adapter without performing I/O.
func New(config Config) (*Store, error) {
	if config.Flavor == "" {
		config.Flavor = Platform
	}
	if config.Flavor != Platform && config.Flavor != SelfHosted && config.Flavor != VolcPlatform {
		return nil, fmt.Errorf("%w: unknown mem0 flavor %q", errInvalidInput, config.Flavor)
	}
	if config.Flavor != SelfHosted && strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%w: mem0 %s api_key is required", errInvalidInput, config.Flavor)
	}
	if config.Endpoint == "" {
		if config.Flavor == Platform {
			config.Endpoint = "https://api.mem0.ai"
		} else if config.Flavor == SelfHosted {
			return nil, fmt.Errorf("%w: self-hosted mem0 endpoint is required", errInvalidInput)
		} else {
			return nil, fmt.Errorf("%w: volc mem0 endpoint is required", errInvalidInput)
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

func (s *Store) usesPlatformAPI() bool { return s.config.Flavor != SelfHosted }

func (*Store) SupportsDirectFactObservation() bool { return true }

// Observe submits raw messages for Mem0 extraction or one structured Fact
// through Mem0 direct import.
func (s *Store) Observe(ctx context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	if err := validateObservation(observation); err != nil {
		return observeResult{}, err
	}
	scope, err := normalizeEntityScope(observation.Scope)
	if err != nil {
		return observeResult{}, err
	}
	if len(observation.Facts) > 0 {
		return s.observeDirectFact(ctx, scope, observation)
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
	operationMarker := ""
	if s.config.Flavor == VolcPlatform {
		operationMarker = rand.Text()
		metadata[mem0EntityScopeMetadata] = encodeSelfHostedScope(scope)
		metadata[mem0OperationMarkerMetadata] = operationMarker
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
	if s.config.Flavor == VolcPlatform {
		payload["async_mode"] = true
	}
	for key, value := range s.entityFields(scope) {
		payload[key] = value
	}
	path := "/memories"
	if s.usesPlatformAPI() {
		path = "/v3/memories/add/"
	}
	if s.config.Flavor == VolcPlatform {
		path = "/v1/memories/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return observeResult{}, err
	}
	operationNativeID, err := response.operationID(s.config.Flavor)
	if err != nil {
		return observeResult{}, err
	}
	if operationNativeID != "" {
		if s.config.Flavor == VolcPlatform {
			operationNativeID = encodeVolcOperationNativeID(operationNativeID, operationMarker)
		}
		operationID := encodeOperationLocator(scope, operationNativeID)
		return observeResult{Operation: &memorystore.Operation{ID: operationID, Status: operationPending}}, nil
	}
	facts, err := s.scopedFacts(response.entries(), scope)
	if err != nil {
		return observeResult{}, err
	}
	return observeResult{Facts: facts}, nil
}

func (s *Store) observeDirectFact(ctx context.Context, scope scope, observation observation) (observeResult, error) {
	if len(observation.Facts) != 1 {
		return observeResult{}, fmt.Errorf("%w: mem0 direct observation requires exactly one fact", errUnsupported)
	}
	if strings.TrimSpace(observation.ID) == "" {
		return observeResult{}, fmt.Errorf("%w: mem0 direct observation requires an id", errInvalidInput)
	}
	if strings.TrimSpace(observation.Text) != "" || len(observation.Turns) > 0 || len(observation.Context) > 0 {
		return observeResult{}, fmt.Errorf("%w: mem0 direct observation accepts only a structured fact", errUnsupported)
	}
	if err := validateMem0Metadata(observation.Facts[0].Attributes); err != nil {
		return observeResult{}, err
	}
	digest, err := memorystore.ObservationPayloadDigest(observation)
	if err != nil {
		return observeResult{}, err
	}
	s.directMu.Lock()
	defer s.directMu.Unlock()
	if existing, found, err := s.findDirectObservation(ctx, scope, observation.ID, digest); err != nil {
		return observeResult{}, err
	} else if found {
		return observeResult{Facts: existing}, nil
	}
	metadata := cloneMap(observation.Facts[0].Attributes)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[mem0ObservationIDMetadata] = observation.ID
	metadata[mem0ObservationDigestMetadata] = digest
	operationMarker := ""
	if s.config.Flavor == VolcPlatform {
		operationMarker = rand.Text()
		metadata[mem0EntityScopeMetadata] = encodeSelfHostedScope(scope)
		metadata[mem0OperationMarkerMetadata] = operationMarker
	}
	payload := map[string]any{
		"messages": []mem0Message{{Role: roleUser, Content: strings.TrimSpace(observation.Facts[0].Text)}},
		"metadata": metadata,
		"infer":    false,
	}
	if s.config.Flavor == VolcPlatform {
		// Volc rejects asynchronous direct imports: async_mode must be false
		// when infer is false, and the response contains the created fact.
		payload["async_mode"] = false
	}
	for key, value := range s.entityFields(scope) {
		payload[key] = value
	}
	path := "/memories"
	if s.usesPlatformAPI() {
		path = "/v3/memories/add/"
	}
	if s.config.Flavor == VolcPlatform {
		path = "/v1/memories/"
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		if existing, found, reconcileErr := s.findDirectObservation(ctx, scope, observation.ID, digest); reconcileErr == nil && found {
			return observeResult{Facts: existing}, nil
		}
		return observeResult{}, err
	}
	operationNativeID := ""
	if s.config.Flavor != VolcPlatform {
		operationNativeID, err = response.operationID(s.config.Flavor)
		if err != nil {
			return observeResult{}, err
		}
	}
	if operationNativeID != "" {
		operationID := encodeOperationLocator(scope, operationNativeID)
		return observeResult{Operation: &memorystore.Operation{ID: operationID, Status: operationPending}}, nil
	}
	entries := response.entries()
	for index := range entries {
		if len(entries[index].Metadata) == 0 {
			entries[index].Metadata = cloneMap(metadata)
		}
		if strings.TrimSpace(entries[index].Memory) == "" && strings.TrimSpace(entries[index].Text) == "" {
			entries[index].Memory = strings.TrimSpace(observation.Facts[0].Text)
		}
	}
	if len(entries) != 1 {
		return observeResult{}, fmt.Errorf("%w: mem0 direct import returned %d facts", errUnavailable, len(entries))
	}
	facts, err := s.scopedFacts(entries, scope)
	if err != nil {
		return observeResult{}, err
	}
	return observeResult{Facts: facts}, nil
}

func (s *Store) findDirectObservation(ctx context.Context, scope scope, observationID, digest string) ([]fact, bool, error) {
	filters := s.mem0ScopeFilter(scope)
	metadataFilter := map[string]any{mem0ObservationIDMetadata: observationID}
	if s.usesPlatformAPI() {
		metadataFilter = map[string]any{"metadata": metadataFilter}
	}
	filters = map[string]any{"AND": []any{filters, metadataFilter}}
	payload := map[string]any{"query": observationID, "top_k": 10, "filters": filters}
	path := "/search"
	method := http.MethodPost
	if s.usesPlatformAPI() {
		path = "/v3/memories/"
		payload = map[string]any{"filters": filters, "page": 1, "page_size": 100}
	}
	if s.config.Flavor == VolcPlatform {
		query := url.Values{}
		for key, value := range s.entityFields(scope) {
			query.Set(key, value)
		}
		path = "/v1/memories/?" + query.Encode()
		method = http.MethodGet
		payload = nil
	}
	var response mem0Envelope
	if err := s.client.do(ctx, method, path, payload, &response); err != nil {
		return nil, false, err
	}
	entries := response.entries()
	if len(entries) == 0 {
		return nil, false, nil
	}
	facts := make([]fact, 0, len(entries))
	for _, entry := range entries {
		storedID, _ := entry.Metadata[mem0ObservationIDMetadata].(string)
		if storedID != observationID {
			continue
		}
		storedDigest, _ := entry.Metadata[mem0ObservationDigestMetadata].(string)
		if storedDigest != digest {
			return nil, false, fmt.Errorf("%w: observation %q payload changed", errConflict, observationID)
		}
		mapped, err := s.scopedFact(entry, scope)
		if err != nil {
			return nil, false, err
		}
		facts = append(facts, mapped)
	}
	if len(facts) == 0 {
		return nil, false, nil
	}
	if len(facts) != 1 {
		return nil, false, fmt.Errorf("%w: observation %q resolved to multiple facts", errConflict, observationID)
	}
	return facts, true, nil
}

func validateMem0Metadata(metadata map[string]any) error {
	for _, key := range []string{mem0ObservationIDMetadata, mem0TurnIDsMetadata, mem0ObservationDigestMetadata, mem0EntityScopeMetadata, mem0OperationMarkerMetadata} {
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
	scope, err := normalizeEntityScope(query.Scope)
	if err != nil {
		return recallResult{}, err
	}
	filters, err := s.mem0Filters(scope, query.Filters)
	if err != nil {
		return recallResult{}, err
	}
	payload := map[string]any{"query": query.Text, "top_k": query.Limit, "filters": filters}
	path := "/search"
	if s.usesPlatformAPI() {
		path = "/v3/memories/search/"
	}
	if s.config.Flavor == VolcPlatform {
		path = "/v1/memories/search/"
		payload = map[string]any{"query": query.Text, "top_k": query.Limit}
		for key, value := range s.entityFields(scope) {
			payload[key] = value
		}
		if len(query.Filters) > 0 {
			payload["filters"] = filters
		}
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodPost, path, payload, &response); err != nil {
		return recallResult{}, err
	}
	entries := response.entries()
	if s.config.Flavor == VolcPlatform {
		entries, err = decodeVolcSearchResults(response.Results)
		if err != nil {
			return recallResult{}, err
		}
	}
	result := recallResult{Matches: make([]match, len(entries))}
	for index, entry := range entries {
		fact, err := s.scopedFact(entry, scope)
		if err != nil {
			return recallResult{}, err
		}
		result.Matches[index] = match{Fact: fact, Score: entry.Score}
	}
	return result, nil
}

// Update revises one Mem0 memory. Mem0 does not expose conditional writes.
func (s *Store) Update(ctx context.Context, request memorystore.UpdateRequest) (memorystore.Fact, error) {
	if err := validateUpdate(request); err != nil {
		return fact{}, err
	}
	scope, err := normalizeEntityScope(request.Scope)
	if err != nil {
		return fact{}, err
	}
	locatorScope, nativeID, err := decodeFactLocator(request.ID)
	if err != nil {
		return fact{}, err
	}
	if locatorScope != scope {
		return fact{}, fmt.Errorf("%w: fact locator scope does not match update scope", errInvalidInput)
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
	if _, err := s.getScopedFact(ctx, scope, nativeID); err != nil {
		return fact{}, err
	}
	path := "/v1/memories/" + url.PathEscape(nativeID) + "/"
	if s.config.Flavor == SelfHosted {
		path = "/memories/" + url.PathEscape(nativeID)
	}
	if err := s.client.do(ctx, http.MethodPut, path, payload, &response); err != nil {
		return fact{}, err
	}
	facts := response.facts()
	if len(facts) > 0 {
		return s.scopedFact(response.entries()[0], scope)
	}
	updated, err := s.getScopedFact(ctx, scope, nativeID)
	if err != nil {
		return fact{}, err
	}
	return updated, nil
}

// Delete removes one Mem0 memory. Mem0 does not expose conditional deletes.
func (s *Store) Delete(ctx context.Context, request memorystore.DeleteRequest) error {
	if err := validateDelete(request); err != nil {
		return err
	}
	scope, err := normalizeEntityScope(request.Scope)
	if err != nil {
		return err
	}
	locatorScope, nativeID, err := decodeFactLocator(request.ID)
	if err != nil {
		return err
	}
	if locatorScope != scope {
		return fmt.Errorf("%w: fact locator scope does not match delete scope", errInvalidInput)
	}
	if request.ExpectedRevision != "" {
		return fmt.Errorf("%w: mem0 does not expose conditional deletes", errUnsupported)
	}
	if _, err := s.getScopedFact(ctx, scope, nativeID); err != nil {
		return err
	}
	path := "/v1/memories/" + url.PathEscape(nativeID) + "/"
	if s.config.Flavor == SelfHosted {
		path = "/memories/" + url.PathEscape(nativeID)
	}
	return s.client.do(ctx, http.MethodDelete, path, nil, nil)
}

// Wait polls an asynchronous Mem0 Platform event or Volc job.
func (s *Store) Wait(ctx context.Context, request memorystore.OperationRequest) (memorystore.ObserveResult, error) {
	if err := validateOperationRequest(request); err != nil {
		return observeResult{}, err
	}
	scope, err := normalizeEntityScope(request.Scope)
	if err != nil {
		return observeResult{}, err
	}
	locatorScope, nativeID, err := decodeOperationLocator(request.ID)
	if err != nil {
		return observeResult{}, err
	}
	if locatorScope != scope {
		return observeResult{}, fmt.Errorf("%w: operation locator scope does not match wait scope", errInvalidInput)
	}
	operationMarker := ""
	if s.config.Flavor == VolcPlatform {
		nativeID, operationMarker, err = decodeVolcOperationNativeID(nativeID)
		if err != nil {
			return observeResult{}, err
		}
	}
	if s.config.Flavor == SelfHosted {
		return observeResult{}, fmt.Errorf("%w: self-hosted mem0 has no event API", errUnsupported)
	}
	interval := s.config.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		var response mem0Envelope
		path := "/v1/event/" + url.PathEscape(nativeID) + "/"
		if s.config.Flavor == VolcPlatform {
			path = "/v1/job/" + url.PathEscape(nativeID) + "/"
		}
		if err := s.client.do(ctx, http.MethodGet, path, nil, &response); err != nil {
			return observeResult{}, err
		}
		status := strings.ToLower(strings.TrimSpace(response.Status))
		switch status {
		case "completed", "complete", "succeeded", "success":
			entries := response.resultEntries()
			var facts []fact
			if s.config.Flavor == VolcPlatform && len(entries) == 0 {
				facts, err = s.listVolcScopedFacts(ctx, scope, operationMarker)
			} else {
				facts, err = s.loadScopedFacts(ctx, scope, entries)
			}
			if err != nil {
				return observeResult{}, err
			}
			return observeResult{Facts: facts, Operation: &memorystore.Operation{ID: request.ID, Status: operationSucceeded}}, nil
		case "failed", "error":
			return observeResult{Operation: &memorystore.Operation{ID: request.ID, Status: operationFailed, Error: "mem0 operation failed"}}, nil
		case "pending", "queued", "running", "processing", "in_progress":
		default:
			return observeResult{}, fmt.Errorf("%w: mem0 operation returned unknown status", errUnavailable)
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

func (s *Store) listVolcScopedFacts(ctx context.Context, scope scope, operationMarker string) ([]fact, error) {
	if operationMarker == "" {
		return nil, fmt.Errorf("%w: volc mem0 operation has no reconciliation marker", errUnavailable)
	}
	query := url.Values{}
	for key, value := range s.entityFields(scope) {
		query.Set(key, value)
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodGet, "/v1/memories/?"+query.Encode(), nil, &response); err != nil {
		return nil, err
	}
	entries := response.entries()
	filtered := entries[:0]
	for _, entry := range entries {
		storedMarker, _ := entry.Metadata[mem0OperationMarkerMetadata].(string)
		if storedMarker == operationMarker {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("%w: volc mem0 operation materialized no correlated facts", errUnavailable)
	}
	return s.scopedFacts(filtered, scope)
}

func encodeVolcOperationNativeID(jobID, operationMarker string) string {
	return volcOperationNativePrefix + base64.RawURLEncoding.EncodeToString([]byte(jobID)) + ":" +
		base64.RawURLEncoding.EncodeToString([]byte(operationMarker))
}

func decodeVolcOperationNativeID(value string) (string, string, error) {
	if !strings.HasPrefix(value, volcOperationNativePrefix) {
		return value, "", nil
	}
	parts := strings.Split(strings.TrimPrefix(value, volcOperationNativePrefix), ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: invalid volc mem0 operation locator", errInvalidInput)
	}
	jobID, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(jobID) == 0 {
		return "", "", fmt.Errorf("%w: invalid volc mem0 operation job id", errInvalidInput)
	}
	operationMarker, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("%w: invalid volc mem0 operation marker", errInvalidInput)
	}
	return string(jobID), string(operationMarker), nil
}

func normalizeEntityScope(input scope) (scope, error) {
	input.AppID = strings.TrimSpace(input.AppID)
	input.UserID = strings.TrimSpace(input.UserID)
	input.AgentID = strings.TrimSpace(input.AgentID)
	input.RunID = strings.TrimSpace(input.RunID)
	if input == (scope{}) {
		return scope{}, fmt.Errorf("%w: mem0 requires one entity scope", errInvalidInput)
	}
	return input, nil
}

func platformEntityFields(scope scope) map[string]string {
	fields := make(map[string]string, 1)
	for key, value := range map[string]string{
		"app_id": scope.AppID, "user_id": scope.UserID, "agent_id": scope.AgentID, "run_id": scope.RunID,
	} {
		if value != "" {
			fields[key] = value
		}
	}
	return fields
}

func (s *Store) entityFields(scope scope) map[string]string {
	if s.config.Flavor == SelfHosted {
		return map[string]string{"user_id": encodeSelfHostedScope(scope)}
	}
	fields := platformEntityFields(scope)
	if s.config.Flavor == VolcPlatform && scope.UserID == "" {
		fields["user_id"] = volcScopeUserID(scope)
	}
	return fields
}

func volcScopeUserID(input scope) string {
	return volcScopeUserPrefix + strings.TrimPrefix(encodeSelfHostedScope(input), selfHostedScopeIdentifier)
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
	if s.config.Flavor == SelfHosted {
		return map[string]any{"user_id": encodeSelfHostedScope(scope)}
	}
	clauses := make([]any, 0, 4)
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "app_id", value: scope.AppID},
		{name: "user_id", value: scope.UserID},
		{name: "agent_id", value: scope.AgentID},
		{name: "run_id", value: scope.RunID},
	} {
		if field.value != "" {
			clauses = append(clauses, map[string]any{field.name: field.value})
		}
	}
	if len(clauses) == 1 {
		return clauses[0].(map[string]any)
	}
	return map[string]any{"AND": clauses}
}

func (s *Store) getScopedFact(ctx context.Context, scope scope, nativeID string) (fact, error) {
	path := "/v1/memories/" + url.PathEscape(nativeID) + "/"
	if s.config.Flavor == SelfHosted {
		path = "/memories/" + url.PathEscape(nativeID)
	}
	var response mem0Envelope
	if err := s.client.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return fact{}, err
	}
	if err := validateEnvelopeScope(response, scope, s.config.Flavor); err != nil {
		return fact{}, err
	}
	return s.scopedFact(response, scope)
}

func (s *Store) loadScopedFacts(ctx context.Context, scope scope, entries []mem0Envelope) ([]fact, error) {
	facts := make([]fact, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) == "" {
			return nil, fmt.Errorf("%w: mem0 operation returned a fact without an id", errUnavailable)
		}
		fact, err := s.getScopedFact(ctx, scope, entry.ID)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func validateEnvelopeScope(entry mem0Envelope, expected scope, flavor Flavor) error {
	if flavor == SelfHosted {
		actual, err := decodeSelfHostedScope(strings.TrimSpace(entry.UserID))
		if err != nil {
			return fmt.Errorf("%w: self-hosted mem0 memory does not expose its encoded scope", errUnsupported)
		}
		if actual != expected {
			return fmt.Errorf("%w: mem0 memory entity scope does not match request", errInvalidInput)
		}
		if strings.TrimSpace(entry.AppID) != "" ||
			strings.TrimSpace(entry.AgentID) != "" ||
			strings.TrimSpace(entry.RunID) != "" {
			return fmt.Errorf("%w: self-hosted mem0 memory exposes conflicting entity fields", errInvalidInput)
		}
		return nil
	}
	if flavor == VolcPlatform {
		encoded, _ := entry.Metadata[mem0EntityScopeMetadata].(string)
		if strings.TrimSpace(encoded) != "" {
			actual, err := decodeSelfHostedScope(encoded)
			if err != nil || actual != expected {
				return fmt.Errorf("%w: volc mem0 memory entity scope does not match request", errInvalidInput)
			}
			return validateReturnedEnvelopeScope(entry, expected, flavor)
		}
	}
	actual := scope{
		AppID: strings.TrimSpace(entry.AppID), UserID: strings.TrimSpace(entry.UserID),
		AgentID: strings.TrimSpace(entry.AgentID), RunID: strings.TrimSpace(entry.RunID),
	}
	if flavor == VolcPlatform && expected.UserID == "" && actual.UserID == volcScopeUserID(expected) {
		actual.UserID = ""
	}
	if actual == (scope{}) {
		return fmt.Errorf("%w: mem0 memory does not expose its entity scope", errUnsupported)
	}
	if actual != expected {
		return fmt.Errorf("%w: mem0 memory entity scope does not match request", errInvalidInput)
	}
	return nil
}

func (s *Store) scopedFacts(entries []mem0Envelope, scope scope) ([]fact, error) {
	facts := make([]fact, len(entries))
	for index, entry := range entries {
		fact, err := s.scopedFact(entry, scope)
		if err != nil {
			return nil, err
		}
		facts[index] = fact
	}
	return facts, nil
}

func (s *Store) scopedFact(entry mem0Envelope, scope scope) (fact, error) {
	if strings.TrimSpace(entry.ID) == "" {
		return fact{}, fmt.Errorf("%w: mem0 response returned a fact without an id", errUnavailable)
	}
	if err := validateReturnedEnvelopeScope(entry, scope, s.config.Flavor); err != nil {
		return fact{}, err
	}
	result := entry.fact()
	if s.config.Flavor == VolcPlatform {
		for _, key := range []string{"project_id", "__fraq__", "__freq__", "__strategy__"} {
			delete(result.Attributes, key)
		}
	}
	result.ID = encodeFactLocator(scope, result.ID)
	return result, nil
}

func validateReturnedEnvelopeScope(entry mem0Envelope, expected scope, flavor Flavor) error {
	if flavor == SelfHosted {
		userID := strings.TrimSpace(entry.UserID)
		if userID == "" {
			return nil
		}
		actual, err := decodeSelfHostedScope(userID)
		if err != nil || actual != expected {
			return fmt.Errorf("%w: self-hosted mem0 response scope does not match request", errInvalidInput)
		}
		if strings.TrimSpace(entry.AppID) != "" ||
			strings.TrimSpace(entry.AgentID) != "" ||
			strings.TrimSpace(entry.RunID) != "" {
			return fmt.Errorf("%w: self-hosted mem0 response exposes conflicting entity fields", errInvalidInput)
		}
		return nil
	}
	if flavor == VolcPlatform {
		encoded, _ := entry.Metadata[mem0EntityScopeMetadata].(string)
		if strings.TrimSpace(encoded) != "" {
			actual, err := decodeSelfHostedScope(encoded)
			if err != nil || actual != expected {
				return fmt.Errorf("%w: volc mem0 response scope does not match request", errInvalidInput)
			}
		}
	}
	for _, field := range []struct {
		name     string
		actual   string
		expected string
	}{
		{name: "app_id", actual: entry.AppID, expected: expected.AppID},
		{name: "user_id", actual: entry.UserID, expected: expected.UserID},
		{name: "agent_id", actual: entry.AgentID, expected: expected.AgentID},
		{name: "run_id", actual: entry.RunID, expected: expected.RunID},
	} {
		actual := strings.TrimSpace(field.actual)
		if flavor == VolcPlatform && field.name == "user_id" && expected.UserID == "" && actual == volcScopeUserID(expected) {
			continue
		}
		if actual != "" && actual != field.expected {
			return fmt.Errorf(
				"%w: mem0 response %s does not match request scope",
				errInvalidInput, field.name,
			)
		}
	}
	return nil
}

func (s *Store) mem0FilterClause(filter filter) (map[string]any, error) {
	field := strings.TrimSpace(filter.Field)
	if field == mem0ObservationIDMetadata || field == mem0TurnIDsMetadata || field == mem0ObservationDigestMetadata || field == mem0EntityScopeMetadata || field == mem0OperationMarkerMetadata || isMem0RoutingField(field) {
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
	UserID     string          `json:"user_id"`
	AgentID    string          `json:"agent_id"`
	AppID      string          `json:"app_id"`
	RunID      string          `json:"run_id"`
	Results    json.RawMessage `json:"results"`
	Data       json.RawMessage `json:"data"`
}

func (e mem0Envelope) operationID(flavor Flavor) (string, error) {
	if flavor != VolcPlatform {
		return strings.TrimSpace(e.EventID), nil
	}
	entries, err := decodeVolcResults(e.Results)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("%w: volc mem0 add response has no results.event_id", errUnavailable)
	}
	if len(entries) != 1 || strings.TrimSpace(entries[0].EventID) == "" {
		return "", fmt.Errorf("%w: volc mem0 add response must contain one results.event_id", errUnavailable)
	}
	return strings.TrimSpace(entries[0].EventID), nil
}

func decodeVolcResults(raw json.RawMessage) ([]mem0Envelope, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var entries []mem0Envelope
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("%w: volc mem0 results must be an array", errUnavailable)
	}
	return entries, nil
}

func decodeVolcSearchResults(raw json.RawMessage) ([]mem0Envelope, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, fmt.Errorf("%w: volc mem0 search response must contain a results array", errUnavailable)
	}
	return decodeVolcResults(raw)
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
			return items
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
	delete(attributes, mem0ObservationDigestMetadata)
	delete(attributes, mem0EntityScopeMetadata)
	delete(attributes, mem0OperationMarkerMetadata)
	var sources []sourceRef
	if observationID != "" || len(turnIDs) > 0 {
		sources = []sourceRef{{ObservationID: observationID, TurnIDs: turnIDs}}
	}
	return fact{ID: e.ID, Revision: revision, Text: text, Attributes: attributes, Sources: sources, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
}

var _ storeContract = (*Store)(nil)
var _ operationWaiterContract = (*Store)(nil)
var _ memorystore.DirectFactObserver = (*Store)(nil)
