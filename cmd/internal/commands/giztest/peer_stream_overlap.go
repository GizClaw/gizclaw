package giztestcmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// invokeOverlappingPeerInput owns two paced inputs on one logical stream. It
// never closes/reopens the stream or sends an explicit interruption. Receipt
// timestamps, rather than processing times, prove that the second input's first audio packet
// was sent before the first response's audio EOS.
func invokeOverlappingPeerInput(ctx context.Context, stream peerStream, op *giztest.PeerStreamOperation, input any) (operationResult, error) {
	audio, ok := input.([]byte)
	if !ok {
		return operationResult{}, fmt.Errorf("overlap_input requires Opus audio")
	}
	packets, err := decodeOpusPackets(audio)
	if err != nil {
		return operationResult{}, err
	}
	if op.Mode == "realtime" {
		packets, err = appendRealtimeTailSilence(packets, realtimeTailSilence)
		if err != nil {
			return operationResult{}, err
		}
	}
	pause := 20 * time.Millisecond
	if op.Pacing != "" {
		pause, err = time.ParseDuration(op.Pacing)
		if err != nil || pause < 0 {
			return operationResult{}, fmt.Errorf("invalid overlap_input pacing %q", op.Pacing)
		}
	}
	ids := [2]string{}
	for i := range ids {
		ids[i], err = newStreamID()
		if err != nil {
			return operationResult{}, err
		}
	}
	chunks := audioInputChunks(op.Mode, ids[0], "audio/opus", packets)
	started := time.Now()
	var firstAudio, firstEnd, secondInput time.Time
	var firstID, secondID string
	responses := map[string]*peerStreamResponseProgress{}
	events, turn, cursor := 0, 0, 0
	firstInterrupted := false
	evidence := func() map[string]any {
		result := map[string]any{"events": events, "session_connection_reused": true, "input_overlap": !secondInput.IsZero() && firstEnd.After(secondInput) && secondInput.After(firstAudio), "first_response_interrupted": firstInterrupted, "second_input_sent": turn == 1 && cursor == len(chunks)}
		for key, value := range map[string]time.Time{"first_audio_ms": firstAudio, "first_audio_eos_ms": firstEnd, "second_input_audio_ms": secondInput} {
			if !value.IsZero() {
				result[key] = value.Sub(started).Milliseconds()
			}
		}
		progress := map[string]any{}
		for name, id := range map[string]string{"first": firstID, "second": secondID} {
			if response := responses[id]; response != nil {
				progress[name] = map[string]bool{"text": response.textObserved, "audio": response.audioObserved, "text_eos": response.textEOS, "audio_eos": response.audioEOS, "interrupted": response.interrupted}
			}
		}
		result["response_progress"] = progress
		return result
	}
	fail := func(err error) (operationResult, error) { return operationResult{evidence: evidence()}, err }
	// The caller owns Close; cancelling this reader stops its channel delivery.
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	next := readPeerStream(readCtx, stream, nil)
	timer := time.NewTimer(0)
	defer timer.Stop()
	var send <-chan time.Time = timer.C
	for {
		// The first input (including realtime VAD silence) must be fully sent.
		// The second input begins as soon as first-response audio is available.
		if turn == 0 && cursor == len(chunks) && !firstAudio.IsZero() {
			if !firstEnd.IsZero() {
				return fail(fmt.Errorf("first response ended before second input could overlap"))
			}
			turn, cursor = 1, 0
			chunks = audioInputChunks(op.Mode, ids[1], "audio/opus", packets)
			timer.Reset(0)
			send = timer.C
		}
		first, second := responses[firstID], responses[secondID]
		if turn == 1 && cursor == len(chunks) && first != nil && second != nil && first.textEOS && first.audioEOS && second.textEOS && second.audioEOS {
			if !firstEnd.After(secondInput) || secondInput.IsZero() || !secondInput.After(firstAudio) {
				return fail(fmt.Errorf("second input did not overlap first response audio"))
			}
			if !second.textObserved || !second.audioObserved || second.interrupted {
				return fail(fmt.Errorf("second response did not complete with text and audio"))
			}
			result := evidence()
			result["second_text_eos"], result["second_audio_eos"] = true, true
			return operationResult{assertion: result, saved: result, evidence: result}, nil
		}
		select {
		case <-ctx.Done():
			return fail(fmt.Errorf("overlapping input: %w", context.Cause(ctx)))
		case <-send:
			chunk := chunks[cursor]
			if err := stream.Push(ctx, chunk); err != nil {
				return fail(fmt.Errorf("send overlapping input turn %d: %w", turn+1, err))
			}
			if turn == 1 && cursor == 1 {
				secondInput = time.Now()
			}
			cursor++
			send = nil
			if cursor < len(chunks) {
				timer.Reset(pause)
				send = timer.C
			}
		case received := <-next:
			if received.err != nil {
				return fail(fmt.Errorf("overlapping input output: %w", received.err))
			}
			chunk := received.chunk
			if chunk == nil {
				return fail(fmt.Errorf("overlapping input output closed"))
			}
			events++
			ctrl := chunk.Ctrl
			if ctrl != nil && (strings.TrimSpace(ctrl.Error) != "" || ctrl.ErrorCode != "") && !strings.EqualFold(strings.TrimSpace(ctrl.Error), "interrupted") {
				return fail(fmt.Errorf("stream error: code=%q message=%q", ctrl.ErrorCode, ctrl.Error))
			}
			if ctrl == nil || ctrl.Label != "assistant" {
				continue
			}
			id := strings.TrimSpace(ctrl.StreamID)
			if id == "" {
				return fail(fmt.Errorf("assistant response has no stream ID"))
			}
			response := responses[id]
			hasText, hasAudio := false, false
			switch part := chunk.Part.(type) {
			case genx.Text:
				hasText = strings.TrimSpace(string(part)) != ""
			case *genx.Blob:
				hasAudio = len(part.Data) > 0 && strings.HasPrefix(part.MIMEType, "audio/")
			}
			if response == nil {
				// Empty route announcements and late terminals do not establish a reply.
				// Some realtime providers announce a provisional route before ASR
				// produces the actual response under another ID.
				if !hasText && !hasAudio {
					continue
				}
				if firstID == "" {
					firstID = id
				} else if secondID == "" && !secondInput.IsZero() && received.receivedAt.After(secondInput) {
					secondID = id
				} else {
					return fail(fmt.Errorf("unexpected assistant response before second input or extra response (turn=%d input_audio_sent=%t incoming_bos=%t incoming_text=%t incoming_audio=%t)", turn+1, !secondInput.IsZero(), chunk.IsBeginOfStream(), hasText, hasAudio))
				}
				response = &peerStreamResponseProgress{}
				responses[id] = response
			}
			if strings.EqualFold(strings.TrimSpace(ctrl.Error), "interrupted") {
				if id != firstID || secondInput.IsZero() || !received.receivedAt.After(secondInput) {
					return fail(fmt.Errorf("unexpected interrupted response"))
				}
				response.interrupted, firstInterrupted = true, true
			}
			response.textObserved = response.textObserved || hasText
			response.audioObserved = response.audioObserved || hasAudio
			if id == firstID && hasAudio && firstAudio.IsZero() {
				firstAudio = received.receivedAt
			}
			if chunk.IsEndOfStream() {
				mime, _ := chunk.MIMEType()
				if mime == "text/plain" {
					response.textEOS = true
				}
				if strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "application/ogg") {
					response.audioEOS = true
					if id == firstID {
						firstEnd = received.receivedAt
						if secondInput.IsZero() || !firstEnd.After(secondInput) {
							return fail(fmt.Errorf("first audio EOS arrived before second input audio"))
						}
					}
				}
			}
		}
	}
}
