package sfu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
)

// State describes one Peer attachment to the SFU Room.
type State string

const (
	// StateConnected reports an attached participant forwarding audio.
	StateConnected State = "connected"
	// StateReconnecting reports a bounded reconnect after a network error or
	// SFU restart; uplink frames are dropped and downlink routes are closed.
	StateReconnecting State = "reconnecting"
	// StateClosed reports that the attachment ended; Err carries the cause.
	StateClosed State = "closed"
)

// SessionStatus is the observable state of one Peer attachment.
type SessionStatus struct {
	Peer  string
	Room  string
	State State
	// Err is the terminal cause once State is StateClosed. It is nil for a
	// clean cancel and ErrRevoked, ErrDuplicateIdentity or a connector error
	// otherwise.
	Err error
}

const (
	audioOpusMIME       = "audio/opus"
	reconnectMinBackoff = 250 * time.Millisecond
	reconnectMaxBackoff = 5 * time.Second
	// routeIdleFlush releases packets held for a missing predecessor when the
	// track goes quiet (DTX, mute) so the tail of a burst is not delayed
	// until the next burst.
	routeIdleFlush = 60 * time.Millisecond
	// routeBurstIdle ends a talk burst once no voiced packet arrived for this
	// long. Closing the mixer route lets the device downlink go quiet between
	// utterances instead of carrying encoded silence for as long as the
	// remote participant stays subscribed.
	routeBurstIdle = 300 * time.Millisecond
	// opusSilenceFrameMaxBytes bounds the Opus frames that carry no speech: a
	// bare TOC byte (DTX) or the canonical three-byte silence packet. Such
	// frames never open a burst.
	opusSilenceFrameMaxBytes = 3
)

// opusSilencePrefix is the CELT silence packet (TOC 0xf8 followed by the
// range-coded silence flag). The SFU forwards it, optionally zero-padded to a
// fixed size, for an idle or muted publisher.
var opusSilencePrefix = []byte{0xf8, 0xff, 0xfe}

// session bridges one Peer's GenX streams to one SFU participant. The
// Transform context owns its lifetime.
type session struct {
	agent     *Agent
	peer      string
	binding   socialutil.SFUWorkspaceBinding
	connector roomConnector
	config    Config
	logger    *slog.Logger

	ctx    context.Context
	cancel context.CancelCauseFunc
	out    *outputStream
	done   chan struct{}

	disconnects chan disconnectReason

	mu         sync.Mutex
	state      State
	err        error
	client     roomClient
	forwarding bool
	routes     map[string]*remoteRoute
	routeSeq   atomic.Uint64
	finished   bool
}

func newSession(parent context.Context, agent *Agent, peer string, binding socialutil.SFUWorkspaceBinding) *session {
	ctx, cancel := context.WithCancelCause(parent)
	return &session{
		agent:       agent,
		peer:        peer,
		binding:     binding,
		connector:   agent.connector,
		config:      agent.config,
		logger:      agent.logger,
		ctx:         ctx,
		cancel:      cancel,
		out:         newOutputStream(),
		done:        make(chan struct{}),
		disconnects: make(chan disconnectReason, 8),
		state:       StateReconnecting,
		routes:      make(map[string]*remoteRoute),
	}
}

func (s *session) params() connectParams {
	return connectParams{URL: s.binding.SFU.URL, Room: s.binding.SFU.RoomToken, Identity: s.peer}
}

// connect performs the initial join synchronously so Transform can report
// an unreachable SFU as a reload failure.
func (s *session) connect(ctx context.Context) error {
	client, err := s.connector.connect(ctx, s.params(), s)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.client = client
	s.forwarding = true
	s.state = StateConnected
	s.mu.Unlock()
	return nil
}

// start launches the session goroutines after a successful connect.
func (s *session) start(input genx.Stream) {
	go s.supervise()
	go s.consumeInput(input)
	go s.recheck()
}

func (s *session) status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionStatus{Peer: s.peer, Room: s.binding.SFU.RoomToken, State: s.state, Err: s.err}
}

