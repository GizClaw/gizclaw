//go:build gizclaw_e2e

package chat

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/gizcli"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

type personaDriver struct {
	cfg                 config
	client              openai.Client
	runtimeClient       runControlClient
	transport           *chatTransport
	newTransport        func() (*chatTransport, error)
	history             []roundHistory
	reloadAgent         func(context.Context) error
	generateUtterance   func(context.Context, int) (string, error)
	synthesizeAudio     func(context.Context, string) ([]byte, [][]byte, error)
	transcribeAudioFile func(context.Context, string) (string, error)
	runtimeHistoryItems int
	runtimeHistorySig   string
}

type roundHistory struct {
	UserText      string
	InputASR      string
	Transcript    string
	AssistantText string
}

type roundStats struct {
	Index                    int
	UserText                 string
	InputASR                 string
	Transcript               string
	AssistantText            string
	AssistantAudioASR        string
	FirstAssistantText       string
	InputOpusPackets         int
	InputOpusBytes           int
	DownlinkPackets          int
	DownlinkBytes            int
	EventCount               int
	UplinkSend               time.Duration
	FirstTranscriptChunk     time.Duration
	TranscriptDone           time.Duration
	FirstTranscriptBeforeEOS bool
	FirstAssistantTextChunk  time.Duration
	AssistantTextDone        time.Duration
	FirstAudioChunk          time.Duration
	FirstAudioBeforeTextDone bool
	ResponseTotal            time.Duration
	WorkspaceTotal           time.Duration
}

type roundEventTrace struct {
	items []string
}

func (t *roundEventTrace) add(format string, args ...any) {
	const maxItems = 80
	item := fmt.Sprintf(format, args...)
	if len(t.items) >= maxItems {
		copy(t.items, t.items[1:])
		t.items[len(t.items)-1] = item
		return
	}
	t.items = append(t.items, item)
}

func (t roundEventTrace) String() string {
	if len(t.items) == 0 {
		return "none"
	}
	return strings.Join(t.items, " | ")
}

type interruptStats struct {
	Index                   int
	FirstUser               string
	SecondUser              string
	FirstAssistantStarted   bool
	DownlinkBeforeInterrupt int
	InterruptedAfter        time.Duration
	InterruptedStreamID     string
	SecondTranscript        string
	SecondAssistantText     string
	SecondAssistantAudioASR string
	SecondDownlinkPackets   int
	SecondTranscriptDone    time.Duration
	SecondFirstText         time.Duration
	SecondAssistantTextDone time.Duration
	SecondFirstAudio        time.Duration
	SecondAudioDone         time.Duration
	SecondResponseTotal     time.Duration
}

func (d *personaDriver) close() {
	if d == nil || d.transport == nil {
		return
	}
	d.transport.close()
	d.transport = nil
}

func (d *personaDriver) resetTransport() error {
	if d == nil {
		return fmt.Errorf("persona driver is nil")
	}
	if d.newTransport == nil {
		if d.transport != nil {
			return nil
		}
		return fmt.Errorf("transport factory is required")
	}
	if d.transport != nil {
		d.transport.close()
		d.transport = nil
	}
	transport, err := d.newTransport()
	if err != nil {
		return err
	}
	if transport == nil {
		return fmt.Errorf("transport factory returned nil")
	}
	d.transport = transport
	return nil
}

func (d *personaDriver) waitFlowcraftHistoryProgress(ctx context.Context, reason string) error {
	if d == nil || !d.cfg.isFlowcraftAgent() || d.runtimeClient == nil {
		return nil
	}
	limit := 200
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		history, err := d.runtimeClient.ListServerRunWorkspaceHistory(ctx, "workspacetest.runtime.history.progress", rpcapi.ServerListRunWorkspaceHistoryRequest{Limit: &limit})
		if err != nil {
			return fmt.Errorf("%s: wait flowcraft history: %w", reason, err)
		}
		if history != nil && history.Available {
			count := len(history.Items)
			sig := flowcraftHistoryProgressSignature(history.Items)
			if count > d.runtimeHistoryItems || (count == d.runtimeHistoryItems && sig != "" && sig != d.runtimeHistorySig) {
				d.runtimeHistoryItems = count
				d.runtimeHistorySig = sig
				return nil
			}
		}
		select {
		case <-ctx.Done():
			if d.allowStableFlowcraftHistory(reason, history) {
				return nil
			}
			return fmt.Errorf("%s: wait flowcraft history progress: %w", reason, ctx.Err())
		case <-deadline.C:
			message := ""
			if history != nil {
				if history.Message != nil {
					message = *history.Message
				}
			}
			if d.allowStableFlowcraftHistory(reason, history) {
				return nil
			}
			count := 0
			if history != nil {
				count = len(history.Items)
			}
			return fmt.Errorf("%s: flowcraft history did not advance from %d, current=%d message=%q", reason, d.runtimeHistoryItems, count, message)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (d *personaDriver) allowStableFlowcraftHistory(reason string, history *rpcapi.ServerListRunWorkspaceHistoryResponse) bool {
	if history == nil || !history.Available || len(history.Items) == 0 {
		return false
	}
	if len(history.Items) < d.runtimeHistoryItems {
		return false
	}
	previous := d.runtimeHistoryItems
	d.runtimeHistoryItems = len(history.Items)
	d.runtimeHistorySig = flowcraftHistoryProgressSignature(history.Items)
	fmt.Printf("workspace_progress event=flowcraft_history_no_advance workspace=%s reason=%q previous=%d current=%d\n", d.cfg.Workspace, reason, previous, len(history.Items))
	return true
}

func flowcraftHistoryProgressSignature(items []rpcapi.PeerRunHistoryEntry) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(item.Id)
		b.WriteByte('\x1f')
		b.WriteString(string(item.Type))
		appendStringPtr(&b, item.GearId)
		b.WriteByte('\x1f')
		b.WriteString(item.Name)
		b.WriteByte('\x1f')
		b.WriteString(item.Text)
		fmt.Fprintf(&b, "replay=%t", item.ReplayAvailable)
		b.WriteByte('\n')
	}
	return b.String()
}

func appendStringPtr(b *strings.Builder, v *string) {
	b.WriteByte('\x1f')
	if v != nil {
		b.WriteString(*v)
	}
}

type conversationMode struct {
	Realtime                 bool
	SkipInputASR             bool
	SkipTranscriptSimilarity bool
	SkipAssistantAudioASR    bool
	AssistantAudioASRReason  string
	LightweightInterrupt     bool
}

const flowcraftSelfStartStreamID = "flowcraft-self-start"
const assistantAudioASRMinRatio = 0.35

