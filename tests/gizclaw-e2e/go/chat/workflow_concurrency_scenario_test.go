//go:build gizclaw_e2e

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type workflowConcurrencyTurnObservation struct {
	result             workflowConcurrencyTurnResult
	assistant          strings.Builder
	assistantStreams   map[string]struct{}
	transcript         string
	audioEpoch         time.Time
	textInterruptedAt  time.Time
	audioInterruptedAt time.Time
	cutoverSent        bool
	sendDone           bool
	assistantBOSCount  int
	assistantTextDone  bool
	assistantAudioDone bool
	terminalErrors     []string
}

type workflowConcurrencySendResult struct {
	turn int
	err  error
}

func runWorkflowConcurrencyScenario(
	ctx context.Context,
	lane *workflowConcurrencyLane,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
	inputs workflowConcurrencyInputs,
) error {
	turnCount := 1
	if scenario == workflowConcurrencyInterrupt {
		turnCount = 3
	}
	if len(inputs.Texts) != turnCount || spec.RequireAudio && len(inputs.Packets) != turnCount {
		return fmt.Errorf("input count mismatch: text=%d packets=%d want=%d", len(inputs.Texts), len(inputs.Packets), turnCount)
	}
	if lane == nil || lane.Transport == nil || lane.Client == nil {
		return fmt.Errorf("lane is not prepared")
	}
	lane.Transport.drain()
	var downlinkDecoder *opus.Decoder
	if spec.RequireAudio {
		var err error
		downlinkDecoder, err = opus.NewDecoder(16000, 1)
		if err != nil {
			return fmt.Errorf("create downlink Opus decoder: %w", err)
		}
		defer downlinkDecoder.Close()
	}
	observations := make([]workflowConcurrencyTurnObservation, turnCount)
	sendResults := make(chan workflowConcurrencySendResult, turnCount)
	current := 0
	var trace roundEventTrace
	defer func() {
		lane.Result.Turns = make([]workflowConcurrencyTurnResult, len(observations))
		for index := range observations {
			observations[index].result.AssistantTextChars = runeCount(strings.TrimSpace(observations[index].assistant.String()))
			observations[index].result.TranscriptChars = runeCount(strings.TrimSpace(observations[index].transcript))
			lane.Result.Turns[index] = observations[index].result
		}
	}()

	sendTurn := func(index int, cutover bool) error {
		streamID := fmt.Sprintf("workflow-concurrency-%02d-%02d-%s", lane.Index, index+1, genx.NewStreamID())
		observations[index].result = workflowConcurrencyTurnResult{
			Index: index + 1, InputStreamID: streamID, StartedAt: time.Now(),
		}
		if spec.RequireAudio {
			packets := cloneOpusPackets(inputs.Packets[index])
			if cutover {
				if err := lane.Transport.sendAudioTurnBOS(ctx, streamID); err != nil {
					return err
				}
				go func() {
					err := lane.Transport.sendAudioTurnAudioObservedMIME(ctx, streamID, "audio/opus", packets, !spec.KeepRealtimeInputOpen, nil)
					sendResults <- workflowConcurrencySendResult{turn: index, err: err}
				}()
				return nil
			}
			go func() {
				err := lane.Transport.sendAudioTurnObservedMIMEWithEnd(ctx, streamID, "audio/opus", packets, !spec.KeepRealtimeInputOpen, nil)
				sendResults <- workflowConcurrencySendResult{turn: index, err: err}
			}()
			return nil
		}
		text := inputs.Texts[index]
		go func() {
			err := lane.Transport.sendTextTurn(ctx, streamID, text)
			sendResults <- workflowConcurrencySendResult{turn: index, err: err}
		}()
		return nil
	}

	if err := sendTurn(0, false); err != nil {
		return fmt.Errorf("turn 1 input send: %w", err)
	}
	handleEvent := func(received timedPeerEvent) error {
		event := received.event
		turn := workflowConcurrencyEventTurn(observations, current, event)
		trace.add("turn=%d stream=%s label=%s type=%s text_chars=%d error=%s", turn+1, eventStreamID(event), eventLabel(event), event.Type, runeCount(eventText(event)), eventError(event))
		if turn < 0 || turn >= len(observations) {
			return nil
		}
		if err := observeWorkflowConcurrencyEvent(&observations[turn], event, received.receivedAt); err != nil {
			return fmt.Errorf("turn %d: %w", turn+1, err)
		}
		return nil
	}
	for {
		if scenario == workflowConcurrencyInterrupt && current < turnCount-1 {
			observation := &observations[current]
			if workflowConcurrencyResponseComplete(observation, spec) && !observation.cutoverSent {
				return fmt.Errorf("turn %d response completed before interruption; recent events: %s", current+1, trace.String())
			}
			if workflowConcurrencyInterruptGateReady(observation, spec) && !observation.cutoverSent {
				observation.cutoverSent = true
				if err := sendTurn(current+1, true); err != nil {
					return fmt.Errorf("turn %d interrupt BOS: %w", current+2, err)
				}
				if err := verifyWorkflowConcurrencyRuntime(ctx, lane, current+1); err != nil {
					return err
				}
				current++
			}
		}
		if current == turnCount-1 && workflowConcurrencyResponseComplete(&observations[current], spec) {
			if err := validateWorkflowConcurrencyTurns(observations, spec, scenario); err != nil {
				return fmt.Errorf("%w; recent events: %s", err, trace.String())
			}
			return nil
		}
		// readChunks receives stream events and audio packets in one ordered
		// sequence, then fans them into separate buffered channels. Consume any
		// already queued boundaries before selecting an audio packet so a new
		// epoch BOS cannot be skipped merely because select chose the packet
		// channel first. receivedAt still assigns packets that arrived before a
		// boundary to the preceding epoch.
		select {
		case received := <-lane.Transport.events:
			if err := handleEvent(received); err != nil {
				return err
			}
			continue
		default:
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait turn %d: %w; recent events: %s", current+1, ctx.Err(), trace.String())
		case sent := <-sendResults:
			if sent.err != nil {
				return fmt.Errorf("turn %d input send: %w", sent.turn+1, sent.err)
			}
			observations[sent.turn].sendDone = true
			observations[sent.turn].result.InputEOSSent = !spec.KeepRealtimeInputOpen && spec.RequireAudio
			observations[sent.turn].result.InputDoneAt = time.Now()
		case err := <-lane.Transport.errs:
			return fmt.Errorf("logical PeerStream: %w; recent events: %s", err, trace.String())
		case received := <-lane.Transport.events:
			if err := handleEvent(received); err != nil {
				return err
			}
		case packet := <-lane.Transport.opusPackets:
			// The single stream reader enqueues boundaries before later audio,
			// but this select reads from two channels and may choose the packet
			// even when the corresponding BOS is already queued. Drain those
			// boundaries before assigning the packet by receive timestamp.
			for {
				select {
				case received := <-lane.Transport.events:
					if err := handleEvent(received); err != nil {
						return err
					}
				default:
					goto packetReady
				}
			}
		packetReady:
			turn := workflowConcurrencyAudioTurn(observations, packet.receivedAt)
			if turn < 0 {
				continue
			}
			observation := &observations[turn]
			audible, err := workflowConcurrencyAudibleOpusPacket(downlinkDecoder, packet.frame)
			if err != nil {
				return fmt.Errorf("decode downlink Opus packet: %w", err)
			}
			if workflowConcurrencyAudioAfterInterruption(observation, packet.receivedAt) {
				// The peer audio transport is a continuous mixed stream. Once an
				// epoch closes, discard in-flight packets until the next audio BOS
				// establishes a replacement epoch.
				continue
			}
			if !audible {
				continue
			}
			observation.result.AudioPackets++
			observation.result.AudioBytes += len(packet.frame)
		}
	}
}