// supervise owns the connection lifecycle: context cancellation, SFU
// disconnects and bounded reconnects all resolve here.
func (s *session) supervise() {
	for {
		select {
		case <-s.ctx.Done():
			s.finish(terminalCause(s.ctx))
			return
		case reason := <-s.disconnects:
			switch reason {
			case disconnectLeave:
				continue
			case disconnectDuplicateIdentity:
				s.finish(ErrDuplicateIdentity)
				return
			default:
			}
			if err := s.reconnect(); err != nil {
				s.finish(err)
				return
			}
		}
	}
}

func terminalCause(ctx context.Context) error {
	cause := context.Cause(ctx)
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return nil
	}
	return cause
}

// reconnect retries the join with exponential backoff until
// Config.ReconnectTimeout elapses. Remote routes are closed first so the
// mixer does not wait on tracks that will be re-subscribed with new IDs.
func (s *session) reconnect() error {
	s.mu.Lock()
	s.forwarding = false
	s.state = StateReconnecting
	old := s.client
	s.client = nil
	s.mu.Unlock()
	s.closeRoutes(nil)
	if old != nil {
		old.Disconnect()
	}
	s.logger.Warn("sfu: participant disconnected, reconnecting", "room", s.binding.SFU.RoomToken)

	deadline := time.Now().Add(s.config.ReconnectTimeout)
	// Let the SFU settle before the first attempt. A Room that is being torn
	// down (restart, room delete) keeps accepting joins for a few
	// milliseconds, so participants that re-dial immediately can be split
	// across the closing and the replacement Room instance and stop hearing
	// each other; a participant that waits rejoins the surviving instance.
	select {
	case <-s.ctx.Done():
		return terminalCause(s.ctx)
	case <-time.After(reconnectMinBackoff):
	}
	backoff := reconnectMinBackoff
	for attempt := 1; ; attempt++ {
		attemptCtx, cancel := context.WithDeadline(s.ctx, deadline)
		client, err := s.connector.connect(attemptCtx, s.params(), s)
		cancel()
		if err == nil {
			s.drainDisconnects()
			s.mu.Lock()
			if s.finished {
				s.mu.Unlock()
				client.Disconnect()
				return nil
			}
			s.client = client
			s.forwarding = true
			s.state = StateConnected
			s.mu.Unlock()
			s.logger.Info("sfu: participant reconnected", "room", s.binding.SFU.RoomToken, "attempts", attempt)
			return nil
		}
		if s.ctx.Err() != nil {
			return terminalCause(s.ctx)
		}
		if time.Now().Add(backoff).After(deadline) {
			return fmt.Errorf("sfu: reconnect to room timed out after %s: %w", s.config.ReconnectTimeout, err)
		}
		select {
		case <-s.ctx.Done():
			return terminalCause(s.ctx)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, reconnectMaxBackoff)
	}
}

// drainDisconnects discards disconnect events that belong to the connection
// that was just replaced.
func (s *session) drainDisconnects() {
	for {
		select {
		case <-s.disconnects:
		default:
			return
		}
	}
}

// recheck re-validates the binding on Config.RecheckInterval and fails
// closed on any resolver error or generation change.
func (s *session) recheck() {
	ticker := time.NewTicker(s.config.RecheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		binding, err := s.agent.bindings.ResolveSFUWorkspaceBinding(s.ctx, s.agent.workspaceID, s.peer)
		if err == nil && binding.SFU.Generation == s.binding.SFU.Generation && binding.SFU.RoomToken == s.binding.SFU.RoomToken {
			continue
		}
		if s.ctx.Err() != nil {
			return
		}
		s.revoke(err)
		return
	}
}

// revoke stops forwarding immediately and terminates the attachment with
// ErrRevoked.
func (s *session) revoke(cause error) {
	s.mu.Lock()
	s.forwarding = false
	s.mu.Unlock()
	if cause != nil {
		s.logger.Warn("sfu: workspace binding revoked", "room", s.binding.SFU.RoomToken, "error", cause)
	} else {
		s.logger.Warn("sfu: workspace binding generation changed", "room", s.binding.SFU.RoomToken)
	}
	s.cancel(ErrRevoked)
}