func (d *personaDriver) runRound(ctx context.Context, index int, mode conversationMode) (roundStats, error) {
	stat := roundStats{Index: index}

	userText, err := d.nextUtterance(ctx, index)
	if err != nil {
		return stat, err
	}
	stat.UserText = userText
	fmt.Printf("workspace_progress event=round_start workspace=%s round=%d mode=%s user_chars=%d\n", d.cfg.Workspace, index, progressModeName(mode), runeCount(userText))

	audio, packets, err := d.synthesizeOpus(ctx, userText)
	if err != nil {
		return stat, err
	}
	stat.InputOpusPackets = len(packets)
	for _, packet := range packets {
		stat.InputOpusBytes += len(packet)
	}
	if len(packets) == 0 {
		return stat, fmt.Errorf("round %d: speech returned no opus audio packets", index)
	}

	if mode.SkipInputASR {
		stat.InputASR = "skipped: lightweight-behavior"
		fmt.Printf("workspace_progress event=input_audio_ready workspace=%s round=%d packets=%d bytes=%d input_asr=%q\n", d.cfg.Workspace, index, stat.InputOpusPackets, stat.InputOpusBytes, stat.InputASR)
	} else {
		inputPath, err := d.writeRoundAudio(index, audio)
		if err != nil {
			return stat, err
		}
		inputASR, err := d.transcribe(ctx, inputPath)
		if err != nil {
			return stat, err
		}
		stat.InputASR = inputASR
		if err := assertTextSimilar("input asr", userText, inputASR, 0.45); err != nil {
			return stat, fmt.Errorf("round %d: %w", index, err)
		}
		fmt.Printf("workspace_progress event=input_audio_ready workspace=%s round=%d packets=%d bytes=%d input_asr=%q\n", d.cfg.Workspace, index, stat.InputOpusPackets, stat.InputOpusBytes, inputASR)
	}

	streamID := workspaceAudioStreamID(index)
	uplinkStart := time.Now()
	responseStart := uplinkStart
	sendDone := make(chan error, 1)
	if mode.Realtime {
		go func() {
			sendDone <- d.transport.sendAudioTurn(ctx, streamID, packets)
		}()
	} else {
		if err := d.transport.sendAudioTurn(ctx, streamID, packets); err != nil {
			return stat, fmt.Errorf("round %d: send audio turn: %w", index, err)
		}
		stat.UplinkSend = time.Since(uplinkStart)
		responseStart = time.Now()
		fmt.Printf("workspace_progress event=uplink_done workspace=%s round=%d stream=%s duration=%s\n", d.cfg.Workspace, index, streamID, stat.UplinkSend.Truncate(time.Millisecond))
		close(sendDone)
	}

	var transcriptText string
	var assistant strings.Builder
	var downlinkFrames [][]byte
	assistantAudioDone := false
	transcriptStreamID := ""
	assistantStreamID := ""
	var settle <-chan time.Time
	var trace roundEventTrace
	responseTimeout := d.roundResponseTimeout()
	responseDeadline := time.NewTimer(responseTimeout)
	defer responseDeadline.Stop()
	for sendDone != nil || transcriptText == "" || stat.TranscriptDone == 0 || stat.AssistantTextDone == 0 || stat.DownlinkPackets == 0 || !assistantAudioDone || settle != nil {
		select {
		case <-ctx.Done():
			return stat, fmt.Errorf("round %d: wait response: %w; recent events: %s", index, ctx.Err(), trace.String())
		case <-responseDeadline.C:
			return stat, fmt.Errorf("round %d: response timeout after %s; recent events: %s", index, responseTimeout, trace.String())
		case <-settle:
			settle = nil
			switch {
			case transcriptText == "":
				return stat, fmt.Errorf("round %d: missing transcript text after assistant audio EOS; recent events: %s", index, trace.String())
			case stat.TranscriptDone == 0:
				return stat, fmt.Errorf("round %d: missing transcript EOS after assistant audio EOS; recent events: %s", index, trace.String())
			case stat.AssistantTextDone == 0:
				return stat, fmt.Errorf("round %d: missing assistant text EOS after assistant audio EOS; recent events: %s", index, trace.String())
			case stat.DownlinkPackets == 0:
				return stat, fmt.Errorf("round %d: missing downlink audio after assistant audio EOS; recent events: %s", index, trace.String())
			}
		case err := <-d.transport.errs:
			return stat, fmt.Errorf("round %d: transport: %w; recent events: %s", index, err, trace.String())
		case err, ok := <-sendDone:
			if !ok {
				sendDone = nil
				continue
			}
			if err != nil {
				return stat, fmt.Errorf("round %d: send audio turn: %w", index, err)
			}
			stat.UplinkSend = time.Since(uplinkStart)
			fmt.Printf("workspace_progress event=uplink_done workspace=%s round=%d stream=%s duration=%s\n", d.cfg.Workspace, index, streamID, stat.UplinkSend.Truncate(time.Millisecond))
			sendDone = nil
			if assistantAudioDone && settle == nil {
				settle = time.After(700 * time.Millisecond)
			}
		case received := <-d.transport.events:
			event := received.event
			label := eventLabel(event)
			switch label {
			case "transcript":
				if !acceptRoundEventStream(event, streamID, &transcriptStreamID) {
					trace.add("skip event stream=%s want=%s bound=%s label=%s type=%s text=%q error=%s", eventStreamID(event), streamID, transcriptStreamID, label, event.Type, eventText(event), eventError(event))
					continue
				}
			case "assistant":
				if !acceptRoundEventStream(event, streamID, &assistantStreamID) {
					trace.add("skip event stream=%s want=%s bound=%s label=%s type=%s text=%q error=%s", eventStreamID(event), streamID, assistantStreamID, label, event.Type, eventText(event), eventError(event))
					continue
				}
			default:
				if event.StreamId != nil && !streamIDMatches(*event.StreamId, streamID) {
					trace.add("skip event stream=%s want=%s label=%s type=%s text=%q error=%s", *event.StreamId, streamID, label, event.Type, eventText(event), eventError(event))
					continue
				}
			}
			stat.EventCount++
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event), label, event.Type, eventText(event), eventError(event))
			if msg, ok := peerEventError(event); ok {
				return stat, fmt.Errorf("round %d: peer event error: %s; recent events: %s", index, msg, trace.String())
			}
			if event.Type == apitypes.PeerStreamEventTypeEos && label == "assistant" {
				if stat.AssistantTextDone == 0 {
					trace.add("assistant audio segment eos before text done stream=%s", eventStreamID(event))
					continue
				}
				assistantAudioDone = true
				if sendDone == nil {
					settle = time.After(700 * time.Millisecond)
				} else {
					trace.add("assistant audio segment eos before uplink done stream=%s", eventStreamID(event))
				}
				fmt.Printf("workspace_progress event=assistant_audio_done workspace=%s round=%d stream=%s after_eos=%s packets=%d bytes=%d\n", d.cfg.Workspace, index, eventStreamID(event), afterStartLatency(received.receivedAt, responseStart).Truncate(time.Millisecond), stat.DownlinkPackets, stat.DownlinkBytes)
				continue
			}
			textLatency := afterStartLatency(received.receivedAt, responseStart)
			switch label {
			case "transcript":
				if isTranscriptDoneEvent(event) && stat.TranscriptDone == 0 {
					stat.TranscriptDone = textLatency
					fmt.Printf("workspace_progress event=transcript_done workspace=%s round=%d stream=%s after_eos=%s chars=%d\n", d.cfg.Workspace, index, eventStreamID(event), stat.TranscriptDone.Truncate(time.Millisecond), runeCount(transcriptText))
				}
				if event.Text == nil || strings.TrimSpace(*event.Text) == "" {
					continue
				}
				if stat.FirstTranscriptChunk == 0 {
					stat.FirstTranscriptChunk = textLatency
					stat.FirstTranscriptBeforeEOS = sendDone != nil
					fmt.Printf("workspace_progress event=transcript_first workspace=%s round=%d stream=%s after_eos=%s before_eos=%t chunk=%q\n", d.cfg.Workspace, index, eventStreamID(event), stat.FirstTranscriptChunk.Truncate(time.Millisecond), stat.FirstTranscriptBeforeEOS, *event.Text)
				}
				transcriptText = mergeTranscriptText(transcriptText, *event.Text)
			case "assistant":
				if isAssistantTextDoneEvent(event) && stat.AssistantTextDone == 0 {
					stat.AssistantTextDone = textLatency
					fmt.Printf("workspace_progress event=assistant_text_done workspace=%s round=%d stream=%s after_eos=%s chars=%d\n", d.cfg.Workspace, index, eventStreamID(event), stat.AssistantTextDone.Truncate(time.Millisecond), runeCount(assistant.String()))
				}
				if event.Text == nil || strings.TrimSpace(*event.Text) == "" {
					continue
				}
				if stat.FirstAssistantTextChunk == 0 {
					stat.FirstAssistantTextChunk = textLatency
					stat.FirstAssistantText = *event.Text
					fmt.Printf("workspace_progress event=assistant_text_first workspace=%s round=%d stream=%s after_eos=%s chunk=%q\n", d.cfg.Workspace, index, eventStreamID(event), stat.FirstAssistantTextChunk.Truncate(time.Millisecond), stat.FirstAssistantText)
				}
				assistant.WriteString(*event.Text)
			default:
				if event.Text == nil || strings.TrimSpace(*event.Text) == "" {
					continue
				}
			}
		case packet := <-d.transport.opusPackets:
			downlinkFrames = append(downlinkFrames, append([]byte(nil), packet.frame...))
			if stat.FirstAudioChunk == 0 {
				stat.FirstAudioChunk = afterStartLatency(packet.receivedAt, responseStart)
				stat.FirstAudioBeforeTextDone = stat.AssistantTextDone == 0
				fmt.Printf("workspace_progress event=assistant_audio_first workspace=%s round=%d after_eos=%s bytes=%d before_text_done=%t\n", d.cfg.Workspace, index, stat.FirstAudioChunk.Truncate(time.Millisecond), len(packet.frame), stat.FirstAudioBeforeTextDone)
			}
			stat.DownlinkPackets++
			stat.DownlinkBytes += len(packet.frame)
		}
	}

	if sendDone != nil {
		if err := <-sendDone; err != nil {
			return stat, fmt.Errorf("round %d: send audio turn: %w", index, err)
		}
		stat.UplinkSend = time.Since(uplinkStart)
	}
	stat.Transcript = strings.TrimSpace(transcriptText)
	stat.AssistantText = strings.TrimSpace(assistant.String())
	stat.ResponseTotal = time.Since(responseStart)
	stat.WorkspaceTotal = time.Since(uplinkStart)
	if stat.Transcript == "" {
		return stat, fmt.Errorf("round %d: missing transcript text; recent events: %s", index, trace.String())
	}
	if stat.Transcript != "" && !mode.SkipTranscriptSimilarity {
		if err := assertTextSimilar("transcript", userText, stat.Transcript, 0.45); err != nil {
			return stat, fmt.Errorf("round %d: %w", index, err)
		}
	}
	if stat.AssistantText == "" {
		return stat, fmt.Errorf("round %d: missing assistant text; recent events: %s", index, trace.String())
	}
	if stat.AssistantTextDone == 0 {
		return stat, fmt.Errorf("round %d: missing assistant text EOS; recent events: %s", index, trace.String())
	}
	if stat.DownlinkPackets == 0 {
		return stat, fmt.Errorf("round %d: missing downlink audio; recent events: %s", index, trace.String())
	}
	if skipReason := d.assistantAudioASRSkipReason(mode); skipReason == "" {
		stat.AssistantAudioASR, err = d.verifyAssistantAudioASR(ctx, index, "assistant", stat.AssistantText, downlinkFrames)
		if err != nil {
			return stat, fmt.Errorf("round %d: %w", index, err)
		}
	} else {
		stat.AssistantAudioASR = "skipped: " + skipReason
		fmt.Printf("workspace_progress event=assistant_audio_asr_skipped workspace=%s round=%d reason=%s\n", d.cfg.Workspace, index, skipReason)
	}
	fmt.Printf("workspace_progress event=round_done workspace=%s round=%d transcript_chars=%d assistant_chars=%d downlink_packets=%d total=%s\n", d.cfg.Workspace, index, runeCount(stat.Transcript), runeCount(stat.AssistantText), stat.DownlinkPackets, stat.WorkspaceTotal.Truncate(time.Millisecond))
	return stat, nil
}

