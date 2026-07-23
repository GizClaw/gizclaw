//go:build gizclaw_e2e

package chat

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const (
	realtimeAutoSplitHistoryWait    = 180 * time.Second
	realtimeAutoSplitSilence        = 4000 * time.Millisecond
	realtimeAutoSplitSampleRate     = 16000
	realtimeAutoSplitChannels       = 1
	realtimeAutoSplitHistoryMinText = 0.35
)

func (d *personaDriver) runRealtimeAutoSplitHistory(ctx context.Context) error {
	if d == nil {
		return fmt.Errorf("persona driver is nil")
	}
	if d.runtimeClient == nil {
		return fmt.Errorf("realtime auto split history requires runtime client")
	}
	if _, err := d.prepareConversation(ctx, conversationMode{Realtime: true, SkipAssistantAudioASR: true}); err != nil {
		return err
	}
	if d.transport == nil {
		return fmt.Errorf("realtime auto split history requires active transport")
	}

	expected := []string{
		"第一段自动切分测试",
		"第二段自动切分测试",
		"第三段自动切分测试",
	}
	beforeItems, err := d.listRealtimeAutoSplitHistory(ctx)
	if err != nil {
		return err
	}
	before := historyIDSet(beforeItems)
	packets, err := d.realtimeAutoSplitPackets(ctx, expected, realtimeAutoSplitSilence)
	if err != nil {
		return err
	}
	fmt.Printf("workspace_progress event=realtime_auto_split_input_ready workspace=%s segments=%d packets=%d silence_ms=%d\n", d.cfg.Workspace, len(expected), len(packets), realtimeAutoSplitSilence.Milliseconds())

	requireReplay := d.realtimeAutoSplitRequiresReplay()
	streamID := workspaceAudioStreamID(1)
	sendDone := make(chan error, 1)
	started := time.Now()
	go func() {
		sendDone <- d.transport.sendAudioTurn(ctx, streamID, packets)
	}()

	matched, liveStreams, err := d.waitRealtimeAutoSplitHistory(ctx, expected, before, sendDone, requireReplay)
	if err != nil {
		return err
	}
	fmt.Printf("workspace_progress event=realtime_auto_split_history_done workspace=%s stream=%s duration=%s entries=%d live_transcript_streams=%d require_replay=%t\n", d.cfg.Workspace, streamID, time.Since(started).Truncate(time.Millisecond), len(matched), len(liveStreams), requireReplay)

	if !requireReplay {
		return nil
	}
	for i, item := range matched {
		d.drainTransport()
		fmt.Printf("workspace_progress event=realtime_auto_split_history_replay_start workspace=%s index=%d history_id=%s text=%q\n", d.cfg.Workspace, i+1, item.Id, item.Text)
		play, err := d.runtimeClient.PlayServerRunWorkspaceHistory(ctx, fmt.Sprintf("workspacetest.realtime_auto_split.history.play.%d", i+1), rpcapi.ServerPlayRunWorkspaceHistoryRequest{
			HistoryId: item.Id,
		})
		if err != nil {
			return fmt.Errorf("realtime auto split history replay %q: play: %w", item.Id, err)
		}
		if play == nil || !play.Accepted {
			state := ""
			message := ""
			if play != nil {
				state = string(play.State)
				if play.Message != nil {
					message = *play.Message
				}
			}
			return fmt.Errorf("realtime auto split history replay %q rejected state=%s: %s", item.Id, state, message)
		}
		replay, err := d.verifyHistoryReplayWithOptions(ctx, item, historyReplayVerifyOptions{
			SkipTextSimilarity:      true,
			SkipAssistantAudioASR:   true,
			AssistantAudioASRReason: "realtime-auto-split-history",
		})
		if err != nil {
			return fmt.Errorf("realtime auto split history replay %q output: %w", item.Id, err)
		}
		fmt.Printf("workspace_progress event=realtime_auto_split_history_replay_done workspace=%s index=%d history_id=%s packets=%d audio_asr=%q\n", d.cfg.Workspace, i+1, item.Id, replay.DownlinkPackets, replay.AudioASR)
	}
	return nil
}

