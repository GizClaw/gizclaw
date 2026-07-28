package mem0

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreRoutesEachSupportedEntityScope(t *testing.T) {
	t.Parallel()
	var (
		mu     sync.Mutex
		routes []map[string]string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		route := make(map[string]string)
		for _, field := range []string{"app_id", "user_id", "agent_id", "run_id"} {
			if value, ok := body[field].(string); ok {
				route[field] = value
			}
		}
		if filters, ok := body["filters"].(map[string]any); ok {
			for _, field := range []string{"app_id", "user_id", "agent_id", "run_id"} {
				if value, ok := filters[field].(string); ok {
					route[field] = value
				}
			}
		}
		mu.Lock()
		routes = append(routes, route)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "search") {
			_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"remembered","score":0.9}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"remembered"}]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scopes := []Scope{
		{AppID: "app"},
		{UserID: "user"},
		{AgentID: "agent"},
		{RunID: "run"},
	}
	for _, scope := range scopes {
		if _, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Recall(context.Background(), Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	want := []map[string]string{
		{"app_id": "app"}, {"app_id": "app"},
		{"user_id": "user"}, {"user_id": "user"},
		{"agent_id": "agent"}, {"agent_id": "agent"},
		{"run_id": "run"}, {"run_id": "run"},
	}
	if !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %#v, want %#v", routes, want)
	}
}

func TestStoreRejectsSynchronousFactsWithoutNativeIDs(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"memory":"missing id"}]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AppID: "workspace"}
	if _, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Observe() error = %v, want ErrUnavailable", err)
	}
	if _, err := store.Recall(context.Background(), Query{Scope: scope, Text: "remember", Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Recall() error = %v, want ErrUnavailable", err)
	}
}

func TestStoreRejectsConflictingResponseScopes(t *testing.T) {
	t.Parallel()
	store := &Store{config: Config{Flavor: Platform}}
	expected := scope{AppID: "app", UserID: "user", AgentID: "agent", RunID: "run"}
	for _, test := range []struct {
		name  string
		entry mem0Envelope
	}{
		{name: "app", entry: mem0Envelope{AppID: "other-app"}},
		{name: "user", entry: mem0Envelope{UserID: "other-user"}},
		{name: "agent", entry: mem0Envelope{AgentID: "other-agent"}},
		{name: "run", entry: mem0Envelope{RunID: "other-run"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.entry.ID = "fact"
			if _, err := store.scopedFact(test.entry, expected); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("scopedFact() error = %v, want ErrInvalidInput", err)
			}
		})
	}
	entry := mem0Envelope{
		ID: "fact", AppID: expected.AppID, UserID: expected.UserID,
		AgentID: expected.AgentID, RunID: expected.RunID,
	}
	if _, err := store.scopedFact(entry, expected); err != nil {
		t.Fatalf("scopedFact(matching scope) error = %v", err)
	}
	entry = mem0Envelope{ID: "fact"}
	if _, err := store.scopedFact(entry, expected); err != nil {
		t.Fatalf("scopedFact(omitted provider scope) error = %v", err)
	}
	if _, err := store.scopedFact(
		mem0Envelope{ID: "fact", AppID: "unexpected-app"},
		scope{UserID: "user"},
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("scopedFact(unexpected dimension) error = %v, want ErrInvalidInput", err)
	}
}

