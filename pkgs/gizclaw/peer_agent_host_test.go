package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type peerAgentHostTestResolver struct{}

func (peerAgentHostTestResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return agenthost.Spec{}, nil
}

type peerAgentHostHistoryStore struct{}

func (*peerAgentHostHistoryStore) Append(_ context.Context, records []logstore.Record) ([]logstore.RecordKey, error) {
	keys := make([]logstore.RecordKey, len(records))
	for index, record := range records {
		keys[index] = record.Key()
	}
	return keys, nil
}
func (*peerAgentHostHistoryStore) Query(context.Context, logstore.Query) (logstore.Page, error) {
	return logstore.Page{}, nil
}
func (*peerAgentHostHistoryStore) Replace(context.Context, logstore.Record) error { return nil }
func (*peerAgentHostHistoryStore) Delete(context.Context, logstore.RecordKey) error {
	return nil
}
func (*peerAgentHostHistoryStore) Close() error { return nil }

type peerAgentHostMemoryStore struct{}

func (*peerAgentHostMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}
func (*peerAgentHostMemoryStore) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}
func (*peerAgentHostMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}
func (*peerAgentHostMemoryStore) Delete(context.Context, memory.DeleteRequest) error { return nil }

func TestNewPeerAgentHostRegistersBuiltInAgents(t *testing.T) {
	base := agenthost.New(peerAgentHostTestResolver{})
	petConfig := petagent.Config{GenerateModel: "chat", ExtractModel: "extract", ASRModel: "asr"}
	history := &peerAgentHostHistoryStore{}
	memoryStore := &peerAgentHostMemoryStore{}
	got := newPeerAgentHost(base, nil, nil, petConfig, history, memoryStore)
	if got == nil {
		t.Fatal("newPeerAgentHost() = nil")
	}
	if got.Resolver != base.Resolver {
		t.Fatal("newPeerAgentHost() did not preserve resolver")
	}
	if got.Coordinator != base.Coordinator {
		t.Fatal("newPeerAgentHost() did not preserve coordinator")
	}
	if got.WorkspaceRuntimes() != base.WorkspaceRuntimes() {
		t.Fatal("newPeerAgentHost() did not preserve workspace runtime registry")
	}
	for _, agentType := range []string{dashscoperealtime.Type, doubaorealtime.Type, eino.Type, flowcraft.Type, petagent.Type} {
		t.Run(agentType, func(t *testing.T) {
			if _, ok := got.Registry.Get(agentType); !ok {
				t.Fatalf("agent type %q was not registered", agentType)
			}
		})
	}
	for _, transformerType := range []string{asttranslate.Type, chatroom.Type} {
		t.Run(transformerType, func(t *testing.T) {
			if _, ok := got.Registry.Get(transformerType); ok {
				t.Fatalf("ordinary transformer %q was registered as an agent", transformerType)
			}
			if _, ok := got.Transformers.Get(transformerType); !ok {
				t.Fatalf("transformer type %q was not registered", transformerType)
			}
		})
	}
	registered, ok := got.Registry.Get(petagent.Type)
	if !ok {
		t.Fatal("pet agent was not registered")
	}
	petFactory, ok := registered.(petagent.Factory)
	if !ok {
		t.Fatalf("pet factory = %T, want pet.Factory", registered)
	}
	if petFactory.Config != petConfig {
		t.Fatalf("pet factory config = %#v, want %#v", petFactory.Config, petConfig)
	}
	if petFactory.History != history {
		t.Fatal("pet factory did not receive Flowcraft history store")
	}
	registered, ok = got.Registry.Get(flowcraft.Type)
	if !ok {
		t.Fatal("flowcraft agent was not registered")
	}
	flowcraftFactory, ok := registered.(flowcraft.Factory)
	if !ok {
		t.Fatalf("flowcraft factory = %T, want flowcraft.Factory", registered)
	}
	if flowcraftFactory.History != history {
		t.Fatal("flowcraft factory did not receive history store")
	}
	if flowcraftFactory.Memory != memoryStore {
		t.Fatal("flowcraft factory did not receive memory store")
	}
	registered, ok = got.Registry.Get(eino.Type)
	if !ok {
		t.Fatal("eino agent was not registered")
	}
	einoFactory, ok := registered.(eino.Factory)
	if !ok || einoFactory.History != history || einoFactory.Memory != memoryStore {
		t.Fatalf("eino factory = %#v, want injected history and memory", einoFactory)
	}
	registered, ok = got.Registry.Get(petagent.Type)
	petFactory = registered.(petagent.Factory)
	if petFactory.Memory != memoryStore {
		t.Fatal("pet factory did not receive memory store")
	}
}

func TestNewPeerAgentHostNilBase(t *testing.T) {
	if got := newPeerAgentHost(nil, nil, nil, petagent.Config{}, nil, nil); got != nil {
		t.Fatalf("newPeerAgentHost(nil) = %#v, want nil", got)
	}
}
