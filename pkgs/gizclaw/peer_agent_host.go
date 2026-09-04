package gizclaw

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtimeduplex"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

func newPeerAgentHost(
	base *agenthost.Host,
	workspaces peerAgentWorkspaceResolver,
	peerGenX *peergenx.Service,
	ownerGenX func(context.Context, string) (*peergenx.Service, error),
	pets petagent.ContextProvider,
	history logstore.MutableStore,
	state kv.Store,
	memoryRoot string,
	memoryStores *memorystore.Registry,
	sfuFactory sfu.Factory,
) *agenthost.Host {
	if base == nil {
		return nil
	}
	resolver := base.Resolver
	if workspaces != nil {
		resolver = peerAgentResolver{base: resolver, workspaces: workspaces}
	}
	host := agenthost.New(resolver)
	host.Coordinator = base.Coordinator
	host.RuntimeRegistry = base.WorkspaceRuntimes()

	var transformer genx.TransformerMux
	if peerGenX != nil {
		transformer = peerGenX.Transformer()
	}
	transformerForOwner := func(ctx context.Context, owner string) (genx.TransformerMux, error) {
		if ownerGenX == nil {
			return nil, fmt.Errorf("owner GenX resolver is not configured")
		}
		service, err := ownerGenX(ctx, owner)
		if err != nil {
			return nil, err
		}
		if service == nil {
			return nil, fmt.Errorf("owner GenX resolver returned no service")
		}
		return service.Transformer(), nil
	}
	_ = host.Register(asttranslate.Type, asttranslate.Factory{Transformer: transformer, TransformerForOwner: transformerForOwner})
	_ = host.Register(dashscoperealtime.Type, dashscoperealtime.Factory{GenX: peerGenX, GenXForOwner: ownerGenX})
	_ = host.Register(doubaorealtime.Type, doubaorealtime.Factory{Transformer: transformer, TransformerForOwner: transformerForOwner})
	_ = host.Register(doubaorealtimeduplex.Type, doubaorealtimeduplex.Factory{GenX: peerGenX, GenXForOwner: ownerGenX})
	_ = host.Register(eino.Type, eino.Factory{
		GenX:         peerGenX,
		GenXForOwner: ownerGenX,
		History:      history,
		ServerRoot:   memoryRoot,
		MemoryStores: memoryStores,
	})
	_ = host.Register(flowcraft.Type, flowcraft.Factory{
		GenX:         peerGenX,
		GenXForOwner: ownerGenX,
		History:      history,
		State:        state,
		ServerRoot:   memoryRoot,
		MemoryStores: memoryStores,
	})
	_ = host.Register(petagent.Type, petagent.Factory{Pets: pets, Factories: host.Registry})
	_ = host.Register(sfu.Type, sfuFactory)
	return host
}

type peerAgentWorkspaceResolver interface {
	ResolveRunWorkspaceSelection(context.Context, string) (apitypes.Workspace, *rpcapi.RPCStatus)
}

type canonicalAgentResolver interface {
	ResolveByID(context.Context, string) (agenthost.Spec, error)
}

type peerAgentResolver struct {
	base       agenthost.Resolver
	workspaces peerAgentWorkspaceResolver
}

func (r peerAgentResolver) Resolve(ctx context.Context, pattern string) (agenthost.Spec, error) {
	name, err := agenthost.ParseWorkspacePattern(pattern)
	if err != nil {
		return agenthost.Spec{}, err
	}
	workspace, rpcErr := r.workspaces.ResolveRunWorkspaceSelection(ctx, name)
	if rpcErr != nil {
		return agenthost.Spec{}, errors.New(rpcErr.Message)
	}
	resolver, ok := r.base.(canonicalAgentResolver)
	if !ok {
		return agenthost.Spec{}, errors.New("agenthost: canonical Workspace resolver is required")
	}
	return resolver.ResolveByID(ctx, workspace.Id)
}
