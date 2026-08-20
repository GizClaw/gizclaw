package giztest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const (
	realtimeTailSilence    = 4 * time.Second
	maxAssistantAudioBytes = 16 << 20
)

func invokePeerStream(ctx context.Context, client *gizcli.Client, step Step, input any) (operationResult, error) {
	started := time.Now()
	op := step.PeerStream
	if op == nil {
		return operationResult{}, fmt.Errorf("peer_stream operation required")
	}
	stream, err := client.OpenPeerStream(64)
	if err != nil {
		return operationResult{}, err
	}
	defer func() { _ = stream.Close() }()
	streamID, err := generateValue("string")
	if err != nil {
		return operationResult{}, err
	}
	interrupted := false
	observedInterrupted := false
	firstAssistantStreamID := ""
	secondAssistantStreamID := ""
	var sendInterrupt func() error
	switch op.Mode {
	case "text":
		text, ok := input.(string)
		if !ok {
			return operationResult{}, fmt.Errorf("text peer_stream input must be string")
		}
		chunks := []*genx.MessageChunk{
			{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", BeginOfStream: true}},
			{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user"}},
			{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", EndOfStream: true}},
		}
		for _, chunk := range chunks {
			if err := stream.Push(ctx, chunk); err != nil {
				return operationResult{}, err
			}
		}
	case "push-to-talk", "realtime":
		audio, ok := input.([]byte)
		if !ok {
			return operationResult{}, fmt.Errorf("audio peer_stream input must be in-memory Opus bytes")
		}
		mimeType := "audio/opus"
		packets, err := decodeOpusPackets(audio)
		if err != nil {
			return operationResult{}, err
		}
		if op.Mode == "realtime" {
			packets, err = appendRealtimeTailSilence(packets, realtimeTailSilence)
			if err != nil {
				return operationResult{}, fmt.Errorf("prepare realtime tail silence: %w", err)
			}
		}
		pause := time.Duration(0)
		if op.Pacing != "" {
			pause, err = time.ParseDuration(op.Pacing)
			if err != nil || pause < 0 {
				return operationResult{}, fmt.Errorf("invalid pacing %q", op.Pacing)
			}
		}
		pushTurn := func(sendCtx context.Context, id string) error {
			chunks := audioInputChunks(op.Mode, id, mimeType, packets)
			for _, chunk := range chunks {
				if err := stream.Push(sendCtx, chunk); err != nil {
					return err
				}
				if pause > 0 {
					timer := time.NewTimer(pause)
					select {
					case <-timer.C:
					case <-sendCtx.Done():
						timer.Stop()
						return context.Cause(sendCtx)
					}
				}
			}
			return nil
		}
		if err := pushTurn(ctx, streamID); err != nil {
			return operationResult{}, err
		}
		sendInterrupt = func() error {
			replacementID, err := generateValue("string")
			if err != nil {
				return err
			}
			interrupted = true
			if err := pushTurn(ctx, replacementID); err != nil {
				return err
			}
			return nil
		}
	default:
		return operationResult{}, fmt.Errorf("peer_stream mode %q requires an existing stream", op.Mode)
	}
	next := readPeerStream(ctx, stream)
	var interrupt <-chan time.Time
	var interruptTimer *time.Timer
	var interruptDelay time.Duration
	if op.InterruptAfter != "" {
		duration, parseErr := time.ParseDuration(op.InterruptAfter)
		if parseErr != nil || duration <= 0 || sendInterrupt == nil {
			return operationResult{}, fmt.Errorf("invalid interrupt_after %q", op.InterruptAfter)
		}
		interruptDelay = duration
		defer func() {
			if interruptTimer != nil {
				interruptTimer.Stop()
			}
		}()
	}
	var texts []string
	var assistantPackets [][]byte
	audioBytes, events := 0, 0
	assistantTextEvents, assistantAudioEvents, assistantEOS := 0, 0, 0
	transcriptTextEvents, transcriptEOS, otherEOS := 0, 0, 0
	var firstTextMS, firstAudioMS, textEOSMS, audioEOSMS int64
	textEOS, audioEOS := false, false
	maxEvents := op.MaxEvents
	if maxEvents == 0 {
		maxEvents = 1024
	}
	terminalLabel := strings.TrimSpace(op.TerminalLabel)
	if terminalLabel == "" {
		terminalLabel = "assistant"
	}
	for events < maxEvents {
		select {
		case <-interrupt:
			interrupt = nil
			if err := stream.Close(); err != nil {
				return operationResult{}, fmt.Errorf("close interrupted PeerStream: %w", err)
			}
			stream, err = client.OpenPeerStream(64)
			if err != nil {
				return operationResult{}, fmt.Errorf("reopen PeerStream after interrupt: %w", err)
			}
			next = readPeerStream(ctx, stream)
			if err := sendInterrupt(); err != nil {
				return operationResult{}, fmt.Errorf("send interrupting turn: %w", err)
			}
			textEOS, audioEOS = false, false
			textEOSMS, audioEOSMS = 0, 0
		case result := <-next:
			if result.err != nil {
				if result.err == io.EOF {
					return operationResult{}, fmt.Errorf("peer_stream closed before terminal output")
				}
				return operationResult{}, result.err
			}
			if result.chunk == nil {
				return operationResult{}, fmt.Errorf("peer_stream returned an empty chunk")
			}
			events++
			label := ""
			actualStreamID := ""
			if result.chunk.Ctrl != nil {
				label = strings.TrimSpace(result.chunk.Ctrl.Label)
				actualStreamID = strings.TrimSpace(result.chunk.Ctrl.StreamID)
				if label == "assistant" && strings.EqualFold(strings.TrimSpace(result.chunk.Ctrl.Error), "interrupted") {
					observedInterrupted = true
				}
			}
			if label == "" {
				switch result.chunk.Part.(type) {
				case genx.Text, *genx.Blob:
					label = "assistant"
				}
			}
			if label == "assistant" && actualStreamID != "" {
				switch {
				case !interrupted && firstAssistantStreamID == "":
					firstAssistantStreamID = actualStreamID
				case interrupted && firstAssistantStreamID != "" && streamIDMatches(actualStreamID, firstAssistantStreamID):
					// The first response may emit its interrupted terminal event on
					// the reopened logical PeerStream.
				case interrupted && secondAssistantStreamID == "":
					secondAssistantStreamID = actualStreamID
				}
			}
			switch part := result.chunk.Part.(type) {
			case genx.Text:
				if label == "assistant" && strings.TrimSpace(string(part)) != "" {
					assistantTextEvents++
				} else if label == "transcript" && strings.TrimSpace(string(part)) != "" {
					transcriptTextEvents++
				}
				if firstTextMS == 0 && label == "assistant" && strings.TrimSpace(string(part)) != "" {
					firstTextMS = time.Since(started).Milliseconds()
					if interruptDelay > 0 && interruptTimer == nil {
						interruptTimer = time.NewTimer(interruptDelay)
						interrupt = interruptTimer.C
					}
				}
				texts = append(texts, string(part))
			case *genx.Blob:
				if label == "assistant" && len(part.Data) > 0 {
					assistantAudioEvents++
					if audioBytes > maxAssistantAudioBytes-len(part.Data) {
						return operationResult{}, fmt.Errorf("assistant audio exceeds %d bytes", maxAssistantAudioBytes)
					}
					assistantPackets = append(assistantPackets, append([]byte(nil), part.Data...))
				}
				if firstAudioMS == 0 && len(part.Data) > 0 {
					firstAudioMS = time.Since(started).Milliseconds()
				}
				audioBytes += len(part.Data)
			}
			if result.chunk.IsEndOfStream() {
				switch label {
				case "assistant":
					assistantEOS++
				case "transcript":
					transcriptEOS++
				default:
					otherEOS++
				}
				if label != terminalLabel {
					continue
				}
				if interruptDelay > 0 && !interrupted {
					continue
				}
				if interrupted {
					if label == "assistant" && actualStreamID != "" && streamIDMatches(actualStreamID, firstAssistantStreamID) {
						continue
					}
					if label == "assistant" && secondAssistantStreamID == "" {
						continue
					}
				}
				nowMS := time.Since(started).Milliseconds()
				if label == "assistant" {
					mimeType, _ := result.chunk.MIMEType()
					switch {
					case mimeType == "text/plain":
						textEOS = true
						textEOSMS = nowMS
					case strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "application/ogg"):
						audioEOS = true
						audioEOSMS = nowMS
					}
					if !textEOS || !audioEOS {
						continue
					}
				}
				object := map[string]any{"text": texts, "audio_bytes": audioBytes, "events": events, "text_eos": textEOS, "audio_eos": audioEOS, "interrupted": interrupted, "interrupt_observed": observedInterrupted, "first_text_ms": firstTextMS, "first_audio_ms": firstAudioMS, "text_eos_ms": textEOSMS, "audio_eos_ms": audioEOSMS}
				if len(assistantPackets) > 0 {
					var audio bytes.Buffer
					if err := codecconv.OpusPacketsToOgg(&audio, int(opus.SampleRate16K), 1, assistantPackets); err != nil {
						return operationResult{}, fmt.Errorf("encode assistant audio evidence: %w", err)
					}
					object["audio"] = append([]byte(nil), audio.Bytes()...)
				}
				if op.WaitForHistory {
					historyName, err := waitForWorkspaceHistory(ctx, client, step.ID)
					if err != nil {
						return operationResult{}, err
					}
					object["history_name"] = historyName
				}
				return operationResult{assertion: object, saved: object, evidence: map[string]any{"events": events, "audio_bytes": audioBytes, "first_text_ms": firstTextMS, "first_audio_ms": firstAudioMS, "text_eos_ms": textEOSMS, "audio_eos_ms": audioEOSMS}}, nil
			}
		case <-ctx.Done():
			return operationResult{}, fmt.Errorf("%w (events=%d assistant_text=%d assistant_audio=%d assistant_eos=%d transcript_text=%d transcript_eos=%d other_eos=%d interrupt_sent=%t interrupt_observed=%t)", context.Cause(ctx), events, assistantTextEvents, assistantAudioEvents, assistantEOS, transcriptTextEvents, transcriptEOS, otherEOS, interrupted, observedInterrupted)
		}
	}
	return operationResult{}, fmt.Errorf("peer_stream exceeded max_events %d", maxEvents)
}

func audioInputChunks(mode, streamID, mimeType string, packets [][]byte) []*genx.MessageChunk {
	chunks := []*genx.MessageChunk{{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", BeginOfStream: true}}}
	for _, packet := range packets {
		chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: mimeType, Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user"}})
	}
	if mode == "push-to-talk" {
		chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "user", EndOfStream: true}})
	}
	return chunks
}

func streamIDMatches(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	return actual == expected || (expected != "" && strings.HasPrefix(actual, expected+":"))
}

func appendRealtimeTailSilence(packets [][]byte, duration time.Duration) ([][]byte, error) {
	if duration <= 0 {
		return packets, nil
	}
	const sampleRate = 16000
	const channels = 1
	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		return nil, fmt.Errorf("create Opus encoder: %w", err)
	}
	defer func() { _ = enc.Close() }()
	frameSize := sampleRate / 50
	frame := make([]int16, frameSize*channels)
	frameCount := int((duration + 20*time.Millisecond - 1) / (20 * time.Millisecond))
	out := make([][]byte, 0, len(packets)+frameCount)
	out = append(out, packets...)
	for range frameCount {
		packet, err := enc.Encode(frame, frameSize)
		if err != nil {
			return nil, fmt.Errorf("encode Opus silence: %w", err)
		}
		out = append(out, packet)
	}
	return out, nil
}

