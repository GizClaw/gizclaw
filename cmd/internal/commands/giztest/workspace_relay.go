package giztest

import (
	"bytes"
	"context"
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
	name   string
	stream relayStream
	next   <-chan nextPeerStreamResult

	everActive     bool
	streamsSeen    map[string]bool
	discardStreams map[string]bool
	turns          int
	firstText      relayMinMax
	textRunes      relayMinMax
	firstAudio     relayMinMax
	audioBytes     relayMinMax

	turnStarted     time.Time
	turnFirstMS     int64
	turnRunes       int
	turnAudio       int
	forwardID       string
	forwardBegan    bool
	forwardMIME     string
	terminalTexts   []string
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
	s.forwardID = ""
	s.forwardBegan = false
	s.forwardMIME = ""
}

func invokeWorkspaceRelay(ctx context.Context, clients *clientSet, step Step, input any, audioCaptureMaxBytes int) (operationResult, error) {
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
	defer func() { _ = firstStream.Close() }()
	secondStream, err := secondClient.OpenPeerStream(64)
	if err != nil {
		return operationResult{}, fmt.Errorf("open %s PeerStream: %w", op.SecondClient, err)
	}
	defer func() { _ = secondStream.Close() }()
	return runWorkspaceRelay(ctx, op, firstStream, secondStream, input, audioCaptureMaxBytes)
}

// runWorkspaceRelay owns the paired streams for one bounded relay: it pushes
// the initial input to the first side, alternates assistant output from the
// active side into user input for the other side chunk by chunk, and stops at
// the terminal turn without forwarding it again.
func runWorkspaceRelay(ctx context.Context, op *WorkspaceRelayOperation, firstStream, secondStream relayStream, input any, audioCaptureMaxBytes int) (operationResult, error) {
	sides := [2]*relaySide{
		{name: op.FirstClient, stream: firstStream},
		{name: op.SecondClient, stream: secondStream},
	}
	for _, side := range sides {
		side.next = readRelayStream(ctx, side.stream)
		side.streamsSeen = map[string]bool{}
		side.discardStreams = map[string]bool{}
		side.resetTurn()
	}
	if err := pushRelayInput(ctx, op, sides[0].stream, input); err != nil {
		return operationResult{}, fmt.Errorf("push relay input to %s: %w", op.FirstClient, err)
	}
	active := 0
	sides[active].turnStarted = time.Now()
	sides[active].everActive = true
	completed, totalEvents, turnEvents, totalTextBytes, totalAudioBytes := 0, 0, 0, 0, 0
	fail := func(side *relaySide, format string, args ...any) (operationResult, error) {
		detail := fmt.Sprintf(format, args...)
		return operationResult{}, fmt.Errorf("workspace_relay client %s turn %d: %s", side.name, completed+1, detail)
	}
	for {
		var result nextPeerStreamResult
		var from int
		select {
		case result = <-sides[0].next:
			from = 0
		case result = <-sides[1].next:
			from = 1
		case <-ctx.Done():
			return operationResult{}, fmt.Errorf("workspace_relay cancelled at turn %d with %s active: %w", completed+1, sides[active].name, context.Cause(ctx))
		}
		side, receiver := sides[from], sides[1-from]
		if result.err != nil {
			if result.err == io.EOF {
				return fail(side, "PeerStream closed before relay completion")
			}
			return fail(side, "PeerStream failed: %v", result.err)
		}
		chunk := result.chunk
		if chunk == nil {
			return fail(side, "PeerStream returned an empty chunk")
		}
		totalEvents++
		turnEvents++
		if turnEvents > relayMaxTurnEvents {
			return fail(side, "exceeded the fixed %d-event relay turn limit", relayMaxTurnEvents)
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
					return fail(side, "terminal stream error")
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
		switch part := chunk.Part.(type) {
		case genx.Text:
			text := string(part)
			if label != "assistant" || strings.TrimSpace(text) == "" {
				break
			}
			if op.Media != "text" {
				break // audio relays consume assistant text without forwarding it
			}
			if !isActive {
				return fail(side, "unexpected text output from the inactive side (%s)", side.streamOrigin(chunk))
			}
			totalTextBytes += len(text)
			if totalTextBytes > relayMaxTextBytes {
				return fail(side, "exceeded the fixed %d-byte relay text limit", relayMaxTextBytes)
			}
			if side.turnFirstMS < 0 {
				side.turnFirstMS = time.Since(side.turnStarted).Milliseconds()
			}
			side.turnRunes += utf8.RuneCountInString(text)
			if final && isActive {
				side.terminalTexts = append(side.terminalTexts, text)
				break
			}
			if err := forwardRelayText(ctx, side, receiver, text); err != nil {
				return fail(receiver, "forward text failed: %v", err)
			}
		case *genx.Blob:
			if label != "assistant" {
				break
			}
			if op.Media == "audio" && isActive && !relayOpusMIME(part.MIMEType) {
				return fail(side, "unsupported relay media type")
			}
			if len(part.Data) == 0 {
				break
			}
			totalAudioBytes += len(part.Data)
			if totalAudioBytes > relayMaxAudioBytes {
				return fail(side, "exceeded the fixed %d-byte relay audio limit", relayMaxAudioBytes)
			}
			if op.Media != "audio" {
				break // text relays consume assistant audio without forwarding it
			}
			if !isActive {
				return fail(side, "unexpected audio output from the inactive side")
			}
			if side.turnFirstMS < 0 {
				side.turnFirstMS = time.Since(side.turnStarted).Milliseconds()
			}
			side.turnAudio += len(part.Data)
			if final && isActive {
				if audioCaptureMaxBytes > 0 {
					if side.terminalBytes > audioCaptureMaxBytes-len(part.Data) {
						return fail(side, "terminal audio exceeds the capture max_bytes %d", audioCaptureMaxBytes)
					}
					side.terminalBytes += len(part.Data)
					side.terminalPackets = append(side.terminalPackets, append([]byte(nil), part.Data...))
				}
				break
			}
			if err := forwardRelayAudio(ctx, side, receiver, part); err != nil {
				return fail(receiver, "forward audio failed: %v", err)
			}
		}
		if !chunk.IsEndOfStream() || label != "assistant" {
			continue
		}
		mimeType, _ := chunk.MIMEType()
		if !relayTerminalMedia(op.Media, mimeType) {
			continue
		}
		if !isActive {
			return fail(side, "unexpected terminal output from the inactive side")
		}
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
			return relayResult(op, sides, side, completed, totalEvents, totalTextBytes, totalAudioBytes)
		}
		if err := forwardRelayTerminal(ctx, op, side, receiver); err != nil {
			return fail(receiver, "forward terminal failed: %v", err)
		}
		side.resetTurn()
		receiver.resetTurn()
		receiver.turnStarted = time.Now()
		receiver.everActive = true
		active = 1 - from
	}
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
	switch mediaType {
	case "audio/opus":
		return true
	case "audio/ogg":
		return strings.EqualFold(strings.TrimSpace(params["codecs"]), "opus")
	}
	return false
}

