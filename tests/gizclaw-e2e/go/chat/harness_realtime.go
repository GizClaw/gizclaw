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
	mode = configureRealtimeConversationMode(d.cfg.Agent, mode)
	return d.runConversation(ctx, mode)
}

func configureRealtimeConversationMode(agent string, mode conversationMode) conversationMode {
	mode.Realtime = true
	if agent == "dashscope-realtime" {
		// Keep the peer transport as raw Opus. The transformer decodes it to the
		// provider input format declared by the workflow.
		mode.InputAudioMIME = "audio/opus"
		mode.AllowSplitAssistantStreams = true
		mode.AllowMissingInputTranscript = true
	}
	if agent == "doubao-realtime-duplex" {
		mode.AllowSplitAssistantStreams = true
		mode.RealtimeTailSilence = 2100 * time.Millisecond
		mode.ReencodeRealtimeTailSilence = true
		mode.KeepRealtimeInputOpen = true
	}
	return mode
}
