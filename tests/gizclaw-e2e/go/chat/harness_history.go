//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type historyReplayStats struct {
	Text            string
	AudioASR        string
	DownlinkPackets int
}

type historyReplayVerifyOptions struct {
	audioASRMinRatio        float64
	SkipTextSimilarity      bool
	SkipAssistantAudioASR   bool
	AssistantAudioASRReason string
}

func defaultHistoryReplayVerifyOptions() historyReplayVerifyOptions {
	return historyReplayVerifyOptions{audioASRMinRatio: assistantAudioASRMinRatio}
}

func (d *personaDriver) verifyHistoryReplay(ctx context.Context, item rpcapi.PeerRunHistoryEntry) (historyReplayStats, error) {
	return d.verifyHistoryReplayWithOptions(ctx, item, defaultHistoryReplayVerifyOptions())
}

func (d *personaDriver) verifyHistoryReplayWithOptions(ctx context.Context, item rpcapi.PeerRunHistoryEntry, options historyReplayVerifyOptions) (historyReplayStats, error) {
	var stats historyReplayStats
	if d == nil || d.transport == nil {
		return stats, nil
	}
	if options.audioASRMinRatio <= 0 {
		options.audioASRMinRatio = assistantAudioASRMinRatio
	}
	expected := strings.TrimSpace(item.Text)
	if expected == "" {
		return stats, fmt.Errorf("history item %q has no display text", item.Id)
	}
	expectedLabel := "assistant"
	isTextDone := isAssistantTextDoneEvent
	if item.Type == rpcapi.PeerRunHistoryEntryTypeGear {
		expectedLabel = "transcript"
		isTextDone = isTranscriptDoneEvent
	}
	var text strings.Builder
	var frames [][]byte
	textDone := false
	audioDone := false
	streamID := ""
	var trace roundEventTrace
	start := time.Now()
	responseTimeout := d.historyReplayResponseTimeout()
	deadline := time.NewTimer(responseTimeout)
	defer deadline.Stop()
	for !textDone || !audioDone || stats.DownlinkPackets == 0 {
		select {
		case <-ctx.Done():
			return stats, fmt.Errorf("wait replay: %w; recent events: %s", ctx.Err(), trace.String())
		case <-deadline.C:
			return stats, fmt.Errorf("replay timeout after %s; recent events: %s", responseTimeout, trace.String())
		case err := <-d.transport.errs:
			return stats, fmt.Errorf("transport: %w; recent events: %s", err, trace.String())
		case received := <-d.transport.events:
			event := received.event
			label := eventLabel(event)
			if label != expectedLabel {
				trace.add("skip event label=%s type=%s stream=%s text=%q error=%s", label, event.Type, eventStreamID(event), eventText(event), eventError(event))
				continue
			}
			if !acceptHistoryReplayStream(event, &streamID) {
				trace.add("skip event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), label, event.Type, eventText(event), eventError(event))
				continue
			}
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), label, event.Type, eventText(event), eventError(event))
			if msg, ok := peerEventError(event); ok {
				return stats, fmt.Errorf("peer event error: %s; recent events: %s", msg, trace.String())
			}
			if event.Type == peerStreamEventTypeEos {
				audioDone = true
				fmt.Printf("workspace_progress event=history_replay_audio_done workspace=%s stream=%s after_play=%s packets=%d bytes=%d\n", d.cfg.Workspace, eventStreamID(event), time.Since(start).Truncate(time.Millisecond), stats.DownlinkPackets, totalFrameBytes(frames))
				continue
			}
			if isTextDone(event) {
				textDone = true
				fmt.Printf("workspace_progress event=history_replay_text_done workspace=%s stream=%s after_play=%s chars=%d\n", d.cfg.Workspace, eventStreamID(event), time.Since(start).Truncate(time.Millisecond), runeCount(text.String()))
				continue
			}
			if event.Text != nil && strings.TrimSpace(*event.Text) != "" {
				text.WriteString(*event.Text)
			}
		case packet := <-d.transport.opusPackets:
			frames = append(frames, append([]byte(nil), packet.frame...))
			stats.DownlinkPackets++
		}
	}
	stats.Text = strings.TrimSpace(text.String())
	if !options.SkipTextSimilarity {
		if err := assertTextSimilar("history replay text", expected, stats.Text, 0.35); err != nil {
			return stats, err
		}
	}
	if options.SkipAssistantAudioASR {
		reason := strings.TrimSpace(options.AssistantAudioASRReason)
		if reason == "" {
			reason = "lightweight-history-replay"
		}
		stats.AudioASR = "skipped: " + reason
		fmt.Printf("workspace_progress event=history_replay_audio_asr_skipped workspace=%s reason=%s\n", d.cfg.Workspace, reason)
		return stats, nil
	}
	if skipReason := d.assistantAudioASRSkipReason(conversationMode{}); skipReason == "" {
		audioASR, err := d.verifyAssistantAudioASRWithMinRatio(ctx, 0, "history-replay", expected, frames, options.audioASRMinRatio)
		if err != nil {
			return stats, fmt.Errorf("history replay audio asr: %w", err)
		}
		stats.AudioASR = audioASR
	} else {
		stats.AudioASR = "skipped: " + skipReason
		fmt.Printf("workspace_progress event=history_replay_audio_asr_skipped workspace=%s reason=%s\n", d.cfg.Workspace, skipReason)
	}
	return stats, nil
}

func (d *personaDriver) historyReplayResponseTimeout() time.Duration {
	timeout := d.roundResponseTimeout()
	if timeout > historyReplayResponseTimeout {
		return historyReplayResponseTimeout
	}
	return timeout
}

const historyReplayResponseTimeout = 60 * time.Second

func (d *personaDriver) drainTransport() {
	if d == nil || d.transport == nil {
		return
	}
	for {
		select {
		case <-d.transport.events:
		case <-d.transport.opusPackets:
		case <-d.transport.errs:
		default:
			return
		}
	}
}

func acceptHistoryReplayStream(event peerStreamEvent, boundStreamID *string) bool {
	if event.StreamId == nil || strings.TrimSpace(*event.StreamId) == "" {
		return true
	}
	actual := strings.TrimSpace(*event.StreamId)
	if strings.HasPrefix(actual, "history-replay-") {
		if boundStreamID != nil && strings.TrimSpace(*boundStreamID) == "" {
			*boundStreamID = actual
		}
		return true
	}
	return boundStreamID != nil && streamIDMatches(actual, *boundStreamID)
}

func totalFrameBytes(frames [][]byte) int {
	var total int
	for _, frame := range frames {
		total += len(frame)
	}
	return total
}
