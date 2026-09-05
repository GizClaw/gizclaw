package observability

import (
	"context"
	"log/slog"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

const CompletionMessage = "gizclaw: request completed"

// Log emits one scalar structured completion record through the global logger.
func Log(ctx context.Context, outcome *Outcome) {
	if outcome == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	level, attrs := outcome.logRecord()
	// The logger reserves identity attributes and accepts them only through
	// context. Carry the authenticated outcome identity into that channel.
	for _, attr := range attrs {
		if attr.Key == "peer_public_key" {
			ctx = gizlog.WithPeerPublicKey(ctx, attr.Value.String())
			break
		}
	}
	slog.LogAttrs(context.WithoutCancel(ctx), level, CompletionMessage, attrs...)
}
