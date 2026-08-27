package giztest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const (
	realtimeTailSilence = 4 * time.Second
)

// peerStream is the PeerStream surface invokePeerStream drives.
// *gizcli.PeerStream satisfies it; tests substitute an in-memory fake.
type peerStream interface {
	Push(ctx context.Context, chunk *genx.MessageChunk) error
	Next() (*genx.MessageChunk, error)
	Close() error
}

// peerStreamOpener dials one logical PeerStream for a peer_stream step.
type peerStreamOpener func() (peerStream, error)

// audioObserver receives one copied assistant Opus packet in stream order.
// It is nil for normal Giztest runs; play mode installs the only implementation.
type audioObserver func(client string, packet []byte) error

func openClientPeerStream(client *gizcli.Client) peerStreamOpener {
	return func() (peerStream, error) { return client.OpenPeerStream(64) }
}

func invokePeerStream(ctx context.Context, client *gizcli.Client, open peerStreamOpener, step Step, input any, audioCaptureMaxBytes int, observers ...audioObserver) (operationResult, error) {
	started := time.Now()
	op := step.PeerStream
	if op == nil {
		return operationResult{}, fmt.Errorf("peer_stream operation required")
	}
	var observeAudio audioObserver
	if len(observers) > 0 {
		observeAudio = observers[0]
	}
	firstResponse := op.Completion == "first_response"
	var idleTimeout time.Duration
	if op.IdleTimeout != "" {
		duration, parseErr := time.ParseDuration(op.IdleTimeout)
		if parseErr != nil || duration <= 0 {
			return operationResult{}, fmt.Errorf("invalid idle_timeout %q", op.IdleTimeout)
		}
		idleTimeout = duration
	}
	stream, err := open()
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
		pushTextTurn := func(sendCtx context.Context, id string) error {
			chunks := []*genx.MessageChunk{
				{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user", BeginOfStream: true}},
				{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user"}},
				{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user", EndOfStream: true}},
			}
			for _, chunk := range chunks {
				if err := stream.Push(sendCtx, chunk); err != nil {
					return err
				}
			}
			return nil
		}
		if err := pushTextTurn(ctx, streamID); err != nil {
			return operationResult{}, err
		}
		sendInterrupt = func() error {
			replacementID, err := generateValue("string")
			if err != nil {
				return err
			}
			interrupted = true
			return pushTextTurn(ctx, replacementID)
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
	responseStarted := time.Now()
	var arrivals *peerStreamFirstResponseArrivals
	if firstResponse {
		arrivals = &peerStreamFirstResponseArrivals{started: responseStarted}
	}
	next := readPeerStream(ctx, stream, arrivals)
	// The inactivity timer is armed once the turn input has been pushed and is
	// reset on every chunk the PeerStream delivers, regardless of label or part.
	var idle <-chan time.Time
	var idleTimer *time.Timer
	armIdle := func() {
		if idleTimeout <= 0 {
			return
		}
		if idleTimer == nil {
			idleTimer = time.NewTimer(idleTimeout)
		} else {
			idleTimer.Reset(idleTimeout)
		}
		idle = idleTimer.C
	}
	stopIdle := func() {
		if idleTimer != nil {
			idleTimer.Stop()
			idle = nil
		}
	}
	defer stopIdle()
	armIdle()
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
	var terminalErrors []string
	var firstTextMS, firstAudioMS, textEOSMS, audioEOSMS, lastEventMS int64
	var firstTextElapsed, firstAudioElapsed time.Duration
	firstTextObserved, firstAudioObserved := false, false
	textEOS, audioEOS := false, false
	counters := func() string {
		return fmt.Sprintf("events=%d assistant_text=%d assistant_audio=%d assistant_eos=%d transcript_text=%d transcript_eos=%d other_eos=%d interrupt_sent=%t interrupt_observed=%t", events, assistantTextEvents, assistantAudioEvents, assistantEOS, transcriptTextEvents, transcriptEOS, otherEOS, interrupted, observedInterrupted)
	}
	baseEvidence := func() map[string]any {
		evidence := map[string]any{"events": events, "last_event_ms": lastEventMS}
		if idleTimeout > 0 {
			evidence["idle_timeout_ms"] = idleTimeout.Milliseconds()
		}
		return evidence
	}
	failedEvidence := func(deadline string) map[string]any {
		evidence := baseEvidence()
		evidence["deadline"] = deadline
		evidence["first_text_ms"] = firstTextMS
		evidence["first_audio_ms"] = firstAudioMS
		return evidence
	}
	terminalLabel := strings.TrimSpace(op.TerminalLabel)
	if terminalLabel == "" {
		terminalLabel = "assistant"
	}
	requireText := op.RequireText == nil || *op.RequireText
	requireAudio := op.RequireAudio == nil || *op.RequireAudio
	var firstTextDeadline, firstAudioDeadline <-chan time.Time
	var firstTextTimer, firstAudioTimer *time.Timer
	var firstTextTimeout, firstAudioTimeout time.Duration
	if firstResponse {
		firstTextTimeout, _ = time.ParseDuration(op.FirstTextTimeout)
		firstAudioTimeout, _ = time.ParseDuration(op.FirstAudioTimeout)
		firstTextTimer = time.NewTimer(firstTextTimeout)
		firstAudioTimer = time.NewTimer(firstAudioTimeout)
		firstTextDeadline = firstTextTimer.C
		firstAudioDeadline = firstAudioTimer.C
		defer firstTextTimer.Stop()
		defer firstAudioTimer.Stop()
	}
	finish := func() (operationResult, error) {
		object := map[string]any{"text": texts, "audio_bytes": audioBytes, "events": events, "text_eos": textEOS, "audio_eos": audioEOS, "interrupted": interrupted, "interrupt_observed": observedInterrupted, "first_text_ms": firstTextMS, "first_audio_ms": firstAudioMS, "text_eos_ms": textEOSMS, "audio_eos_ms": audioEOSMS}
		if len(assistantPackets) > 0 {
			var audio bytes.Buffer
			if err := codecconv.OpusPacketsToOgg(&audio, int(opus.SampleRate16K), 1, assistantPackets); err != nil {
				return operationResult{}, fmt.Errorf("encode assistant audio evidence: %w", err)
			}
			object["audio"] = append([]byte(nil), audio.Bytes()...)
		}
		stopIdle()
		if op.WaitForHistory {
			historyName, err := waitForWorkspaceHistory(ctx, client, step.ID)
			if err != nil {
				return operationResult{}, err
			}
			object["history_name"] = historyName
		}
		evidence := baseEvidence()
		evidence["audio_bytes"] = audioBytes
		evidence["first_text_ms"] = firstTextMS
		evidence["first_audio_ms"] = firstAudioMS
		evidence["text_eos_ms"] = textEOSMS
		evidence["audio_eos_ms"] = audioEOSMS
		return operationResult{assertion: object, saved: object, evidence: evidence}, nil
	}
	for {
		select {
		case <-interrupt:
			interrupt = nil
			// The inactivity bound covers received output, not the reopen and
			// replacement push; it restarts once the interrupting turn is sent.
			stopIdle()
			if err := stream.Close(); err != nil {
				return operationResult{}, fmt.Errorf("close interrupted PeerStream: %w", err)
			}
			stream, err = open()
			if err != nil {
				return operationResult{}, fmt.Errorf("reopen PeerStream after interrupt: %w", err)
			}
			next = readPeerStream(ctx, stream, arrivals)
			if err := sendInterrupt(); err != nil {
				return operationResult{}, fmt.Errorf("send interrupting turn: %w", err)
			}
			textEOS, audioEOS = false, false
			textEOSMS, audioEOSMS = 0, 0
			armIdle()
		case <-idle:
			return operationResult{evidence: failedEvidence("idle_timeout")}, fmt.Errorf("peer_stream idle timeout exceeded after %s (deadline=idle_timeout last_event_ms=%d %s)", op.IdleTimeout, lastEventMS, counters())
		case <-firstTextDeadline:
			if arrivals.firstTextWithin(firstTextTimeout) {
				firstTextDeadline = nil
				continue
			}
			return operationResult{evidence: failedEvidence("first_text_timeout")}, fmt.Errorf("peer_stream first text timeout exceeded after %s (deadline=first_text_timeout %s): %w", op.FirstTextTimeout, counters(), context.DeadlineExceeded)
		case <-firstAudioDeadline:
			if arrivals.firstAudioWithin(firstAudioTimeout) {
				firstAudioDeadline = nil
				continue
			}
			return operationResult{evidence: failedEvidence("first_audio_timeout")}, fmt.Errorf("peer_stream first audio timeout exceeded after %s (deadline=first_audio_timeout %s): %w", op.FirstAudioTimeout, counters(), context.DeadlineExceeded)
		case result := <-next:
			eventElapsed := time.Since(started)
			if firstResponse {
				eventElapsed = result.receivedAt.Sub(responseStarted)
			}
			if result.err != nil {
				if result.err == io.EOF {
					return operationResult{evidence: baseEvidence()}, fmt.Errorf("peer_stream closed before terminal output")
				}
				return operationResult{evidence: baseEvidence()}, result.err
			}
			if result.chunk == nil {
				return operationResult{evidence: baseEvidence()}, fmt.Errorf("peer_stream returned an empty chunk")
			}
			events++
			lastEventMS = eventElapsed.Milliseconds()
			armIdle()
			label := ""
			actualStreamID := ""
			if result.chunk.Ctrl != nil {
				label = strings.TrimSpace(result.chunk.Ctrl.Label)
				actualStreamID = strings.TrimSpace(result.chunk.Ctrl.StreamID)
				terminalError := peerStreamTerminalError(result.chunk)
				if terminalError != "" {
					terminalErrors = append(terminalErrors, terminalError)
				}
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
				if !firstTextObserved && label == "assistant" && strings.TrimSpace(string(part)) != "" {
					firstTextObserved = true
					firstTextElapsed = eventElapsed
					firstTextMS = firstTextElapsed.Milliseconds()
					if firstTextTimer != nil {
						firstTextTimer.Stop()
						firstTextDeadline = nil
					}
					if interruptDelay > 0 && interruptTimer == nil {
						interruptTimer = time.NewTimer(interruptDelay)
						interrupt = interruptTimer.C
					}
				}
				texts = append(texts, string(part))
			case *genx.Blob:
				if label == "assistant" && len(part.Data) > 0 {
					assistantAudioEvents++
					if observeAudio != nil && relayOpusMIME(part.MIMEType) {
						if err := observeAudio(step.Client, append([]byte(nil), part.Data...)); err != nil {
							return operationResult{evidence: baseEvidence()}, fmt.Errorf("observe assistant audio: %w", err)
						}
					}
					if audioCaptureMaxBytes > 0 {
						if audioBytes > audioCaptureMaxBytes-len(part.Data) {
							return operationResult{}, fmt.Errorf("captured assistant audio exceeds output variable max_bytes %d", audioCaptureMaxBytes)
						}
						assistantPackets = append(assistantPackets, append([]byte(nil), part.Data...))
					}
				}
				if !firstAudioObserved && label == "assistant" && len(part.Data) > 0 {
					firstAudioObserved = true
					firstAudioElapsed = eventElapsed
					firstAudioMS = firstAudioElapsed.Milliseconds()
					if firstAudioTimer != nil {
						firstAudioTimer.Stop()
						firstAudioDeadline = nil
					}
				}
				audioBytes += len(part.Data)
			}
			if firstResponse && firstTextObserved && firstAudioObserved {
				if len(terminalErrors) != 0 {
					evidence := baseEvidence()
					evidence["terminal_errors"] = len(terminalErrors)
					return operationResult{evidence: evidence}, fmt.Errorf("peer_stream terminal error: %s", strings.Join(terminalErrors, "; "))
				}
				if firstTextElapsed > firstTextTimeout {
					return operationResult{evidence: failedEvidence("first_text_timeout")}, fmt.Errorf("peer_stream first text timeout exceeded after %s (deadline=first_text_timeout %s): %w", op.FirstTextTimeout, counters(), context.DeadlineExceeded)
				}
				if firstAudioElapsed > firstAudioTimeout {
					return operationResult{evidence: failedEvidence("first_audio_timeout")}, fmt.Errorf("peer_stream first audio timeout exceeded after %s (deadline=first_audio_timeout %s): %w", op.FirstAudioTimeout, counters(), context.DeadlineExceeded)
				}
				return finish()
			}
			if result.chunk.IsEndOfStream() {
				if len(terminalErrors) != 0 {
					evidence := baseEvidence()
					evidence["terminal_errors"] = len(terminalErrors)
					return operationResult{evidence: evidence}, fmt.Errorf("peer_stream terminal error: %s", strings.Join(terminalErrors, "; "))
				}
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
				nowMS := eventElapsed.Milliseconds()
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
					if (requireText && !textEOS) || (requireAudio && !audioEOS) {
						continue
					}
				}
				return finish()
			}
		case <-ctx.Done():
			cause := context.Cause(ctx)
			deadline := "cancelled"
			if errors.Is(cause, context.DeadlineExceeded) {
				deadline = "timeout"
			}
			return operationResult{evidence: failedEvidence(deadline)}, fmt.Errorf("%w (deadline=%s %s)", cause, deadline, counters())
		}
	}
}

func peerStreamTerminalError(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	err := strings.TrimSpace(chunk.Ctrl.Error)
	if strings.EqualFold(err, "interrupted") {
		return ""
	}
	return err
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
	chunk      *genx.MessageChunk
	err        error
	receivedAt time.Time
}

type peerStreamFirstResponseArrivals struct {
	started    time.Time
	firstText  atomic.Int64
	firstAudio atomic.Int64
}

func (a *peerStreamFirstResponseArrivals) observe(chunk *genx.MessageChunk, receivedAt time.Time) {
	if a == nil || chunk == nil {
		return
	}
	label := ""
	if chunk.Ctrl != nil {
		label = strings.TrimSpace(chunk.Ctrl.Label)
	}
	if label == "" {
		switch chunk.Part.(type) {
		case genx.Text, *genx.Blob:
			label = "assistant"
		}
	}
	if label != "assistant" {
		return
	}
	elapsed := receivedAt.Sub(a.started).Nanoseconds() + 1
	switch part := chunk.Part.(type) {
	case genx.Text:
		if strings.TrimSpace(string(part)) != "" {
			a.firstText.CompareAndSwap(0, elapsed)
		}
	case *genx.Blob:
		if len(part.Data) > 0 {
			a.firstAudio.CompareAndSwap(0, elapsed)
		}
	}
}

func (a *peerStreamFirstResponseArrivals) firstTextWithin(timeout time.Duration) bool {
	return firstResponseArrivalWithin(&a.firstText, timeout)
}

func (a *peerStreamFirstResponseArrivals) firstAudioWithin(timeout time.Duration) bool {
	return firstResponseArrivalWithin(&a.firstAudio, timeout)
}

func firstResponseArrivalWithin(arrival *atomic.Int64, timeout time.Duration) bool {
	elapsed := arrival.Load()
	return elapsed > 0 && time.Duration(elapsed-1) <= timeout
}

func readPeerStream(ctx context.Context, stream peerStream, arrivals *peerStreamFirstResponseArrivals) <-chan nextPeerStreamResult {
	next := make(chan nextPeerStreamResult, 1)
	go func() {
		for {
			chunk, err := stream.Next()
			receivedAt := time.Now()
			arrivals.observe(chunk, receivedAt)
			select {
			case next <- nextPeerStreamResult{chunk: chunk, err: err, receivedAt: receivedAt}:
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