func TestStoreObserveAndRecallRejectCrossScopeFacts(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"private","app_id":"other-workspace"}]}`)
	}))
	defer server.Close()
	store, err := New(Config{
		Endpoint: server.URL, APIKey: "secret", Flavor: Platform,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	requestScope := Scope{AppID: "workspace"}
	if _, err := store.Observe(t.Context(), Observation{
		Scope: requestScope, Text: "remember",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Observe() error = %v, want ErrInvalidInput", err)
	}
	if _, err := store.Recall(t.Context(), Query{
		Scope: requestScope, Text: "remember", Limit: 1,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Recall() error = %v, want ErrInvalidInput", err)
	}
}

func TestStoreUpdateDeleteVerifyFactScopeBeforeMutation(t *testing.T) {
	t.Parallel()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"id":"fact","memory":"current","app_id":"workspace"}`)
		case http.MethodPut:
			_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"updated","app_id":"workspace"}]}`)
		}
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AppID: "workspace"}
	locator := encodeFactLocator(scope, "fact")
	text := "updated"
	if _, err := store.Update(context.Background(), UpdateRequest{Scope: scope, ID: locator, Text: &text}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{Scope: scope, ID: locator}); err != nil {
		t.Fatal(err)
	}
	if got, want := methods, []string{"GET /v1/memories/fact/", "PUT /v1/memories/fact/", "GET /v1/memories/fact/", "DELETE /v1/memories/fact/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestStoreSelfHostedEncodesCompleteScopeAsNativeUser(t *testing.T) {
	t.Parallel()
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Flavor: SelfHosted, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AppID: "app", UserID: "user", AgentID: "agent", RunID: "run"}
	if _, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Recall(context.Background(), Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	encoded := encodeSelfHostedScope(scope)
	if got := bodies[0]["user_id"]; got != encoded {
		t.Fatalf("observe user_id = %#v, want %q", got, encoded)
	}
	if got := bodies[1]["filters"]; !reflect.DeepEqual(got, map[string]any{"user_id": encoded}) {
		t.Fatalf("recall filters = %#v", got)
	}
	for _, body := range bodies {
		for _, forbidden := range []string{"app_id", "agent_id", "run_id"} {
			if _, ok := body[forbidden]; ok {
				t.Fatalf("body contains %q: %+v", forbidden, body)
			}
		}
	}
}

func TestStoreSelfHostedVerifiesEncodedScopeBeforeMutation(t *testing.T) {
	t.Parallel()
	scope := Scope{AppID: "workspace", UserID: "user"}
	encoded := encodeSelfHostedScope(scope)
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = fmt.Fprintf(w, `{"id":"fact","memory":"current","user_id":%q}`, encoded)
		case http.MethodPut:
			_, _ = fmt.Fprintf(w, `{"results":[{"id":"fact","memory":"updated","user_id":%q}]}`, encoded)
		}
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Flavor: SelfHosted, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	locator := encodeFactLocator(scope, "fact")
	text := "updated"
	if _, err := store.Update(t.Context(), UpdateRequest{Scope: scope, ID: locator, Text: &text}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(t.Context(), DeleteRequest{Scope: scope, ID: locator}); err != nil {
		t.Fatal(err)
	}
	if want := []string{http.MethodGet, http.MethodPut, http.MethodGet, http.MethodDelete}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("methods = %v, want %v", methods, want)
	}
}

func TestStoreDirectFactObservationIsIdempotent(t *testing.T) {
	var (
		mu        sync.Mutex
		saved     *mem0Envelope
		addCalls  int
		addBodies []map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/v3/memories/" {
			if saved == nil {
				_, _ = io.WriteString(w, `{"results":[]}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []mem0Envelope{*saved}})
			return
		}
		addCalls++
		addBodies = append(addBodies, body)
		metadata, _ := body["metadata"].(map[string]any)
		saved = &mem0Envelope{
			ID: "fact-1", Memory: "Pet completed care.", AppID: "workspace",
			Metadata: metadata, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []mem0Envelope{*saved}})
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{
		Scope: Scope{AppID: "workspace"}, ID: "gameplay/drive/reward_grant/grant-1",
		Facts: []FactCandidate{{
			Text:       "Pet completed care.",
			Attributes: map[string]any{"kind": "event", "source_id": "grant-1"},
		}},
		ObservedAt: time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC),
	}
	const workers = 8
	var wait sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wait.Go(func() {
			result, err := store.Observe(context.Background(), observation)
			if err == nil && (len(result.Facts) != 1 ||
				len(result.Facts[0].Sources) != 1 ||
				result.Facts[0].Sources[0].ObservationID != observation.ID ||
				result.Facts[0].Attributes["source_id"] != "grant-1") {
				err = fmt.Errorf("unexpected result %#v", result)
			}
			errs <- err
		})
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	if addCalls != 1 {
		t.Fatalf("add calls = %d, want 1", addCalls)
	}
	if len(addBodies) != 1 || addBodies[0]["infer"] != false {
		t.Fatalf("direct add body = %#v", addBodies)
	}
	mu.Unlock()
	changed := observation
	changed.Facts = []FactCandidate{{Text: "changed", Attributes: map[string]any{"kind": "event"}}}
	if _, err := store.Observe(context.Background(), changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("Observe(changed) error = %v, want ErrConflict", err)
	}
}

func TestStoreDirectFactReconcilesLostProviderResponse(t *testing.T) {
	var (
		mu    sync.Mutex
		saved *mem0Envelope
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/v3/memories/" {
			w.Header().Set("Content-Type", "application/json")
			if saved == nil {
				_, _ = io.WriteString(w, `{"results":[]}`)
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"results": []mem0Envelope{*saved}})
			}
			return
		}
		metadata, _ := body["metadata"].(map[string]any)
		saved = &mem0Envelope{
			ID: "fact-accepted", Memory: "accepted before disconnect",
			AppID: "workspace", Metadata: metadata,
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not support hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{
		Scope: Scope{AppID: "workspace"}, ID: "lost-response",
		Facts:      []FactCandidate{{Text: "accepted before disconnect", Attributes: map[string]any{"kind": "event"}}},
		ObservedAt: time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC),
	}
	result, err := store.Observe(context.Background(), observation)
	if err != nil || len(result.Facts) != 1 || result.Facts[0].Sources[0].ObservationID != observation.ID {
		t.Fatalf("Observe() = %#v, %v", result, err)
	}
}

