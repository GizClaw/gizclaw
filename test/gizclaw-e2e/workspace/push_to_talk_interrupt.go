package main

import "context"

func (d *personaDriver) runPushToTalkInterrupt(ctx context.Context) ([]interruptStats, error) {
	return d.runInterruptRounds(ctx, conversationMode{})
}