func (d *personaDriver) verifyAssistantAudioASR(ctx context.Context, index int, name, assistantText string, frames [][]byte) (string, error) {
	return d.verifyAssistantAudioASRWithMinRatio(ctx, index, name, assistantText, frames, assistantAudioASRMinRatio)
}

func (d *personaDriver) verifyAssistantAudioASRWithMinRatio(ctx context.Context, index int, name, assistantText string, frames [][]byte, minRatio float64) (string, error) {
	if len(frames) == 0 {
		return "", fmt.Errorf("missing assistant audio chunks")
	}
	if minRatio <= 0 {
		minRatio = assistantAudioASRMinRatio
	}
	expectedText := cleanAssistantSpokenText(assistantText)
	if expectedText == "" {
		expectedText = cleanUtterance(assistantText)
	}
	chunks := assistantASRFrameChunks(len(frames))
	fmt.Printf("workspace_progress event=assistant_audio_asr_start workspace=%s round=%d name=%s frames=%d parts=%d assistant_chars=%d spoken_chars=%d\n", d.cfg.Workspace, index, name, len(frames), len(chunks), runeCount(assistantText), runeCount(expectedText))
	var parts []string
	started := time.Now()
	for _, chunk := range chunks {
		partName := name
		if len(frames) > assistantASRFramesPerChunk {
			partName = fmt.Sprintf("%s-part-%02d", name, len(parts)+1)
		}
		partStart := time.Now()
		fmt.Printf("workspace_progress event=assistant_audio_asr_part_start workspace=%s round=%d name=%s part=%d frames=%d\n", d.cfg.Workspace, index, name, len(parts)+1, chunk.end-chunk.start)
		audioASR, err := d.transcribeAssistantAudioFrames(ctx, index, partName, frames[chunk.start:chunk.end])
		if err != nil {
			return "", fmt.Errorf("assistant audio asr part %d: %w", len(parts)+1, err)
		}
		parts = append(parts, audioASR)
		fmt.Printf("workspace_progress event=assistant_audio_asr_part_done workspace=%s round=%d name=%s part=%d duration=%s text_chars=%d\n", d.cfg.Workspace, index, name, len(parts), time.Since(partStart).Truncate(time.Millisecond), runeCount(audioASR))
	}
	audioASR := strings.TrimSpace(strings.Join(parts, " "))
	if err := assertTextSimilar("assistant audio asr", expectedText, audioASR, minRatio); err != nil {
		return "", err
	}
	fmt.Printf("workspace_progress event=assistant_audio_asr_done workspace=%s round=%d name=%s duration=%s text_chars=%d\n", d.cfg.Workspace, index, name, time.Since(started).Truncate(time.Millisecond), runeCount(audioASR))
	return audioASR, nil
}

func (d *personaDriver) transcribeAssistantAudioFrames(ctx context.Context, index int, name string, frames [][]byte) (string, error) {
	oggAudio, err := oggOpusFromPackets(48000, 1, frames)
	if err != nil {
		return "", fmt.Errorf("encode assistant audio: %w", err)
	}
	var pcm bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(oggAudio), opus.SampleRate16K); err != nil {
		return "", fmt.Errorf("decode assistant audio for asr: %w", err)
	}
	audio, err := pcm16WAV(16000, 1, pcm.Bytes())
	if err != nil {
		return "", fmt.Errorf("encode assistant audio: %w", err)
	}
	path, err := d.writeRoundAudioFileExt(index, name, ".wav", audio)
	if err != nil {
		return "", err
	}
	audioASR, err := d.transcribeAssistantAudio(ctx, path)
	if err == nil {
		return audioASR, nil
	}
	if ctx.Err() != nil {
		return "", err
	}
	if len(frames) <= assistantASRMinRetryFrames {
		return "", err
	}
	mid := len(frames) / 2
	left, leftErr := d.transcribeAssistantAudioFrames(ctx, index, name+"a", frames[:mid])
	if leftErr != nil {
		return "", fmt.Errorf("%w; split-left: %w", err, leftErr)
	}
	right, rightErr := d.transcribeAssistantAudioFrames(ctx, index, name+"b", frames[mid:])
	if rightErr != nil {
		return "", fmt.Errorf("%w; split-right: %w", err, rightErr)
	}
	return strings.TrimSpace(strings.TrimSpace(left) + " " + strings.TrimSpace(right)), nil
}

