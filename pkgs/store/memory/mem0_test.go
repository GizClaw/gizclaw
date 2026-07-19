package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMem0PlatformLifecycle(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	eventPolls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v3/memories/add/":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["user_id"] != "user" {
				t.Errorf("add user_id = %v", body["user_id"])
			}
			metadata, _ := body["metadata"].(map[string]any)
			if metadata[mem0ObservationIDMetadata] != "observation" {
				t.Errorf("add metadata = %v", metadata)
			}
			_, _ = w.Write([]byte(`{"event_id":"event-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/event/event-1/":
			mu.Lock()
			eventPolls++
			poll := eventPolls
			mu.Unlock()
			if poll == 1 {
				_, _ = w.Write([]byte(`{"status":"pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"completed","results":[{"id":"fact-1","memory":"Alice likes tea","metadata":{"lane":"preferences","gizclaw.observation_id":"observation","gizclaw.turn_ids":["turn"]}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v3/memories/search/":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			filters, _ := body["filters"].(map[string]any)
			and, _ := filters["AND"].([]any)
			if len(and) != 2 {
				t.Errorf("search filters = %#v", filters)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":"fact-1","memory":"Alice likes tea","score":0.91,"metadata":{"lane":"preferences","hash":"rev-1","score":"user-value"}}]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/memories/fact-1":
			_, _ = w.Write([]byte(`{"id":"fact-1","memory":"Alice prefers tea","metadata":{"hash":"rev-2"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/memories/fact-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	store, err := NewMem0Store(Mem0Config{Endpoint: server.URL, APIKey: "secret", Flavor: Mem0Platform, UserID: "user", PollInterval: time.Millisecond}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	observed, err := store.Observe(context.Background(), Observation{ID: "observation", Text: "I like tea", Turns: []Turn{{ID: "turn", Role: RoleUser, Text: "Tea is best"}}})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Wait(context.Background(), observed.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed.Facts) != 1 || completed.Facts[0].ID != "fact-1" {
		t.Fatalf("Wait() = %+v", completed)
	}
	if len(completed.Facts[0].Sources) != 1 || completed.Facts[0].Sources[0].ObservationID != "observation" || len(completed.Facts[0].Sources[0].TurnIDs) != 1 {
		t.Fatalf("Wait() sources = %+v", completed.Facts[0].Sources)
	}
	result, err := store.Recall(context.Background(), Query{Text: "drink", Limit: 3, Filters: []Filter{{Field: "lane", Operator: FilterEqual, Value: "preferences"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Score != 0.91 || result.Matches[0].Fact.Revision != "rev-1" {
		t.Fatalf("Recall() = %+v", result)
	}
	if result.Matches[0].Fact.Attributes["score"] != "user-value" {
		t.Fatalf("Recall() attributes = %+v", result.Matches[0].Fact.Attributes)
	}
	text := "Alice prefers tea"
	updated, err := store.Update(context.Background(), UpdateRequest{ID: "fact-1", Text: &text})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Text != text {
		t.Fatalf("Update() = %+v", updated)
	}
	if err := store.Delete(context.Background(), DeleteRequest{ID: "fact-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestMem0RejectsUnsupportedConditionalAndAttributeUpdates(t *testing.T) {
	t.Parallel()
	store, err := NewMem0Store(Mem0Config{Endpoint: "https://example.invalid", Flavor: Mem0Platform}, nil)
	if err != nil {
		t.Fatal(err)
	}
	text := "updated"
	if _, err := store.Update(context.Background(), UpdateRequest{ID: "fact", Text: &text, ExpectedRevision: "revision"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("conditional update error = %v", err)
	}
	if _, err := store.Update(context.Background(), UpdateRequest{ID: "fact", Attributes: AttributePatch{Set: map[string]any{"lane": "clues"}}}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("attribute update error = %v", err)
	}
}

func TestMem0TransportRedactsAPIKey(t *testing.T) {
	t.Parallel()
	client := roundTripClient(func(*http.Request) (*http.Response, error) { return nil, errors.New("request secret-value failed") })
	store, err := NewMem0Store(Mem0Config{Endpoint: "https://example.invalid", APIKey: "secret-value"}, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Recall(context.Background(), Query{Text: "x", Limit: 1})
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("Recall() error = %v", err)
	}
}

func TestMem0SelfHostedLifecycle(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-API-Key") != "self-hosted-secret" || request.Header.Get("Authorization") != "" {
			t.Errorf("auth headers = X-API-Key %q, Authorization %q", request.Header.Get("X-API-Key"), request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method + " " + request.URL.Path {
		case "POST /memories":
			_, _ = w.Write([]byte(`{"results":[{"id":"fact","memory":"stored"}]}`))
		case "POST /search":
			_, _ = w.Write([]byte(`{"results":[{"id":"fact","memory":"stored","score":0.5}]}`))
		case "PUT /memories/fact":
			_, _ = w.Write([]byte(`{"id":"fact","memory":"updated"}`))
		case "DELETE /memories/fact":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	store, err := NewMem0Store(Mem0Config{Endpoint: server.URL, APIKey: "self-hosted-secret", Flavor: Mem0SelfHosted, UserID: "user"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Observe(context.Background(), Observation{Turns: []Turn{{Role: RoleAssistant, Text: "stored"}}}); err != nil || len(result.Facts) != 1 {
		t.Fatalf("Observe() = %+v, %v", result, err)
	}
	if result, err := store.Recall(context.Background(), Query{Text: "stored", Limit: 1}); err != nil || len(result.Matches) != 1 {
		t.Fatalf("Recall() = %+v, %v", result, err)
	}
	text := "updated"
	if fact, err := store.Update(context.Background(), UpdateRequest{ID: "fact", Text: &text}); err != nil || fact.Text != text {
		t.Fatalf("Update() = %+v, %v", fact, err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{ID: "fact"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), "event"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestMem0FiltersUseV3OperatorNames(t *testing.T) {
	t.Parallel()
	store, err := NewMem0Store(Mem0Config{Endpoint: "https://example.test", UserID: "user"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	filters, err := store.mem0Filters([]Filter{
		{Field: "lane", Operator: FilterEqual, Value: "clues"},
		{Field: "score", Operator: FilterGreaterEqual, Value: 0.5},
		{Field: "kind", Operator: FilterIn, Value: []string{"note", "preference"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(filters)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{`"lane":"clues"`, `"gte":0.5`, `"in":["note","preference"]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("filters = %s, missing %s", got, want)
		}
	}
	if strings.Contains(got, `"$`) {
		t.Fatalf("filters contain undocumented operators: %s", got)
	}
	for _, filter := range []Filter{
		{Field: "kind", Operator: FilterNotIn, Value: []string{"episode"}},
		{Field: "lane", Operator: FilterExists, Value: true},
	} {
		if _, err := store.mem0Filters([]Filter{filter}); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("mem0Filters(%+v) error = %v", filter, err)
		}
	}
}

func TestMem0ConstructorValidation(t *testing.T) {
	t.Parallel()
	for name, config := range map[string]Mem0Config{
		"flavor":               {Endpoint: "https://example.test", Flavor: "unknown"},
		"endpoint":             {Endpoint: "relative", Flavor: Mem0Platform},
		"userinfo":             {Endpoint: "https://user:pass@example.test", Flavor: Mem0Platform},
		"scheme":               {Endpoint: "ftp://example.test", Flavor: Mem0Platform},
		"poll":                 {Endpoint: "https://example.test", Flavor: Mem0Platform, PollInterval: -1},
		"self hosted endpoint": {Flavor: Mem0SelfHosted},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewMem0Store(config, nil); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMem0HTTPStatusErrors(t *testing.T) {
	t.Parallel()
	for status, want := range map[int]error{
		http.StatusBadRequest: ErrInvalidInput, http.StatusNotFound: ErrNotFound,
		http.StatusConflict: ErrConflict, http.StatusNotImplemented: ErrUnsupported,
		http.StatusTooManyRequests: ErrUnavailable, http.StatusBadGateway: ErrUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := roundTripClient(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: http.NoBody, Header: make(http.Header)}, nil
			})
			store, err := NewMem0Store(Mem0Config{Endpoint: "https://example.test"}, client)
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Recall(context.Background(), Query{Text: "x", Limit: 1})
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
		})
	}
}

