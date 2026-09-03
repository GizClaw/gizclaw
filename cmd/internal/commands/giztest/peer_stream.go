package giztest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
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

type peerStreamSession struct {
	client   string
	stream   peerStream
	streamID string
	next     <-chan nextPeerStreamResult
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.RWMutex
	arrivals *peerStreamFirstResponseArrivals
}

func newPeerStreamSession(client string, stream peerStream) *peerStreamSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &peerStreamSession{client: client, stream: stream, ctx: ctx, cancel: cancel}
}

func (s *peerStreamSession) startReader() {
	if s.next == nil {
		s.next = readPeerStreamObserved(s.ctx, s.stream, s.observeArrival)
	}
}

func (s *peerStreamSession) observeArrival(chunk *genx.MessageChunk, receivedAt time.Time) {
	s.mu.RLock()
	arrivals := s.arrivals
	s.mu.RUnlock()
	if arrivals != nil {
		arrivals.observe(chunk, receivedAt)
	}
}

func (s *peerStreamSession) setArrivals(arrivals *peerStreamFirstResponseArrivals) {
	s.mu.Lock()
	s.arrivals = arrivals
	s.mu.Unlock()
}

func (s *peerStreamSession) Close() error {
	s.cancel()
	return s.stream.Close()
}

type peerStreamSessions struct {
	items map[string]*peerStreamSession
}

func newPeerStreamSessions() *peerStreamSessions {
	return &peerStreamSessions{items: make(map[string]*peerStreamSession)}
}

func (s *peerStreamSessions) add(name string, session *peerStreamSession) error {
	if _, exists := s.items[name]; exists {
		return fmt.Errorf("peer_stream session %q is already open", name)
	}
	s.items[name] = session
	return nil
}

func (s *peerStreamSessions) take(name, client string) (*peerStreamSession, error) {
	session, exists := s.items[name]
	if !exists {
		return nil, fmt.Errorf("peer_stream session %q is not open", name)
	}
	if session.client != client {
		return nil, fmt.Errorf("peer_stream session %q belongs to client %q, not %q", name, session.client, client)
	}
	delete(s.items, name)
	return session, nil
}

func (s *peerStreamSessions) Close() error {
	var errs []error
	for name, session := range s.items {
		if err := session.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close peer_stream session %q: %w", name, err))
		}
		delete(s.items, name)
	}
	return errors.Join(errs...)
}

// audioObserver receives one user or assistant Opus packet as it arrives.
// end marks the end of that logical utterance and flushes any jitter buffer.
// It is nil for normal Giztest runs; play mode installs the only implementation.
type audioObserver func(client, role string, packet []byte, end bool) error

type peerAudioPacing struct {
	firstAt         time.Time
	lastAt          time.Time
	gaps            []time.Duration
	packetDurations []time.Duration
}

func (p *peerAudioPacing) observe(receivedAt time.Time, packets [][]byte) {
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		if p.firstAt.IsZero() {
			p.firstAt = receivedAt
		} else {
			p.gaps = append(p.gaps, receivedAt.Sub(p.lastAt))
		}
		p.lastAt = receivedAt
		ticks := codecconv.OpusPacketRTPTicks(packet)
		p.packetDurations = append(p.packetDurations, time.Duration(ticks)*time.Second/48000)
	}
}