func workflowConcurrencyInterruptGateReady(observation *workflowConcurrencyTurnObservation, spec workflowConcurrencySpec) bool {
	if observation == nil || strings.TrimSpace(observation.assistant.String()) == "" {
		return false
	}
	if spec.KeepRealtimeInputOpen && !observation.sendDone {
		return false
	}
	if !spec.RequireAudio {
		return !observation.assistantTextDone
	}
	return !observation.assistantAudioDone && observation.result.AudioPackets > 0 && !observation.audioEpoch.IsZero()
}

func workflowConcurrencyResponseComplete(observation *workflowConcurrencyTurnObservation, spec workflowConcurrencySpec) bool {
	if observation == nil || !observation.sendDone || strings.TrimSpace(observation.assistant.String()) == "" || !observation.assistantTextDone {
		return false
	}
	if !spec.RequireAudio {
		return true
	}
	return observation.assistantAudioDone && observation.result.AudioPackets > 0
}

func workflowConcurrencyEventTurn(observations []workflowConcurrencyTurnObservation, current int, event peerStreamEvent) int {
	streamID := eventStreamID(event)
	if streamID != "" {
		for index := range observations {
			observation := &observations[index]
			if observation.result.InputStreamID != "" && streamIDMatches(streamID, observation.result.InputStreamID) {
				return index
			}
			if observation.result.AssistantStreamID != "" && streamIDMatches(streamID, observation.result.AssistantStreamID) {
				return index
			}
			if _, ok := observation.assistantStreams[streamID]; ok {
				return index
			}
			if observation.result.TranscriptStreamID != "" && streamIDMatches(streamID, observation.result.TranscriptStreamID) {
				return index
			}
		}
	}
	if eventLabel(event) == "assistant" && event.Error != nil && strings.TrimSpace(*event.Error) == "interrupted" {
		for index := current - 1; index >= 0; index-- {
			observation := &observations[index]
			if !observation.cutoverSent {
				continue
			}
			switch event.Kind {
			case eventpb.StreamKind_STREAM_KIND_TEXT:
				if observation.result.InterruptedText == 0 {
					return index
				}
			case eventpb.StreamKind_STREAM_KIND_AUDIO:
				if observation.result.InterruptedAudio == 0 {
					return index
				}
			default:
				return index
			}
		}
	}
	if current >= 0 && current < len(observations) {
		return current
	}
	return -1
}

