package voice

import (
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/peergenx"
)

// NewTransformer returns a server-owned GenX transformer for voice/<id> and supported model patterns.
func NewTransformer(service peergenx.Service) genx.Transformer {
	return peergenx.New(service).Transformer()
}