func progressModeName(mode conversationMode) string {
	if mode.Realtime {
		return "realtime"
	}
	return "push-to-talk"
}

func (d *personaDriver) roundResponseTimeout() time.Duration {
	if d != nil && d.cfg.timeout > 0 {
		return d.cfg.timeout
	}
	return workspaceRoundResponseTimeout
}

const (
	workspaceRoundResponseTimeout = 180 * time.Second
	assistantASRFramesPerChunk    = 600
	assistantASRMinRetryFrames    = 120
	assistantASRMinTailFrames     = 100
)

type frameChunkRange struct {
	start int
	end   int
}

func assistantASRFrameChunks(frameCount int) []frameChunkRange {
	if frameCount <= 0 {
		return nil
	}
	chunks := make([]frameChunkRange, 0, frameCount/assistantASRFramesPerChunk+1)
	for start := 0; start < frameCount; start += assistantASRFramesPerChunk {
		end := min(start+assistantASRFramesPerChunk, frameCount)
		if end-start < assistantASRMinTailFrames && len(chunks) > 0 {
			chunks[len(chunks)-1].end = end
			continue
		}
		chunks = append(chunks, frameChunkRange{start: start, end: end})
	}
	return chunks
}

func (d *personaDriver) runInterruptRounds(ctx context.Context, mode conversationMode) ([]interruptStats, error) {
	count := d.interruptRoundCount()
	stats := make([]interruptStats, 0, count)
	for i := 1; i <= count; i++ {
		stat, err := d.runInterruptScenario(ctx, i, mode)
		stats = append(stats, stat)
		if err != nil {
			return stats, err
		}
		if err := d.waitFlowcraftHistoryProgress(ctx, fmt.Sprintf("interrupt round %d", i)); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (d *personaDriver) interruptRoundCount() int {
	if d != nil && d.cfg.Interrupt.Rounds > 0 {
		return d.cfg.Interrupt.Rounds
	}
	return 1
}

func (d *personaDriver) runInterruptScenario(ctx context.Context, index int, mode conversationMode) (interruptStats, error) {
	stat := interruptStats{
		Index:      index,
		FirstUser:  strings.TrimSpace(d.cfg.Interrupt.FirstUtterance),
		SecondUser: strings.TrimSpace(d.cfg.Interrupt.SecondUtterance),
	}
	if stat.FirstUser == "" {
		stat.FirstUser = "请用三句话介绍北京今天适合做什么"
	}
	if stat.SecondUser == "" {
		stat.SecondUser = "请用一句话回答收到并继续测试"
	}
	_, firstPackets, err := d.synthesizeOpus(ctx, stat.FirstUser)
	if err != nil {
		return stat, err
	}
	_, secondPackets, err := d.synthesizeOpus(ctx, stat.SecondUser)
	if err != nil {
		return stat, err
	}
	if len(firstPackets) == 0 || len(secondPackets) == 0 {
		return stat, fmt.Errorf("interrupt test needs non-empty opus packets")
	}
	if d.reloadAgent != nil && index == 1 {
		if err := d.reloadAgent(ctx); err != nil && !isAgentAlreadyRunning(err) {
			return stat, fmt.Errorf("interrupt reload workspace: %w", err)
		}
	}
	if err := d.resetTransport(); err != nil {
		return stat, fmt.Errorf("interrupt open transport: %w", err)
	}
	if index == 1 && d.cfg.flowcraftStartsSelf() {
		if _, ok, err := d.consumeSelfStart(ctx, mode.SkipAssistantAudioASR); err != nil {
			return stat, fmt.Errorf("interrupt consume self-start: %w", err)
		} else if ok {
			if err := d.waitFlowcraftHistoryProgress(ctx, "interrupt self-start"); err != nil {
				return stat, err
			}
			if err := d.resetTransport(); err != nil {
				return stat, fmt.Errorf("interrupt reopen transport after self-start: %w", err)
			}
		}
	}
	d.transport.drain()
	firstStreamID := workspaceAudioStreamID(index*2 - 1)
	secondStreamID := workspaceAudioStreamID(index * 2)
	firstSendDone := make(chan error, 1)
	secondSendDone := make(chan error, 1)
	if mode.Realtime {
		go func() {
			firstSendDone <- d.transport.sendAudioTurn(ctx, firstStreamID, firstPackets)
		}()
	} else {
		if err := d.transport.sendAudioTurn(ctx, firstStreamID, firstPackets); err != nil {
			return stat, fmt.Errorf("interrupt send first audio: %w", err)
		}
		close(firstSendDone)
	}
	start := time.Now()
	sentInterrupt := false
	sendSecondTurn := func() error {
		if sentInterrupt {
			return nil
		}
		sentInterrupt = true
		if mode.Realtime {
			if err := d.transport.sendAudioTurnBOS(ctx, secondStreamID); err != nil {
				return fmt.Errorf("interrupt send second BOS: %w", err)
			}
			go func() {
				secondSendDone <- d.transport.sendAudioTurnAudioAndEOS(ctx, secondStreamID, secondPackets)
			}()
			return nil
		}
		if err := d.transport.sendAudioTurn(ctx, secondStreamID, secondPackets); err != nil {
			return fmt.Errorf("interrupt send second audio: %w", err)
		}
		close(secondSendDone)
		return nil
	}
	var interruptedAt time.Time
	var secondTranscript string
	var secondAssistant strings.Builder
	secondAssistantTextDone := false
	secondAssistantAudioDone := false
	var secondFrames [][]byte
	var settle <-chan time.Time
	var trace roundEventTrace
	for {
		if mode.LightweightInterrupt && settle == nil && !interruptedAt.IsZero() && secondTranscript != "" && stat.SecondDownlinkPackets > 0 && strings.TrimSpace(secondAssistant.String()) != "" {
			settle = time.After(700 * time.Millisecond)
		}
		if !interruptedAt.IsZero() && secondTranscript != "" && stat.SecondTranscriptDone > 0 && stat.SecondDownlinkPackets > 0 && secondAssistantTextDone && secondAssistantAudioDone && settle == nil {
			stat.SecondTranscript = strings.TrimSpace(secondTranscript)
			stat.SecondAssistantText = strings.TrimSpace(secondAssistant.String())
			stat.SecondResponseTotal = time.Since(interruptedAt)
			if !stat.FirstAssistantStarted {
				return stat, fmt.Errorf("interrupt did not receive first response before sending second turn; recent events: %s", trace.String())
			}
			if stat.InterruptedStreamID == "" {
				return stat, fmt.Errorf("interrupt missing interrupted assistant stream id; recent events: %s", trace.String())
			}
			if stat.SecondTranscript == "" {
				return stat, fmt.Errorf("interrupt missing second transcript; recent events: %s", trace.String())
			}
			if err := assertTextSimilar("interrupt second transcript", stat.SecondUser, stat.SecondTranscript, 0.45); err != nil {
				return stat, err
			}
			if stat.SecondAssistantText == "" {
				return stat, fmt.Errorf("interrupt missing second assistant text; recent events: %s", trace.String())
			}
			if skipReason := d.assistantAudioASRSkipReason(mode); skipReason == "" {
				audioASR, err := d.verifyAssistantAudioASR(ctx, index, "interrupt-second-assistant", stat.SecondAssistantText, secondFrames)
				if err != nil {
					return stat, fmt.Errorf("interrupt second response: %w", err)
				}
				stat.SecondAssistantAudioASR = audioASR
			} else {
				stat.SecondAssistantAudioASR = "skipped: " + skipReason
				fmt.Printf("workspace_progress event=assistant_audio_asr_skipped workspace=%s round=%d reason=%s\n", d.cfg.Workspace, index, skipReason)
			}
			return stat, nil
		}
		select {
		case <-ctx.Done():
			return stat, fmt.Errorf("interrupt wait response: %w; recent events: %s", ctx.Err(), trace.String())
		case <-settle:
			settle = nil
			if mode.LightweightInterrupt {
				stat.SecondTranscript = strings.TrimSpace(secondTranscript)
				stat.SecondAssistantText = strings.TrimSpace(secondAssistant.String())
				stat.SecondResponseTotal = time.Since(interruptedAt)
				stat.SecondAssistantAudioASR = "skipped: lightweight-interrupt"
				if !stat.FirstAssistantStarted {
					return stat, fmt.Errorf("interrupt did not receive first response before sending second turn; recent events: %s", trace.String())
				}
				if stat.InterruptedStreamID == "" {
					return stat, fmt.Errorf("interrupt missing interrupted assistant stream id; recent events: %s", trace.String())
				}
				if stat.SecondTranscript == "" {
					return stat, fmt.Errorf("interrupt missing second transcript; recent events: %s", trace.String())
				}
				if stat.SecondAssistantText == "" {
					return stat, fmt.Errorf("interrupt missing second assistant text; recent events: %s", trace.String())
				}
				if stat.SecondDownlinkPackets == 0 {
					return stat, fmt.Errorf("interrupt missing second assistant audio; recent events: %s", trace.String())
				}
				return stat, nil
			}
			switch {
			case secondTranscript == "":
				return stat, fmt.Errorf("interrupt missing second transcript after audio EOS; recent events: %s", trace.String())
			case stat.SecondTranscriptDone == 0:
				return stat, fmt.Errorf("interrupt missing second transcript EOS after audio EOS; recent events: %s", trace.String())
			case !secondAssistantTextDone:
				return stat, fmt.Errorf("interrupt missing second assistant text EOS after audio EOS; recent events: %s", trace.String())
			case stat.SecondDownlinkPackets == 0:
				return stat, fmt.Errorf("interrupt missing second assistant audio after audio EOS; recent events: %s", trace.String())
			}
		case err := <-d.transport.errs:
			return stat, fmt.Errorf("interrupt transport: %w; recent events: %s", err, trace.String())
		case err, ok := <-firstSendDone:
			if !ok {
				firstSendDone = nil
				continue
			}
			if err != nil {
				return stat, fmt.Errorf("interrupt send first audio: %w", err)
			}
			firstSendDone = nil
		case err, ok := <-secondSendDone:
			if !ok {
				secondSendDone = nil
				continue
			}
			if err != nil {
				return stat, fmt.Errorf("interrupt send second audio: %w", err)
			}
			secondSendDone = nil
		case event := <-d.transport.events:
			trace.add("event stream=%s label=%s type=%s text=%q error=%s", eventStreamID(event.event), eventLabel(event.event), event.event.Type, eventText(event.event), eventError(event.event))
			if eventLabel(event.event) == "assistant" && event.event.Error != nil && *event.event.Error == "interrupted" {
				if event.event.StreamId != nil && !streamIDMatches(*event.event.StreamId, firstStreamID) {
					continue
				}
				stat.InterruptedAfter = event.since(start)
				if event.event.StreamId != nil {
					stat.InterruptedStreamID = *event.event.StreamId
				}
				interruptedAt = event.receivedAt
				continue
			}
			if msg, ok := peerEventError(event.event); ok {
				return stat, fmt.Errorf("interrupt peer event error: %s; recent events: %s", msg, trace.String())
			}
			if interruptedAt.IsZero() {
				if event.event.StreamId != nil && streamIDMatches(*event.event.StreamId, firstStreamID) && eventLabel(event.event) == "assistant" && event.event.Text != nil && strings.TrimSpace(*event.event.Text) != "" {
					stat.FirstAssistantStarted = true
					if err := sendSecondTurn(); err != nil {
						return stat, err
					}
				}
				if event.event.StreamId == nil || !streamIDMatches(*event.event.StreamId, secondStreamID) {
					continue
				}
				return stat, fmt.Errorf("interrupt second stream started before interrupted assistant EOS: stream=%s label=%s type=%s; recent events: %s", *event.event.StreamId, eventLabel(event.event), event.event.Type, trace.String())
			}
			if event.event.StreamId != nil && !streamIDMatches(*event.event.StreamId, secondStreamID) {
				if streamIDMatches(*event.event.StreamId, firstStreamID) && !interruptedAt.IsZero() && eventLabel(event.event) == "assistant" && event.event.Text != nil && strings.TrimSpace(*event.event.Text) != "" {
					return stat, fmt.Errorf("interrupt first response continued after interruption: stream=%s text=%q", *event.event.StreamId, *event.event.Text)
				}
				continue
			}
			label := eventLabel(event.event)
			eventLatency := event.since(interruptedAt)
			switch label {
			case "transcript":
				if isTranscriptDoneEvent(event.event) && stat.SecondTranscriptDone == 0 {
					stat.SecondTranscriptDone = eventLatency
				}
				if event.event.Text == nil || strings.TrimSpace(*event.event.Text) == "" {
					continue
				}
				secondTranscript = mergeTranscriptText(secondTranscript, *event.event.Text)
			case "assistant":
				if event.event.Type == apitypes.PeerStreamEventTypeEos {
					if !secondAssistantTextDone {
						trace.add("assistant audio segment eos before text done stream=%s", eventStreamID(event.event))
						continue
					}
					secondAssistantAudioDone = true
					stat.SecondAudioDone = eventLatency
					settle = time.After(700 * time.Millisecond)
					continue
				}
				if isAssistantTextDoneEvent(event.event) {
					secondAssistantTextDone = true
					if stat.SecondAssistantTextDone == 0 {
						stat.SecondAssistantTextDone = eventLatency
					}
				}
				if event.event.Text == nil || strings.TrimSpace(*event.event.Text) == "" {
					continue
				}
				if stat.SecondFirstText == 0 {
					stat.SecondFirstText = eventLatency
				}
				secondAssistant.WriteString(*event.event.Text)
			}
		case packet := <-d.transport.opusPackets:
			if sentInterrupt && interruptedAt.IsZero() {
				return stat, fmt.Errorf("interrupt downlink continued before interrupted assistant EOS; recent events: %s", trace.String())
			}
			if !interruptedAt.IsZero() {
				if !packet.receivedAt.IsZero() && packet.receivedAt.Before(interruptedAt) {
					continue
				}
				if stat.SecondFirstAudio == 0 {
					stat.SecondFirstAudio = packet.since(interruptedAt)
				}
				secondFrames = append(secondFrames, append([]byte(nil), packet.frame...))
				stat.SecondDownlinkPackets++
				continue
			}
			stat.DownlinkBeforeInterrupt++
			stat.FirstAssistantStarted = true
			if err := sendSecondTurn(); err != nil {
				return stat, err
			}
		}
	}
}

func workspaceAudioStreamID(index int) string {
	return fmt.Sprintf("audio-e2e-%d-%d", time.Now().UnixNano(), index)
}

func streamIDMatches(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	return actual == expected || (expected != "" && strings.HasPrefix(actual, expected+":"))
}

func acceptRoundEventStream(event apitypes.PeerStreamEvent, inputStreamID string, boundStreamID *string) bool {
	if event.StreamId == nil || strings.TrimSpace(*event.StreamId) == "" {
		return true
	}
	actual := strings.TrimSpace(*event.StreamId)
	if streamIDMatches(actual, inputStreamID) {
		return true
	}
	if boundStreamID == nil {
		return false
	}
	if strings.TrimSpace(*boundStreamID) == "" {
		*boundStreamID = actual
		return true
	}
	return streamIDMatches(actual, *boundStreamID)
}

func isTranscriptDoneEvent(event apitypes.PeerStreamEvent) bool {
	switch event.Type {
	case apitypes.PeerStreamEventTypeTextDone, apitypes.PeerStreamEventTypeEos:
		return true
	default:
		return false
	}
}

func isAssistantTextDoneEvent(event apitypes.PeerStreamEvent) bool {
	return event.Type == apitypes.PeerStreamEventTypeTextDone
}

func (d *personaDriver) nextUtterance(ctx context.Context, index int) (string, error) {
	if d.generateUtterance != nil {
		return d.generateUtterance(ctx, index)
	}
	var prompt strings.Builder
	prompt.WriteString(d.cfg.Persona)
	prompt.WriteString("\nGenerate the next user utterance for a voice-link E2E test.")
	prompt.WriteString("\nRules: Simplified Chinese only, 8 to 24 Chinese characters, no markdown, no quotes.")
	if len(d.history) > 0 {
		prompt.WriteString("\nPrior rounds:\n")
		for i, h := range d.history {
			fmt.Fprintf(&prompt, "%d. user=%s transcript=%s assistant=%s\n", i+1, h.UserText, h.Transcript, h.AssistantText)
		}
	}
	fmt.Fprintf(&prompt, "\nRound: %d", index)

	completion, err := d.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(d.cfg.Models.LLM),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You generate deterministic short voice test utterances."),
			openai.UserMessage(prompt.String()),
		},
	})
	if err != nil {
		return "", fmt.Errorf("generate persona utterance: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("generate persona utterance: no choices")
	}
	text := cleanUtterance(completion.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("generate persona utterance: empty text")
	}
	return text, nil
}