func observeWorkflowConcurrencyEvent(observation *workflowConcurrencyTurnObservation, event peerStreamEvent, receivedAt time.Time) error {
	label := eventLabel(event)
	if label == "assistant" {
		streamID := eventStreamID(event)
		if streamID != "" {
			if observation.assistantStreams == nil {
				observation.assistantStreams = make(map[string]struct{})
			}
			observation.assistantStreams[streamID] = struct{}{}
		}
	}
	if label == "assistant" && event.Error != nil && strings.TrimSpace(*event.Error) == "interrupted" {
		switch event.Kind {
		case eventpb.StreamKind_STREAM_KIND_TEXT:
			observation.result.InterruptedText++
			if observation.result.InterruptedText > 1 {
				return fmt.Errorf("duplicate text interrupted terminal for stream %q", eventStreamID(event))
			}
			observation.textInterruptedAt = receivedAt
		case eventpb.StreamKind_STREAM_KIND_AUDIO:
			observation.result.InterruptedAudio++
			if observation.result.InterruptedAudio > 1 {
				return fmt.Errorf("duplicate audio interrupted terminal for stream %q", eventStreamID(event))
			}
			observation.audioInterruptedAt = receivedAt
		default:
			// Some voice pipelines also emit a route-level terminal after the
			// independently typed text and audio terminals. It does not satisfy
			// either typed requirement, but it is not a duplicate typed terminal.
			return nil
		}
		if observation.result.InterruptedTerminals == 0 {
			observation.result.InterruptedTerminals = 1
			observation.result.InterruptedAt = receivedAt
		}
		if observation.result.AssistantStreamID == "" {
			observation.result.AssistantStreamID = eventStreamID(event)
		}
		return nil
	}
	terminalMessage, hasTerminalError := peerEventError(event)
	if hasTerminalError && event.Type != peerStreamEventTypeEos &&
		!isAssistantTextDoneEvent(event) && !isTranscriptDoneEvent(event) {
		return fmt.Errorf("peer event error: %s", terminalMessage)
	}
	if !observation.textInterruptedAt.IsZero() && label == "assistant" && event.Kind == eventpb.StreamKind_STREAM_KIND_TEXT && (event.Text != nil || event.Type == peerStreamEventTypeBos || event.Type == peerStreamEventTypeTextDone) {
		return fmt.Errorf("assistant event continued after interrupted terminal: stream=%q type=%s", eventStreamID(event), event.Type)
	}
	switch label {
	case "transcript":
		if observation.result.TranscriptStreamID == "" {
			observation.result.TranscriptStreamID = eventStreamID(event)
		}
		if event.Text != nil && strings.TrimSpace(*event.Text) != "" {
			observation.transcript = mergeTranscriptText(observation.transcript, *event.Text)
			observation.result.EventCount++
		}
		if isTranscriptDoneEvent(event) {
			observation.result.TranscriptDone = true
		}
	case "assistant":
		if observation.result.AssistantStreamID == "" {
			observation.result.AssistantStreamID = eventStreamID(event)
		}
		if event.Type == peerStreamEventTypeBos {
			observation.assistantBOSCount++
			observation.result.AssistantBOS = observation.assistantBOSCount
			if observation.assistantBOSCount == 2 {
				observation.audioEpoch = receivedAt
				observation.result.AudioEpochAt = receivedAt
			}
		}
		if event.Text != nil && strings.TrimSpace(*event.Text) != "" {
			observation.assistant.WriteString(*event.Text)
			observation.result.EventCount++
		}
		if isAssistantTextDoneEvent(event) {
			observation.assistantTextDone = true
			observation.result.AssistantTextDone = true
			if strings.TrimSpace(observation.assistant.String()) == "" {
				return fmt.Errorf("assistant text response is empty")
			}
			if !observation.result.InputDoneAt.IsZero() && observation.audioEpoch.IsZero() {
				observation.result.CompletedAt = receivedAt
			}
		}
		if event.Type == peerStreamEventTypeEos {
			if !observation.assistantTextDone {
				return nil
			}
			observation.assistantAudioDone = true
			observation.result.AssistantAudioDone = true
			observation.result.CompletedAt = receivedAt
		}
	}
	if hasTerminalError {
		observation.terminalErrors = append(observation.terminalErrors, terminalMessage)
		if event.Type == peerStreamEventTypeEos {
			return fmt.Errorf("peer terminal error: %s", strings.Join(observation.terminalErrors, "; "))
		}
	}
	return nil
}

