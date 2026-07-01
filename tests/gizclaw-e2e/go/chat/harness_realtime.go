//go:build gizclaw_e2e

package chat

import "context"

func (d *personaDriver) runRealtimeRoundtrip(ctx context.Context) ([]roundStats, error) {
	return d.runRealtimeRoundtripWithMode(ctx, conversationMode{})
}

func (d *personaDriver) runRealtimeRoundtripWithMode(ctx context.Context, mode conversationMode) ([]roundStats, error) {
	d.useRoundtripUtterances()
	mode.Realtime = true
	return d.runConversation(ctx, mode)
}