func (d *personaDriver) synthesizeOpus(ctx context.Context, text string) ([]byte, [][]byte, error) {
	if d.synthesizeAudio != nil {
		return d.synthesizeAudio(ctx, text)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		audio, packets, err := d.synthesizeOpusOnce(ctx, text)
		if err == nil {
			return audio, packets, nil
		}
		lastErr = err
		if !isRetryableSpeechError(err) || attempt == 3 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, fmt.Errorf("speech model=%s voice=%s text_chars=%d: %w", d.cfg.Models.TTS, d.cfg.Voice, utf8.RuneCountInString(text), ctx.Err())
		case <-timer.C:
		}
	}
	return nil, nil, fmt.Errorf("speech model=%s voice=%s text_chars=%d: %w", d.cfg.Models.TTS, d.cfg.Voice, utf8.RuneCountInString(text), lastErr)
}

func (d *personaDriver) synthesizeOpusOnce(ctx context.Context, text string) ([]byte, [][]byte, error) {
	speech, err := d.client.Audio.Speech.New(ctx, openai.AudioSpeechNewParams{
		Input:          text,
		Model:          openai.SpeechModel(d.cfg.Models.TTS),
		Voice:          openai.AudioSpeechNewParamsVoice(d.cfg.Voice),
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormatOpus,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("speech: %w", err)
	}
	defer speech.Body.Close()
	audio, err := io.ReadAll(speech.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read speech audio: %w", err)
	}
	if len(audio) == 0 {
		return nil, nil, fmt.Errorf("speech returned empty audio")
	}
	audio, packets, err := opusAudioAndPackets(audio)
	if err != nil {
		return nil, nil, err
	}
	return audio, packets, nil
}

func isRetryableSpeechError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr interface {
		Temporary() bool
		Timeout() bool
	}
	if errors.As(err, &netErr) && (netErr.Temporary() || netErr.Timeout()) {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "speech returned empty audio") {
		return true
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case 400, 408, 409, 429:
		return true
	default:
		return apiErr.StatusCode >= 500
	}
}

