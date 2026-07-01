//go:build gizclaw_e2e

package chat

import "context"

func (d *personaDriver) runPushToTalkInterrupt(ctx context.Context) ([]interruptStats, error) {
	return d.runInterruptRounds(ctx, conversationMode{
		LightweightInterrupt: true,
	})
}
