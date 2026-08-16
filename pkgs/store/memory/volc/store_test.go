package volc

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
	"testing"
	"time"

	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory/mem0"
)

type resolverFunc func(context.Context, Config) (string, error)

func (f resolverFunc) ResolveMem0APIKey(ctx context.Context, config Config) (string, error) {
	return f(ctx, config)
}

type errorHTTPClient struct{ err error }

func (c errorHTTPClient) Do(*http.Request) (*http.Response, error) { return nil, c.err }

func TestOpenUsesInjectedResolverAndMem0Client(t *testing.T) {
	t.Parallel()
	var (
		gotAuthorization string
		gotUserIDs       []string
		resolverCalls    int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		defer r.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotUserID, _ := body["user_id"].(string)
		if filters, ok := body["filters"].(map[string]any); ok {
			gotUserID, _ = filters["user_id"].(string)
		}
		gotUserIDs = append(gotUserIDs, gotUserID)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/memories/" {
			_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{
		Mem0: mem0.Config{Endpoint: server.URL, HTTPClient: server.Client()}, APIKeyID: "key-id",
		Resolver: resolverFunc(func(context.Context, Config) (string, error) {
			resolverCalls++
			return "resolved-key", nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []memorystore.Scope{{UserID: "conversation"}, {UserID: "other-conversation"}} {
		if _, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, Text: "remember"}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Recall(context.Background(), memorystore.Query{Scope: scope, Text: "remember", Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if resolverCalls != 1 || gotAuthorization != "Token resolved-key" || !reflect.DeepEqual(gotUserIDs, []string{"conversation", "conversation", "other-conversation", "other-conversation"}) {
		t.Fatalf("calls=%d auth=%q user_ids=%q", resolverCalls, gotAuthorization, gotUserIDs)
	}
}

func TestOpenExplicitKeySkipsResolver(t *testing.T) {
	t.Parallel()
	called := false
	store, err := Open(context.Background(), Config{
		Mem0: mem0.Config{Endpoint: "https://example.test", APIKey: "explicit"},
		Resolver: resolverFunc(func(context.Context, Config) (string, error) {
			called = true
			return "", errors.New("unexpected")
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if called || store == nil {
		t.Fatalf("resolver called=%v store=%v", called, store)
	}
}

func TestStoreRedactsVolcDataPlaneKey(t *testing.T) {
	t.Parallel()
	store, err := Open(context.Background(), Config{Mem0: mem0.Config{
		Endpoint: "https://example.test", APIKey: "must-not-leak",
		HTTPClient: errorHTTPClient{err: errors.New("must-not-leak transport")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Observe(context.Background(), memorystore.Observation{
		Scope: memorystore.Scope{UserID: "user"}, Text: "remember",
	})
	if !errors.Is(err, memorystore.ErrUnavailable) || strings.Contains(fmt.Sprint(err), "must-not-leak") {
		t.Fatalf("Observe() error = %v", err)
	}
}

func TestStoreUsesVolcJobProtocol(t *testing.T) {
	t.Parallel()
	var paths []string
	var transportUserID string
	var encodedScope string
	var operationMarker string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/memories/":
			if r.Method == http.MethodPost {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode add: %v", err)
				}
				transportUserID, _ = body["user_id"].(string)
				metadata, _ := body["metadata"].(map[string]any)
				encodedScope, _ = metadata["gizclaw.entity_scope"].(string)
				operationMarker, _ = metadata["gizclaw.operation_marker"].(string)
				if body["app_id"] != "workspace" || body["run_id"] != "run" || body["async_mode"] != true || transportUserID != "user" || encodedScope == "" || operationMarker == "" {
					t.Errorf("unexpected Volc add routing")
				}
				_, _ = io.WriteString(w, `{"results":[{"event_id":"job-id"}]}`)
				return
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"results":[{"id":"fact-id","memory":"remembered","user_id":%q,"metadata":{"gizclaw.observation_id":"observation","gizclaw.entity_scope":%q,"gizclaw.operation_marker":%q}}]}`, transportUserID, encodedScope, operationMarker))
		case "/v1/job/job-id/":
			_, _ = io.WriteString(w, `{"status":"SUCCEEDED","results":[]}`)
		case "/v1/memories/fact-id/":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, fmt.Sprintf(`{"id":"fact-id","memory":"remembered","user_id":%q,"metadata":{"gizclaw.entity_scope":%q}}`, transportUserID, encodedScope))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{Mem0: mem0.Config{
		Endpoint: server.URL, APIKey: "key", PollInterval: time.Millisecond, HTTPClient: server.Client(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	scope := memorystore.Scope{AppID: "workspace", UserID: "user", RunID: "run"}
	observed, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, ID: "observation", Text: "remember"})
	if err != nil || observed.Operation == nil {
		t.Fatalf("Observe() = %#v, %v", observed, err)
	}
	completed, err := store.Wait(context.Background(), memorystore.OperationRequest{Scope: scope, ID: observed.Operation.ID})
	if err != nil || completed.Operation == nil || completed.Operation.Status != memorystore.OperationSucceeded || len(completed.Facts) != 1 {
		t.Fatalf("Wait() = %#v, %v", completed, err)
	}
	if err := store.Delete(context.Background(), memorystore.DeleteRequest{Scope: scope, ID: completed.Facts[0].ID}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	want := []string{"POST /v1/memories/", "GET /v1/job/job-id/", "GET /v1/memories/", "GET /v1/memories/fact-id/", "DELETE /v1/memories/fact-id/"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestStoreUsesVolcV1DirectFactProtocol(t *testing.T) {
	t.Parallel()
	var (
		transportUser string
		operationMark string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/memories/":
			_, _ = io.WriteString(w, `{"results":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/memories/":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode direct add: %v", err)
			}
			transportUser, _ = body["user_id"].(string)
			metadata, _ := body["metadata"].(map[string]any)
			operationMark, _ = metadata["gizclaw.operation_marker"].(string)
			if body["infer"] != false || body["async_mode"] != false || body["agent_id"] != "agent" || transportUser == "" || operationMark == "" {
				t.Errorf("unexpected direct add payload")
			}
			_, _ = io.WriteString(w, `{"results":[{"id":"fact","memory":"direct","event":"ADD"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{Mem0: mem0.Config{
		Endpoint: server.URL, APIKey: "key", PollInterval: time.Millisecond, HTTPClient: server.Client(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	scope := memorystore.Scope{AgentID: "agent"}
	observed, err := store.Observe(context.Background(), memorystore.Observation{
		Scope: scope, ID: "direct-observation", Facts: []memorystore.FactCandidate{{Text: "direct"}},
	})
	if err != nil || observed.Operation != nil || len(observed.Facts) != 1 || observed.Facts[0].Text != "direct" {
		t.Fatalf("Observe() = %#v, %v", observed, err)
	}
}

func TestStoreRejectsVolcJobWithoutCorrelatedFacts(t *testing.T) {
	t.Parallel()
	var operationMarker string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/memories/":
			if r.Method == http.MethodPost {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode add: %v", err)
				}
				metadata, _ := body["metadata"].(map[string]any)
				operationMarker, _ = metadata["gizclaw.operation_marker"].(string)
				_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"results":[{"id":"old","memory":"unrelated","user_id":"user","metadata":{"gizclaw.operation_marker":"old-operation"}}]}`)
		case "/v1/job/job/":
			_, _ = io.WriteString(w, `{"status":"SUCCEEDED","results":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{Mem0: mem0.Config{
		Endpoint: server.URL, APIKey: "key", PollInterval: time.Millisecond, HTTPClient: server.Client(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	scope := memorystore.Scope{UserID: "user"}
	observed, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, Text: "remember"})
	if err != nil || operationMarker == "" {
		t.Fatalf("Observe() = %#v, marker = %q, error = %v", observed, operationMarker, err)
	}
	_, err = store.Wait(context.Background(), memorystore.OperationRequest{Scope: scope, ID: observed.Operation.ID})
	if !errors.Is(err, memorystore.ErrUnavailable) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestStoreRejectsAmbiguousVolcJobIdentifiers(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"event_id":"wrong","results":[{"event_id":"one"},{"event_id":"two"}]}`,
		`{"event_id":"wrong","results":[]}`,
		`{"event_id":"wrong","results":{"event_id":"one"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			store, err := Open(context.Background(), Config{Mem0: mem0.Config{Endpoint: server.URL, APIKey: "key", HTTPClient: server.Client()}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.Observe(context.Background(), memorystore.Observation{Scope: memorystore.Scope{UserID: "user"}, Text: "remember"})
			if !errors.Is(err, memorystore.ErrUnavailable) {
				t.Fatalf("Observe() error = %v", err)
			}
		})
	}
}

func TestStoreHandlesVolcJobFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		jobStatus  int
		jobBody    string
		wantError  error
		wantStatus memorystore.OperationStatus
	}{
		{name: "terminal failure", jobStatus: http.StatusOK, jobBody: `{"status":"FAILED"}`, wantStatus: memorystore.OperationFailed},
		{name: "missing job", jobStatus: http.StatusNotFound, jobBody: `{"secret":"must-not-leak"}`, wantError: memorystore.ErrNotFound},
		{name: "malformed job", jobStatus: http.StatusOK, jobBody: `{`, wantError: memorystore.ErrUnavailable},
		{name: "unknown status", jobStatus: http.StatusOK, jobBody: `{"status":"MYSTERY"}`, wantError: memorystore.ErrUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/memories/" {
					_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
					return
				}
				w.WriteHeader(test.jobStatus)
				_, _ = io.WriteString(w, test.jobBody)
			}))
			defer server.Close()
			store, err := Open(context.Background(), Config{Mem0: mem0.Config{
				Endpoint: server.URL, APIKey: "must-not-leak", PollInterval: time.Millisecond, HTTPClient: server.Client(),
			}})
			if err != nil {
				t.Fatal(err)
			}
			scope := memorystore.Scope{UserID: "user"}
			observed, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, ID: "observation", Text: "remember"})
			if err != nil {
				t.Fatal(err)
			}
			completed, err := store.Wait(context.Background(), memorystore.OperationRequest{Scope: scope, ID: observed.Operation.ID})
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) || strings.Contains(fmt.Sprint(err), "must-not-leak") {
					t.Fatalf("Wait() error = %v", err)
				}
				return
			}
			if err != nil || completed.Operation == nil || completed.Operation.Status != test.wantStatus || strings.Contains(completed.Operation.Error, "must-not-leak") {
				t.Fatalf("Wait() = %#v, %v", completed, err)
			}
		})
	}
}

func TestStorePreservesVolcWaitCancellationAndScope(t *testing.T) {
	t.Parallel()
	jobCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/memories/" {
			_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
			return
		}
		jobCalls++
		_, _ = io.WriteString(w, `{"status":"RUNNING"}`)
	}))
	defer server.Close()
	store, err := Open(context.Background(), Config{Mem0: mem0.Config{
		Endpoint: server.URL, APIKey: "key", PollInterval: time.Millisecond, HTTPClient: server.Client(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	scope := memorystore.Scope{UserID: "user"}
	observed, err := store.Observe(context.Background(), memorystore.Observation{Scope: scope, ID: "observation", Text: "remember"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Wait(context.Background(), memorystore.OperationRequest{
		Scope: memorystore.Scope{UserID: "other"}, ID: observed.Operation.ID,
	}); !errors.Is(err, memorystore.ErrInvalidInput) || jobCalls != 0 {
		t.Fatalf("cross-scope Wait() error = %v, job calls = %d", err, jobCalls)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := store.Wait(ctx, memorystore.OperationRequest{Scope: scope, ID: observed.Operation.ID}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestOpenValidation(t *testing.T) {
	t.Parallel()
	for _, config := range []Config{
		{},
		{Mem0: mem0.Config{Endpoint: "https://example.test", APIKey: "key", Flavor: mem0.Platform}},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, Resolver: resolverFunc(func(context.Context, Config) (string, error) { return "", nil })},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, AccessKeyID: "id"},
		{Mem0: mem0.Config{Endpoint: "https://example.test"}, AccessKeyID: "id", AccessKeySecret: "secret", Region: "INVALID"},
	} {
		if _, err := Open(context.Background(), config); !errors.Is(err, memorystore.ErrInvalidInput) && !errors.Is(err, memorystore.ErrUnavailable) {
			t.Fatalf("Open(%+v) error = %v", config, err)
		}
	}
}

func TestConfigHasNoSerializationTags(t *testing.T) {
	t.Parallel()
	typeOfConfig := reflect.TypeFor[Config]()
	for field := range typeOfConfig.Fields() {
		if field.Tag.Get("yaml") != "" || field.Tag.Get("json") != "" {
			t.Fatalf("Config.%s contains serialization tags", field.Name)
		}
	}
}

func TestVolcControlAddress(t *testing.T) {
	t.Parallel()
	scheme, host, err := volcControlAddress("", "cn-shanghai")
	if err != nil || scheme != "https" || host != "mem0.cn-shanghai.volcengineapi.com" {
		t.Fatalf("default address = %q %q, %v", scheme, host, err)
	}
	for _, endpoint := range []string{"ftp://example.test", "https://user@example.test", "https://example.test/path"} {
		if _, _, err := volcControlAddress(endpoint, "cn-beijing"); !errors.Is(err, memorystore.ErrInvalidInput) {
			t.Fatalf("endpoint %q error = %v", endpoint, err)
		}
	}
}
