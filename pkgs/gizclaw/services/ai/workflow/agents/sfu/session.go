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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media"
)

// State describes one Peer attachment to the SFU Room.
type State string

const (
	// StateConnected reports an attached participant forwarding audio.
	StateConnected State = "connected"
	// StateReconnecting reports a bounded reconnect after a network error or
	// SFU restart; uplink frames are dropped and the downlink floor is
	// released.
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
	// DroppedPackets counts voiced remote Opus packets that were discarded
	// because their participant did not hold the floor.
	DroppedPackets uint64
	// RejectedData counts talk data packets that were malformed or carried
	// an unsupported protocol version.
	RejectedData uint64
}

const (
	audioOpusMIME       = "audio/opus"
	reconnectMinBackoff = 250 * time.Millisecond
	reconnectMaxBackoff = 5 * time.Second
	// routeIdleFlush releases packets held for a missing predecessor when the
	// holder's track goes quiet (DTX, mute) so the tail of an utterance is
	// not delayed until the next packet.
	routeIdleFlush = 60 * time.Millisecond
	// maxPrerollPackets bounds the voiced packets a non-holder track keeps so
	// that the packets which raced ahead of their sender's BOS on the data
	// channel can still be delivered once that sender takes the floor.
	maxPrerollPackets = 16
	// opusEmptyFrameMaxBytes is the largest Opus packet that carries no frame
	// data at all: a bare TOC byte, whose single code-0 frame has zero
	// compressed bytes. That is how DTX reaches the connector.
	opusEmptyFrameMaxBytes = 1
)

// opusSilencePrefix is the CELT silence packet (TOC 0xf8 followed by the
// range-coded silence flag). The SFU forwards it, optionally zero-padded to a
// fixed size, for an idle or muted publisher.
var opusSilencePrefix = []byte{0xf8, 0xff, 0xfe}

// session bridges one Peer's GenX streams to one SFU participant. The
// Transform context owns its lifetime.
//
// Uplink: the device's Opus frames are split into talk utterances that are
// announced on the Room data channel (see talk.go) and written to the local
// track. Downlink: the session grants the floor to one remote utterance at a
// time and forwards only that participant's raw Opus packets to the device
// as passthrough audio; nothing is decoded on this path.
//
// The floor is per-listener state, not a room-wide decision: there is no
// arbitrator and no total order over the data channel. When several
// participants open an utterance at once, the reliable messages can reach two
// listeners in different orders, so those listeners can lock onto different
// speakers. That is intended. What half-duplex guarantees here is that any one
// listener hears exactly one speaker at a time and that a speaker receives no
// downlink at all, not that the whole Room converges on the same speaker.
// Room-wide convergence would need an arbitrator, which would put a network
// round trip in front of every utterance.
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
	talkNotify  chan struct{}

	// mu guards every field below. Timer callbacks and connector callbacks
	// take it briefly; no network or blocking I/O runs under it.
	mu         sync.Mutex
	state      State
	err        error
	client     roomClient
	forwarding bool
	finished   bool

	talk      talkState
	talkQueue []talkMessage

	tracks         map[string]*remoteTrack
	utterances     map[string]*remoteUtterance
	utteranceOrder uint64
	floor          *floorHold
	floorGen       uint64
	floorIdle      *time.Timer
	floorIdleGen   uint64

	routeSeq       atomic.Uint64
	talkSeq        atomic.Uint64
	droppedPackets atomic.Uint64
	rejectedData   atomic.Uint64
}

// talkState is the Peer's own open utterance on the uplink.
type talkState struct {
	open      bool
	utterance string
	gen       uint64
	// hangover is re-armed on every voiced frame; hangoverGen records which
	// utterance generation its callback checks so a stale timer is replaced
	// instead of reset.
	hangover    *time.Timer
	hangoverGen uint64
}