type nextPeerStreamResult struct {
	chunk *genx.MessageChunk
	err   error
}

func readPeerStream(ctx context.Context, stream *gizcli.PeerStream) <-chan nextPeerStreamResult {
	next := make(chan nextPeerStreamResult, 1)
	go func() {
		for {
			chunk, err := stream.Next()
			select {
			case next <- nextPeerStreamResult{chunk: chunk, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return next
}

func waitForWorkspaceHistory(ctx context.Context, client *gizcli.Client, stepID string) (string, error) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	historyStep := Step{
		ID: stepID + "-history",
		RPC: &RPCOperation{
			Method:  "server.run.workspace.history",
			Request: map[string]any{"limit": 20},
		},
	}
	for {
		result, err := invokeUnary(ctx, client, historyStep, historyStep.RPC.Request)
		if err != nil {
			return "", fmt.Errorf("query Workspace history: %w", err)
		}
		if name := firstHistoryName(result); name != "" {
			return name, nil
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			return "", fmt.Errorf("timed out waiting for Workspace history persistence")
		case <-ctx.Done():
			return "", context.Cause(ctx)
		}
	}
}

func firstHistoryName(result map[string]any) string {
	items, _ := result["items"].([]any)
	if len(items) == 0 {
		return ""
	}
	item, _ := items[0].(map[string]any)
	name, _ := item["name"].(string)
	return name
}

func decodeOpusPackets(audio []byte) ([][]byte, error) {
	if !bytes.HasPrefix(audio, []byte("OggS")) {
		if len(audio) == 0 {
			return nil, fmt.Errorf("Opus input is empty")
		}
		return [][]byte{append([]byte(nil), audio...)}, nil
	}
	var packets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			return nil, fmt.Errorf("decode Ogg Opus input: %w", err)
		}
		if bytes.HasPrefix(packet.Data, []byte("OpusHead")) || bytes.HasPrefix(packet.Data, []byte("OpusTags")) {
			continue
		}
		packets = append(packets, append([]byte(nil), packet.Data...))
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("Ogg input contains no Opus audio packets")
	}
	return packets, nil
}
