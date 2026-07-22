package mem0

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreRoutesEachOperationScopeAsUserID(t *testing.T) {
	t.Parallel()
	var (
		mu       sync.Mutex
		userIDs  []string
		requests []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		userID := ""
		if value, ok := body["user_id"].(string); ok {
			userID = value
		}
		if filters, ok := body["filters"].(map[string]any); ok {
			userID, _ = filters["user_id"].(string)
		}
		mu.Lock()
		userIDs = append(userIDs, userID)
		requests = append(requests, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "search") {
			_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"remembered","user_id":"`+userID+`","score":0.9}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"remembered"}]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []Scope{"conversation-a", "conversation-b"} {
		if _, err := store.Observe(context.Background(), Observation{Scope: scope, Text: "remember"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Recall(context.Background(), Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := userIDs, []string{"conversation-a", "conversation-a", "conversation-b", "conversation-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("user ids = %v, want %v (paths %v)", got, want, requests)
	}
}

func TestStoreUpdateDeleteUseOnlyFactID(t *testing.T) {
	t.Parallel()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"updated"}]}`)
		}
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	text := "updated"
	if _, err := store.Update(context.Background(), UpdateRequest{ID: "fact", Text: &text}); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), DeleteRequest{ID: "fact"}); err != nil {
		t.Fatal(err)
	}
	if got, want := methods, []string{"PUT /v1/memories/fact/", "DELETE /v1/memories/fact/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestStoreSelfHostedUsesSharedScopeMapping(t *testing.T) {
	t.Parallel()
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, Flavor: SelfHosted, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Observe(context.Background(), Observation{Scope: "scope", Text: "remember"}); err != nil {
		t.Fatal(err)
	}
	if body["user_id"] != "scope" {
		t.Fatalf("body = %+v", body)
	}
	for _, forbidden := range []string{"app_id", "agent_id", "run_id"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("body contains %q: %+v", forbidden, body)
		}
	}
}

func TestStoreRejectsDirectFactCandidates(t *testing.T) {
	t.Parallel()
	store, err := New(Config{Endpoint: "https://example.test", APIKey: "secret", Flavor: Platform})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Observe(context.Background(), Observation{
		Scope: "scope",
		Facts: []FactCandidate{{Text: "structured fact"}},
	})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Observe() error = %v, want ErrUnsupported", err)
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
			Scope: "scope", Text: "remember", Limit: 1,
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
		_, _ = io.WriteString(w, `{"status":"completed","results":[{"id":"fact","memory":"done"}]}`)
	}))
	defer server.Close()
	store, err := New(Config{Endpoint: server.URL, APIKey: "secret", Flavor: Platform, PollInterval: time.Millisecond, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Wait(context.Background(), "operation")
	if err != nil || result.Operation == nil || result.Operation.Status != OperationSucceeded || len(result.Facts) != 1 {
		t.Fatalf("Wait() = %+v, %v", result, err)
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
	_, err = store.Recall(context.Background(), Query{Scope: "scope", Text: "x", Limit: 1})
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "top-secret") {
		t.Fatalf("error = %v", err)
	}
}