// remoteUtterance is one open utterance announced by a remote participant.
type remoteUtterance struct {
	id  string
	seq uint64
	// order ranks utterances by arrival so a release hands the floor to the
	// earliest one still open.
	order uint64
	// idle marks an utterance the floor released because its holder went
	// quiet, muted or lost its track. It stays open but is not eligible for
	// the floor until a voiced packet arrives from that participant again;
	// otherwise a silent-but-open realtime stream would take the floor,
	// idle out and take it again forever.
	idle bool
}

// floorHold names the participant currently forwarded to the device.
type floorHold struct {
	identity  string
	utterance string
	streamID  string
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
		talkNotify:  make(chan struct{}, 1),
		state:       StateReconnecting,
		tracks:      make(map[string]*remoteTrack),
		utterances:  make(map[string]*remoteUtterance),
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
	go s.publishTalk()
	go s.recheck()
}

func (s *session) status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SessionStatus{
		Peer:           s.peer,
		Room:           s.binding.SFU.RoomToken,
		State:          s.state,
		Err:            s.err,
		DroppedPackets: s.droppedPackets.Load(),
		RejectedData:   s.rejectedData.Load(),
	}
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
// Config.ReconnectTimeout elapses. The floor, every remote track and the
// Peer's open utterance are dropped first: the new participant sees the Room
// afresh and remote utterances are learned again from their next BOS.
func (s *session) reconnect() error {
	s.mu.Lock()
	s.forwarding = false
	s.state = StateReconnecting
	old := s.client
	s.client = nil
	s.resetTalkLocked()
	tracks := s.resetDownlinkLocked()
	s.mu.Unlock()
	for _, track := range tracks {
		track.close()
	}
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
//
// This poll is the only mechanism that ends an established attachment.
// Social commits a friend deletion, a group deletion or a member removal and
// pushes nothing: there is no cancellation callback and no cross-Server event
// delivery, so a Peer homed on any Server stops the same way and within the
// same bound. Revocation is therefore eventually consistent, at most one
// interval late. New turns do not wait for it — every inbound BOS and Opus
// packet re-checks membership before it is admitted.
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

// consumeInput splits the device uplink into talk utterances and forwards
// the Opus frames of open utterances to the local track.
//
// Push-to-talk (BOS, frames, EOS per press) and realtime (one BOS, a
// continuous stream, EOS only at the end) are handled by one rule: an
// utterance opens on the first voiced frame and closes when the device sends
// EOS or when no voiced frame arrived for Config.TalkHangover. Voiced means
// not an Opus silence frame (see isOpusSilenceFrame); that frame-based rule
// is the only uplink voice activity detection. A device that streams raw,
// un-gated audio in realtime mode therefore keeps one utterance open for as
// long as it streams and holds the floor of every listener; the fix for such
// a device is a sender-side energy VAD that emits DTX or stops sending while
// nobody speaks, which is deliberately out of scope here.
//
// Silence frames are written to the track only while an utterance is open,
// so between utterances the SFU sees DTX and nothing else. Device BOS and
// EOS never touch the connection.
func (s *session) consumeInput(input genx.Stream) {
	for {
		chunk, err := input.Next()
		if err != nil {
			return
		}
		if s.ctx.Err() != nil {
			return
		}
		if frame, ok := opusFrame(chunk); ok {
			s.forwardUplinkFrame(frame)
		}
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
			s.mu.Lock()
			s.closeTalkLocked()
			s.mu.Unlock()
		}
	}
}