func TestStoreRejectsNativeRoutingFilters(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Endpoint: "https://example.test", APIKey: "secret", Flavor: Platform})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"user_id", "app_id", "agent_id", "run_id"} {
		_, err := store.Recall(context.Background(), Query{
			Scope: Scope{UserID: "scope"}, Text: "remember", Limit: 1,
			Filters: []Filter{{Field: field, Operator: FilterEqual, Value: "override"}},
		})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("field %q error = %v, want ErrUnsupported", field, err)
		}
	}
}

func TestStoreWaitPollsPlatformEvent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/event/") {
			_, _ = io.WriteString(w, `{"status":"completed","results":[{"id":"fact","memory":"done"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"fact","memory":"done","run_id":"run"}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, PollInterval: time.Millisecond, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{RunID: "run"}
	operationID := encodeOperationLocator(scope, "operation")
	result, err := store.Wait(context.Background(), operationRequest{Scope: scope, ID: operationID})
	if err != nil || result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 1 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
	if result.Operation.ID != operationID || result.Facts[0].ID != encodeFactLocator(scope, "fact") {
		t.Fatalf("Wait() locators = operation %q fact %q", result.Operation.ID, result.Facts[0].ID)
	}
}

func TestStorePreservesIndependentEntityCombinations(t *testing.T) {
	t.Parallel()
	var observeBody map[string]any
	var recallBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/search/") {
			recallBody = body
		} else {
			observeBody = body
		}
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AppID: "app", UserID: "user", AgentID: "agent", RunID: "run"}
	if _, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember"}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if _, err := store.Recall(context.Background(), Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	for field, want := range map[string]string{
		"app_id": "app", "user_id": "user", "agent_id": "agent", "run_id": "run",
	} {
		if got := observeBody[field]; got != want {
			t.Fatalf("observe %s = %#v, want %q", field, got, want)
		}
	}
	wantFilters := map[string]any{"AND": []any{
		map[string]any{"app_id": "app"},
		map[string]any{"user_id": "user"},
		map[string]any{"agent_id": "agent"},
		map[string]any{"run_id": "run"},
	}}
	if got := recallBody["filters"]; !reflect.DeepEqual(got, wantFilters) {
		t.Fatalf("recall filters = %#v, want %#v", got, wantFilters)
	}
}

func TestStoreRejectsCrossScopeLocatorsBeforeNetwork(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	appA := Scope{AppID: "app-a"}
	appB := Scope{AppID: "app-b"}
	text := "updated"
	if _, err := store.Update(context.Background(), UpdateRequest{Scope: appB, ID: encodeFactLocator(appA, "fact"), Text: &text}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{Scope: appB, ID: encodeFactLocator(appA, "fact")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Delete() error = %v, want ErrInvalidInput", err)
	}
	if _, err := store.Wait(context.Background(), operationRequest{Scope: appB, ID: encodeOperationLocator(appA, "operation")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Wait() error = %v, want ErrInvalidInput", err)
	}
	if calls != 0 {
		t.Fatalf("network calls = %d, want 0", calls)
	}
}

func TestStoreRejectsProviderRecordScopeMismatchBeforeMutation(t *testing.T) {
	t.Parallel()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fact","memory":"private","app_id":"app-b"}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	scope := Scope{AppID: "app-a"}
	locator := encodeFactLocator(scope, "fact")
	text := "updated"
	if _, err := store.Update(context.Background(), UpdateRequest{Scope: scope, ID: locator, Text: &text}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{Scope: scope, ID: locator}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Delete() error = %v, want ErrInvalidInput", err)
	}
	if !reflect.DeepEqual(methods, []string{http.MethodGet, http.MethodGet}) {
		t.Fatalf("methods = %v, want only verification GETs", methods)
	}
}

func TestConfigValidationAndTags(t *testing.T) {
	t.Parallel()
	for _, config := range []Config{
		{Endpoint: "https://example.test", Flavor: "unknown"},
		{Endpoint: "https://example.test", Flavor: Platform},
		{Flavor: SelfHosted},
		{Endpoint: "relative", APIKey: "key", Flavor: Platform},
		{Endpoint: "https://example.test", APIKey: "key", PollInterval: -1},
	} {
		if _, err := New(config); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("New(%+v) error = %v", config, err)
		}
	}
	typeOfConfig := reflect.TypeFor[Config]()
	for field := range typeOfConfig.Fields() {
		if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" {
			t.Fatalf("Config.%s contains serialization tags", field.Name)
		}
	}
}

type errorHTTPClient struct{ err error }

func (c errorHTTPClient) Do(*http.Request) (*http.Response, error) { return nil, c.err }

func TestClientRedactsSecrets(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Endpoint: "https://example.test", APIKey: "top-secret", Flavor: Platform, HTTPClient: errorHTTPClient{err: errors.New("top-secret transport")}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Recall(context.Background(), Query{Scope: Scope{UserID: "scope"}, Text: "x", Limit: 1})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("error = %v", err)
	}
}
