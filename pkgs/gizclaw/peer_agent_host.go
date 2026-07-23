package gizclaw

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	petagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/pet"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func newPeerAgentHost(base *agenthost.Host, peerGenX *peergenx.Service, ownerGenX func(context.Context, string) (*peergenx.Service, error), pets petagent.ContextProvider, history logstore.MutableStore, state kv.Store, memoryObjects objectstore.ObjectStore) *agenthost.Host {
	if base == nil {
		return nil
	}
	host := agenthost.New(base.Resolver)
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
	_ = host.Register(chatroom.Type, chatroom.Factory{Transformer: transformer, TransformerForOwner: transformerForOwner})
	_ = host.Register(doubaorealtime.Type, doubaorealtime.Factory{Transformer: transformer, TransformerForOwner: transformerForOwner})
	_ = host.Register(flowcraft.Type, flowcraft.Factory{GenX: peerGenX, GenXForOwner: ownerGenX, History: history, State: state, MemoryObjects: memoryObjects})
	_ = host.Register(petagent.Type, petagent.Factory{Pets: pets, Factories: host.Registry})
	return host
}
