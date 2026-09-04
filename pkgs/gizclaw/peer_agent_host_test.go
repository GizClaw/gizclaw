package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtimeduplex"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type peerAgentHostTestResolver struct{}

func (peerAgentHostTestResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return agenthost.Spec{}, nil
}

type peerAgentCanonicalTestResolver struct {
	resolvedID string
}

func (*peerAgentCanonicalTestResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return agenthost.Spec{}, nil
}

func (r *peerAgentCanonicalTestResolver) ResolveByID(_ context.Context, id string) (agenthost.Spec, error) {
	r.resolvedID = id
	return agenthost.Spec{Workspace: apitypes.Workspace{Id: id}}, nil
}

type peerAgentWorkspaceTestResolver struct {
	resolvedName string
}

func (r *peerAgentWorkspaceTestResolver) ResolveRunWorkspaceSelection(_ context.Context, name string) (apitypes.Workspace, *rpcapi.RPCStatus) {
	r.resolvedName = name
	return apitypes.Workspace{Id: "01K1HZZZ9PV2KYRHZJ4V94Z0DQ", Name: name}, nil
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
	got := newPeerAgentHost(base, nil, nil, nil, nil, history, state, t.TempDir(), nil, sfu.Factory{})
	if got == nil {
		t.Fatal("newPeerAgentHost() = nil", sfu.Factory{})
	}
	if got.Resolver != base.Resolver {
		t.Fatal("newPeerAgentHost() did not preserve resolver", sfu.Factory{})
	}
	if got.Coordinator != base.Coordinator {
		t.Fatal("newPeerAgentHost() did not preserve coordinator", sfu.Factory{})
	}
	if got.WorkspaceRuntimes() != base.WorkspaceRuntimes() {
		t.Fatal("newPeerAgentHost() did not preserve workspace runtime registry", sfu.Factory{})
	}
	for _, agentType := range []string{
		asttranslate.Type,
		dashscoperealtime.Type,
		doubaorealtime.Type,
		doubaorealtimeduplex.Type,
		eino.Type,
		flowcraft.Type,
		petagent.Type,
		sfu.Type,
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
	registered, ok = got.Registry.Get(eino.Type)
	if !ok {
		t.Fatal("eino agent was not registered")
	}
	einoFactory, ok := registered.(eino.Factory)
	if !ok {
		t.Fatalf("eino factory = %T, want eino.Factory", registered)
	}
	if einoFactory.History != history {
		t.Fatal("eino factory did not receive history store")
	}
}

func TestNewPeerAgentHostNilBase(t *testing.T) {
	if got := newPeerAgentHost(nil, nil, nil, nil, nil, nil, nil, "", nil, sfu.Factory{}); got != nil {
		t.Fatalf("newPeerAgentHost(nil) = %#v, want nil", got)
	}
}

func TestPeerAgentResolverResolvesPeerNameToCanonicalWorkspaceID(t *testing.T) {
	base := &peerAgentCanonicalTestResolver{}
	workspaces := &peerAgentWorkspaceTestResolver{}
	resolver := peerAgentResolver{base: base, workspaces: workspaces}

	spec, err := resolver.Resolve(t.Context(), "/workspaces/shared-room")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if workspaces.resolvedName != "shared-room" {
		t.Fatalf("Peer Workspace selection = %q, want shared-room", workspaces.resolvedName)
	}
	if base.resolvedID != "01K1HZZZ9PV2KYRHZJ4V94Z0DQ" {
		t.Fatalf("canonical Workspace resolution = %q", base.resolvedID)
	}
	if spec.Workspace.Id != base.resolvedID {
		t.Fatalf("resolved Workspace ID = %q, want %q", spec.Workspace.Id, base.resolvedID)
	}
}