func (d *personaDriver) transcribe(ctx context.Context, audioPath string) (string, error) {
	if d.transcribeAudioFile != nil {
		return d.transcribeAudioFile(ctx, audioPath)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	stat, statErr := os.Stat(audioPath)
	fileSize := int64(-1)
	if statErr == nil {
		fileSize = stat.Size()
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		text, err := d.transcribeOnce(ctx, audioPath)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !isRetryableTranscriptionError(err) || attempt == 3 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", fmt.Errorf("transcription %s size=%d: %w", audioPath, fileSize, ctx.Err())
		case <-timer.C:
		}
	}
	return "", fmt.Errorf("transcription %s size=%d: %w", audioPath, fileSize, lastErr)
}

func (d *personaDriver) transcribeOnce(ctx context.Context, audioPath string) (string, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("open audio for asr: %w", err)
	}
	defer file.Close()
	transcription, err := d.client.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  file,
		Model: openai.AudioModel(d.cfg.Models.ASR),
	})
	if err != nil {
		return "", fmt.Errorf("transcription: %w", err)
	}
	text := strings.TrimSpace(transcription.Text)
	if text == "" {
		return "", fmt.Errorf("transcription returned empty text")
	}
	return text, nil
}

func isRetryableTranscriptionError(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case 400, 408, 409, 429:
		return true
	default:
		return apiErr.StatusCode >= 500
	}
}

func (d *personaDriver) transcribeAssistantAudio(ctx context.Context, audioPath string) (string, error) {
	return d.transcribe(ctx, audioPath)
}

func (d *personaDriver) assistantAudioASRSkipReason(mode conversationMode) string {
	if mode.SkipAssistantAudioASR {
		if strings.TrimSpace(mode.AssistantAudioASRReason) != "" {
			return strings.TrimSpace(mode.AssistantAudioASRReason)
		}
		return "human-review"
	}
	if d == nil || !d.cfg.isASTTranslateAgent() {
		return ""
	}
	if mode.Realtime {
		return "ast-translate-realtime-provider-segmented-audio"
	}
	pair := strings.ToLower(strings.TrimSpace(d.cfg.Workflow.Parameters.LangPair))
	if pair == "" || pair == "auto" {
		return ""
	}
	parts := strings.Split(pair, "/")
	if len(parts) != 2 {
		return ""
	}
	switch strings.TrimSpace(parts[1]) {
	case "jp", "ja":
		return "ast-translate-target-jp-human-review"
	default:
		return ""
	}
}

func (d *personaDriver) writeRoundAudio(index int, audio []byte) (string, error) {
	return d.writeRoundAudioFile(index, "input", audio)
}

func (d *personaDriver) writeRoundAudioFile(index int, name string, audio []byte) (string, error) {
	return d.writeRoundAudioFileExt(index, name, ".ogg", audio)
}

