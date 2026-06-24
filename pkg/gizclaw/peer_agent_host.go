package gizclaw

import (
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow/agents/chatroom"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/runtime/agenthost"
)

func newPeerAgentHost(base *agenthost.Host, peerGenX *peergenx.Service) *agenthost.Host {
	if base == nil {
		return nil
	}
	host := agenthost.New(base.Resolver)
	host.Coordinator = base.Coordinator
	host.RuntimeRegistry = base.WorkspaceRuntimes()

	var transformer genx.Transformer
	if peerGenX != nil {
		transformer = peerGenX.Transformer()
	}
	_ = host.Register(asttranslate.Type, asttranslate.Factory{Transformer: transformer})
	_ = host.Register(chatroom.Type, chatroom.Factory{Transformer: transformer})
	_ = host.Register(doubaorealtime.Type, doubaorealtime.Factory{Transformer: transformer})
	_ = host.Register(flowcraft.Type, flowcraft.Factory{GenX: peerGenX})
	return host
}
