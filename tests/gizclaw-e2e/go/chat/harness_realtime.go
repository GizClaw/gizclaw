//go:build gizclaw_e2e

package chat

import (
	"context"
	"time"
)

func (d *personaDriver) runRealtimeRoundtrip(ctx context.Context) ([]roundStats, error) {
	return d.runRealtimeRoundtripWithMode(ctx, conversationMode{})
}

func (d *personaDriver) runRealtimeRoundtripWithMode(ctx context.Context, mode conversationMode) ([]roundStats, error) {
	d.useRoundtripUtterances()
	mode.Realtime = true
	if d.cfg.Agent == "dashscope-realtime" {
		mode.AllowSplitAssistantStreams = true
		mode.AllowMissingInputTranscript = true
	}
	if d.cfg.Agent == "doubao-realtime-duplex" {
		mode.AllowSplitAssistantStreams = true
		mode.RealtimeTailSilence = 2100 * time.Millisecond
		mode.ReencodeRealtimeTailSilence = true
		mode.KeepRealtimeInputOpen = true
	}
	return d.runConversation(ctx, mode)
}
