package eino

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestFactoryBuildsTypedConfig(t *testing.T) {
	history := &noopLogStore{}
	memoryStore := &noopMemoryStore{}
	prompt := "be concise"
	maxSteps := 7
	maxCalls := 3
	var gotModelID string
	factory := Factory{
		GenX: peergenx.New(peergenx.Service{}), History: history, Memory: memoryStore,
		modelFactory: func(_ context.Context, _ *peergenx.Service, modelID string) (model.ToolCallingChatModel, error) {
			gotModelID = modelID
			return staticChatModel{}, nil
		},
	}
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverEino, Eino: &apitypes.EinoWorkflowSpec{
			Model: " chat ", SystemPrompt: &prompt, MaxSteps: &maxSteps, MaxToolCalls: &maxCalls,
		}}},
	}
	config, err := factory.config(t.Context(), spec)
	if err != nil {
		t.Fatalf("config() error = %v", err)
	}
	if gotModelID != "chat" || config.Model == nil || config.Toolkit == nil || config.Memory != memoryStore || config.SystemPrompt != prompt || config.MaxSteps != maxSteps || config.MaxToolCalls != maxCalls {
		t.Fatalf("config = %#v, model id = %q", config, gotModelID)
	}
	if config.ToolTimeout != defaultToolTimeout {
		t.Fatalf("tool timeout = %v, want %v", config.ToolTimeout, defaultToolTimeout)
	}
	if config.History == nil || config.History.Store != history || config.History.Stream != "agent.eino.demo" || config.History.RecentLimit != 100 {
		t.Fatalf("history config = %#v", config.History)
	}
	agent, err := factory.NewAgent(t.Context(), spec)
	if err != nil || agent == nil {
		t.Fatalf("NewAgent() = %T, %v", agent, err)
	}
	status, err := agent.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.HistoryAvailable == nil || !*status.HistoryAvailable || status.MemoryStatsAvailable == nil || !*status.MemoryStatsAvailable || status.RecallAvailable == nil || !*status.RecallAvailable {
		t.Fatalf("status = %+v", status)
	}
	historyResponse, err := agent.ListHistory(t.Context(), apitypes.PeerRunHistoryListRequest{})
	if err != nil || !historyResponse.Available {
		t.Fatalf("ListHistory() = %+v, %v", historyResponse, err)
	}
	memoryResponse, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil || !memoryResponse.Available || !memoryResponse.Enabled {
		t.Fatalf("MemoryStats() = %+v, %v", memoryResponse, err)
	}
	recallResponse, err := agent.Recall(t.Context(), apitypes.PeerRunRecallRequest{Query: "tea"})
	if err != nil || !recallResponse.Available {
		t.Fatalf("Recall() = %+v, %v", recallResponse, err)
	}
}

func TestFactoryValidation(t *testing.T) {
	if _, err := (Factory{}).config(t.Context(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "peergenx service is required") {
		t.Fatalf("nil GenX error = %v", err)
	}
	factory := Factory{GenX: peergenx.New(peergenx.Service{}), modelFactory: func(context.Context, *peergenx.Service, string) (model.ToolCallingChatModel, error) {
		return staticChatModel{}, nil
	}}
	if _, err := factory.config(t.Context(), agenthost.Spec{}); err == nil || !strings.Contains(err.Error(), "spec is required") {
		t.Fatalf("missing spec error = %v", err)
	}
	if _, err := factory.config(t.Context(), agenthost.Spec{Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{Eino: &apitypes.EinoWorkflowSpec{}}}}); err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("missing model error = %v", err)
	}
	factory.History = &noopLogStore{}
	if _, err := factory.config(t.Context(), agenthost.Spec{Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{Eino: &apitypes.EinoWorkflowSpec{Model: "chat"}}}}); err == nil || !strings.Contains(err.Error(), "workspace name is required") {
		t.Fatalf("missing workspace error = %v", err)
	}
}

type staticChatModel struct{}

func (staticChatModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return staticChatModel{}, nil
}
func (staticChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("ok", nil), nil
}
func (staticChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("ok", nil)}), nil
}

type noopLogStore struct{}

func (*noopLogStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}
func (*noopLogStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (*noopLogStore) Replace(context.Context, logstore.Record) error   { return nil }
func (*noopLogStore) Delete(context.Context, logstore.RecordKey) error { return nil }
func (*noopLogStore) Close() error                                     { return nil }

type noopMemoryStore struct{}

func (*noopMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}
func (*noopMemoryStore) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}
func (*noopMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}
func (*noopMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }
