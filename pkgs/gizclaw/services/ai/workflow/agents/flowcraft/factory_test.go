package flowcraft

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	flowmemorystore "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	flowretrievalstore "github.com/GizClaw/flowcraft/memory/retrieval/workspace"
	genxflowcraft "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestMapGraphSupportsPublicNodesAndDerivesPublishers(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{
			"name":"graph","entry":"prepare",
			"nodes":[
				{"id":"prepare","type":"script","config":{"source":"board.setVar('ok', true);"}},
				{"id":"route","type":"passthrough"},
				{"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":128}}
			],
			"edges":[{"from":"prepare","to":"route"},{"from":"route","to":"answer"},{"from":"answer","to":"__end__"}]
		}}
	}`)
	graph, publish, err := mapGraph(spec.Agent.Graph)
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
		"agent":{"id":"assistant","name":"Assistant","graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}}
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

func TestFactoryUsesWorkspaceOwnerGenX(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}}
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

func TestFactoryRequiresDurableBackendForEnabledMemory(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}},
		"memory":{"enabled":true,"extract":{"enabled":false}}
	}`)
	_, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a"},
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec}},
	})
	if err == nil || !strings.Contains(err.Error(), `workspace "workspace-a" memory requires a server object store`) {
		t.Fatalf("NewAgent() error = %v", err)
	}
}