func (d *personaDriver) writeRoundAudioFileExt(index int, name, ext string, audio []byte) (string, error) {
	if d.cfg.OutputDir == "" {
		dir, err := os.MkdirTemp("", "gizclaw-workspacetest-*")
		if err != nil {
			return "", fmt.Errorf("create output dir: %w", err)
		}
		d.cfg.OutputDir = dir
	}
	if err := os.MkdirAll(d.cfg.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	ext = strings.TrimSpace(ext)
	if ext == "" {
		ext = ".ogg"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	path := filepath.Join(d.cfg.OutputDir, fmt.Sprintf("round-%02d-%s%s", index, name, ext))
	if err := os.WriteFile(path, audio, 0o644); err != nil {
		return "", fmt.Errorf("write round audio: %w", err)
	}
	return path, nil
}

func pcm16WAV(sampleRate, channels int, pcm []byte) ([]byte, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("wav sample rate must be positive")
	}
	if channels <= 0 {
		return nil, fmt.Errorf("wav channels must be positive")
	}
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("wav pcm length must be even, got %d", len(pcm))
	}
	byteRate := sampleRate * channels * 2
	blockAlign := channels * 2
	dataLen := len(pcm)
	out := make([]byte, 44+dataLen)
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16)
	binary.LittleEndian.PutUint16(out[20:22], 1)
	binary.LittleEndian.PutUint16(out[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(out[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(out[34:36], 16)
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	copy(out[44:], pcm)
	return out, nil
}

func opusPacketsFromOgg(audio []byte) ([][]byte, error) {
	var packets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			return nil, fmt.Errorf("read ogg opus packets: %w", err)
		}
		if len(packet.Data) == 0 {
			continue
		}
		if bytes.HasPrefix(packet.Data, []byte("OpusHead")) || bytes.HasPrefix(packet.Data, []byte("OpusTags")) {
			continue
		}
		packets = append(packets, append([]byte(nil), packet.Data...))
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("no opus payload packets found")
	}
	return packets, nil
}

func opusAudioAndPackets(audio []byte) ([]byte, [][]byte, error) {
	switch {
	case bytes.HasPrefix(audio, []byte("OggS")):
		packets, err := opusPacketsFromOgg(audio)
		if err != nil {
			return nil, nil, err
		}
		return audio, packets, nil
	case isMP3Audio(audio):
		packets, err := opusPacketsFromMP3(audio, 16000, 1)
		if err != nil {
			return nil, nil, err
		}
		oggAudio, err := oggOpusFromPackets(16000, 1, packets)
		if err != nil {
			return nil, nil, fmt.Errorf("wrap mp3-derived opus packets: %w", err)
		}
		return oggAudio, packets, nil
	default:
		return nil, nil, fmt.Errorf("unsupported speech audio container")
	}
}

func opusPacketsFromMP3(audio []byte, targetSampleRate, targetChannels int) ([][]byte, error) {
	decoded, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(audio))
	if err != nil {
		return nil, fmt.Errorf("decode mp3 speech audio: %w", err)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("decode mp3 speech audio: invalid sample rate %d", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("decode mp3 speech audio: unsupported channels %d", channels)
	}
	srcFmt := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	dstFmt := resampler.Format{SampleRate: targetSampleRate, Stereo: targetChannels == 2}
	pcm := decoded
	if srcFmt != dstFmt {
		rs, err := resampler.New(bytes.NewReader(decoded), srcFmt, dstFmt)
		if err != nil {
			return nil, fmt.Errorf("create mp3 speech resampler: %w", err)
		}
		defer func() {
			_ = rs.Close()
		}()
		pcm, err = io.ReadAll(rs)
		if err != nil {
			return nil, fmt.Errorf("resample mp3 speech pcm: %w", err)
		}
	}

	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("mp3 speech pcm length must be even, got %d", len(pcm))
	}
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("mp3 speech decoded to empty pcm")
	}

	enc, err := opus.NewEncoder(targetSampleRate, targetChannels, opus.ApplicationAudio)
	if err != nil {
		return nil, fmt.Errorf("create speech opus encoder: %w", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	frameSize := targetSampleRate / 50
	samplesPerFrame := frameSize * targetChannels
	packets := make([][]byte, 0, (len(samples)+samplesPerFrame-1)/samplesPerFrame)
	for offset := 0; offset < len(samples); offset += samplesPerFrame {
		frame := make([]int16, samplesPerFrame)
		copy(frame, samples[offset:min(offset+samplesPerFrame, len(samples))])
		packet, err := enc.Encode(frame, frameSize)
		if err != nil {
			return nil, fmt.Errorf("encode speech opus frame: %w", err)
		}
		packets = append(packets, packet)
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("mp3 speech produced no opus packets")
	}
	return packets, nil
}

func isMP3Audio(audio []byte) bool {
	return len(audio) >= 3 && string(audio[:3]) == "ID3" || len(audio) >= 2 && audio[0] == 0xff && audio[1]&0xe0 == 0xe0
}

func oggOpusFromPackets(sampleRate, channels int, packets [][]byte) ([]byte, error) {
	var out bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&out, sampleRate, channels, packets); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func cleanUtterance(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`\"'“”‘’ \t\r\n")
	text = strings.ReplaceAll(text, "\n", "")
	return strings.TrimSpace(text)
}

var (
	assistantSpokenXMLBlockPattern = regexp.MustCompile(`(?is)<\s*[A-Za-z][A-Za-z0-9:_-]*\b[^<>]*>.*?</\s*[A-Za-z][A-Za-z0-9:_-]*\s*>`)
	assistantSpokenXMLTagPattern   = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9:_-]*(?:\s+[^<>]*)?/?>`)
)

func cleanAssistantSpokenText(text string) string {
	text = cleanUtterance(text)
	if text == "" {
		return ""
	}
	text = trimAssistantToolMarkupTail(text)
	text = removeAssistantXMLBlocks(text)
	text = assistantSpokenXMLTagPattern.ReplaceAllString(text, "")
	return cleanUtterance(text)
}

func trimAssistantToolMarkupTail(text string) string {
	lower := strings.ToLower(text)
	cut := len(text)
	for _, marker := range []string{
		"<seed:_tool_call",
		"<tool_call",
		"<node id=\"tool_call\"",
		"<node id='tool_call'",
		"<function",
		"<parameter",
	} {
		if idx := strings.Index(lower, marker); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	return strings.TrimSpace(text[:cut])
}

func removeAssistantXMLBlocks(text string) string {
	for {
		next := assistantSpokenXMLBlockPattern.ReplaceAllString(text, "")
		if next == text {
			return text
		}
		text = next
	}
}

func eventLabel(event apitypes.PeerStreamEvent) string {
	if event.Label == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*event.Label))
}

func eventStreamID(event apitypes.PeerStreamEvent) string {
	if event.StreamId == nil {
		return ""
	}
	return *event.StreamId
}

func eventText(event apitypes.PeerStreamEvent) string {
	if event.Text == nil {
		return ""
	}
	text := strings.TrimSpace(*event.Text)
	runes := []rune(text)
	if len(runes) > 24 {
		return string(runes[:24]) + "..."
	}
	return text
}

func eventError(event apitypes.PeerStreamEvent) string {
	if event.Error == nil {
		return ""
	}
	return *event.Error
}

func peerEventError(event apitypes.PeerStreamEvent) (string, bool) {
	if event.Error == nil {
		return "", false
	}
	message := strings.TrimSpace(*event.Error)
	if message == "" {
		return "", false
	}
	return message, true
}

func assertTextSimilar(name, expected, actual string, minRatio float64) error {
	expectedNorm := normalizeTranscript(expected)
	actualNorm := normalizeTranscript(actual)
	if expectedNorm == "" || actualNorm == "" {
		return fmt.Errorf("%s empty after normalization: expected=%q actual=%q", name, expected, actual)
	}
	if strings.Contains(actualNorm, expectedNorm) {
		return nil
	}
	ratio := lcsRatio(expectedNorm, actualNorm)
	if ratio >= minRatio {
		return nil
	}
	return fmt.Errorf("%s mismatch: similarity %.2f below %.2f: expected %q normalized %q, got %q normalized %q", name, ratio, minRatio, expected, expectedNorm, actual, actualNorm)
}

func isAgentAlreadyRunning(err error) bool {
	return err != nil && strings.Contains(err.Error(), "workspace already has a running agent")
}

func mergeTranscriptText(current, chunk string) string {
	current = strings.TrimSpace(current)
	chunk = strings.TrimSpace(chunk)
	if chunk == "" {
		return current
	}
	if current == "" {
		return chunk
	}
	currentNorm := normalizeTranscript(current)
	chunkNorm := normalizeTranscript(chunk)
	if chunkNorm == "" {
		return current
	}
	if currentNorm == "" {
		return chunk
	}
	if strings.Contains(currentNorm, chunkNorm) {
		if transcriptTextScore(chunk) > transcriptTextScore(current) {
			return chunk
		}
		return current
	}
	if strings.Contains(chunkNorm, currentNorm) {
		return chunk
	}
	if lcsRatio(currentNorm, chunkNorm) >= 0.80 {
		if transcriptTextScore(chunk) >= transcriptTextScore(current) {
			return chunk
		}
		return current
	}
	if suffix := normalizedSuffixAfterPrefix(currentNorm, chunk); suffix != "" {
		return current + suffix
	}
	return current + chunk
}

func transcriptTextScore(text string) int {
	score := runeCount(text)
	for _, r := range text {
		switch r {
		case '，', '。', '？', '！', ',', '.', '?', '!':
			score += 2
		}
	}
	return score
}