// relayResult builds the assertion object with the terminal capture surface
// and the content-free evidence map that reaches the report.
func relayResult(op *WorkspaceRelayOperation, sides [2]*relaySide, terminal *relaySide, completed, events, textBytes, audioBytes int) (operationResult, error) {
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
		turns[side.name] = entry
	}
	terminalObject := map[string]any{"client": terminal.name}
	if op.Media == "text" {
		terminalObject["text"] = strings.Join(terminal.terminalTexts, "")
	} else if len(terminal.terminalPackets) > 0 {
		var audio bytes.Buffer
		if err := codecconv.OpusPacketsToOgg(&audio, int(opus.SampleRate16K), 1, terminal.terminalPackets); err != nil {
			return operationResult{}, fmt.Errorf("encode terminal relay audio: %w", err)
		}
		terminalObject["audio"] = append([]byte(nil), audio.Bytes()...)
	}
	assertion := map[string]any{
		"completed_turns": completed,
		"terminal":        terminalObject,
		"turns":           turns,
		"events":          events,
		"text_bytes":      textBytes,
		"audio_bytes":     audioBytes,
	}
	evidence := map[string]any{
		"completed_turns": completed,
		"terminal_client": terminal.name,
		"turns":           turns,
		"events":          events,
		"text_bytes":      textBytes,
		"audio_bytes":     audioBytes,
	}
	return operationResult{assertion: assertion, evidence: evidence}, nil
}

func readRelayStream(ctx context.Context, stream relayStream) <-chan nextPeerStreamResult {
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