func (s *session) forwardUplinkFrame(frame []byte) {
	duration, err := OpusPacketDuration(frame)
	if err != nil {
		return
	}
	voiced := !isOpusSilenceFrame(frame)
	s.mu.Lock()
	client := s.client
	if !s.forwarding || client == nil {
		// Frames are dropped while reconnecting; the utterance cannot be
		// announced, so it is closed silently and reopens on the next voiced
		// frame after the rejoin.
		s.resetTalkLocked()
		s.mu.Unlock()
		return
	}
	if voiced {
		s.openTalkLocked()
		s.armTalkHangoverLocked()
	} else if !s.talk.open {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if err := client.WriteAudio(media.Sample{Data: frame, Duration: duration}); err != nil {
		s.logger.Debug("sfu: write uplink sample", "error", err)
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

// openTalkLocked opens the Peer's utterance: it announces BOS and, because
// the link is half-duplex, releases the downlink floor. No floor is taken
// again until the utterance closes.
func (s *session) openTalkLocked() {
	if s.talk.open {
		return
	}
	s.talk.open = true
	s.talk.utterance = newUtteranceID()
	s.talk.gen++
	s.queueTalkLocked(talkTypeBOS, s.talk.utterance)
	s.releaseFloorLocked(false)
	s.logger.Debug("sfu: talk utterance opened", "utterance", s.talk.utterance)
}

// closeTalkLocked closes the Peer's utterance, announces EOS and lets the
// earliest open remote utterance take the floor.
func (s *session) closeTalkLocked() {
	if !s.talk.open {
		return
	}
	s.queueTalkLocked(talkTypeEOS, s.talk.utterance)
	s.logger.Debug("sfu: talk utterance closed", "utterance", s.talk.utterance)
	s.resetTalkLocked()
	s.acquireFloorLocked()
}

// resetTalkLocked forgets the open utterance without announcing anything.
func (s *session) resetTalkLocked() {
	if s.talk.hangover != nil {
		s.talk.hangover.Stop()
	}
	s.talk.gen++
	s.talk.open = false
	s.talk.utterance = ""
}

func (s *session) armTalkHangoverLocked() {
	gen := s.talk.gen
	if s.talk.hangover != nil && s.talk.hangoverGen == gen {
		s.talk.hangover.Reset(s.config.TalkHangover)
		return
	}
	if s.talk.hangover != nil {
		s.talk.hangover.Stop()
	}
	s.talk.hangoverGen = gen
	s.talk.hangover = time.AfterFunc(s.config.TalkHangover, func() { s.onTalkHangover(gen) })
}

func (s *session) onTalkHangover(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || !s.talk.open || s.talk.gen != gen {
		return
	}
	s.closeTalkLocked()
}

func (s *session) queueTalkLocked(kind, utterance string) {
	s.talkQueue = append(s.talkQueue, talkMessage{
		V:         talkProtocolVersion,
		Type:      kind,
		Utterance: utterance,
		Seq:       s.talkSeq.Add(1),
	})
	select {
	case s.talkNotify <- struct{}{}:
	default:
	}
}

// publishTalk publishes queued talk messages on the reliable data channel
// in queue order. It is the only goroutine that touches the data channel,
// so the order the state machine produced is the order the Room sees, and
// no network I/O runs under s.mu.
func (s *session) publishTalk() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.talkNotify:
		}
		for {
			s.mu.Lock()
			if len(s.talkQueue) == 0 {
				s.mu.Unlock()
				break
			}
			message := s.talkQueue[0]
			s.talkQueue = s.talkQueue[1:]
			client := s.client
			forwarding := s.forwarding
			s.mu.Unlock()
			if client == nil || !forwarding {
				continue
			}
			payload, err := message.encode()
			if err != nil {
				s.logger.Warn("sfu: encode talk message", "error", err)
				continue
			}
			if err := client.PublishData(talkTopic, payload); err != nil {
				s.logger.Debug("sfu: publish talk message", "type", message.Type, "error", err)
			}
		}
	}
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
	s.resetTalkLocked()
	tracks := s.resetDownlinkLocked()
	s.mu.Unlock()

	s.cancel(context.Canceled)
	for _, track := range tracks {
		track.close()
	}
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

// resetDownlinkLocked releases the floor, forgets every remote utterance and
// detaches every remote track. The caller closes the returned tracks
// outside the lock.
func (s *session) resetDownlinkLocked() []*remoteTrack {
	s.releaseFloorLocked(false)
	clear(s.utterances)
	tracks := make([]*remoteTrack, 0, len(s.tracks))
	for id, track := range s.tracks {
		tracks = append(tracks, track)
		delete(s.tracks, id)
	}
	return tracks
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
	track := &remoteTrack{
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
	previous := s.tracks[trackID]
	s.tracks[trackID] = track
	s.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	go track.run()
}

func (s *session) onTrackUnsubscribed(trackID string) {
	s.mu.Lock()
	track := s.tracks[trackID]
	s.mu.Unlock()
	if track != nil {
		track.close()
	}
}

// onTrackMuted releases the floor when the holder's track is muted. The
// utterance stays open but idle until the participant is heard again.
func (s *session) onTrackMuted(trackID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	track := s.tracks[trackID]
	if track == nil || !s.holdsFloorLocked(track.identity) {
		return
	}
	s.releaseFloorLocked(true)
	s.acquireFloorLocked()
}

func (s *session) onParticipantDisconnected(identity string) {
	s.mu.Lock()
	delete(s.utterances, identity)
	var closing []*remoteTrack
	for id, track := range s.tracks {
		if track.identity == identity {
			closing = append(closing, track)
			delete(s.tracks, id)
		}
	}
	if s.holdsFloorLocked(identity) {
		s.releaseFloorLocked(false)
		s.acquireFloorLocked()
	}
	s.mu.Unlock()
	for _, track := range closing {
		track.close()
	}
}

// onDataPacket applies one remote talk message. Malformed messages are
// counted and ignored; a stale EOS for an utterance this session never saw
// open (for example one announced while this participant was rejoining) is
// ignored without counting.
func (s *session) onDataPacket(identity, topic string, payload []byte) {
	if topic != talkTopic || identity == "" || identity == s.peer {
		return
	}
	message, err := decodeTalkMessage(payload)
	if err != nil {
		s.rejectedData.Add(1)
		s.logger.Debug("sfu: talk message rejected", "participant", identity, "error", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	switch message.Type {
	case talkTypeBOS:
		if current := s.utterances[identity]; current != nil && current.id == message.Utterance {
			return
		}
		if s.holdsFloorLocked(identity) {
			// The holder opened a new utterance without closing the previous
			// one (its EOS was lost); hand the floor over cleanly.
			s.releaseFloorLocked(false)
		}
		s.utteranceOrder++
		s.utterances[identity] = &remoteUtterance{id: message.Utterance, seq: message.Seq, order: s.utteranceOrder}
		s.acquireFloorLocked()
	case talkTypeEOS:
		current := s.utterances[identity]
		if current == nil || current.id != message.Utterance {
			s.logger.Debug("sfu: stale talk EOS ignored", "participant", identity, "utterance", message.Utterance)
			return
		}
		delete(s.utterances, identity)
		if s.holdsFloorLocked(identity) {
			s.releaseFloorLocked(false)
			s.acquireFloorLocked()
		}
	}
}

func (s *session) holdsFloorLocked(identity string) bool {
	return s.floor != nil && s.floor.identity == identity
}

// acquireFloorLocked grants the floor to the earliest open, non-idle remote
// utterance when the floor is free, this Peer is not talking and the
// participant is connected. It opens the passthrough route and replays the
// holder's recent preroll packets. It reports whether a floor was taken.
func (s *session) acquireFloorLocked() bool {
	if s.floor != nil || s.talk.open || !s.forwarding || s.finished {
		return false
	}
	var (
		holder    string
		utterance *remoteUtterance
	)
	for identity, candidate := range s.utterances {
		if candidate.idle {
			continue
		}
		if utterance == nil || candidate.order < utterance.order {
			holder, utterance = identity, candidate
		}
	}
	if utterance == nil {
		return false
	}
	s.floorGen++
	s.floor = &floorHold{identity: holder, utterance: utterance.id, streamID: s.nextStreamID(holder)}
	s.logger.Debug("sfu: floor granted", "participant", holder, "utterance", utterance.id)
	s.out.push(s.chunk(holder, s.floor.streamID, nil, true, false))
	now := time.Now()
	for _, track := range s.tracks {
		if track.identity != holder {
			continue
		}
		// Whatever the reorder buffer expected before this hold is gone;
		// start ordering from the first packet of this hold.
		track.reorder = newReorderBuffer(0, 0)
		track.stopIdleLocked()
		for _, held := range track.preroll {
			if now.Sub(held.at) > s.config.FloorIdle {
				s.droppedPackets.Add(1)
				continue
			}
			s.forwardLocked(track, held.packet, true)
		}
		track.preroll = nil
	}
	s.armFloorIdleLocked()
	return true
}

// releaseFloorLocked closes the passthrough route of the current holder.
// markIdle records that the holder went quiet so its still-open utterance
// does not immediately retake the floor.
func (s *session) releaseFloorLocked(markIdle bool) {
	if s.floor == nil {
		return
	}
	floor := s.floor
	for _, track := range s.tracks {
		if track.identity != floor.identity {
			continue
		}
		track.stopIdleLocked()
		s.emitLocked(track.reorder.Flush())
	}
	s.out.push(s.chunk(floor.identity, floor.streamID, nil, false, true))
	if markIdle {
		if utterance := s.utterances[floor.identity]; utterance != nil && utterance.id == floor.utterance {
			utterance.idle = true
		}
	}
	s.floor = nil
	s.floorGen++
	if s.floorIdle != nil {
		s.floorIdle.Stop()
	}
	s.logger.Debug("sfu: floor released", "participant", floor.identity, "idle", markIdle)
}

func (s *session) armFloorIdleLocked() {
	gen := s.floorGen
	if s.floorIdle != nil && s.floorIdleGen == gen {
		s.floorIdle.Reset(s.config.FloorIdle)
		return
	}
	if s.floorIdle != nil {
		s.floorIdle.Stop()
	}
	s.floorIdleGen = gen
	s.floorIdle = time.AfterFunc(s.config.FloorIdle, func() { s.onFloorIdle(gen) })
}

func (s *session) onFloorIdle(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.floor == nil || s.floorGen != gen {
		return
	}
	s.releaseFloorLocked(true)
	s.acquireFloorLocked()
}

// onPacket routes one remote packet: the holder's packets are reordered and
// forwarded, a voiced packet from anyone else may take a free floor or is
// held briefly for preroll, and silence from non-holders is ignored.
func (s *session) onPacket(track *remoteTrack, packet *rtp.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if track.closed || s.finished {
		return
	}
	voiced := !isOpusSilenceFrame(packet.Payload)
	if s.holdsFloorLocked(track.identity) {
		s.forwardLocked(track, packet, voiced)
		return
	}
	if !voiced {
		return
	}
	if utterance := s.utterances[track.identity]; utterance != nil && utterance.idle {
		utterance.idle = false
	}
	if s.acquireFloorLocked() && s.holdsFloorLocked(track.identity) {
		s.forwardLocked(track, packet, true)
		return
	}
	track.holdPrerollLocked(packet)
}

// forwardLocked reorders one packet of the floor holder and emits everything
// that became deliverable. A voiced packet re-arms the floor idle timer.
func (s *session) forwardLocked(track *remoteTrack, packet *rtp.Packet, voiced bool) {
	track.stopIdleLocked()
	s.emitLocked(track.reorder.Push(packet))
	if track.reorder.Len() > 0 {
		gen := s.floorGen
		track.idle = time.AfterFunc(routeIdleFlush, func() { track.flushIdle(gen) })
	}
	if voiced {
		s.armFloorIdleLocked()
	}
}

// emitLocked pushes ordered payloads of the floor holder as passthrough
// packets. The output push never blocks.
func (s *session) emitLocked(packets []*rtp.Packet) {
	if s.floor == nil {
		return
	}
	for _, packet := range packets {
		s.out.push(s.chunk(s.floor.identity, s.floor.streamID, packet.Payload, false, false))
	}
}

func (s *session) nextStreamID(identity string) string {
	return fmt.Sprintf("sfu/%s/%d", identity, s.routeSeq.Add(1))
}

// chunk builds one passthrough downlink chunk. The label is the remote
// participant identity and each floor hold uses a fresh stream_id.
func (s *session) chunk(identity, streamID string, payload []byte, bos, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: identity,
		Part: &genx.Blob{MIMEType: agenthost.OpusPassthroughMIME, Data: payload},
		Ctrl: &genx.StreamCtrl{
			StreamID:      streamID,
			Label:         identity,
			BeginOfStream: bos,
			EndOfStream:   eos,
		},
	}
}

// isOpusSilenceFrame reports an Opus packet that carries no speech. Only the
// two encodings LiveKit actually forwards for an idle publisher qualify: a
// packet with no frame data (an empty payload or a bare TOC byte, which is how
// DTX arrives) and the canonical CELT silence packet, which the SFU pads with
// trailing zeros to a fixed size. Every other packet counts as speech, because
// a valid low-bitrate frame can be two or three bytes and a length threshold
// would drop real audio instead of opening the utterance.
func isOpusSilenceFrame(payload []byte) bool {
	if len(payload) <= opusEmptyFrameMaxBytes {
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

// remoteTrack is one subscribed remote audio track. Its reader goroutine
// drains RTP for the lifetime of the subscription; whether a packet is
// forwarded is decided by the session's floor.
type remoteTrack struct {
	session  *session
	identity string
	trackID  string
	reader   rtpReader

	// The fields below are guarded by session.mu.
	reorder *reorderBuffer
	idle    *time.Timer
	preroll []prerollPacket
	closed  bool
}

// prerollPacket is a voiced packet received while its participant did not
// hold the floor, kept so a BOS that arrives just after its first packets
// still delivers them.
type prerollPacket struct {
	packet *rtp.Packet
	at     time.Time
}

func (t *remoteTrack) run() {
	defer t.close()
	for {
		packet, _, err := t.reader.ReadRTP()
		if err != nil {
			return
		}
		if packet == nil || len(packet.Payload) == 0 {
			continue
		}
		t.session.onPacket(t, packet)
	}
}

func (t *remoteTrack) holdPrerollLocked(packet *rtp.Packet) {
	if len(t.preroll) >= maxPrerollPackets {
		t.session.droppedPackets.Add(1)
		t.preroll = t.preroll[1:]
	}
	t.preroll = append(t.preroll, prerollPacket{packet: packet, at: time.Now()})
}

func (t *remoteTrack) stopIdleLocked() {
	if t.idle != nil {
		t.idle.Stop()
	}
}

// flushIdle releases packets waiting for a lost predecessor once the holder
// went quiet; a stale timer from an earlier hold does nothing.
func (t *remoteTrack) flushIdle(gen uint64) {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.closed || s.floorGen != gen || !s.holdsFloorLocked(t.identity) {
		return
	}
	s.emitLocked(t.reorder.Flush())
}

// close detaches the track. When it belongs to the floor holder the floor is
// released as idle and the next open utterance takes it.
func (t *remoteTrack) close() {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	t.stopIdleLocked()
	s.droppedPackets.Add(uint64(len(t.preroll)))
	t.preroll = nil
	if s.tracks[t.trackID] == t {
		delete(s.tracks, t.trackID)
	}
	if s.holdsFloorLocked(t.identity) && !s.finished {
		s.releaseFloorLocked(true)
		s.acquireFloorLocked()
	}
}