// consumeInput forwards every raw Opus frame to the local track. BOS and
// EOS only delimit talk bursts; they never touch the connection.
func (s *session) consumeInput(input genx.Stream) {
	for {
		chunk, err := input.Next()
		if err != nil {
			return
		}
		if s.ctx.Err() != nil {
			return
		}
		frame, ok := opusFrame(chunk)
		if !ok {
			continue
		}
		duration, err := OpusPacketDuration(frame)
		if err != nil {
			continue
		}
		s.mu.Lock()
		client := s.client
		forwarding := s.forwarding
		s.mu.Unlock()
		if !forwarding || client == nil {
			continue
		}
		if err := client.WriteAudio(media.Sample{Data: frame, Duration: duration}); err != nil {
			s.logger.Debug("sfu: write uplink sample", "error", err)
		}
	}
}

func opusFrame(chunk *genx.MessageChunk) ([]byte, bool) {
	if chunk == nil {
		return nil, false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 {
		return nil, false
	}
	base, _, err := mime.ParseMediaType(blob.MIMEType)
	if err != nil || !strings.EqualFold(base, audioOpusMIME) {
		return nil, false
	}
	return blob.Data, true
}

// finish tears the attachment down exactly once. A nil cause closes the
// output cleanly; any other cause closes it with that error.
func (s *session) finish(cause error) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.forwarding = false
	s.state = StateClosed
	s.err = cause
	client := s.client
	s.client = nil
	s.mu.Unlock()

	s.cancel(context.Canceled)
	s.closeRoutes(nil)
	if client != nil {
		client.Disconnect()
	}
	if cause != nil {
		_ = s.out.CloseWithError(cause)
	} else {
		_ = s.out.Close()
	}
	s.agent.removeSession(s.peer, s)
	close(s.done)
}

// roomEvents implementation. Every callback returns promptly.

func (s *session) onDisconnected(reason disconnectReason) {
	select {
	case s.disconnects <- reason:
	default:
	}
}

func (s *session) onReconnecting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.state != StateConnected {
		return
	}
	s.forwarding = false
	s.state = StateReconnecting
}

func (s *session) onReconnected() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.client == nil {
		return
	}
	s.forwarding = true
	s.state = StateConnected
}

func (s *session) onTrackSubscribed(identity, trackID string, reader rtpReader) {
	if reader == nil || identity == "" || identity == s.peer {
		return
	}
	route := &remoteRoute{
		session:  s,
		identity: identity,
		trackID:  trackID,
		reader:   reader,
		reorder:  newReorderBuffer(0, 0),
	}
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	previous := s.routes[trackID]
	s.routes[trackID] = route
	s.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	go route.run()
}

func (s *session) onTrackUnsubscribed(trackID string) {
	s.mu.Lock()
	route := s.routes[trackID]
	delete(s.routes, trackID)
	s.mu.Unlock()
	if route != nil {
		route.close()
	}
}

func (s *session) onTrackMuted(trackID string) {
	s.mu.Lock()
	route := s.routes[trackID]
	s.mu.Unlock()
	if route != nil {
		route.endBurst()
	}
}

func (s *session) onParticipantDisconnected(identity string) {
	s.closeRoutes(func(route *remoteRoute) bool { return route.identity == identity })
}

func (s *session) closeRoutes(match func(*remoteRoute) bool) {
	s.mu.Lock()
	var closing []*remoteRoute
	for id, route := range s.routes {
		if match == nil || match(route) {
			closing = append(closing, route)
			delete(s.routes, id)
		}
	}
	s.mu.Unlock()
	for _, route := range closing {
		route.close()
	}
}

func (s *session) removeRoute(route *remoteRoute) {
	s.mu.Lock()
	if s.routes[route.trackID] == route {
		delete(s.routes, route.trackID)
	}
	s.mu.Unlock()
}

func (s *session) nextStreamID(identity, trackID string) string {
	return fmt.Sprintf("sfu/%s/%s/%d", identity, trackID, s.routeSeq.Add(1))
}

