package giztest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Fixed v1 relay safety limits; the schema deliberately exposes no tuning
// fields. The event limit applies per completed turn — voice-enabled
// Workspaces stream hundreds of Opus packets per response, so a whole-relay
// event total would not survive real dialogues — while the byte limits bound
// the whole relay.
const (
	relayMaxTurnEvents = 4096
	relayMaxTextBytes  = 1 << 20
	relayMaxAudioBytes = 16 << 20
)

// relayStream is the PeerStream surface the relay drives. *gizcli.PeerStream
// implements it; tests substitute in-memory fakes.
type relayStream interface {
	Push(ctx context.Context, chunk *genx.MessageChunk) error
	Next() (*genx.MessageChunk, error)
	Close() error
}

type relayMinMax struct {
	min, max int64
	seen     bool
}

type relayTimeoutError struct{ message string }

func (e *relayTimeoutError) Error() string { return e.message }
func (e *relayTimeoutError) Unwrap() error { return context.DeadlineExceeded }

func (m *relayMinMax) observe(v int64) {
	if !m.seen || v < m.min {
		m.min = v
	}
	if !m.seen || v > m.max {
		m.max = v
	}
	m.seen = true
}

func (m *relayMinMax) object() map[string]any {
	if !m.seen {
		return map[string]any{"min": int64(0), "max": int64(0)}
	}
	return map[string]any{"min": m.min, "max": m.max}
}

type relaySide struct {
	name       string
	stream     relayStream
	next       <-chan nextPeerStreamResult
	readerDone <-chan struct{}

	everActive     bool
	streamsSeen    map[string]bool
	discardStreams map[string]bool
	turns          int
	firstText      relayMinMax
	textRunes      relayMinMax
	firstAudio     relayMinMax
	audioBytes     relayMinMax
	texts          []string

	turnStarted     time.Time
	turnFirstMS     int64
	turnRunes       int
	turnAudio       int
	turnTexts       []string
	turnSawText     bool
	turnSawAudio    bool
	forwardID       string
	forwardBegan    bool
	forwardMIME     string
	terminalPackets [][]byte
	terminalBytes   int
}