func TestMem0WaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"pending"}`)) }))
	t.Cleanup(server.Close)
	store, err := NewMem0Store(Mem0Config{Endpoint: server.URL, PollInterval: time.Hour}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Wait(ctx, "event"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestMem0WaitFailureAndInvalidID(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"failed","error":"do not expose this"}`))
	}))
	t.Cleanup(server.Close)
	store, err := NewMem0Store(Mem0Config{Endpoint: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty Wait() error = %v", err)
	}
	result, err := store.Wait(context.Background(), "event")
	if err != nil || result.Operation == nil || result.Operation.Status != OperationFailed || strings.Contains(result.Operation.Error, "do not expose") {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
}

func TestMem0ResponseValidation(t *testing.T) {
	t.Parallel()
	for name, handler := range map[string]http.HandlerFunc{
		"invalid json": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{`)) },
		"too large":    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(make([]byte, maxMem0ResponseBytes+1)) },
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			store, err := NewMem0Store(Mem0Config{Endpoint: server.URL}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Recall(context.Background(), Query{Text: "x", Limit: 1}); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Recall() error = %v", err)
			}
		})
	}
}

type roundTripClient func(*http.Request) (*http.Response, error)

func (f roundTripClient) Do(request *http.Request) (*http.Response, error) { return f(request) }