func normalizedSuffixAfterPrefix(prefixNorm, text string) string {
	if prefixNorm == "" {
		return text
	}
	matched := 0
	for i, r := range text {
		norm := normalizeTranscript(string(r))
		if norm == "" {
			continue
		}
		if matched >= len(prefixNorm) || !strings.HasPrefix(prefixNorm[matched:], norm) {
			return ""
		}
		matched += len(norm)
		if matched == len(prefixNorm) {
			return text[i+len(string(r)):]
		}
	}
	return ""
}

func lcsRatio(a, b string) float64 {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for i := range ar {
		for j := range br {
			if ar[i] == br[j] {
				curr[j+1] = prev[j] + 1
			} else if curr[j] > prev[j+1] {
				curr[j+1] = curr[j]
			} else {
				curr[j+1] = prev[j+1]
			}
		}
		prev, curr = curr, prev
		clear(curr)
	}
	return float64(prev[len(br)]) / float64(max(len(ar), len(br)))
}

func normalizeTranscript(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func encodeJSONLine(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func runeCount(s string) int {
	return utf8.RuneCountInString(s)
}

type chatTransport struct {
	client         *gizcli.Client
	stream         peerChunkStream
	packetInterval time.Duration
	audioTap       *humanReviewPlayback
	events         chan timedPeerEvent
	opusPackets    <-chan timedPeerPacket
	errs           chan error
}

type peerChunkStream interface {
	genx.Stream
	Push(context.Context, *genx.MessageChunk) error
}

type timedPeerEvent struct {
	event      apitypes.PeerStreamEvent
	receivedAt time.Time
}

func newTimedPeerEvent(event apitypes.PeerStreamEvent) timedPeerEvent {
	return timedPeerEvent{event: event, receivedAt: time.Now()}
}

func (e timedPeerEvent) since(start time.Time) time.Duration {
	if e.receivedAt.IsZero() {
		return time.Since(start)
	}
	return e.receivedAt.Sub(start)
}

type timedPeerPacket struct {
	frame      []byte
	receivedAt time.Time
}

func newTimedPeerPacket(frame []byte) timedPeerPacket {
	return timedPeerPacket{frame: append([]byte(nil), frame...), receivedAt: time.Now()}
}

func (p timedPeerPacket) since(start time.Time) time.Duration {
	if p.receivedAt.IsZero() {
		return time.Since(start)
	}
	return p.receivedAt.Sub(start)
}

func afterStartLatency(receivedAt, start time.Time) time.Duration {
	latency, _ := afterStartLatencyStatus(receivedAt, start)
	return latency
}

func afterStartLatencyStatus(receivedAt, start time.Time) (time.Duration, bool) {
	if receivedAt.IsZero() {
		return time.Since(start), false
	}
	latency := receivedAt.Sub(start)
	if latency <= 0 {
		return time.Nanosecond, true
	}
	return latency, false
}

func newChatTransport(client *gizcli.Client) (*chatTransport, error) {
	stream, err := client.OpenPeerStream(128)
	if err != nil {
		return nil, err
	}
	packets := make(chan timedPeerPacket, 2048)
	t := &chatTransport{
		client: client,
		stream: stream,
		// Realtime dialogue backends expect microphone-like audio cadence.
		packetInterval: 20 * time.Millisecond,
		events:         make(chan timedPeerEvent, 2048),
		opusPackets:    packets,
		errs:           make(chan error, 1),
	}
	go t.readChunks(packets)
	return t, nil
}

func (t *chatTransport) close() {
	if t == nil {
		return
	}
	if t.stream != nil {
		_ = t.stream.Close()
	}
}

func (t *chatTransport) drain() {
	if t == nil {
		return
	}
	for {
		select {
		case <-t.events:
		case <-t.opusPackets:
		default:
			return
		}
	}
}

func (t *chatTransport) readChunks(packets chan<- timedPeerPacket) {
	for {
		chunk, err := t.stream.Next()
		if err != nil {
			if err == io.EOF {
				return
			}
			select {
			case t.errs <- err:
			default:
			}
			return
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && strings.EqualFold(strings.TrimSpace(blob.MIMEType), "audio/opus") && len(blob.Data) > 0 {
			packet := newTimedPeerPacket(blob.Data)
			if t.audioTap != nil {
				label := strings.TrimSpace(chunk.Name)
				if label == "" && chunk.Ctrl != nil {
					label = strings.TrimSpace(chunk.Ctrl.Label)
				}
				if label == "" {
					label = "assistant output"
				}
				if err := t.audioTap.TapOutputPacket(context.Background(), label, packet.frame); err != nil {
					select {
					case t.errs <- err:
					default:
					}
					return
				}
			}
			select {
			case packets <- packet:
			default:
			}
			for _, event := range transportEventsFromChunk(chunk) {
				t.events <- newTimedPeerEvent(event)
			}
			continue
		}
		for _, event := range transportEventsFromChunk(chunk) {
			t.events <- newTimedPeerEvent(event)
		}
	}
}

func (t *chatTransport) sendAudioTurn(ctx context.Context, streamID string, packets [][]byte) error {
	if err := t.sendAudioTurnBOS(ctx, streamID); err != nil {
		return err
	}
	return t.sendAudioTurnAudioAndEOS(ctx, streamID, packets)
}

func (t *chatTransport) sendAudioTurnBOS(ctx context.Context, streamID string) error {
	label := "workspacetest"
	return t.stream.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: true},
	})
}

func (t *chatTransport) sendAudioTurnAudioAndEOS(ctx context.Context, streamID string, packets [][]byte) error {
	label := "workspacetest"
	timestamp := time.Now().UnixMilli()
	for i, packet := range packets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.stream.Push(ctx, &genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, Timestamp: timestamp},
		}); err != nil {
			return err
		}
		if t.audioTap != nil {
			if err := t.audioTap.TapInputPacket(ctx, "simulator input", packet); err != nil {
				return err
			}
		}
		timestamp += 20
		if t.packetInterval > 0 && i+1 < len(packets) {
			timer := time.NewTimer(t.packetInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return t.stream.Push(ctx, &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true},
	})
}

func transportEventsFromChunk(chunk *genx.MessageChunk) []apitypes.PeerStreamEvent {
	if chunk == nil {
		return nil
	}
	var out []apitypes.PeerStreamEvent
	if chunk.IsBeginOfStream() {
		out = append(out, transportEventFromChunk(chunk, apitypes.PeerStreamEventTypeBos, nil))
	}
	if text, ok := chunk.Part.(genx.Text); ok {
		value := string(text)
		eventType := apitypes.PeerStreamEventTypeTextDelta
		if chunk.IsEndOfStream() {
			eventType = apitypes.PeerStreamEventTypeTextDone
		}
		out = append(out, transportEventFromChunk(chunk, eventType, &value))
		return out
	}
	if chunk.IsEndOfStream() {
		out = append(out, transportEventFromChunk(chunk, apitypes.PeerStreamEventTypeEos, nil))
	}
	return out
}

func transportEventFromChunk(chunk *genx.MessageChunk, eventType apitypes.PeerStreamEventType, text *string) apitypes.PeerStreamEvent {
	event := apitypes.PeerStreamEvent{V: 1, Type: eventType, Text: text}
	if chunk == nil || chunk.Ctrl == nil {
		return event
	}
	if chunk.Ctrl.StreamID != "" {
		event.StreamId = &chunk.Ctrl.StreamID
	}
	if chunk.Ctrl.Label != "" {
		event.Label = &chunk.Ctrl.Label
	}
	if chunk.Ctrl.Error != "" {
		event.Error = &chunk.Ctrl.Error
	}
	if chunk.Ctrl.Timestamp != 0 {
		event.Timestamp = &chunk.Ctrl.Timestamp
	}
	return event
}