func (p *peerAudioPacing) summary() map[string]any {
	if len(p.packetDurations) == 0 {
		return nil
	}
	audioDuration := time.Duration(0)
	for _, duration := range p.packetDurations {
		audioDuration += duration
	}
	result := map[string]any{
		"packets":  len(p.packetDurations),
		"audio_ms": audioDuration.Milliseconds(),
	}
	if len(p.gaps) == 0 {
		return result
	}
	targetSpan := audioDuration - p.packetDurations[len(p.packetDurations)-1]
	receiveSpan := p.lastAt.Sub(p.firstAt)
	maximumGap := time.Duration(0)
	for _, gap := range p.gaps {
		maximumGap = max(maximumGap, gap)
	}
	sortedGaps := slices.Clone(p.gaps)
	slices.Sort(sortedGaps)
	p95Gap := sortedGaps[(len(sortedGaps)*95+99)/100-1]
	drift := receiveSpan - targetSpan
	absDrift := max(drift, -drift)
	intervals := float64(len(p.gaps))
	result["target_span_ms"] = targetSpan.Milliseconds()
	result["receive_span_ms"] = receiveSpan.Milliseconds()
	result["mean_packet_ms"] = float64(targetSpan) / intervals / float64(time.Millisecond)
	result["mean_interval_ms"] = float64(receiveSpan) / intervals / float64(time.Millisecond)
	result["p95_interval_ms"] = float64(p95Gap) / float64(time.Millisecond)
	result["max_interval_ms"] = float64(maximumGap) / float64(time.Millisecond)
	result["drift_ms"] = float64(drift) / float64(time.Millisecond)
	result["absolute_drift_ms"] = float64(absDrift) / float64(time.Millisecond)
	result["buffer_surplus_ms"] = float64(-drift) / float64(time.Millisecond)
	return result
}

func observeAudioPackets(observer audioObserver, client, role string, packets [][]byte) error {
	if observer == nil {
		return nil
	}
	for _, packet := range packets {
		if err := observer(client, role, packet, false); err != nil {
			return err
		}
	}
	return observer(client, role, nil, true)
}

func openClientPeerStream(client *gizcli.Client) peerStreamOpener {
	return func() (peerStream, error) { return client.OpenPeerStream(64) }
}

func invokePeerStream(ctx context.Context, client *gizcli.Client, open peerStreamOpener, step Step, input any, audioCaptureMaxBytes int, observers ...audioObserver) (operationResult, error) {
	stream, err := open()
	if err != nil {
		return operationResult{}, err
	}
	defer func() { _ = stream.Close() }()
	if step.PeerStream != nil && step.PeerStream.Mode == "listen" {
		return listenPeerStream(ctx, stream, step, audioCaptureMaxBytes, observers...)
	}
	return invokePeerStreamOnStream(ctx, client, open, stream, nil, "", step, input, audioCaptureMaxBytes, nil, observers...)
}

// maxListenTextFragments bounds the text fragments a listen step retains.
const maxListenTextFragments = 1024