func workflowConcurrencyAudibleOpusPacket(decoder *opus.Decoder, packet []byte) (bool, error) {
	if decoder == nil || len(packet) == 0 {
		return false, nil
	}
	decoded, err := decoder.Decode(packet, 16000*120/1000, false)
	if err != nil {
		return false, err
	}
	const audibleRMSThreshold = int64(96)
	var sumSquares int64
	for _, sample := range decoded {
		value := int64(sample)
		sumSquares += value * value
	}
	return len(decoded) != 0 && sumSquares > audibleRMSThreshold*audibleRMSThreshold*int64(len(decoded)), nil
}

func workflowConcurrencyAudioTurn(observations []workflowConcurrencyTurnObservation, receivedAt time.Time) int {
	for index := len(observations) - 1; index >= 0; index-- {
		observation := &observations[index]
		if observation.audioEpoch.IsZero() || receivedAt.IsZero() || receivedAt.Before(observation.audioEpoch) {
			continue
		}
		return index
	}
	return -1
}

func workflowConcurrencyAudioAfterInterruption(observation *workflowConcurrencyTurnObservation, receivedAt time.Time) bool {
	return observation != nil && !observation.audioInterruptedAt.IsZero() &&
		!receivedAt.IsZero() && !receivedAt.Before(observation.audioInterruptedAt)
}

func validateWorkflowConcurrencyTurns(
	observations []workflowConcurrencyTurnObservation,
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
) error {
	for index := range observations {
		observation := &observations[index]
		if spec.KeepRealtimeInputOpen && observation.result.InputEOSSent {
			return fmt.Errorf("turn %d realtime input sent client EOS", index+1)
		}
		text := strings.TrimSpace(observation.assistant.String())
		if spec.RequireText && text == "" {
			return fmt.Errorf("turn %d Assistant text is empty", index+1)
		}
		if scenario == workflowConcurrencyInterrupt && index < len(observations)-1 {
			if !observation.cutoverSent {
				return fmt.Errorf("turn %d was not cut over", index+1)
			}
			if !observation.assistantTextDone && observation.result.InterruptedText != 1 {
				return fmt.Errorf("turn %d text route neither completed nor interrupted: interrupted terminals=%d", index+1, observation.result.InterruptedText)
			}
			if spec.RequireAudio && !observation.assistantAudioDone && observation.result.InterruptedAudio != 1 {
				return fmt.Errorf("turn %d audio route neither completed nor interrupted: interrupted terminals=%d", index+1, observation.result.InterruptedAudio)
			}
			continue
		}
		if !observation.assistantTextDone {
			return fmt.Errorf("turn %d Assistant text terminal is missing", index+1)
		}
		if spec.RequireAudio {
			if observation.audioEpoch.IsZero() || observation.result.AudioPackets == 0 || !observation.assistantAudioDone {
				return fmt.Errorf("turn %d Assistant audio incomplete: epoch=%t packets=%d eos=%t", index+1, !observation.audioEpoch.IsZero(), observation.result.AudioPackets, observation.assistantAudioDone)
			}
			if strings.TrimSpace(observation.transcript) == "" || !observation.result.TranscriptDone {
				return fmt.Errorf("turn %d transcript incomplete", index+1)
			}
		}
		if len(observation.terminalErrors) != 0 {
			return fmt.Errorf("turn %d peer terminal error: %s", index+1, strings.Join(observation.terminalErrors, "; "))
		}
	}
	return nil
}

func verifyWorkflowConcurrencyRuntime(ctx context.Context, lane *workflowConcurrencyLane, cutover int) error {
	state, err := lane.Client.GetServerRunWorkspace(ctx, fmt.Sprintf("workflow-concurrency.cutover.%02d", cutover))
	if err != nil {
		return fmt.Errorf("cutover %d get runtime: %w", cutover, err)
	}
	if state.RuntimeState != rpcapi.PeerRunStatusStateRunning || state.WorkspaceName != lane.Config.Workspace {
		return fmt.Errorf("cutover %d runtime state=%s workspace=%q, want running %q", cutover, state.RuntimeState, state.WorkspaceName, lane.Config.Workspace)
	}
	if state.StartedAt == nil || state.StartedAt.IsZero() || !state.StartedAt.Equal(lane.StartedAt) {
		return fmt.Errorf("cutover %d runtime StartedAt=%v, want %s", cutover, state.StartedAt, lane.StartedAt.Format(time.RFC3339Nano))
	}
	lane.Result.RuntimeChecks = append(lane.Result.RuntimeChecks, *state.StartedAt)
	return nil
}
