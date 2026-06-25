//go:build gizclaw_e2e

package chat

import "context"

func (d *personaDriver) runRealtimeRoundtrip(ctx context.Context) ([]roundStats, error) {
	d.useRoundtripUtterances()
	return d.runConversation(ctx, conversationMode{Realtime: true})
}
