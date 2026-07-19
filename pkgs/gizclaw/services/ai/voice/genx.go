package voice

import (
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

// NewTransformer returns a server-owned GenX transformer for voice/<id> and supported model patterns.
func NewTransformer(service peergenx.Service) genx.TransformerMux {
	return peergenx.New(service).Transformer()
}
