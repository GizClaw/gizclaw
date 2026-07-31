//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
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

func (d *personaDriver) runFlowcraftRealtimeChatRoundtrip(ctx context.Context) ([]roundStats, error) {
	stats, err := d.runRealtimeRoundtripWithMode(ctx, conversationMode{
		KeepRealtimeInputOpen: true,
		RealtimeTailSilence:   4 * time.Second,
	})
	if err != nil {
		return stats, err
	}
	for _, stat := range stats {
		if !stat.FirstTranscriptBeforeEOS {
			return stats, fmt.Errorf("round %d: transcript started only after client audio EOS", stat.Index)
		}
		if stat.InputEOSSent {
			return stats, fmt.Errorf("round %d: client audio EOS was sent", stat.Index)
		}
	}
	return stats, nil
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
