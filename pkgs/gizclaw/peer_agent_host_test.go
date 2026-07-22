package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
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

func TestNewPeerAgentHostRegistersBuiltInAgents(t *testing.T) {
	base := agenthost.New(peerAgentHostTestResolver{})
	petConfig := petagent.Config{GenerateModel: "chat", ExtractModel: "extract", ASRModel: "asr"}
	history := &peerAgentHostHistoryStore{}
	got := newPeerAgentHost(base, nil, nil, nil, petConfig, history, nil, nil)
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
	for _, agentType := range []string{asttranslate.Type, chatroom.Type, doubaorealtime.Type, flowcraft.Type, petagent.Type} {
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
}

func TestNewPeerAgentHostNilBase(t *testing.T) {
	if got := newPeerAgentHost(nil, nil, nil, nil, petagent.Config{}, nil, nil, nil); got != nil {
		t.Fatalf("newPeerAgentHost(nil) = %#v, want nil", got)
	}
}