// streamOrigin classifies a chunk's stream against the streams this side has
// already produced, for content-free failure diagnostics.
func (s *relaySide) streamOrigin(chunk *genx.MessageChunk) string {
	id := ""
	if chunk.Ctrl != nil {
		id = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	if id == "" {
		return "no stream id"
	}
	for seen := range s.streamsSeen {
		if streamIDMatches(id, seen) || streamIDMatches(seen, id) {
			return "continuation of an earlier response stream"
		}
	}
	return "new response stream"
}

func (s *relaySide) observeStream(chunk *genx.MessageChunk) {
	if chunk.Ctrl == nil {
		return
	}
	if id := strings.TrimSpace(chunk.Ctrl.StreamID); id != "" {
		s.streamsSeen[id] = true
	}
}

// markDiscard records a stream whose events never participate in the relay: a
// self-start reply that began before this side's first turn, or a response the
// runtime reports as interrupted. Later events on the stream stay consume-only.
func (s *relaySide) markDiscard(chunk *genx.MessageChunk) {
	if chunk.Ctrl == nil {
		return
	}
	if id := strings.TrimSpace(chunk.Ctrl.StreamID); id != "" {
		s.discardStreams[id] = true
	}
}

func (s *relaySide) isDiscardStream(chunk *genx.MessageChunk) bool {
	if chunk.Ctrl == nil {
		return false
	}
	id := strings.TrimSpace(chunk.Ctrl.StreamID)
	if id == "" {
		return false
	}
	for seen := range s.discardStreams {
		if streamIDMatches(id, seen) || streamIDMatches(seen, id) {
			return true
		}
	}
	return false
}

func (s *relaySide) resetTurn() {
	s.turnFirstMS = -1
	s.turnRunes = 0
	s.turnAudio = 0
	s.turnTexts = nil
	s.turnSawText = false
	s.turnSawAudio = false
	s.forwardID = ""
	s.forwardBegan = false
	s.forwardMIME = ""
}

func invokeWorkspaceRelay(ctx context.Context, clients *clientSet, step Step, input any, audioCaptureMaxBytes int, fullEvidence bool, observers ...audioObserver) (operationResult, error) {
	op := step.WorkspaceRelay
	if op == nil {
		return operationResult{}, fmt.Errorf("workspace_relay operation required")
	}
	firstClient, err := clients.get(op.FirstClient)
	if err != nil {
		return operationResult{}, err
	}
	secondClient, err := clients.get(op.SecondClient)
	if err != nil {
		return operationResult{}, err
	}
	firstStream, err := firstClient.OpenPeerStream(64)
	if err != nil {
		return operationResult{}, fmt.Errorf("open %s PeerStream: %w", op.FirstClient, err)
	}
	secondStream, err := secondClient.OpenPeerStream(64)
	if err != nil {
		_ = firstStream.Close()
		return operationResult{}, fmt.Errorf("open %s PeerStream: %w", op.SecondClient, err)
	}
	return runWorkspaceRelayWithEvidence(ctx, op, firstStream, secondStream, input, audioCaptureMaxBytes, fullEvidence, observers...)
}

// runWorkspaceRelay owns the paired streams for one bounded relay: it pushes
// the initial input to the first side, alternates assistant output from the
// active side into user input for the other side chunk by chunk, and stops at
// the terminal turn without forwarding it again.
func runWorkspaceRelay(ctx context.Context, op *WorkspaceRelayOperation, firstStream, secondStream relayStream, input any, audioCaptureMaxBytes int) (operationResult, error) {
	return runWorkspaceRelayWithEvidence(ctx, op, firstStream, secondStream, input, audioCaptureMaxBytes, false)
}

func runWorkspaceRelayWithEvidence(ctx context.Context, op *WorkspaceRelayOperation, firstStream, secondStream relayStream, input any, audioCaptureMaxBytes int, fullEvidence bool, observers ...audioObserver) (operationResult, error) {
	var observeAudio audioObserver
	if len(observers) > 0 {
		observeAudio = observers[0]
	}
	readerCtx, cancelReaders := context.WithCancel(ctx)
	sides := [2]*relaySide{
		{name: op.FirstClient, stream: firstStream},
		{name: op.SecondClient, stream: secondStream},
	}
	for _, side := range sides {
		side.next, side.readerDone = readRelayStream(readerCtx, side.stream)
		side.streamsSeen = map[string]bool{}
		side.discardStreams = map[string]bool{}
		side.resetTurn()
	}
	defer func() {
		for _, side := range sides {
			_ = side.stream.Close()
		}
		cancelReaders()
		for _, side := range sides {
			<-side.readerDone
		}
	}()
	if observeAudio != nil && op.Media == "audio" {
		audio, ok := input.([]byte)
		if !ok {
			return operationResult{}, fmt.Errorf("audio workspace_relay input must be in-memory Opus bytes")
		}
		packets, err := decodeOpusPackets(audio)
		if err != nil {
			return operationResult{}, err
		}
		if err := observeAudio(op.FirstClient, "user", packets); err != nil {
			return operationResult{}, fmt.Errorf("play relay user audio: %w", err)
		}
	}
	if err := pushRelayInput(ctx, op, sides[0].stream, input); err != nil {
		return operationResult{}, fmt.Errorf("push relay input to %s: %w", op.FirstClient, err)
	}
	active := 0
	sides[active].turnStarted = time.Now()
	sides[active].everActive = true
	completed, totalEvents, turnEvents, totalTextBytes, totalAudioBytes := 0, 0, 0, 0, 0
	started := time.Now()
	lastEventMS := int64(0)
	idleTimeout, _ := time.ParseDuration(op.IdleTimeout)
	var idleTimer *time.Timer
	var idle <-chan time.Time
	armIdle := func() {
		if idleTimeout <= 0 {
			return
		}
		if idleTimer == nil {
			idleTimer = time.NewTimer(idleTimeout)
		} else {
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(idleTimeout)
		}
		idle = idleTimer.C
	}
	if idleTimeout > 0 {
		defer idleTimerStop(&idleTimer)
		armIdle()
	}
	failureEvidence := func(deadline string) map[string]any {
		evidence := relayEvidence(op, sides, nil, completed, totalEvents, totalTextBytes, totalAudioBytes, fullEvidence)
		evidence["active_client"] = sides[active].name
		evidence["active_turn"] = completed + 1
		evidence["last_event_ms"] = lastEventMS
		evidence["observed_text"] = sides[active].turnSawText
		evidence["observed_audio"] = sides[active].turnSawAudio
		if idleTimeout > 0 {
			evidence["idle_timeout_ms"] = idleTimeout.Milliseconds()
		}
		if deadline != "" {
			evidence["deadline"] = deadline
		}
		return evidence
	}
	fail := func(side *relaySide, deadline, format string, args ...any) (operationResult, error) {
		detail := fmt.Sprintf(format, args...)
		message := fmt.Sprintf("workspace_relay client %s turn %d: %s", side.name, completed+1, detail)
		if deadline == "idle_timeout" {
			return operationResult{evidence: failureEvidence(deadline)}, &relayTimeoutError{message: message}
		}
		return operationResult{evidence: failureEvidence(deadline)}, errors.New(message)
	}
	var observedTurnPackets [][]byte
	defer func() {
		if observeAudio != nil && len(observedTurnPackets) > 0 {
			_ = observeAudio(sides[active].name, "assistant", observedTurnPackets)
		}
	}()
	for {
		var result nextPeerStreamResult
		var from int
		select {
		case result = <-sides[0].next:
			from = 0
		case result = <-sides[1].next:
			from = 1
		case <-idle:
			return fail(sides[active], "idle_timeout", "idle timeout exceeded after %s (media=%s terminal_media=%s)", op.IdleTimeout, op.Media, relayTerminalMediaName(op))
		case <-ctx.Done():
			deadline := "cancelled"
			if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
				deadline = "timeout"
			}
			return operationResult{evidence: failureEvidence(deadline)}, fmt.Errorf("workspace_relay cancelled at turn %d with %s active: %w", completed+1, sides[active].name, context.Cause(ctx))
		}
		side, receiver := sides[from], sides[1-from]
		if result.err != nil {
			if result.err == io.EOF {
				return fail(side, "", "PeerStream closed before relay completion")
			}
			return fail(side, "", "PeerStream failed: %v", result.err)
		}
		chunk := result.chunk
		if chunk == nil {
			return fail(side, "", "PeerStream returned an empty chunk")
		}
		totalEvents++
		turnEvents++
		if turnEvents > relayMaxTurnEvents {
			return fail(side, "", "exceeded the fixed %d-event relay turn limit", relayMaxTurnEvents)
		}
		label := ""
		interrupted := false
		if chunk.Ctrl != nil {
			label = strings.TrimSpace(chunk.Ctrl.Label)
			if ctrlErr := strings.TrimSpace(chunk.Ctrl.Error); ctrlErr != "" {
				// "interrupted" marks a response cancelled by newer input — for
				// example a self-start reply cut off by the first forwarded
				// turn — and is benign, matching peer_stream semantics. Any
				// other stream error is terminal.
				if !strings.EqualFold(ctrlErr, "interrupted") {
					return fail(side, "", "terminal stream error")
				}
				interrupted = true
			}
		}
		if label == "" {
			switch chunk.Part.(type) {
			case genx.Text, *genx.Blob:
				label = "assistant"
			}
		}
		if side.isDiscardStream(chunk) {
			continue
		}
		isActive := from == active
		final := completed == op.MaxTurns-1
		if label == "assistant" {
			// A stream that begins before this side's first turn is a
			// self-start reply, and an interrupted stream never completes a
			// turn: every later event on such a stream is consumed without
			// forwarding, metrics, or turn completion.
			if interrupted || (!isActive && !side.everActive) {
				side.markDiscard(chunk)
			}
			if interrupted || side.isDiscardStream(chunk) {
				continue
			}
			if isActive {
				side.observeStream(chunk)
			}
		}
		if isActive {
			lastEventMS = time.Since(started).Milliseconds()
			armIdle()
			if label == "assistant" {
				mimeType, _ := chunk.MIMEType()
				side.turnSawText = side.turnSawText || mimeType == "text/plain"
				side.turnSawAudio = side.turnSawAudio || relayOpusMIME(mimeType)
			}
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			text := string(part)
			if label != "assistant" || strings.TrimSpace(text) == "" {
				break
			}
			if !isActive {
				if op.Media == "text" {
					return fail(side, "", "unexpected text output from the inactive side (%s)", side.streamOrigin(chunk))
				}
				break
			}
			totalTextBytes += len(text)
			if totalTextBytes > relayMaxTextBytes {
				return fail(side, "", "exceeded the fixed %d-byte relay text limit", relayMaxTextBytes)
			}
			side.turnTexts = append(side.turnTexts, text)
			if op.Media != "text" {
				break // audio relays retain assistant text without forwarding it
			}
			if side.turnFirstMS < 0 {
				side.turnFirstMS = time.Since(side.turnStarted).Milliseconds()
			}
			side.turnRunes += utf8.RuneCountInString(text)
			if final && isActive {
				break
			}
			if err := forwardRelayText(ctx, side, receiver, text); err != nil {
				return fail(receiver, "", "forward text failed: %v", err)
			}
		case *genx.Blob:
			if label != "assistant" {
				break
			}
			if isActive && (op.Media == "audio" || relayTerminalMediaName(op) == "audio") && !relayOpusMIME(part.MIMEType) {
				return fail(side, "", "unsupported relay media type")
			}
			if len(part.Data) == 0 {
				break
			}
			if isActive && observeAudio != nil && relayOpusMIME(part.MIMEType) {
				packets, err := decodeOpusPackets(part.Data)
				if err != nil {
					return fail(side, "", "decode assistant audio failed: %v", err)
				}
				observedTurnPackets = append(observedTurnPackets, packets...)
			}
			totalAudioBytes += len(part.Data)
			if totalAudioBytes > relayMaxAudioBytes {
				return fail(side, "", "exceeded the fixed %d-byte relay audio limit", relayMaxAudioBytes)
			}
			if op.Media != "audio" {
				break // text relays consume assistant audio without forwarding it
			}
			if !isActive {
				return fail(side, "", "unexpected audio output from the inactive side")
			}
			if side.turnFirstMS < 0 {
				side.turnFirstMS = time.Since(side.turnStarted).Milliseconds()
			}
			side.turnAudio += len(part.Data)
			if final && isActive {
				if audioCaptureMaxBytes > 0 {
					if side.terminalBytes > audioCaptureMaxBytes-len(part.Data) {
						return fail(side, "", "terminal audio exceeds the capture max_bytes %d", audioCaptureMaxBytes)
					}
					side.terminalBytes += len(part.Data)
					side.terminalPackets = append(side.terminalPackets, append([]byte(nil), part.Data...))
				}
				break
			}
			if err := forwardRelayAudio(ctx, side, receiver, part); err != nil {
				return fail(receiver, "", "forward audio failed: %v", err)
			}
		}
		if !chunk.IsEndOfStream() || label != "assistant" {
			continue
		}
		mimeType, _ := chunk.MIMEType()
		if !relayTerminalMedia(relayTerminalMediaName(op), mimeType) {
			continue
		}
		if !isActive {
			return fail(side, "", "unexpected terminal output from the inactive side")
		}
		if observeAudio != nil && len(observedTurnPackets) > 0 {
			packets := observedTurnPackets
			observedTurnPackets = nil
			if err := observeAudio(side.name, "assistant", packets); err != nil {
				return fail(side, "", "play assistant audio failed: %v", err)
			}
		}
		side.texts = append(side.texts, strings.Join(side.turnTexts, ""))
		completed++
		turnEvents = 0
		side.turns++
		if side.turnFirstMS >= 0 {
			if op.Media == "text" {
				side.firstText.observe(side.turnFirstMS)
			} else {
				side.firstAudio.observe(side.turnFirstMS)
			}
		}
		if op.Media == "text" {
			side.textRunes.observe(int64(side.turnRunes))
		} else {
			side.audioBytes.observe(int64(side.turnAudio))
		}
		if completed == op.MaxTurns {
			return relayResult(op, sides, side, completed, totalEvents, totalTextBytes, totalAudioBytes, fullEvidence)
		}
		if err := forwardRelayTerminal(ctx, op, side, receiver); err != nil {
			return fail(receiver, "", "forward terminal failed: %v", err)
		}
		side.resetTurn()
		receiver.resetTurn()
		receiver.turnStarted = time.Now()
		receiver.everActive = true
		active = 1 - from
		armIdle()
	}
}