func (d *personaDriver) waitRealtimeAutoSplitHistory(ctx context.Context, expected []string, before map[string]struct{}, sendDone <-chan error, requireReplay bool) ([]rpcapi.PeerRunHistoryEntry, map[string]struct{}, error) {
	live := realtimeAutoSplitLiveTranscript{doneStreams: make(map[string]struct{})}
	deadline := time.NewTimer(realtimeAutoSplitHistoryWait)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	var matched []rpcapi.PeerRunHistoryEntry
	sendComplete := false
	var trace roundEventTrace

	for {
		if sendComplete && len(matched) >= len(expected) && len(live.doneStreams) >= len(expected) {
			return matched, live.doneStreams, nil
		}
		select {
		case <-ctx.Done():
			return matched, live.doneStreams, fmt.Errorf("wait realtime auto split history: %w; recent events: %s", ctx.Err(), trace.String())
		case <-deadline.C:
			return matched, live.doneStreams, fmt.Errorf("realtime auto split history timeout: matched=%d/%d live_streams=%d/%d send_complete=%t recent events: %s", len(matched), len(expected), len(live.doneStreams), len(expected), sendComplete, trace.String())
		case err := <-sendDone:
			if err != nil {
				return matched, live.doneStreams, fmt.Errorf("send realtime auto split input: %w", err)
			}
			sendComplete = true
			sendDone = nil
			fmt.Printf("workspace_progress event=realtime_auto_split_uplink_done workspace=%s\n", d.cfg.Workspace)
		case received := <-d.transport.events:
			event := received.event
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), eventLabel(event), event.Type, eventText(event), eventError(event))
			if msg, ok := peerEventError(event); ok {
				if isRealtimeAutoSplitIgnoredEventError(msg) {
					continue
				}
				return matched, live.doneStreams, fmt.Errorf("peer event error: %s; recent events: %s", msg, trace.String())
			}
			live.observe(event)
		case <-d.transport.opusPackets:
		case err := <-d.transport.errs:
			return matched, live.doneStreams, fmt.Errorf("transport: %w; recent events: %s", err, trace.String())
		case <-poll.C:
			items, err := d.listRealtimeAutoSplitHistory(ctx)
			if err != nil {
				return matched, live.doneStreams, err
			}
			candidates := filterRealtimeAutoSplitGearHistory(items, before, requireReplay)
			if len(candidates) < len(expected) {
				continue
			}
			next, err := matchRealtimeAutoSplitHistory(expected, candidates)
			if err != nil {
				continue
			}
			matched = next
		}
	}
}

func isRealtimeAutoSplitIgnoredEventError(message string) bool {
	return strings.TrimSpace(message) == "interrupted"
}

func (d *personaDriver) realtimeAutoSplitRequiresReplay() bool {
	if d == nil {
		return true
	}
	return strings.ToLower(strings.TrimSpace(d.cfg.Agent)) != "doubao-realtime"
}

type realtimeAutoSplitLiveTranscript struct {
	doneStreams map[string]struct{}
}

func (c *realtimeAutoSplitLiveTranscript) observe(event peerStreamEvent) {
	if c == nil || eventLabel(event) != "transcript" || !isTranscriptDoneEvent(event) {
		return
	}
	streamID := strings.TrimSpace(eventStreamID(event))
	if streamID == "" {
		return
	}
	c.doneStreams[streamID] = struct{}{}
}

func (d *personaDriver) listRealtimeAutoSplitHistory(ctx context.Context) ([]rpcapi.PeerRunHistoryEntry, error) {
	limit := 200
	order := rpcapi.PeerRunHistoryListRequestOrderAsc
	history, err := d.runtimeClient.ListServerRunWorkspaceHistory(ctx, "workspacetest.realtime_auto_split.history", rpcapi.ServerListRunWorkspaceHistoryRequest{Limit: &limit, Order: &order})
	if err != nil {
		return nil, fmt.Errorf("realtime auto split history list: %w", err)
	}
	if history == nil || !history.Available {
		message := ""
		if history != nil && history.Message != nil {
			message = *history.Message
		}
		return nil, fmt.Errorf("realtime auto split history unavailable: %s", message)
	}
	return history.Items, nil
}

func historyIDSet(items []rpcapi.PeerRunHistoryEntry) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[item.Id] = struct{}{}
	}
	return out
}

func filterRealtimeAutoSplitGearHistory(items []rpcapi.PeerRunHistoryEntry, before map[string]struct{}, requireReplay bool) []rpcapi.PeerRunHistoryEntry {
	out := make([]rpcapi.PeerRunHistoryEntry, 0, len(items))
	for _, item := range items {
		if _, ok := before[item.Id]; ok {
			continue
		}
		if item.Type != rpcapi.PeerRunHistoryEntryTypeGear || item.Name != "transcript" || strings.TrimSpace(item.Text) == "" {
			continue
		}
		if requireReplay && !item.ReplayAvailable {
			continue
		}
		out = append(out, item)
	}
	return out
}