// remoteRoute turns one remote audio track into ordered GenX audio chunks.
// Each talk burst uses a fresh stream_id; the label is always the remote
// participant identity so the mixer keeps one route per participant.
type remoteRoute struct {
	session  *session
	identity string
	trackID  string
	reader   rtpReader
	reorder  *reorderBuffer

	mu        sync.Mutex
	streamID  string
	idle      *time.Timer
	burstIdle *time.Timer
	closed    bool
}

func (r *remoteRoute) run() {
	defer r.close()
	for {
		packet, _, err := r.reader.ReadRTP()
		if err != nil {
			return
		}
		if packet == nil || len(packet.Payload) == 0 {
			continue
		}
		r.push(packet)
	}
}

// push reorders one packet and emits everything that became deliverable.
// Reordering and emission share r.mu so the idle flush cannot interleave
// with a concurrent read.
func (r *remoteRoute) push(packet *rtp.Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.idle != nil {
		r.idle.Stop()
	}
	r.emitLocked(r.reorder.Push(packet))
	if r.reorder.Len() > 0 {
		r.idle = time.AfterFunc(routeIdleFlush, r.flushIdle)
	}
}

func (r *remoteRoute) flushIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.emitLocked(r.reorder.Flush())
}

// emitLocked pushes ordered payloads to the output, opening a new stream_id
// for the first voiced packet of a burst. Silence frames are forwarded while
// a burst is open so the mixer timing stays continuous, but they never open
// one. The output push never blocks.
func (r *remoteRoute) emitLocked(packets []*rtp.Packet) {
	for _, packet := range packets {
		voiced := !isOpusSilenceFrame(packet.Payload)
		if r.streamID == "" {
			if !voiced {
				continue
			}
			r.streamID = r.session.nextStreamID(r.identity, r.trackID)
			r.session.logger.Debug("sfu: downlink burst opened", "participant", r.identity, "track", r.trackID, "payload_bytes", len(packet.Payload))
			r.session.out.push(r.chunk(r.streamID, nil, true, false))
		}
		r.session.out.push(r.chunk(r.streamID, packet.Payload, false, false))
		if voiced {
			r.armBurstIdleLocked()
		}
	}
}

// isOpusSilenceFrame reports an Opus packet that carries no speech: a DTX or
// canonical silence frame, or that silence frame zero-padded by the SFU.
func isOpusSilenceFrame(payload []byte) bool {
	if len(payload) <= opusSilenceFrameMaxBytes {
		return true
	}
	if !bytes.HasPrefix(payload, opusSilencePrefix) {
		return false
	}
	for _, b := range payload[len(opusSilencePrefix):] {
		if b != 0 {
			return false
		}
	}
	return true
}

func (r *remoteRoute) armBurstIdleLocked() {
	if r.burstIdle == nil {
		r.burstIdle = time.AfterFunc(routeBurstIdle, r.endBurstIdle)
		return
	}
	r.burstIdle.Stop()
	r.burstIdle.Reset(routeBurstIdle)
}

func (r *remoteRoute) endBurstIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.endBurstLocked()
}

// endBurstLocked flushes pending packets and closes the current mixer
// route; the next voiced packet opens a new stream_id.
func (r *remoteRoute) endBurstLocked() {
	r.emitLocked(r.reorder.Flush())
	if r.burstIdle != nil {
		r.burstIdle.Stop()
	}
	if r.streamID == "" {
		return
	}
	r.session.out.push(r.chunk(r.streamID, nil, false, true))
	r.streamID = ""
}

// endBurst closes the current mixer route without ending the track.
func (r *remoteRoute) endBurst() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.endBurstLocked()
}

func (r *remoteRoute) close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.idle != nil {
		r.idle.Stop()
	}
	if r.burstIdle != nil {
		r.burstIdle.Stop()
	}
	r.endBurstLocked()
	r.mu.Unlock()
	r.session.removeRoute(r)
}

func (r *remoteRoute) chunk(streamID string, payload []byte, bos, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: r.identity,
		Part: &genx.Blob{MIMEType: audioOpusMIME, Data: payload},
		Ctrl: &genx.StreamCtrl{
			StreamID:      streamID,
			Label:         r.identity,
			BeginOfStream: bos,
			EndOfStream:   eos,
		},
	}
}