func idleTimerStop(timer **time.Timer) {
	if *timer != nil {
		(*timer).Stop()
	}
}

func relayTerminalMediaName(op *WorkspaceRelayOperation) string {
	if op.TerminalMedia != "" {
		return op.TerminalMedia
	}
	return op.Media
}

func pushRelayInput(ctx context.Context, op *WorkspaceRelayOperation, stream relayStream, input any) error {
	streamID, err := generateValue("string")
	if err != nil {
		return err
	}
	id := streamID
	if op.Media == "text" {
		text, ok := input.(string)
		if !ok {
			return fmt.Errorf("text workspace_relay input must be string")
		}
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user", BeginOfStream: true}},
			{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user"}},
			{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user", EndOfStream: true}},
		} {
			if err := stream.Push(ctx, chunk); err != nil {
				return err
			}
		}
		return nil
	}
	audio, ok := input.([]byte)
	if !ok {
		return fmt.Errorf("audio workspace_relay input must be in-memory Opus bytes")
	}
	packets, err := decodeOpusPackets(audio)
	if err != nil {
		return err
	}
	for _, chunk := range audioInputChunks("push-to-talk", id, "audio/opus", packets) {
		if err := stream.Push(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

// forwardRelayText delivers one assistant fragment as receiving-side user
// input under a fresh receiving stream ID, beginning the user turn on the
// first eligible fragment.
func forwardRelayText(ctx context.Context, side, receiver *relaySide, text string) error {
	if !side.forwardBegan {
		streamID, err := generateValue("string")
		if err != nil {
			return err
		}
		side.forwardID = streamID
		side.forwardBegan = true
		begin := &genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user", BeginOfStream: true}}
		if err := receiver.stream.Push(ctx, begin); err != nil {
			return err
		}
	}
	chunk := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user"}}
	return receiver.stream.Push(ctx, chunk)
}

func forwardRelayAudio(ctx context.Context, side, receiver *relaySide, blob *genx.Blob) error {
	if !side.forwardBegan {
		streamID, err := generateValue("string")
		if err != nil {
			return err
		}
		side.forwardID = streamID
		side.forwardBegan = true
		side.forwardMIME = blob.MIMEType
		begin := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: blob.MIMEType}, Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user", BeginOfStream: true}}
		if err := receiver.stream.Push(ctx, begin); err != nil {
			return err
		}
	}
	chunk := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: blob.MIMEType, Data: append([]byte(nil), blob.Data...)}, Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user"}}
	return receiver.stream.Push(ctx, chunk)
}

// forwardRelayTerminal rewrites the source terminal event as the receiving
// user turn's own terminal fragment; an empty source turn still yields a
// complete, empty user turn.
func forwardRelayTerminal(ctx context.Context, op *WorkspaceRelayOperation, side, receiver *relaySide) error {
	if op.Media == "text" {
		if !side.forwardBegan {
			if err := forwardRelayText(ctx, side, receiver, ""); err != nil {
				return err
			}
		}
		end := &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user", EndOfStream: true}}
		return receiver.stream.Push(ctx, end)
	}
	if !side.forwardBegan {
		if err := forwardRelayAudio(ctx, side, receiver, &genx.Blob{MIMEType: "audio/opus"}); err != nil {
			return err
		}
	}
	end := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: side.forwardMIME}, Ctrl: &genx.StreamCtrl{StreamID: side.forwardID, Label: "user", EndOfStream: true}}
	return receiver.stream.Push(ctx, end)
}

