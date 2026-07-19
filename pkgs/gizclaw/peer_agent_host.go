package gizclaw

import (
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
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

func newPeerAgentHost(base *agenthost.Host, peerGenX *peergenx.Service, pets petagent.ContextProvider, petConfig petagent.Config, history logstore.MutableStore, memoryStore memory.Store) *agenthost.Host {
	if base == nil {
		return nil
	}
	host := agenthost.New(base.Resolver)
	host.Coordinator = base.Coordinator
	host.RuntimeRegistry = base.WorkspaceRuntimes()

	var transformer genx.Transformer
	var agentTransformer genx.Transformer
	if peerGenX != nil {
		transformer = peerGenX.Transformer()
		agentTransformer = peerGenX.AgentTransformer()
	}
	_ = host.RegisterTransformer(asttranslate.Type, asttranslate.Factory{Transformer: transformer})
	_ = host.RegisterTransformer(chatroom.Type, chatroom.Factory{Transformer: transformer})
	_ = host.Register(doubaorealtime.Type, doubaorealtime.Factory{Transformer: agentTransformer})
	_ = host.Register(dashscoperealtime.Type, dashscoperealtime.Factory{Transformer: agentTransformer})
	_ = host.Register(eino.Type, eino.Factory{GenX: peerGenX, History: history, Memory: memoryStore})
	_ = host.Register(flowcraft.Type, flowcraft.Factory{GenX: peerGenX, History: history, Memory: memoryStore})
	_ = host.Register(petagent.Type, petagent.Factory{GenX: peerGenX, Pets: pets, Config: petConfig, History: history, Memory: memoryStore})
	return host
}