// listenPeerStream drives the receive-only listen mode: it pushes nothing and
// records every chunk the PeerStream delivers for the declared duration. Any
// Opus blob counts as received audio regardless of its label, because SFU
// downlink labels identify the remote participant rather than the assistant.
// Receiving no audio is a valid outcome that the document asserts on.
func listenPeerStream(ctx context.Context, stream peerStream, step Step, audioCaptureMaxBytes int, observers ...audioObserver) (operationResult, error) {
	op := step.PeerStream
	duration, err := time.ParseDuration(op.Duration)
	if err != nil || duration <= 0 || duration > maxListenDuration {
		return operationResult{}, fmt.Errorf("invalid listen duration %q", op.Duration)
	}
	var observeAudio audioObserver
	if len(observers) > 0 {
		observeAudio = observers[0]
	}
	started := time.Now()
	window := time.NewTimer(duration)
	defer window.Stop()
	next := readPeerStream(ctx, stream, nil)
	var texts []string
	var captured [][]byte
	var pacing peerAudioPacing
	audioBytes, packets, events, droppedText := 0, 0, 0, 0
	streams := make(map[string]struct{})
	var firstAudioMS, lastEventMS int64
	firstAudioObserved := false
	observationOpen := false
	defer func() {
		if observeAudio != nil && observationOpen {
			_ = observeAudio(step.Client, "assistant", nil, true)
		}
	}()
	evidence := func() map[string]any {
		return map[string]any{
			"mode": "listen", "duration_ms": duration.Milliseconds(), "events": events, "audio_bytes": audioBytes,
			"packets": packets, "streams": len(streams), "first_audio_ms": firstAudioMS, "last_event_ms": lastEventMS,
		}
	}
	counters := func() string {
		return fmt.Sprintf("events=%d audio_bytes=%d packets=%d streams=%d", events, audioBytes, packets, len(streams))
	}
	finish := func() (operationResult, error) {
		if observeAudio != nil && observationOpen {
			observationOpen = false
			if err := observeAudio(step.Client, "assistant", nil, true); err != nil {
				return operationResult{}, fmt.Errorf("play received audio: %w", err)
			}
		}
		object := map[string]any{
			"text": texts, "audio_bytes": audioBytes, "packets": packets, "events": events, "streams": len(streams),
			"first_audio_ms": firstAudioMS, "last_event_ms": lastEventMS, "duration_ms": duration.Milliseconds(),
			"listened_ms": time.Since(started).Milliseconds(), "dropped_text": droppedText,
		}
		result := evidence()
		if summary := pacing.summary(); summary != nil {
			object["audio_pacing"] = summary
			result["audio_pacing"] = summary
		}
		if len(captured) > 0 {
			var audio bytes.Buffer
			if err := codecconv.OpusPacketsToOgg(&audio, int(opus.SampleRate16K), 1, captured); err != nil {
				return operationResult{}, fmt.Errorf("encode received audio evidence: %w", err)
			}
			object["audio"] = append([]byte(nil), audio.Bytes()...)
		}
		return operationResult{assertion: object, saved: object, evidence: result}, nil
	}
	for {
		select {
		case <-window.C:
			return finish()
		case <-ctx.Done():
			cause := context.Cause(ctx)
			deadline := "cancelled"
			if errors.Is(cause, context.DeadlineExceeded) {
				deadline = "timeout"
			}
			failed := evidence()
			failed["deadline"] = deadline
			return operationResult{evidence: failed}, fmt.Errorf("%w (deadline=%s %s)", cause, deadline, counters())
		case result := <-next:
			if result.err != nil {
				if result.err == io.EOF {
					return operationResult{evidence: evidence()}, fmt.Errorf("peer_stream closed before the listen duration elapsed (%s)", counters())
				}
				return operationResult{evidence: evidence()}, result.err
			}
			if result.chunk == nil {
				return operationResult{evidence: evidence()}, fmt.Errorf("peer_stream returned an empty chunk")
			}
			events++
			elapsed := result.receivedAt.Sub(started)
			lastEventMS = elapsed.Milliseconds()
			if result.chunk.Ctrl != nil {
				if id := strings.TrimSpace(result.chunk.Ctrl.StreamID); id != "" {
					streams[id] = struct{}{}
				}
				if terminalError := peerStreamTerminalError(result.chunk); terminalError != "" {
					failed := evidence()
					failed["terminal_errors"] = 1
					return operationResult{evidence: failed}, fmt.Errorf("peer_stream terminal error: %s", terminalError)
				}
			}
			switch part := result.chunk.Part.(type) {
			case genx.Text:
				if strings.TrimSpace(string(part)) == "" {
					continue
				}
				if len(texts) >= maxListenTextFragments {
					droppedText++
					continue
				}
				texts = append(texts, string(part))
			case *genx.Blob:
				if len(part.Data) == 0 || !relayOpusMIME(part.MIMEType) {
					continue
				}
				opusPackets, err := decodeOpusPackets(part.Data)
				if err != nil {
					return operationResult{evidence: evidence()}, fmt.Errorf("decode received audio: %w", err)
				}
				pacing.observe(result.receivedAt, opusPackets)
				packets += len(opusPackets)
				if !firstAudioObserved {
					firstAudioObserved = true
					firstAudioMS = max(int64(1), elapsed.Milliseconds())
				}
				if observeAudio != nil {
					for _, packet := range opusPackets {
						if err := observeAudio(step.Client, "assistant", packet, false); err != nil {
							return operationResult{evidence: evidence()}, fmt.Errorf("play received audio: %w", err)
						}
						observationOpen = true
					}
				}
				if audioCaptureMaxBytes > 0 {
					if audioBytes > audioCaptureMaxBytes-len(part.Data) {
						return operationResult{evidence: evidence()}, fmt.Errorf("captured audio exceeds output variable max_bytes %d", audioCaptureMaxBytes)
					}
					captured = append(captured, opusPackets...)
				}
				audioBytes += len(part.Data)
			}
		}
	}
}