func relayTerminalMedia(media, mimeType string) bool {
	if media == "text" {
		return mimeType == "text/plain"
	}
	return relayOpusMIME(mimeType)
}

// relayOpusMIME reports whether a MIME type names the Opus media an audio
// relay may forward: audio/opus, or audio/ogg with codecs=opus.
func relayOpusMIME(mimeType string) bool {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err != nil {
		return false
	}
	codecs, declared := params["codecs"]
	switch mediaType {
	case "audio/opus":
		// A codecs parameter that contradicts the media type is unsupported.
		return !declared || strings.EqualFold(strings.TrimSpace(codecs), "opus")
	case "audio/ogg":
		return declared && strings.EqualFold(strings.TrimSpace(codecs), "opus")
	}
	return false
}

func relayTurns(op *WorkspaceRelayOperation, sides [2]*relaySide, includeTexts bool) map[string]any {
	turns := map[string]any{}
	for _, side := range sides {
		entry := map[string]any{"count": side.turns}
		if op.Media == "text" {
			entry["first_text_ms"] = side.firstText.object()
			entry["text_runes"] = side.textRunes.object()
		} else {
			entry["first_audio_ms"] = side.firstAudio.object()
			entry["audio_bytes"] = side.audioBytes.object()
		}
		if includeTexts {
			texts := make([]any, len(side.texts))
			for i, text := range side.texts {
				texts[i] = text
			}
			entry["texts"] = texts
		}
		turns[side.name] = entry
	}
	return turns
}

