package gizclaw

import (
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow/agents/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow/agents/doubaorealtime"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow/agents/flowcraft"
)

func newPeerAgentHost(base *agenthost.Host, peerGenX *peergenx.Service) *agenthost.Host {
	if base == nil {
		return nil
	}
	host := agenthost.New(base.Resolver)
	host.Coordinator = base.Coordinator
	host.WorkspaceStore = base.WorkspaceStore

	var transformer genx.Transformer
	if peerGenX != nil {
		transformer = peerGenX.Transformer()
	}
	_ = host.Register(asttranslate.Type, asttranslate.Factory{Transformer: transformer})
	_ = host.Register(doubaorealtime.Type, doubaorealtime.Factory{Transformer: transformer})
	_ = host.Register(flowcraft.Type, flowcraft.Factory{GenX: peerGenX})
	return host
}
