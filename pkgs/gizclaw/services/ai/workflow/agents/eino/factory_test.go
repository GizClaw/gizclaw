package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestGenXModelContextPreservesToolsCallsAndResults(t *testing.T) {
	t.Parallel()
	toolInfo := &schema.ToolInfo{
		Name: "current_peer", Desc: "Read from the current Peer.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"key": {Type: schema.String, Required: true},
		}),
	}
	context, err := genXModelContext([]*schema.Message{
		schema.UserMessage("read it"),
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "call-1", Type: "function",
				Function: schema.FunctionCall{Name: "current_peer", Arguments: `{"key":"x"}`},
			}},
		},
		{
			Role: schema.Tool, ToolCallID: "call-1", ToolName: "current_peer",
			Content: `{"value":"ok"}`,
		},
	}, model.WithTools([]*schema.ToolInfo{toolInfo}))
	if err != nil {
		t.Fatalf("genXModelContext() error = %v", err)
	}
	var tools []*genx.FuncTool
	for item := range context.Tools() {
		tool, ok := item.(*genx.FuncTool)
		if !ok {
			t.Fatalf("Tool type = %T", item)
		}
		tools = append(tools, tool)
	}
	if len(tools) != 1 || tools[0].Name != "current_peer" || tools[0].Argument == nil {
		t.Fatalf("Tools = %#v", tools)
	}
	var messages []*genx.Message
	for message := range context.Messages() {
		messages = append(messages, message)
	}
	if len(messages) != 3 {
		t.Fatalf("Messages = %#v", messages)
	}
	call, ok := messages[1].Payload.(*genx.ToolCall)
	if !ok || call.ID != "call-1" || call.FuncCall == nil ||
		call.FuncCall.Name != "current_peer" || call.FuncCall.Arguments != `{"key":"x"}` {
		t.Fatalf("ToolCall = %#v", messages[1].Payload)
	}
	result, ok := messages[2].Payload.(*genx.ToolResult)
	if !ok || result.ID != "call-1" || result.Result != `{"value":"ok"}` {
		t.Fatalf("ToolResult = %#v", messages[2].Payload)
	}
}

func TestEinoToolCallValidatesProviderOutput(t *testing.T) {
	t.Parallel()
	got, err := einoToolCall(&genx.ToolCall{
		ID: "call-1",
		FuncCall: &genx.FuncCall{
			Name: "current_peer", Arguments: `{"key":"x"}`,
		},
	}, 2)
	if err != nil {
		t.Fatalf("einoToolCall() error = %v", err)
	}
	if got.Index == nil || *got.Index != 2 || got.ID != "call-1" ||
		got.Function.Name != "current_peer" || got.Function.Arguments != `{"key":"x"}` {
		t.Fatalf("einoToolCall() = %#v", got)
	}
	if _, err := einoToolCall(&genx.ToolCall{
		ID: "call-2", FuncCall: &genx.FuncCall{Name: "bad", Arguments: `{`},
	}, 0); err == nil {
		t.Fatal("einoToolCall() accepted invalid JSON")
	}
}

func TestFactoryAllowsMemorylessWorkflowAndRequiresResolvedStoreForMemoryNodes(t *testing.T) {
	t.Parallel()
	spec := einoFactorySpec(t)
	factory := Factory{GenX: &peergenx.Service{}}
	if _, err := factory.NewAgent(t.Context(), spec); err != nil {
		t.Fatalf("NewAgent(memoryless) error = %v", err)
	}

	addEinoMemoryRecallNode(t, spec.Workflow.Spec.Eino)
	if _, err := factory.NewAgent(t.Context(), spec); err == nil ||
		!strings.Contains(err.Error(), "Graph Memory nodes require Memory") {
		t.Fatalf("NewAgent(memory without store) error = %v", err)
	}
}