func relayEvidence(op *WorkspaceRelayOperation, sides [2]*relaySide, terminal *relaySide, completed, events, textBytes, audioBytes int, full bool) map[string]any {
	evidence := map[string]any{
		"completed_turns": completed,
		"turns":           relayTurns(op, sides, full),
		"events":          events,
		"text_bytes":      textBytes,
		"audio_bytes":     audioBytes,
	}
	if terminal != nil {
		evidence["terminal_client"] = terminal.name
		if full {
			terminalObject := map[string]any{"client": terminal.name}
			if len(terminal.texts) > 0 && (op.Media == "text" || terminal.texts[len(terminal.texts)-1] != "") {
				terminalObject["text"] = terminal.texts[len(terminal.texts)-1]
			}
			evidence["terminal"] = terminalObject
		}
	}
	return evidence
}

// relayResult builds the assertion object with the terminal capture surface
// and report evidence selected by the caller's explicit evidence mode.
func relayResult(op *WorkspaceRelayOperation, sides [2]*relaySide, terminal *relaySide, completed, events, textBytes, audioBytes int, fullEvidence bool) (operationResult, error) {
	terminalObject := map[string]any{"client": terminal.name}
	if len(terminal.texts) > 0 && (op.Media == "text" || terminal.texts[len(terminal.texts)-1] != "") {
		terminalObject["text"] = terminal.texts[len(terminal.texts)-1]
	}
	if op.Media == "audio" && len(terminal.terminalPackets) > 0 {
		var audio bytes.Buffer
		if err := codecconv.OpusPacketsToOgg(&audio, int(opus.SampleRate16K), 1, terminal.terminalPackets); err != nil {
			return operationResult{evidence: relayEvidence(op, sides, terminal, completed, events, textBytes, audioBytes, fullEvidence)}, fmt.Errorf("encode terminal relay audio: %w", err)
		}
		terminalObject["audio"] = append([]byte(nil), audio.Bytes()...)
	}
	assertion := map[string]any{
		"completed_turns": completed,
		"terminal":        terminalObject,
		"turns":           relayTurns(op, sides, true),
		"events":          events,
		"text_bytes":      textBytes,
		"audio_bytes":     audioBytes,
	}
	evidence := relayEvidence(op, sides, terminal, completed, events, textBytes, audioBytes, fullEvidence)
	return operationResult{assertion: assertion, evidence: evidence}, nil
}

func readRelayStream(ctx context.Context, stream relayStream) (<-chan nextPeerStreamResult, <-chan struct{}) {
	next := make(chan nextPeerStreamResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
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
	return next, done
}
