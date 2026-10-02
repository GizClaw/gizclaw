package memorystore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
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
			request := objectStoreTestRequest(t)
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
	request := objectStoreTestRequest(t)
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
	request := objectStoreTestRequest(t)
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

func TestManagedBindingRootUsesServerWorkspaceProfileAndAlias(t *testing.T) {
	root := t.TempDir()
	const profileID = "opaque/profile:id"
	got, err := managedBindingRoot(root, profileID, "pet-memory")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "data", "memory", customid.OpaquePathSegment(profileID), "pet-memory")
	if got != want {
		t.Fatalf("managedBindingRoot() = %q, want %q", got, want)
	}
}

func TestManagedBindingRootRejectsUnsafeAndSymlinkPaths(t *testing.T) {
	root := t.TempDir()
	for _, value := range []string{"", " profile "} {
		if _, err := managedBindingRoot(root, value, "memory"); err == nil {
			t.Errorf("managedBindingRoot(profile=%q) succeeded", value)
		}
	}
	for _, value := range []string{".", "..", "../escape", "a/b", `a\b`, " alias "} {
		if _, err := managedBindingRoot(root, "profile", value); err == nil {
			t.Errorf("managedBindingRoot(alias=%q) succeeded", value)
		}
	}
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := managedBindingRoot(linkRoot, "profile", "memory"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("managedBindingRoot(symlink) error = %v", err)
	}
}

func TestBuildManagedFlowcraftBBHPersistsCanonicalFacts(t *testing.T) {
	request := bbhTestRequest(t)
	root, err := managedBindingRoot(request.ServerRoot, request.ProfileID, request.BindingName)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{AppID: request.WorkspaceID},
		Facts: []memory.FactCandidate{{Text: "managed BBH canonical fact"}},
		ID:    "bbh-observation",
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "state.json")); err != nil {
		t.Fatalf("managed state.json: %v", err)
	}

	second, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	recalled, err := second.Store.Recall(t.Context(), memory.Query{
		Scope: memory.Scope{AppID: request.WorkspaceID}, Text: "canonical", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) != 1 {
		t.Fatalf("Recall() matches = %d, want 1 after managed BBH reopen", len(recalled.Matches))
	}
}

func TestBuildAcceptsCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)

	result, err := Build(t.Context(), request)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Closer != nil {
		t.Cleanup(func() { _ = result.Closer.Close() })
	}
}

func TestBuildFlowcraftObjectStoreRetainsWorkspaceCanonicalFormat(t *testing.T) {
	request := objectStoreTestRequest(t)
	dir := t.TempDir()
	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileFlowcraftObjectStoreConnection(
		apitypes.RuntimeProfileFlowcraftObjectStoreConnection{
			Type:      apitypes.RuntimeProfileFlowcraftObjectStoreConnectionTypeFlowcraftObjectStore,
			Directory: dir,
		},
	); err != nil {
		t.Fatal(err)
	}
	request.Binding.Connection = connection
	result, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := result.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{AppID: request.WorkspaceID},
		Facts: []memory.FactCandidate{{Text: "object store fact"}},
		ID:    "object-store-observation",
	}); err != nil {
		t.Fatal(err)
	}
	if err := result.Closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("object-store state.json: %v", err)
	}
}

func TestBuildRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := Build(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestBuildRejectsEmptyCanonicalLayoutID(t *testing.T) {
	request := objectStoreTestRequest(t)
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

func TestProjectionSignatureExcludesExtractionAndWritePolicy(t *testing.T) {
	policy := testFlowcraftPolicy()
	before, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Extraction.SystemPrompt = new("changed")
	policy.Write.Mode = apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic
	after, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("extraction/write policy changed derived-index identity")
	}
	policy.GraphEnabled = new(false)
	changed, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before == changed {
		t.Fatal("graph policy did not change derived-index identity")
	}
}

func TestFlowcraftConfigIncludesLaneExtractionInstructions(t *testing.T) {
	policy := testFlowcraftPolicy()
	policy.Lanes[0].Description = new("Durable story facts.")
	policy.Lanes[0].Extract = new("Capture only facts already narrated.")
	policy.Lanes[0].Recall = new("Use only after the Graph selects this lane.")
	config, err := flowcraftConfig(policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config.Extraction.SystemPrompt, "Extract: Capture only facts already narrated.") {
		t.Fatalf("extraction prompt = %q", config.Extraction.SystemPrompt)
	}
	if strings.Contains(config.Extraction.SystemPrompt, "Use only after the Graph") {
		t.Fatalf("recall guidance leaked into extraction prompt = %q", config.Extraction.SystemPrompt)
	}
}

func TestFlowcraftConfigCanDisableModelExtraction(t *testing.T) {
	policy := testFlowcraftPolicy()
	policy.Extraction.Model = "extraction"
	policy.Extraction.Enabled = new(false)

	config, err := flowcraftConfig(policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Extraction.Model != "" {
		t.Fatalf("extraction model = %q, want disabled", config.Extraction.Model)
	}
	if len(config.LaneNames) == 0 {
		t.Fatal("disabling model extraction removed direct-Fact lane policy")
	}
}

func TestBuildFlowcraftObjectStoreKeepsWorkspaceScopesIsolated(t *testing.T) {
	request := objectStoreTestRequest(t)
	request.WorkspaceID = "workspace-a"
	request.BindingName = "memory"
	result, err := Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Closer != nil {
		t.Cleanup(func() { _ = result.Closer.Close() })
	}
	if _, err := result.Store.Observe(context.Background(), memory.Observation{
		Scope: memory.Scope{AppID: "workspace-a"},
		Text:  "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}
	recallA, err := result.Store.Recall(context.Background(), memory.Query{
		Scope: memory.Scope{AppID: "workspace-a"},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recallA.Matches) == 0 {
		t.Fatal("workspace-a did not recall its fact")
	}
	recallB, err := result.Store.Recall(context.Background(), memory.Query{
		Scope: memory.Scope{AppID: "workspace-b"},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recallB.Matches) != 0 {
		t.Fatalf("workspace-b recalled %d workspace-a facts", len(recallB.Matches))
	}
}

func TestFlowcraftObjectStoreProjectionRebuildPreservesCanonicalFacts(t *testing.T) {
	request := objectStoreTestRequest(t)
	first, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{AppID: request.WorkspaceID},
		Text:  "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Closer.Close(); err != nil {
		t.Fatal(err)
	}

	request.Layout.Spec.Flowcraft.GraphEnabled = new(false)
	second, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	recalled, err := second.Store.Recall(t.Context(), memory.Query{
		Scope: memory.Scope{AppID: request.WorkspaceID},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) == 0 {
		t.Fatal("derived-index rebuild lost canonical Workspace facts")
	}
}

func testFlowcraftPolicy() apitypes.FlowcraftMemoryLayoutPolicy {
	return apitypes.FlowcraftMemoryLayoutPolicy{
		Extraction: apitypes.FlowcraftMemoryExtractionPolicy{
			Mode: apitypes.FlowcraftMemoryExtractionPolicyModeTwoPass,
		},
		Lanes: []apitypes.FlowcraftMemoryLanePolicy{{
			Name: "facts",
			Kind: apitypes.FlowcraftMemoryLanePolicyKindNote,
		}},
		Write: apitypes.FlowcraftMemoryWritePolicy{
			Mode: apitypes.FlowcraftMemoryWritePolicyModeSync,
			Tier: apitypes.FlowcraftMemoryWritePolicyTierGeneral,
		},
	}
}
