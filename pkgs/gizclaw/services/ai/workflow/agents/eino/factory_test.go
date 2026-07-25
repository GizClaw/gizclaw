package eino

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestFactoryAllowsMemorylessWorkflowAndRequiresConfiguredStore(t *testing.T) {
	t.Parallel()
	spec := einoFactorySpec(t)
	factory := Factory{GenX: &peergenx.Service{}}
	if _, err := factory.NewAgent(t.Context(), spec); err != nil {
		t.Fatalf("NewAgent(memoryless) error = %v", err)
	}

	spec.Workflow.Spec.Eino.Memory = &apitypes.EinoMemory{}
	if _, err := factory.NewAgent(t.Context(), spec); err == nil ||
		!strings.Contains(err.Error(), "agent_host.eino.memory_store") {
		t.Fatalf("NewAgent(memory without store) error = %v", err)
	}
}

func TestFactoryBindsOnlyWorkspaceAppAndReportsConfiguredBackend(t *testing.T) {
	t.Parallel()
	store := &einoMemoryStore{}
	spec := einoFactorySpec(t)
	owner := "owner-public-key"
	spec.Workspace.OwnerPublicKey = &owner
	spec.Workflow.Spec.Eino.Memory = &apitypes.EinoMemory{}
	service := &peergenx.Service{}
	agent, err := (Factory{
		GenX: service,
		GenXForOwner: func(context.Context, string) (*peergenx.Service, error) {
			return service, nil
		},
		Memory:     store,
		MemoryKind: "volc_memory",
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
	if scope.AppID != spec.Workspace.Name || scope.AgentID != spec.Workflow.Name {
		t.Fatalf("Recall scope = %#v, want Workspace AppID and Workflow AgentID", scope)
	}
	if scope.UserID != "" || scope.RunID != "" {
		t.Fatalf("Recall scope rewrote inner dimensions: %#v", scope)
	}
	stats, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if stats.Backend == nil || *stats.Backend != "volc_memory" {
		t.Fatalf("MemoryStats backend = %v", stats.Backend)
	}
	if stats.Metadata == nil {
		t.Fatal("MemoryStats metadata is nil")
	}
	metadataScope, ok := (*stats.Metadata)["scope"].(map[string]any)
	if !ok || metadataScope["app_id"] != spec.Workspace.Name ||
		metadataScope["agent_id"] != spec.Workflow.Name {
		t.Fatalf("MemoryStats scope = %#v", (*stats.Metadata)["scope"])
	}
}

func TestFactoryRejectsUnsupportedMemoryWaitCapabilityDuringConstruction(t *testing.T) {
	t.Parallel()
	spec := einoFactorySpec(t)
	wait := true
	spec.Workflow.Spec.Eino.Memory = &apitypes.EinoMemory{
		Observe: &apitypes.EinoMemoryObserve{
			Enabled:           true,
			WaitForCompletion: &wait,
		},
	}
	_, err := (Factory{
		GenX:   &peergenx.Service{},
		Memory: &einoMemoryStore{},
	}).NewAgent(t.Context(), spec)
	if err == nil || !strings.Contains(err.Error(), "OperationWaiter") {
		t.Fatalf("NewAgent(wait without capability) error = %v", err)
	}
}

func TestFactoryRejectsFactsUnsupportedByConfiguredMemory(t *testing.T) {
	t.Parallel()
	for _, backend := range []string{"mem0", "volc_memory", ""} {
		t.Run(backend, func(t *testing.T) {
			spec := einoFactorySpec(t)
			spec.Workflow.Spec.Eino.Memory = &apitypes.EinoMemory{
				Observe: &apitypes.EinoMemoryObserve{
					Enabled: true,
					Facts: &[]apitypes.EinoMemoryFact{{
						TextFrom: "input.text",
					}},
				},
			}
			_, err := (Factory{
				GenX:       &peergenx.Service{},
				Memory:     &einoMemoryStore{},
				MemoryKind: backend,
			}).NewAgent(t.Context(), spec)
			if err == nil || !strings.Contains(err.Error(), "does not support observe.facts") {
				t.Fatalf("NewAgent(%q facts) error = %v", backend, err)
			}
		})
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
		Workspace: apitypes.Workspace{Name: "workspace-a"},
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