func TestMapMemoryConfigPreservesPolicy(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{"name":"graph","entry":"route","nodes":[{"id":"route","type":"passthrough","publish":true}]}},
		"memory":{
			"enabled":true,
			"extract":{"enabled":false},
			"layout":{"lanes":[{"name":"owner","kind":"fact","recall":"Use only for owner continuity."}]},
			"recall":{"enabled":true,"graph_enabled":true,"include_retired":true,"profiles":{"owner":{"output":"memory_context","query":{"text":"${input} owner facts","kinds":["fact"],"lanes":["owner"]},"render":{"header":"Memory:","max_items":3},"top_k":4}}},
			"write":{"mode":"async_semantic","tier":"core","save_conversation":true,"board_facts":[{"board_var":"profile","kind":"fact","subject":"owner","predicate":"prefers","object":"tea","entities":["owner","tea"],"required_prefix":"profile:"}]}
		}
	}`)
	workspace, err := newObjectWorkspace(objectstore.Dir(t.TempDir()), "memory/workspace-a/assistant")
	if err != nil {
		t.Fatalf("newObjectWorkspace() error = %v", err)
	}
	backend, err := flowmemorystore.New(workspace)
	if err != nil {
		t.Fatalf("memory backend error = %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	retrievalIndex, err := flowretrievalstore.New(workspace)
	if err != nil {
		t.Fatalf("retrieval index error = %v", err)
	}
	t.Cleanup(func() { _ = retrievalIndex.Close() })
	runtimeConfig, mapped, err := mapMemoryConfig(*spec.Memory, nil, backend, retrievalIndex)
	if err != nil {
		t.Fatalf("mapMemoryConfig() error = %v", err)
	}
	if runtimeConfig.AsyncQueue == nil || runtimeConfig.RetrievalIndex != retrievalIndex || runtimeConfig.Tier != "core" || !runtimeConfig.GraphEnabled || !mapped.observe || mapped.observeWait {
		t.Fatalf("runtime config = %#v, mapped = %#v", runtimeConfig, mapped)
	}
	if len(mapped.recallProfiles) != 1 || mapped.recallProfiles[0].BoardVariable != "memory_context" || mapped.recallProfiles[0].QueryText != "${input} owner facts" || mapped.recallProfiles[0].Limit != 4 {
		t.Fatalf("recall profiles = %#v", mapped.recallProfiles)
	}
	filters := mapped.recallProfiles[0].Filters
	if len(filters) != 3 || filters[2].Field != "include_retired" || filters[2].Value != true {
		t.Fatalf("recall filters = %#v", filters)
	}
	rendered, err := mapped.recallProfiles[0].Renderer(t.Context(), []memorystore.Match{{Fact: memorystore.Fact{Text: "likes tea"}}})
	if err != nil {
		t.Fatalf("recall renderer error = %v", err)
	}
	if !strings.Contains(rendered, "Use only for owner continuity.") || !strings.Contains(rendered, "Memory:\n- likes tea") {
		t.Fatalf("rendered recall = %q", rendered)
	}
	observation, err := mapped.observationBuilder(t.Context(), genxflowcraft.ObservationInput{
		UserText: "hello", BoardVariables: map[string]any{"profile": "prefix profile: tea"},
	})
	if err != nil {
		t.Fatalf("observation builder error = %v", err)
	}
	if len(observation.Facts) != 1 {
		t.Fatalf("observation facts = %#v", observation.Facts)
	}
	fact := observation.Facts[0]
	if fact.Text != "profile: tea" || fact.Attributes["kind"] != "fact" || fact.Attributes["subject"] != "owner" || fact.Attributes["predicate"] != "prefers" || fact.Attributes["object"] != "tea" {
		t.Fatalf("observation fact = %#v", fact)
	}
	if entities, ok := fact.Attributes["entities"].([]string); !ok || !reflect.DeepEqual(entities, []string{"owner", "tea"}) {
		t.Fatalf("observation fact entities = %#v", fact.Attributes["entities"])
	}
}

func TestObservationBuilderDoesNotPersistConversationWhenDisabled(t *testing.T) {
	write := apitypes.FlowcraftMemoryWrite{BoardFacts: &[]apitypes.FlowcraftMemoryBoardFact{{BoardVar: "state"}}}
	observation, err := observationBuilder(&write)(t.Context(), genxflowcraft.ObservationInput{
		StreamID: "stream", UserText: "private user turn", DeliveredAssistantText: "private assistant turn",
		BoardVariables: map[string]any{"state": "durable state"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Text != "" || len(observation.Turns) != 0 || len(observation.Facts) != 1 || observation.Facts[0].Text != "durable state" {
		t.Fatalf("observation = %#v", observation)
	}
}

func TestFactoryMemoryRetrievalSurvivesAgentReload(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{"name":"graph","entry":"route","nodes":[{"id":"route","type":"passthrough","publish":true}]}},
		"memory":{"enabled":true,"extract":{"enabled":false}}
	}`)
	factory := Factory{MemoryObjects: objectstore.Dir(t.TempDir())}
	store, closer, _, err := factory.buildMemory(t.Context(), "", "workspace-a", "assistant", *spec.Memory)
	if err != nil {
		t.Fatalf("buildMemory() error = %v", err)
	}
	scope := memorystore.Scope(workspaceAgentScope("", "workspace-a", "assistant"))
	if _, err := store.Observe(t.Context(), memorystore.Observation{
		ID: "turn-1", Scope: scope, Text: "the persistent lantern is blue", ObservedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reloaded, reloadedCloser, _, err := factory.buildMemory(t.Context(), "", "workspace-a", "assistant", *spec.Memory)
	if err != nil {
		t.Fatalf("reload buildMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = reloadedCloser.Close() })
	result, err := reloaded.Recall(t.Context(), memorystore.Query{Scope: scope, Text: "lantern", Limit: 5})
	if err != nil {
		t.Fatalf("Recall() after reload error = %v", err)
	}
	if len(result.Matches) == 0 || !strings.Contains(result.Matches[0].Fact.Text, "persistent lantern") {
		t.Fatalf("Recall() after reload = %#v", result)
	}
}

func TestFactoryMemoryIsolatesWorkspaces(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"agent":{"id":"assistant","name":"Assistant","graph":{"name":"graph","entry":"route","nodes":[{"id":"route","type":"passthrough","publish":true}]}},
		"memory":{"enabled":true,"extract":{"enabled":false}}
	}`)
	factory := Factory{MemoryObjects: objectstore.Dir(t.TempDir())}
	workspaceA, closeA, _, err := factory.buildMemory(t.Context(), "", "workspace-a", "assistant", *spec.Memory)
	if err != nil {
		t.Fatalf("build workspace A memory error = %v", err)
	}
	t.Cleanup(func() { _ = closeA.Close() })
	workspaceB, closeB, _, err := factory.buildMemory(t.Context(), "", "workspace-b", "assistant", *spec.Memory)
	if err != nil {
		t.Fatalf("build workspace B memory error = %v", err)
	}
	t.Cleanup(func() { _ = closeB.Close() })

	scopeA := memorystore.Scope(workspaceAgentScope("", "workspace-a", "assistant"))
	if _, err := workspaceA.Observe(t.Context(), memorystore.Observation{
		ID: "turn-a", Scope: scopeA, Text: "the private compass is silver", ObservedAt: time.Now(),
	}); err != nil {
		t.Fatalf("observe workspace A error = %v", err)
	}
	scopeB := memorystore.Scope(workspaceAgentScope("", "workspace-b", "assistant"))
	result, err := workspaceB.Recall(t.Context(), memorystore.Query{Scope: scopeB, Text: "compass", Limit: 5})
	if err != nil {
		t.Fatalf("recall workspace B error = %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("workspace B recalled workspace A facts: %#v", result.Matches)
	}
}

func TestMemoryObjectPrefixIsStableShortAndIsolated(t *testing.T) {
	first := memoryObjectPrefix("owner-a", "flowcraft-poetry-adventure-li-bai-ptt", "poetry-adventure-li-bai")
	if got, want := len(first), len("fc/")+32; got != want {
		t.Fatalf("memoryObjectPrefix() length = %d, want %d", got, want)
	}
	if repeated := memoryObjectPrefix("owner-a", "flowcraft-poetry-adventure-li-bai-ptt", "poetry-adventure-li-bai"); repeated != first {
		t.Fatalf("memoryObjectPrefix() is not stable: %q != %q", repeated, first)
	}
	if other := memoryObjectPrefix("owner-a", "flowcraft-poetry-adventure-li-bai-ptt", "another-agent"); other == first {
		t.Fatalf("memoryObjectPrefix() did not isolate agents: %q", first)
	}
	if other := memoryObjectPrefix("owner-a", "another-workspace", "poetry-adventure-li-bai"); other == first {
		t.Fatalf("memoryObjectPrefix() did not isolate workspaces: %q", first)
	}
	if other := memoryObjectPrefix("owner-b", "flowcraft-poetry-adventure-li-bai-ptt", "poetry-adventure-li-bai"); other == first {
		t.Fatalf("memoryObjectPrefix() did not isolate owners: %q", first)
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