func matchRealtimeAutoSplitHistory(expected []string, items []rpcapi.PeerRunHistoryEntry) ([]rpcapi.PeerRunHistoryEntry, error) {
	matched := make([]rpcapi.PeerRunHistoryEntry, 0, len(expected))
	start := 0
	for _, want := range expected {
		found := -1
		for i := start; i < len(items); i++ {
			item := items[i]
			if err := assertTextSimilar("realtime auto split history", want, item.Text, realtimeAutoSplitHistoryMinText); err == nil {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, fmt.Errorf("missing realtime auto split segment %q in history candidates: %s", want, realtimeAutoSplitHistoryCandidateText(items))
		}
		matched = append(matched, items[found])
		start = found + 1
	}
	return matched, nil
}

func realtimeAutoSplitHistoryCandidateText(items []rpcapi.PeerRunHistoryEntry) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s:%q", item.Id, item.Text))
	}
	return strings.Join(parts, " | ")
}

func (d *personaDriver) realtimeAutoSplitPackets(ctx context.Context, segments []string, silence time.Duration) ([][]byte, error) {
	var pcm []byte
	for i, segment := range segments {
		audio, _, err := d.synthesizeOpus(ctx, segment)
		if err != nil {
			return nil, fmt.Errorf("synthesize realtime auto split segment %d: %w", i+1, err)
		}
		segmentPCM, err := oggOpusPCM16Mono16K(audio)
		if err != nil {
			return nil, fmt.Errorf("decode realtime auto split segment %d: %w", i+1, err)
		}
		pcm = append(pcm, segmentPCM...)
		pcm = append(pcm, silencePCM16Mono16K(silence)...)
	}
	packets, err := opusPacketsFromPCM16LE(pcm, realtimeAutoSplitSampleRate, realtimeAutoSplitChannels)
	if err != nil {
		return nil, fmt.Errorf("encode realtime auto split input: %w", err)
	}
	return packets, nil
}

func oggOpusPCM16Mono16K(audio []byte) ([]byte, error) {
	if !bytes.HasPrefix(audio, []byte("OggS")) {
		return nil, fmt.Errorf("expected ogg opus audio")
	}
	channels, err := oggOpusChannels(audio)
	if err != nil {
		return nil, err
	}
	var pcm bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(audio), opus.SampleRate16K); err != nil {
		return nil, err
	}
	switch channels {
	case 1:
		return pcm.Bytes(), nil
	case 2:
		return downmixStereoPCM16LE(pcm.Bytes())
	default:
		return nil, fmt.Errorf("unsupported ogg opus channels %d", channels)
	}
}

func oggOpusChannels(audio []byte) (int, error) {
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			return 0, fmt.Errorf("read ogg packets: %w", err)
		}
		if !codecconv.IsOpusHeadPacket(packet.Data) {
			continue
		}
		_, channels, err := codecconv.ParseOpusHeadPacket(packet.Data)
		if err != nil {
			return 0, err
		}
		return channels, nil
	}
	return 0, fmt.Errorf("missing opus head packet")
}

func downmixStereoPCM16LE(pcm []byte) ([]byte, error) {
	if len(pcm)%4 != 0 {
		return nil, fmt.Errorf("stereo pcm length must be divisible by 4, got %d", len(pcm))
	}
	out := make([]byte, len(pcm)/2)
	for in, outOffset := 0, 0; in < len(pcm); in, outOffset = in+4, outOffset+2 {
		left := int16(binary.LittleEndian.Uint16(pcm[in:]))
		right := int16(binary.LittleEndian.Uint16(pcm[in+2:]))
		mixed := int16((int(left) + int(right)) / 2)
		binary.LittleEndian.PutUint16(out[outOffset:], uint16(mixed))
	}
	return out, nil
}

func silencePCM16Mono16K(duration time.Duration) []byte {
	if duration <= 0 {
		return nil
	}
	samples := int((duration * realtimeAutoSplitSampleRate) / time.Second)
	return make([]byte, samples*2)
}

func opusPacketsFromPCM16LE(pcm []byte, sampleRate, channels int) ([][]byte, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("pcm length must be even, got %d", len(pcm))
	}
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("empty pcm")
	}

	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	frameSize := sampleRate / 50
	samplesPerFrame := frameSize * channels
	packets := make([][]byte, 0, (len(samples)+samplesPerFrame-1)/samplesPerFrame)
	for offset := 0; offset < len(samples); offset += samplesPerFrame {
		frame := make([]int16, samplesPerFrame)
		copy(frame, samples[offset:min(offset+samplesPerFrame, len(samples))])
		packet, err := enc.Encode(frame, frameSize)
		if err != nil {
			return nil, fmt.Errorf("encode opus frame: %w", err)
		}
		packets = append(packets, packet)
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("pcm produced no opus packets")
	}
	return packets, nil
}
