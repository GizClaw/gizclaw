package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtimeduplex"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

type peerAgentHostTestResolver struct{}

type peerAgentHostMemoryStore struct{}

func (*peerAgentHostMemoryStore) Recall(context.Context, memory.Query) (memory.RecallResult, error) {
	return memory.RecallResult{}, nil
}

func (*peerAgentHostMemoryStore) Observe(context.Context, memory.Observation) (memory.ObserveResult, error) {
	return memory.ObserveResult{}, nil
}

func (*peerAgentHostMemoryStore) Update(context.Context, memory.UpdateRequest) (memory.Fact, error) {
	return memory.Fact{}, nil
}

func (*peerAgentHostMemoryStore) Delete(context.Context, memory.DeleteRequest) error {
	return nil
}

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

func TestNewPeerAgentHostRegistersBuiltInAgents(t *testing.T) {
	base := agenthost.New(peerAgentHostTestResolver{})
	history := &peerAgentHostHistoryStore{}
	state := kv.NewMemory(nil)
	memoryObjects := objectstore.Dir(t.TempDir())
	memoryStore := &peerAgentHostMemoryStore{}
	got := newPeerAgentHost(base, nil, nil, nil, history, state, memoryObjects, memoryStore, "mem0", memoryStore, "mem0")
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
	for _, agentType := range []string{
		asttranslate.Type,
		chatroom.Type,
		dashscoperealtime.Type,
		doubaorealtime.Type,
		doubaorealtimeduplex.Type,
		eino.Type,
		flowcraft.Type,
		petagent.Type,
	} {
		t.Run(agentType, func(t *testing.T) {
			if _, ok := got.Registry.Get(agentType); !ok {
				t.Fatalf("agent type %q was not registered", agentType)
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
	if petFactory.Factories != got.Registry {
		t.Fatal("pet factory did not receive the shared driver registry")
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
	if flowcraftFactory.State != state {
		t.Fatal("flowcraft factory did not receive state store")
	}
	if flowcraftFactory.MemoryObjects != memoryObjects {
		t.Fatal("flowcraft factory did not receive memory object store")
	}
	if flowcraftFactory.Memory != memoryStore || flowcraftFactory.MemoryKind != "mem0" {
		t.Fatal("flowcraft factory did not receive configured memory store")
	}
	registered, ok = got.Registry.Get(eino.Type)
	if !ok {
		t.Fatal("eino agent was not registered")
	}
	einoFactory, ok := registered.(eino.Factory)
	if !ok {
		t.Fatalf("eino factory = %T, want eino.Factory", registered)
	}
	if einoFactory.Memory != memoryStore || einoFactory.MemoryKind != "mem0" {
		t.Fatal("eino factory did not receive configured memory store")
	}
}

func TestNewPeerAgentHostNilBase(t *testing.T) {
	if got := newPeerAgentHost(nil, nil, nil, nil, nil, nil, nil, nil, "", nil, ""); got != nil {
		t.Fatalf("newPeerAgentHost(nil) = %#v, want nil", got)
	}
}