// opusPacketsDuration sums the RTP clock of every packet.
func opusPacketsDuration(packets [][]byte) time.Duration {
	total := time.Duration(0)
	for _, packet := range packets {
		total += time.Duration(codecconv.OpusPacketRTPTicks(packet)) * time.Second / 48000
	}
	return total
}

func invokePeerStreamWithSessions(ctx context.Context, client *gizcli.Client, open peerStreamOpener, sessions *peerStreamSessions, step Step, input any, audioCaptureMaxBytes int, observers ...audioObserver) (operationResult, error) {
	op := step.PeerStream
	if op == nil || (!op.KeepOpen && op.AwaitRearm == "") {
		return invokePeerStream(ctx, client, open, step, input, audioCaptureMaxBytes, observers...)
	}
	if op.KeepOpen && op.AwaitRearm == "" {
		if _, exists := sessions.items[op.Session]; exists {
			return operationResult{}, fmt.Errorf("peer_stream session %q is already open", op.Session)
		}
		stream, err := open()
		if err != nil {
			return operationResult{}, err
		}
		session := newPeerStreamSession(step.Client, stream)
		result, invokeErr := invokePeerStreamOnStream(ctx, client, open, stream, session, "", step, input, audioCaptureMaxBytes, nil, observers...)
		if invokeErr != nil {
			_ = session.Close()
			return result, invokeErr
		}
		if err := sessions.add(op.Session, session); err != nil {
			_ = session.Close()
			return operationResult{}, err
		}
		result.evidence = mapsWith(result.evidence, map[string]any{"session_retained": true})
		return result, nil
	}
	session, err := sessions.take(op.Session, step.Client)
	if err != nil {
		return operationResult{}, err
	}
	closeSession := true
	defer func() {
		if closeSession {
			_ = session.Close()
		}
	}()
	rearmEvidence, err := waitForPeerStreamRearm(ctx, op.Session, session, op.AwaitRearm)
	if err != nil {
		return operationResult{evidence: rearmEvidence}, err
	}
	replacementID, err := generateValue("string")
	if err != nil {
		return operationResult{evidence: rearmEvidence}, err
	}
	if replacementID == "" || replacementID == session.streamID {
		return operationResult{evidence: rearmEvidence}, fmt.Errorf("peer_stream generated an invalid replacement stream ID")
	}
	result, invokeErr := invokePeerStreamOnStream(ctx, client, open, session.stream, session, replacementID, step, input, audioCaptureMaxBytes, func() {
		rearmEvidence["replacement_bos_sent"] = true
		rearmEvidence["stream_id_changed"] = true
	}, observers...)
	if invokeErr == nil {
		if op.KeepOpen {
			if err := sessions.add(op.Session, session); err != nil {
				result.evidence = mapsWith(result.evidence, rearmEvidence)
				return result, err
			}
			closeSession = false
			rearmEvidence["session_retained"] = true
		}
	}
	result.evidence = mapsWith(result.evidence, rearmEvidence)
	return result, invokeErr
}

