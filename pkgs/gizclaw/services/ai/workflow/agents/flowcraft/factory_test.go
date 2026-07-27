package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	genxflowcraft "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestMapGraphSupportsPublicNodesAndDerivesPublishers(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"prepare",
			"nodes":[
				{"id":"prepare","type":"script","config":{"source":"board.setVar('ok', true);"}},
				{"id":"route","type":"passthrough"},
				{"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":128}}
			],
			"edges":[{"from":"prepare","to":"route"},{"from":"route","to":"answer"},{"from":"answer","to":"__end__"}]
		}
	}`)
	graph, publish, err := mapGraph(spec.Graph)
	if err != nil {
		t.Fatalf("mapGraph() error = %v", err)
	}
	if len(graph.Nodes) != 3 || graph.Nodes[0].Type != "script" || graph.Nodes[1].Type != "passthrough" || graph.Nodes[2].Type != "llm" {
		t.Fatalf("mapped nodes = %#v", graph.Nodes)
	}
	if !reflect.DeepEqual(publish, []string{"answer"}) {
		t.Fatalf("publish nodes = %#v", publish)
	}
	if graph.Nodes[2].Config["model"] != "llm" {
		t.Fatalf("LLM config = %#v", graph.Nodes[2].Config)
	}
}

func TestFactoryConstructsWithoutLocalWorkspace(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	agent, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec,
		}},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if closer, ok := agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}

func TestBuildRuntimeEmbedderRequiresProviderCredential(t *testing.T) {
	makeModel := func(t *testing.T, provider apitypes.ModelProviderKind, data apitypes.ModelProviderData) apitypes.Model {
		t.Helper()
		return apitypes.Model{Id: "embedding", Provider: apitypes.ModelProvider{Kind: provider}, ProviderData: data}
	}
	dashScopeData := apitypes.ModelProviderData{}
	if err := dashScopeData.FromDashScopeTenantModelProviderData(apitypes.DashScopeTenantModelProviderData{}); err != nil {
		t.Fatal(err)
	}
	volcData := apitypes.ModelProviderData{}
	if err := volcData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		config peergenx.EmbeddingConfig
		want   string
	}{
		{
			name: "dashscope api key",
			config: peergenx.EmbeddingConfig{
				Model:      makeModel(t, apitypes.ModelProviderKindDashscopeTenant, dashScopeData),
				Tenant:     peergenx.Tenant{Kind: string(apitypes.ModelProviderKindDashscopeTenant), DashScope: &apitypes.DashScopeTenant{}},
				Credential: apitypes.Credential{Name: "dashscope-empty", Body: testDashScopeCredentialBody("")},
			},
			want: "credential \"dashscope-empty\" has no api_key",
		},
		{
			name: "volc ark api key",
			config: peergenx.EmbeddingConfig{
				Model:      makeModel(t, apitypes.ModelProviderKindVolcTenant, volcData),
				Tenant:     peergenx.Tenant{Kind: string(apitypes.ModelProviderKindVolcTenant), Volc: &apitypes.VolcTenant{}},
				Credential: apitypes.Credential{Name: "volc-empty", Body: testVolcCredentialBodyFromStrings(nil)},
			},
			want: "credential \"volc-empty\" has no ark_api_key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildRuntimeEmbedder(tc.config); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("buildRuntimeEmbedder() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFactoryUsesWorkspaceOwnerGenX(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	owner := "owner-public-key"
	called := false
	factory := Factory{GenXForOwner: func(_ context.Context, gotOwner string) (*peergenx.Service, error) {
		called = true
		if gotOwner != owner {
			t.Fatalf("owner = %q, want %q", gotOwner, owner)
		}
		return peergenx.New(peergenx.Service{}), nil
	}}
	agent, err := factory.NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a", OwnerPublicKey: &owner},
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec}},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if !called {
		t.Fatal("owner GenX resolver was not called")
	}
	if closer, ok := agent.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
}

func TestFactoryUsesResolvedWorkspaceMemory(t *testing.T) {
	backing := &recordingExternalMemoryStore{}
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	agent, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a"},
		Workflow: apitypes.Workflow{Name: "workflow-a", Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec,
		}},
		Memory: backing, MemoryKind: "mem0",
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := agent.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	stats, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if !stats.Enabled || stats.Backend == nil || *stats.Backend != "mem0" {
		t.Fatalf("MemoryStats() = %#v", stats)
	}
}

func TestMapInitiativePreservesWorkspacePolicy(t *testing.T) {
	agent := apitypes.FlowcraftConversationStartsAgent
	peer := apitypes.FlowcraftConversationStartsPeer
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &agent}, "once_when_empty"); got != genxflowcraft.InitiativeOnceWhenEmpty {
		t.Fatalf("once_when_empty = %q", got)
	}
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &agent}, "on_reload"); got != genxflowcraft.InitiativeOnReload {
		t.Fatalf("on_reload = %q", got)
	}
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &peer}, "once_when_empty"); got != genxflowcraft.InitiativeDisabled {
		t.Fatalf("peer initiative = %q", got)
	}
}

func TestWorkspaceAgentScopeIncludesOwnerWhenAvailable(t *testing.T) {
	if got, want := workspaceAgentScope("owner-a", "workspace-a", "assistant"), "o/95256875151043ab/w/0dcf2d98505da17d/a/a39a7ffad4a3013f"; got != want {
		t.Fatalf("workspaceAgentScope() = %q, want %q", got, want)
	}
	if got, want := workspaceAgentScope("", "workspace-a", "assistant"), "w/0dcf2d98505da17d/a/a39a7ffad4a3013f"; got != want {
		t.Fatalf("workspaceAgentScope() without owner = %q, want %q", got, want)
	}
	if got := workspaceAgentScope(strings.Repeat("owner", 100), strings.Repeat("workspace", 100), strings.Repeat("agent", 100)); len(got) != len("o/")+16+len("/w/")+16+len("/a/")+16 {
		t.Fatalf("workspaceAgentScope() length = %d for long identities, got %q", len(got), got)
	}
}

func TestFlowcraftStateStoreUsesCanonicalIsolatedScope(t *testing.T) {
	base := kv.NewMemory(nil)
	scope := workspaceAgentScope("owner-a", "workspace-a", "assistant")
	state := flowcraftStateStore(base, scope)
	if err := state.Set(t.Context(), kv.Key{"checkpoint"}, []byte("private")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	expectedKey := append(kv.Key{"flowcraft"}, strings.Split(scope, "/")...)
	expectedKey = append(expectedKey, "checkpoint")
	value, err := base.Get(t.Context(), expectedKey)
	if err != nil {
		t.Fatalf("base.Get(%v) error = %v", expectedKey, err)
	}
	if string(value) != "private" {
		t.Fatalf("base.Get(%v) = %q", expectedKey, value)
	}

	for _, otherScope := range []string{
		workspaceAgentScope("owner-b", "workspace-a", "assistant"),
		workspaceAgentScope("owner-a", "workspace-b", "assistant"),
		workspaceAgentScope("owner-a", "workspace-a", "other-agent"),
	} {
		if _, err := flowcraftStateStore(base, otherScope).Get(t.Context(), kv.Key{"checkpoint"}); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("scope %q Get() error = %v, want ErrNotFound", otherScope, err)
		}
	}
}

func TestManagedAgentClosesOwnedResourcesOnceConcurrently(t *testing.T) {
	closer := &countingCloser{}
	agent := &managedAgent{owned: []io.Closer{closer}}
	var wait sync.WaitGroup
	for range 16 {
		wait.Go(func() {
			if err := agent.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
	wait.Wait()
	if calls := closer.count(); calls != 1 {
		t.Fatalf("owned close calls = %d, want 1", calls)
	}
}

func decodeFlowcraftSpec(t *testing.T, raw string) apitypes.FlowcraftWorkflowSpec {
	t.Helper()
	var spec apitypes.FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("decode Flowcraft spec: %v", err)
	}
	return spec
}

type countingCloser struct {
	mu    sync.Mutex
	calls int
}

type recordingExternalMemoryStore struct {
	observation memorystore.Observation
}

func (s *recordingExternalMemoryStore) Observe(_ context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	s.observation = observation
	return memorystore.ObserveResult{}, nil
}

func (*recordingExternalMemoryStore) Recall(context.Context, memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{}, nil
}

func (*recordingExternalMemoryStore) Update(context.Context, memorystore.UpdateRequest) (memorystore.Fact, error) {
	return memorystore.Fact{}, nil
}

func (*recordingExternalMemoryStore) Delete(context.Context, memorystore.DeleteRequest) error {
	return nil
}

func (c *countingCloser) Close() error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return nil
}

func (c *countingCloser) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}