func TestFactoryBindsOnlyWorkspaceAppAndReportsConfiguredBackend(t *testing.T) {
	t.Parallel()
	store := &einoMemoryStore{}
	spec := einoFactorySpec(t)
	owner := "owner-public-key"
	spec.Workspace.OwnerPublicKey = &owner
	spec.Memory = store
	spec.MemoryKind = "volc_mem0"
	service := &peergenx.Service{}
	agent, err := (Factory{
		GenX: service,
		GenXForOwner: func(context.Context, string) (*peergenx.Service, error) {
			return service, nil
		},
	}).NewAgent(t.Context(), spec)
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	response, err := agent.Recall(t.Context(), apitypes.PeerRunRecallRequest{Query: "remember"})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Snippet != "remembered" {
		t.Fatalf("Recall() = %#v", response)
	}
	store.mu.Lock()
	scope := store.query.Scope
	store.mu.Unlock()
	if scope.AppID != spec.Workspace.Id || scope.AgentID != "" {
		t.Fatalf("Recall scope = %#v, want only Workspace AppID", scope)
	}
	if scope.UserID != "" || scope.RunID != "" {
		t.Fatalf("Recall scope rewrote inner dimensions: %#v", scope)
	}
	stats, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if stats.Backend == nil || *stats.Backend != "volc_mem0" {
		t.Fatalf("MemoryStats backend = %v", stats.Backend)
	}
	if stats.Metadata == nil {
		t.Fatal("MemoryStats metadata is nil")
	}
	metadataScope, ok := (*stats.Metadata)["scope"].(map[string]any)
	if !ok || metadataScope["app_id"] != spec.Workspace.Id ||
		metadataScope["agent_id"] != "" {
		t.Fatalf("MemoryStats scope = %#v", (*stats.Metadata)["scope"])
	}
}

func addEinoMemoryRecallNode(t testing.TB, spec *apitypes.EinoWorkflowSpec) {
	t.Helper()
	var node apitypes.EinoNode
	if err := node.FromEinoMemoryRecallNode(apitypes.EinoMemoryRecallNode{
		Id:        "recall",
		Type:      apitypes.EinoMemoryRecallNodeTypeMemoryRecall,
		QueryFrom: "input.text",
		Output:    "answer",
		TopK:      5,
	}); err != nil {
		t.Fatal(err)
	}
	spec.Graph.Nodes = append(spec.Graph.Nodes, node)
	spec.Graph.Edges = []apitypes.EinoEdge{
		{From: "start", To: "recall"},
		{From: "recall", To: "answer"},
		{From: "answer", To: "end"},
	}
}

func einoFactorySpec(t testing.TB) agenthost.Spec {
	t.Helper()
	var public apitypes.EinoWorkflowSpec
	if err := json.Unmarshal([]byte(`{
		"graph": {
			"name": "factory-test",
			"compile": {"node_trigger_mode": "any_predecessor"},
			"state": {"fields": [{"name": "answer", "type": "string", "merge": "replace"}]},
			"nodes": [{
				"id": "answer",
				"type": "passthrough",
				"inputs": {"value": {"from": "input.text"}},
				"outputs": {"value": "answer"}
			}],
			"edges": [{"from": "start", "to": "answer"}, {"from": "answer", "to": "end"}],
			"branches": [],
			"outputs": [{
				"node": "answer",
				"field": "answer",
				"name": "assistant",
				"mime_type": "text/plain",
				"primary": true
			}]
		}
	}`), &public); err != nil {
		t.Fatalf("decode Eino factory fixture: %v", err)
	}
	return agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "workspace-a"},
		Workflow: apitypes.Workflow{
			Name: "workflow-a",
			Spec: apitypes.WorkflowSpec{
				Driver: apitypes.WorkflowDriverEino,
				Eino:   &public,
			},
		},
	}
}

type einoMemoryStore struct {
	mu    sync.Mutex
	query memory.Query
}

func (*einoMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (s *einoMemoryStore) Recall(_ context.Context, query memory.Query) (memory.RecallResult, error) {
	s.mu.Lock()
	s.query = query
	s.mu.Unlock()
	return memory.RecallResult{Matches: []memory.Match{{
		Fact:  memory.Fact{ID: "fact-a", Text: "remembered"},
		Score: 1,
	}}}, nil
}

func (*einoMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, errors.New("unexpected Update")
}

func (*einoMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return errors.New("unexpected Delete")
}
