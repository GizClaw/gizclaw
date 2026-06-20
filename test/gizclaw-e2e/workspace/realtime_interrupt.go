package main

import "context"

func (d *personaDriver) runRealtimeInterrupt(ctx context.Context) ([]interruptStats, error) {
	return d.runInterruptRounds(ctx, conversationMode{Realtime: true})
}