func invokePeerStreamOnStream(ctx context.Context, client *gizcli.Client, open peerStreamOpener, stream peerStream, session *peerStreamSession, initialStreamID string, step Step, input any, audioCaptureMaxBytes int, onBOSSent func(), observers ...audioObserver) (operationResult, error) {
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
	inputSent := op.Completion == "input_sent"
	inputPackets, pushedPackets := 0, 0
	var inputDuration time.Duration
	var responseStarted time.Time
	var arrivals *peerStreamFirstResponseArrivals
	var next <-chan nextPeerStreamResult
	if session != nil && session.next != nil {
		responseStarted = time.Now()
		if firstResponse {
			arrivals = &peerStreamFirstResponseArrivals{started: responseStarted}
		}
		session.setArrivals(arrivals)
		defer session.setArrivals(nil)
		next = session.next
	}
	var idleTimeout time.Duration
	if op.IdleTimeout != "" {
		duration, parseErr := time.ParseDuration(op.IdleTimeout)
		if parseErr != nil || duration <= 0 {
			return operationResult{}, fmt.Errorf("invalid idle_timeout %q", op.IdleTimeout)
		}
		idleTimeout = duration
	}
	streamID := initialStreamID
	if streamID == "" {
		var err error
		streamID, err = generateValue("string")
		if err != nil {
			return operationResult{}, err
		}
	}
	if session != nil {
		session.streamID = streamID
	}
	if inputSent && next == nil && session == nil {
		// input_sent records output that arrives while the input is pushed,
		// so the step-owned reader starts before the first chunk goes out.
		next = readPeerStream(ctx, stream, nil)
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
				if chunk.IsBeginOfStream() && onBOSSent != nil {
					onBOSSent()
					onBOSSent = nil
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
		inputPackets, inputDuration = len(packets), opusPacketsDuration(packets)
		if observeAudio != nil {
			if err := observeAudioPackets(observeAudio, step.Client, "user", packets); err != nil {
				return operationResult{}, fmt.Errorf("play user audio: %w", err)
			}
		}
		if op.Mode == "realtime" {
			packets, err = appendRealtimeTailSilence(packets, realtimeTailSilence)
			if err != nil {
				return operationResult{}, fmt.Errorf("prepare realtime tail silence: %w", err)
			}
		}
		pushedPackets = len(packets)
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
				if chunk.IsBeginOfStream() && onBOSSent != nil {
					onBOSSent()
					onBOSSent = nil
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
	if next == nil {
		responseStarted = time.Now()
		if firstResponse {
			arrivals = &peerStreamFirstResponseArrivals{started: responseStarted}
		}
		if session == nil {
			next = readPeerStream(ctx, stream, arrivals)
		} else {
			session.setArrivals(arrivals)
			defer session.setArrivals(nil)
			session.startReader()
			next = session.next
		}
	}
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
	var audioPacing peerAudioPacing
	assistantObservationOpen := false
	defer func() {
		if observeAudio != nil && assistantObservationOpen {
			_ = observeAudio(step.Client, "assistant", nil, true)
		}
	}()
	audioBytes, events := 0, 0
	assistantTextEvents, assistantAudioEvents, assistantEOS := 0, 0, 0
	transcriptTextEvents, transcriptEOS, otherEOS := 0, 0, 0
	var terminalErrors []string
	var firstTranscriptMS, firstTextMS, firstAudioMS, textEOSMS, audioEOSMS, lastEventMS int64
	var firstTextElapsed, firstAudioElapsed time.Duration
	firstTranscriptObserved, firstTextObserved, firstAudioObserved := false, false, false
	textEOS, audioEOS := false, false
	counters := func() string {
		return fmt.Sprintf("events=%d assistant_text=%d assistant_audio=%d assistant_eos=%d transcript_text=%d transcript_eos=%d other_eos=%d interrupt_sent=%t interrupt_observed=%t", events, assistantTextEvents, assistantAudioEvents, assistantEOS, transcriptTextEvents, transcriptEOS, otherEOS, interrupted, observedInterrupted)
	}
	baseEvidence := func() map[string]any {
		evidence := map[string]any{"events": events, "first_transcript_ms": firstTranscriptMS, "last_event_ms": lastEventMS}
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
		if requireText {
			firstTextTimeout, _ = time.ParseDuration(op.FirstTextTimeout)
			firstTextTimer = time.NewTimer(firstTextTimeout)
			firstTextDeadline = firstTextTimer.C
			defer firstTextTimer.Stop()
		}
		if requireAudio {
			firstAudioTimeout, _ = time.ParseDuration(op.FirstAudioTimeout)
			firstAudioTimer = time.NewTimer(firstAudioTimeout)
			firstAudioDeadline = firstAudioTimer.C
			defer firstAudioTimer.Stop()
		}
	}
	finish := func() (operationResult, error) {
		if observeAudio != nil && assistantObservationOpen {
			assistantObservationOpen = false
			if err := observeAudio(step.Client, "assistant", nil, true); err != nil {
				return operationResult{}, fmt.Errorf("play assistant audio: %w", err)
			}
		}
		object := map[string]any{"text": texts, "audio_bytes": audioBytes, "events": events, "text_eos": textEOS, "audio_eos": audioEOS, "interrupted": interrupted, "interrupt_observed": observedInterrupted, "first_transcript_ms": firstTranscriptMS, "first_text_ms": firstTextMS, "first_audio_ms": firstAudioMS, "text_eos_ms": textEOSMS, "audio_eos_ms": audioEOSMS}
		if inputSent {
			object["input_sent"] = true
			object["input_packets"] = inputPackets
			object["input_ms"] = inputDuration.Milliseconds()
			object["pushed_packets"] = pushedPackets
			object["input_sent_ms"] = time.Since(started).Milliseconds()
		}
		pacingSummary := audioPacing.summary()
		if pacingSummary != nil {
			object["audio_pacing"] = pacingSummary
		}
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
		if pacingSummary != nil {
			evidence["audio_pacing"] = pacingSummary
		}
		if inputSent {
			evidence["input_sent"] = true
			evidence["input_packets"] = inputPackets
			evidence["input_ms"] = inputDuration.Milliseconds()
			evidence["pushed_packets"] = pushedPackets
		}
		return operationResult{assertion: object, saved: object, evidence: evidence}, nil
	}
	if inputSent {
		// The turn is complete once its input is on the wire. Output that has
		// already arrived on a step-owned stream is recorded without waiting;
		// a retained session keeps its queue for the step that consumes it.
		for session == nil {
			var result nextPeerStreamResult
			select {
			case result = <-next:
			default:
				return finish()
			}
			if result.err != nil || result.chunk == nil {
				return finish()
			}
			events++
			lastEventMS = time.Since(started).Milliseconds()
			switch part := result.chunk.Part.(type) {
			case genx.Text:
				texts = append(texts, string(part))
				if !firstTextObserved && strings.TrimSpace(string(part)) != "" {
					firstTextObserved = true
					firstTextMS = lastEventMS
				}
			case *genx.Blob:
				if len(part.Data) > 0 && !firstAudioObserved {
					firstAudioObserved = true
					firstAudioMS = lastEventMS
				}
				audioBytes += len(part.Data)
			}
		}
		return finish()
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
			replacement, openErr := open()
			if openErr != nil {
				return operationResult{}, fmt.Errorf("reopen PeerStream after interrupt: %w", openErr)
			}
			stream = replacement
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
				if !firstTranscriptObserved && label == "transcript" && strings.TrimSpace(string(part)) != "" {
					firstTranscriptObserved = true
					firstTranscriptMS = max(int64(1), eventElapsed.Milliseconds())
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
					if relayOpusMIME(part.MIMEType) {
						packets, err := decodeOpusPackets(part.Data)
						if err != nil {
							return operationResult{evidence: baseEvidence()}, fmt.Errorf("decode assistant audio: %w", err)
						}
						audioPacing.observe(result.receivedAt, packets)
						if observeAudio != nil {
							for _, packet := range packets {
								if err := observeAudio(step.Client, "assistant", packet, false); err != nil {
									return operationResult{evidence: baseEvidence()}, fmt.Errorf("play assistant audio: %w", err)
								}
								assistantObservationOpen = true
							}
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
			if firstResponse && (!requireText || firstTextObserved) && (!requireAudio || firstAudioObserved) {
				if len(terminalErrors) != 0 {
					evidence := baseEvidence()
					evidence["terminal_errors"] = len(terminalErrors)
					return operationResult{evidence: evidence}, fmt.Errorf("peer_stream terminal error: %s", strings.Join(terminalErrors, "; "))
				}
				if requireText && firstTextElapsed > firstTextTimeout {
					return operationResult{evidence: failedEvidence("first_text_timeout")}, fmt.Errorf("peer_stream first text timeout exceeded after %s (deadline=first_text_timeout %s): %w", op.FirstTextTimeout, counters(), context.DeadlineExceeded)
				}
				if requireAudio && firstAudioElapsed > firstAudioTimeout {
					return operationResult{evidence: failedEvidence("first_audio_timeout")}, fmt.Errorf("peer_stream first audio timeout exceeded after %s (deadline=first_audio_timeout %s): %w", op.FirstAudioTimeout, counters(), context.DeadlineExceeded)
				}
				return finish()
			}
			if result.chunk.IsEndOfStream() {
				mimeType, _ := result.chunk.MIMEType()
				if label == "assistant" && relayOpusMIME(mimeType) && observeAudio != nil && assistantObservationOpen {
					assistantObservationOpen = false
					if err := observeAudio(step.Client, "assistant", nil, true); err != nil {
						return operationResult{evidence: baseEvidence()}, fmt.Errorf("finish assistant playback: %w", err)
					}
				}
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

func mapsWith(base, extra map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	maps.Copy(base, extra)
	return base
}

func waitForPeerStreamRearm(ctx context.Context, name string, session *peerStreamSession, code string) (map[string]any, error) {
	evidence := map[string]any{
		"session_connection_reused": true,
		"reload_eos_observed":       false,
		"replacement_bos_sent":      false,
		"stream_id_changed":         false,
	}
	for {
		select {
		case result := <-session.next:
			if result.err != nil {
				if result.err == io.EOF {
					return evidence, fmt.Errorf("peer_stream session closed while waiting for re-arm %s", code)
				}
				return evidence, fmt.Errorf("wait for peer_stream re-arm %s: %w", code, result.err)
			}
			chunk := result.chunk
			if chunk == nil || chunk.Ctrl == nil || !chunk.IsEndOfStream() {
				continue
			}
			mimeType, _ := chunk.MIMEType()
			ctrl := chunk.Ctrl
			if ctrl.StreamID != session.streamID || strings.TrimSpace(ctrl.Label) != "user" || !relayOpusMIME(mimeType) || ctrl.ErrorCode != code || ctrl.Error != "input route reloaded" || !ctrl.ErrorRetryable {
				continue
			}
			evidence["reload_eos_observed"] = true
			return evidence, nil
		case <-ctx.Done():
			cause := context.Cause(ctx)
			return evidence, fmt.Errorf("peer_stream timed out waiting for re-arm %s on retained session %q: %w", code, name, cause)
		}
	}
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
	return readPeerStreamObserved(ctx, stream, arrivals.observe)
}

func readPeerStreamObserved(ctx context.Context, stream peerStream, observe func(*genx.MessageChunk, time.Time)) <-chan nextPeerStreamResult {
	next := make(chan nextPeerStreamResult, 64)
	go func() {
		for {
			chunk, err := stream.Next()
			receivedAt := time.Now()
			if observe != nil {
				observe(chunk, receivedAt)
			}
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
