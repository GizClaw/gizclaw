package memorystore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestBuildSelfHostedMem0UsesOSSProtocolAndOptionalAuthentication(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"", "test-key"} {
		t.Run("api-key="+key, func(t *testing.T) {
			var requests []string
			var entity string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				requests = append(requests, r.Method+" "+r.URL.Path)
				if r.Header.Get("X-API-Key") != key || r.Header.Get("Authorization") != "" {
					t.Errorf("unexpected authentication headers")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /memories", "POST /search":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if r.URL.Path == "/memories" {
						if body["prompt"] != "self-hosted instructions" {
							t.Errorf("self-hosted prompt = %v", body["prompt"])
						}
						entity, _ = body["user_id"].(string)
						if entity == "" || body["app_id"] != nil {
							t.Error("write did not use the self-hosted scope encoding")
						}
					} else if body["filters"].(map[string]any)["user_id"] != entity {
						t.Error("recall scope differs from write scope")
					}
					_, _ = io.WriteString(w, `{"results":[]}`)
				case "DELETE /memories", "GET /memories":
					if r.URL.Query().Get("user_id") != entity {
						t.Error("purge scope differs from write scope")
					}
					_, _ = io.WriteString(w, `{"results":[]}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			request := remoteTestRequest(t)
			request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
			request.Layout.Spec.Mem0 = apitypes.Mem0MemoryLayoutPolicy{
				CustomInstructions: new("Cloud instructions"), CustomCategories: &map[string]string{"pet": "Cloud category"},
				Multilingual: new(true), Decay: new(true),
			}
			request.Layout.Spec.Mem0SelfHosted = &apitypes.Mem0SelfHostedMemoryLayoutPolicy{CustomInstructions: new("self-hosted instructions")}
			connection := apitypes.RuntimeProfileMem0SelfHostedConnection{
				Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: server.URL,
			}
			if key != "" {
				connection.ApiKey = &key
			}
			if err := request.Binding.Connection.FromRuntimeProfileMem0SelfHostedConnection(connection); err != nil {
				t.Fatal(err)
			}
			result, err := Build(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			scope := memory.Scope{AppID: request.WorkspaceID}
			if _, err := result.Store.Observe(t.Context(), memory.Observation{Scope: scope, Text: "remember Mochi"}); err != nil {
				t.Fatal(err)
			}
			if _, err := result.Store.Recall(t.Context(), memory.Query{Scope: scope, Text: "Mochi", Limit: 5}); err != nil {
				t.Fatal(err)
			}
			if err := memory.PurgeScope(t.Context(), result.Store, scope); err != nil {
				t.Fatal(err)
			}
			if empty, err := memory.ScopeEmpty(t.Context(), result.Store, scope); err != nil || !empty {
				t.Fatalf("ScopeEmpty() = %v, %v", empty, err)
			}
			if got := strings.Join(requests, ","); got != "POST /memories,POST /search,DELETE /memories,GET /memories" {
				t.Fatalf("requests = %s", got)
			}
		})
	}
}

func TestSharedSelfHostedMem0KeepsEachLayoutGenerationPolicy(t *testing.T) {
	t.Parallel()
	var received []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		received = append(received, payload.Prompt)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[]}`)
	}))
	defer server.Close()
	request := remoteTestRequest(t)
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
	if err := request.Binding.Connection.FromRuntimeProfileMem0SelfHostedConnection(apitypes.RuntimeProfileMem0SelfHostedConnection{
		Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: server.URL,
	}); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	defer registry.Close()
	var stores []Result
	for _, instructions := range []string{"pet policy", "calendar policy"} {
		request.Layout.Spec.Mem0SelfHosted = &apitypes.Mem0SelfHostedMemoryLayoutPolicy{CustomInstructions: &instructions}
		result, err := registry.Resolve(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Closer.Close()
		stores = append(stores, result)
	}
	for _, index := range []int{0, 1, 0} {
		if _, err := stores[index].Store.Observe(t.Context(), memory.Observation{Text: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(received, ","); got != "pet policy,calendar policy,pet policy" {
		t.Fatalf("logical Layout policies leaked: %q", got)
	}
}

func TestSelfHostedMem0RequiresIndependentPolicy(t *testing.T) {
	t.Parallel()
	request := remoteTestRequest(t)
	request.Layout.Spec.Mem0SelfHosted = nil
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
	if err := request.Binding.Connection.FromRuntimeProfileMem0SelfHostedConnection(apitypes.RuntimeProfileMem0SelfHostedConnection{
		Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: "http://localhost:8000",
	}); err != nil {
		t.Fatal(err)
	}
	request.Layout.Spec.Mem0.CustomInstructions = new("cloud instructions")
	for _, build := range []func() error{
		func() error { _, err := Build(t.Context(), request); return err },
		func() error { _, err := ScopeForRequest(request); return err },
	} {
		if err := build(); err == nil || !strings.Contains(err.Error(), "spec.mem0_self_hosted is required") {
			t.Fatalf("missing policy error = %v", err)
		}
	}
}

func TestBuildAcceptsCanonicalLayoutID(t *testing.T) {
	request := remoteTestRequest(t)

	result, err := Build(t.Context(), request)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Closer != nil {
		t.Cleanup(func() { _ = result.Closer.Close() })
	}
}

func TestBuildRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := remoteTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := Build(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestBuildRejectsEmptyCanonicalLayoutID(t *testing.T) {
	request := remoteTestRequest(t)
	request.Layout.Id = ""

	_, err := Build(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestBuildVolcMem0UsesVolcProtocol(t *testing.T) {
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestPath <- request.Method + " " + request.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
	}))
	t.Cleanup(server.Close)

	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileVolcMem0Connection(apitypes.RuntimeProfileVolcMem0Connection{
		ApiKey:          "key",
		Endpoint:        server.URL,
		MemoryProjectId: "project",
		Type:            apitypes.RuntimeProfileVolcMem0ConnectionTypeVolcMem0,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := Build(t.Context(), Request{
		WorkspaceID: "workspace",
		BindingName: "memory",
		Layout:      apitypes.MemoryLayout{Id: "layout-id"},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId:   "layout-id",
			Driver:     apitypes.RuntimeProfileMemoryDriverVolcMem0,
			Connection: connection,
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	observed, err := result.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{UserID: "user"},
		Text:  "remember this",
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observed.Operation == nil {
		t.Fatal("Observe() returned no Volc operation")
	}
	if got := <-requestPath; got != "POST /v1/memories/" {
		t.Fatalf("Volc request = %q, want POST /v1/memories/", got)
	}
}
