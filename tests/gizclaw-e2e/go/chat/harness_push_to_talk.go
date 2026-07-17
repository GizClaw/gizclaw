//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (d *personaDriver) runPushToTalkRoundtrip(ctx context.Context) ([]roundStats, error) {
	return d.runPushToTalkRoundtripWithMode(ctx, conversationMode{})
}

func (d *personaDriver) runPushToTalkRoundtripWithMode(ctx context.Context, mode conversationMode) ([]roundStats, error) {
	d.useRoundtripUtterances()
	return d.runConversation(ctx, mode)
}

func (d *personaDriver) useRoundtripUtterances() {
	if d.generateUtterance != nil {
		return
	}
	if len(d.cfg.Utterances) > 0 {
		utterances := append([]string(nil), d.cfg.Utterances...)
		d.generateUtterance = func(_ context.Context, index int) (string, error) {
			if index <= 0 {
				index = 1
			}
			return utterances[(index-1)%len(utterances)], nil
		}
		return
	}
	d.generateUtterance = func(_ context.Context, index int) (string, error) {
		return roundtripUtterance(index), nil
	}
}

func roundtripUtterance(index int) string {
	utterances := []string{
		"请问你能听清楚我说话吗",
		"请用一句话回答你现在状态好吗",
		"好的我们继续下一轮测试",
	}
	if index <= 0 {
		index = 1
	}
	return utterances[(index-1)%len(utterances)]
}

func (d *personaDriver) runConversation(ctx context.Context, mode conversationMode) ([]roundStats, error) {
	stats, err := d.prepareConversation(ctx, mode)
	if err != nil {
		return nil, err
	}
	for i := 1; i <= d.cfg.Rounds; i++ {
		stat, err := d.runRound(ctx, i, mode)
		stats = append(stats, stat)
		if err != nil {
			return stats, err
		}
		if err := d.waitFlowcraftHistoryProgress(ctx, fmt.Sprintf("round %d", i)); err != nil {
			return stats, err
		}
		d.history = append(d.history, roundHistory{
			UserText:      stat.UserText,
			InputASR:      stat.InputASR,
			Transcript:    stat.Transcript,
			AssistantText: stat.AssistantText,
		})
		if i < d.cfg.Rounds && d.newTransport != nil {
			if err := d.resetTransport(); err != nil {
				return stats, fmt.Errorf("reopen transport after round %d: %w", i, err)
			}
		}
	}
	return stats, nil
}

func (d *personaDriver) prepareConversation(ctx context.Context, mode conversationMode) ([]roundStats, error) {
	if d.reloadAgent != nil {
		if err := d.reloadAgent(ctx); err != nil {
			if !isAgentAlreadyRunning(err) {
				return nil, fmt.Errorf("reload workspace: %w", err)
			}
		}
	}
	if d.transport == nil {
		if err := d.resetTransport(); err != nil {
			return nil, fmt.Errorf("open transport: %w", err)
		}
	}
	if d.cfg.flowcraftStartsSelf() {
		stat, ok, err := d.consumeSelfStart(ctx, mode.SkipAssistantAudioASR)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("self-start is required by workflow but did not run")
		}
		if d.newTransport != nil {
			if err := d.resetTransport(); err != nil {
				return nil, fmt.Errorf("reopen transport after self-start: %w", err)
			}
		}
		return []roundStats{stat}, nil
	}
	d.transport.drain()
	return nil, nil
}

func (d *personaDriver) consumeSelfStart(ctx context.Context, skipAssistantAudioASR bool) (roundStats, bool, error) {
	stat := roundStats{Index: 0}
	if d.transport == nil {
		return stat, false, nil
	}
	fmt.Printf("workspace_progress event=self_start_wait workspace=%s timeout=%s\n", d.cfg.Workspace, d.cfg.timeout)
	firstWait := 45 * time.Second
	if d.cfg.timeout > 0 && d.cfg.timeout/4 < firstWait {
		firstWait = d.cfg.timeout / 4
	}
	if firstWait <= 0 {
		firstWait = time.Second
	}
	fmt.Printf("workspace_progress event=self_start_deadline workspace=%s duration=%s\n", d.cfg.Workspace, firstWait)
	first := time.NewTimer(firstWait)
	defer first.Stop()
	start := time.Now()
	started := false
	textDone := false
	audioDone := false
	var assistantText strings.Builder
	var frames [][]byte
	var settle <-chan time.Time
	var responseDeadline <-chan time.Time
	responseTimeout := d.roundResponseTimeout()
	var trace roundEventTrace
	for {
		if started && textDone && audioDone && settle == nil {
			stat.AssistantText = cleanUtterance(assistantText.String())
			stat.ResponseTotal = time.Since(start)
			stat.WorkspaceTotal = stat.ResponseTotal
			if stat.AssistantText == "" {
				return stat, true, fmt.Errorf("self-start missing assistant text; recent events: %s", trace.String())
			}
			if err := validateAssistantOutputText(stat.AssistantText); err != nil {
				return stat, true, fmt.Errorf("self-start: %w; recent events: %s", err, trace.String())
			}
			if stat.DownlinkPackets == 0 {
				return stat, true, fmt.Errorf("self-start missing downlink audio; recent events: %s", trace.String())
			}
			if !skipAssistantAudioASR {
				audioASR, err := d.verifyAssistantAudioASR(ctx, 0, "self-start-assistant", stat.AssistantText, frames)
				if err != nil {
					return stat, true, fmt.Errorf("self-start: %w", err)
				}
				stat.AssistantAudioASR = audioASR
			} else {
				stat.AssistantAudioASR = "skipped by human-review"
				fmt.Printf("workspace_progress event=assistant_audio_asr_skipped workspace=%s round=0 reason=human-review\n", d.cfg.Workspace)
			}
			fmt.Printf("workspace_progress event=self_start_done workspace=%s assistant_chars=%d downlink_packets=%d total=%s\n", d.cfg.Workspace, runeCount(stat.AssistantText), stat.DownlinkPackets, stat.WorkspaceTotal.Truncate(time.Millisecond))
			fmt.Printf("self_start workspace=%s assistant_chars=%d downlink_packets=%d assistant=%s audio_asr=%s\n", d.cfg.Workspace, runeCount(stat.AssistantText), stat.DownlinkPackets, stat.AssistantText, stat.AssistantAudioASR)
			d.transport.drain()
			return stat, true, nil
		}
		select {
		case <-ctx.Done():
			return stat, true, fmt.Errorf("consume self-start: %w; recent events: %s", ctx.Err(), trace.String())
		case <-responseDeadline:
			return stat, true, fmt.Errorf("self-start response timeout after %s; recent events: %s", responseTimeout, trace.String())
		case <-settle:
			settle = nil
			if !textDone {
				return stat, true, fmt.Errorf("self-start missing assistant text EOS after audio EOS; recent events: %s", trace.String())
			}
		case <-first.C:
			if !started {
				return stat, false, fmt.Errorf("self-start did not emit output within %s", firstWait)
			}
		case err := <-d.transport.errs:
			return stat, true, fmt.Errorf("consume self-start transport: %w; recent events: %s", err, trace.String())
		case received := <-d.transport.events:
			event := received.event
			if event.StreamId != nil && !streamIDMatches(*event.StreamId, flowcraftSelfStartStreamID) {
				continue
			}
			if !started {
				responseDeadline = time.After(responseTimeout)
			}
			started = true
			stat.EventCount++
			label := eventLabel(event)
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), label, event.Type, eventText(event), eventError(event))
			if msg, ok := peerEventError(event); ok {
				return stat, true, fmt.Errorf("self-start peer event error: %s; recent events: %s", msg, trace.String())
			}
			if event.Type == apitypes.PeerStreamEventTypeEos && label == "assistant" {
				if !textDone {
					trace.add("assistant audio segment eos before text done stream=%s", eventStreamID(event))
					continue
				}
				audioDone = true
				settle = time.After(700 * time.Millisecond)
				fmt.Printf("workspace_progress event=assistant_audio_done workspace=%s round=0 stream=%s after_start=%s packets=%d bytes=%d\n", d.cfg.Workspace, eventStreamID(event), time.Since(start).Truncate(time.Millisecond), stat.DownlinkPackets, stat.DownlinkBytes)
				continue
			}
			if label != "assistant" {
				continue
			}
			if isAssistantTextDoneEvent(event) {
				textDone = true
				if stat.AssistantTextDone == 0 {
					stat.AssistantTextDone = received.since(start)
					fmt.Printf("workspace_progress event=assistant_text_done workspace=%s round=0 stream=%s after_start=%s chars=%d\n", d.cfg.Workspace, eventStreamID(event), stat.AssistantTextDone.Truncate(time.Millisecond), runeCount(assistantText.String()))
				}
			}
			if event.Text != nil && strings.TrimSpace(*event.Text) != "" {
				if stat.FirstAssistantTextChunk == 0 {
					stat.FirstAssistantTextChunk = received.since(start)
					stat.FirstAssistantText = *event.Text
					fmt.Printf("self_start_first_text workspace=%s after_start=%s chunk=%q\n", d.cfg.Workspace, stat.FirstAssistantTextChunk.Truncate(time.Millisecond), stat.FirstAssistantText)
				}
				assistantText.WriteString(*event.Text)
			}
		case packet := <-d.transport.opusPackets:
			if started {
				if stat.FirstAudioChunk == 0 {
					stat.FirstAudioChunk = packet.since(start)
					stat.FirstAudioBeforeTextDone = stat.AssistantTextDone == 0
					fmt.Printf("self_start_first_audio workspace=%s after_start=%s bytes=%d before_text_done=%t\n", d.cfg.Workspace, stat.FirstAudioChunk.Truncate(time.Millisecond), len(packet.frame), stat.FirstAudioBeforeTextDone)
				}
				frames = append(frames, append([]byte(nil), packet.frame...))
				stat.DownlinkPackets++
				stat.DownlinkBytes += len(packet.frame)
			}
		}
	}
}
