package gizcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type PeerStream struct {
	events      io.ReadWriteCloser
	packets     <-chan []byte
	unsubscribe func()
	conn        peerPacketWriter

	out  chan *genx.MessageChunk
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
	push func(context.Context, *genx.MessageChunk) error

	resourceEvents chan *eventpb.PeerEvent
	resourceOnce   sync.Once

	audioRouteMu sync.RWMutex
	audioRoute   genx.StreamCtrl
}

type peerPacketWriter interface {
	Write(byte, []byte) (int, error)
}

var _ genx.Stream = (*PeerStream)(nil)

func (c *Client) OpenPeerStream(buffer int) (*PeerStream, error) {
	if buffer < 1 {
		buffer = 1
	}
	eventStream, err := c.DialPeerEventStream()
	if err != nil {
		return nil, err
	}
	packets, unsubscribe := c.subscribePeerPackets(giznet.ProtocolOpusPacket, buffer)
	stream := &PeerStream{
		events:         eventStream,
		packets:        packets,
		unsubscribe:    unsubscribe,
		conn:           c.PeerConn(),
		out:            make(chan *genx.MessageChunk, buffer),
		done:           make(chan struct{}),
		resourceEvents: make(chan *eventpb.PeerEvent, buffer),
	}
	go stream.readEvents()
	go stream.readPackets()
	return stream, nil
}

// ResourceEvents returns connection-level social and Workspace history
// invalidations received on the same single Peer Event Stream used by this
// conversational stream. Delivery is best effort; callers still refresh
// authoritative state when a view opens.
func (s *PeerStream) ResourceEvents() <-chan *eventpb.PeerEvent {
	if s == nil {
		return nil
	}
	return s.resourceEvents
}

func (s *PeerStream) Next() (*genx.MessageChunk, error) {
	if s == nil {
		return nil, io.ErrClosedPipe
	}
	select {
	case chunk := <-s.out:
		return chunk, nil
	case <-s.done:
		select {
		case chunk := <-s.out:
			return chunk, nil
		default:
		}
		return nil, s.closeErr()
	}
}

func (s *PeerStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	if s == nil {
		return io.ErrClosedPipe
	}
	if s.push != nil {
		return s.push(ctx, chunk)
	}
	if chunk == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return s.closeErr()
	default:
	}
	for _, event := range peerStreamEventsFromChunk(chunk) {
		if err := WritePeerStreamEvent(s.events, event); err != nil {
			return err
		}
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || len(blob.Data) == 0 || !isOpusBlob(blob) {
		return nil
	}
	if s.conn == nil {
		return fmt.Errorf("gizclaw: peer stream is not connected")
	}
	_, err := s.conn.Write(giznet.ProtocolOpusPacket, blob.Data)
	return err
}

func (s *PeerStream) Close() error {
	return s.CloseWithError(io.EOF)
}

func (s *PeerStream) CloseWithError(err error) error {
	if s == nil {
		return nil
	}
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		if s.unsubscribe != nil {
			s.unsubscribe()
		}
		if s.events != nil {
			_ = s.events.Close()
		}
		close(s.done)
	})
	return nil
}

func (s *PeerStream) readEvents() {
	defer s.closeResourceEvents()
	for {
		event, err := ReadPeerStreamEvent(s.events)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = s.Close()
				return
			}
			_ = s.CloseWithError(err)
			return
		}
		if isPeerResourceInvalidation(event) {
			s.publishResourceEvent(event)
			if event.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED {
				continue
			}
		}
		if event.Type > eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED {
			continue
		}
		chunk, err := peerStreamEventToChunk(event)
		if err != nil {
			_ = s.CloseWithError(err)
			return
		}
		s.observeAudioRouteBeforeOutput(chunk)
		if err := s.pushOutput(chunk); err != nil {
			_ = s.CloseWithError(err)
			return
		}
		s.observeAudioRouteAfterOutput(chunk)
	}
}

func (s *PeerStream) publishResourceEvent(event *eventpb.PeerEvent) {
	if s == nil || event == nil || s.resourceEvents == nil {
		return
	}
	select {
	case <-s.done:
	case s.resourceEvents <- event:
	default:
	}
}

func (s *PeerStream) closeResourceEvents() {
	if s == nil || s.resourceEvents == nil {
		return
	}
	s.resourceOnce.Do(func() { close(s.resourceEvents) })
}

func isPeerResourceInvalidation(event *eventpb.PeerEvent) bool {
	if event == nil {
		return false
	}
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_RELATIONSHIP_UPDATED,
		eventpb.PeerEventType_PEER_EVENT_TYPE_FRIEND_GROUP_UPDATED:
		return true
	default:
		return false
	}
}

func (s *PeerStream) readPackets() {
	for {
		select {
		case <-s.done:
			return
		case payload, ok := <-s.packets:
			if !ok {
				return
			}
			chunk, ok := opusPacketChunk(payload)
			if !ok {
				continue
			}
			chunk = s.bindOpusPacketRoute(chunk)
			if err := s.pushOutput(chunk); err != nil {
				_ = s.CloseWithError(err)
				return
			}
		}
	}
}

func (s *PeerStream) pushOutput(chunk *genx.MessageChunk) error {
	if chunk == nil {
		return nil
	}
	select {
	case <-s.done:
		return s.closeErr()
	case s.out <- chunk:
		return nil
	}
}

func (s *PeerStream) observeAudioRouteBeforeOutput(chunk *genx.MessageChunk) {
	if s == nil || !chunk.IsBeginOfStream() || !peerStreamChunkIsOpusControl(chunk) {
		return
	}
	route := genx.StreamCtrl{}
	if chunk.Ctrl != nil {
		route.StreamID = chunk.Ctrl.StreamID
		route.Label = chunk.Ctrl.Label
	}
	s.audioRouteMu.Lock()
	s.audioRoute = route
	s.audioRouteMu.Unlock()
}

func (s *PeerStream) observeAudioRouteAfterOutput(chunk *genx.MessageChunk) {
	if s == nil || !chunk.IsEndOfStream() || !peerStreamChunkIsOpusControl(chunk) {
		return
	}
	s.audioRouteMu.Lock()
	s.audioRoute = genx.StreamCtrl{}
	s.audioRouteMu.Unlock()
}

func (s *PeerStream) bindOpusPacketRoute(chunk *genx.MessageChunk) *genx.MessageChunk {
	if s == nil || chunk == nil {
		return chunk
	}
	s.audioRouteMu.RLock()
	route := s.audioRoute
	s.audioRouteMu.RUnlock()
	if route.StreamID == "" && route.Label == "" {
		return chunk
	}
	next := chunk.Clone()
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	if route.StreamID != "" {
		next.Ctrl.StreamID = route.StreamID
	}
	if route.Label != "" {
		next.Ctrl.Label = route.Label
	}
	return next
}

func peerStreamChunkIsOpusControl(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	return ok && isOpusBlob(blob)
}

func (s *PeerStream) closeErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	return io.EOF
}

func peerStreamEventToChunk(event *eventpb.PeerEvent) (*genx.MessageChunk, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	ctrl := &genx.StreamCtrl{
		StreamID:  event.StreamID(),
		Label:     event.Label(),
		Timestamp: event.TimestampUnixMilli(),
	}
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		ctrl.BeginOfStream = true
		return peerStreamEventControlChunk(ctrl, event), nil
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS:
		ctrl.EndOfStream = true
		if streamErr := event.StreamError(); streamErr != nil {
			ctrl.Error = streamErr.GetMessage()
			ctrl.ErrorCode = streamErr.GetCode()
			ctrl.ErrorRetryable = streamErr.GetRetryable()
			if ctrl.Error == "" {
				ctrl.Error = ctrl.ErrorCode
			}
		}
		return peerStreamEventControlChunk(ctrl, event), nil
	case eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED:
		ctrl.Label = "workspace.history.updated"
		return &genx.MessageChunk{Ctrl: ctrl}, nil
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(event.Text()), Ctrl: ctrl}, nil
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		ctrl.EndOfStream = true
		return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(event.Text()), Ctrl: ctrl}, nil
	default:
		return nil, fmt.Errorf("gizclaw: unsupported peer stream event type %s", event.Type)
	}
}

func peerStreamEventControlChunk(ctrl *genx.StreamCtrl, event *eventpb.PeerEvent) *genx.MessageChunk {
	chunk := &genx.MessageChunk{Ctrl: ctrl}
	if blob := peerStreamEventBlobPart(event); blob != nil {
		chunk.Part = blob
	}
	return chunk
}

func peerStreamEventBlobPart(event *eventpb.PeerEvent) *genx.Blob {
	mimeType := ""
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		mimeType = event.GetBos().GetMimeType()
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS:
		mimeType = event.GetEos().GetMimeType()
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" && event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_AUDIO {
		mimeType = "audio/opus"
	}
	if mimeType == "" {
		return nil
	}
	return &genx.Blob{MIMEType: mimeType}
}

func peerStreamEventsFromChunk(chunk *genx.MessageChunk) []*eventpb.PeerEvent {
	if chunk == nil {
		return nil
	}
	var out []*eventpb.PeerEvent
	if chunk.IsBeginOfStream() {
		out = append(out, peerStreamEventFromChunk(chunk, eventpb.PeerEventType_PEER_EVENT_TYPE_BOS, nil))
	}
	if text, ok := chunk.Part.(genx.Text); ok {
		value := string(text)
		eventType := eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA
		if chunk.IsEndOfStream() {
			eventType = eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE
		}
		out = append(out, peerStreamEventFromChunk(chunk, eventType, &value))
		return out
	}
	if chunk.IsEndOfStream() {
		out = append(out, peerStreamEventFromChunk(chunk, eventpb.PeerEventType_PEER_EVENT_TYPE_EOS, nil))
	}
	return out
}

func peerStreamEventFromChunk(chunk *genx.MessageChunk, eventType eventpb.PeerEventType, text *string) *eventpb.PeerEvent {
	ctrl := &genx.StreamCtrl{}
	if chunk != nil && chunk.Ctrl != nil {
		ctrl = chunk.Ctrl
	}
	switch eventType {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_Bos{Bos: &eventpb.StreamBegin{
				StreamId:        ctrl.StreamID,
				TimestampUnixMs: ctrl.Timestamp,
				Kind:            peerStreamKindFromChunk(chunk),
				Label:           ctrl.Label,
				MimeType:        peerStreamChunkMIMEType(chunk),
			}},
		}
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS:
		var eventErr *eventpb.EventError
		if ctrl.Error != "" || ctrl.ErrorCode != "" {
			code := ctrl.ErrorCode
			if code == "" {
				code = "STREAM_ERROR"
			}
			eventErr = &eventpb.EventError{Code: code, Message: ctrl.Error, Retryable: ctrl.ErrorRetryable}
		}
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
				StreamId:        ctrl.StreamID,
				TimestampUnixMs: ctrl.Timestamp,
				Kind:            peerStreamKindFromChunk(chunk),
				Label:           ctrl.Label,
				MimeType:        peerStreamChunkMIMEType(chunk),
				Error:           eventErr,
			}},
		}
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{
				StreamId:        ctrl.StreamID,
				TimestampUnixMs: ctrl.Timestamp,
				Label:           ctrl.Label,
				Text:            peerStreamStringValue(text),
			}},
		}
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		return &eventpb.PeerEvent{
			Version: eventpb.Version,
			Type:    eventType,
			Payload: &eventpb.PeerEvent_TextDone{TextDone: &eventpb.TextDone{
				StreamId:        ctrl.StreamID,
				TimestampUnixMs: ctrl.Timestamp,
				Label:           ctrl.Label,
				Text:            peerStreamStringValue(text),
			}},
		}
	default:
		return nil
	}
}

func peerStreamKindFromChunk(chunk *genx.MessageChunk) eventpb.StreamKind {
	if chunk == nil {
		return eventpb.StreamKind_STREAM_KIND_UNSPECIFIED
	}
	if _, ok := chunk.Part.(genx.Text); ok {
		return eventpb.StreamKind_STREAM_KIND_TEXT
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok {
		mimeType := strings.ToLower(strings.TrimSpace(blob.MIMEType))
		switch {
		case strings.HasPrefix(mimeType, "audio/"):
			return eventpb.StreamKind_STREAM_KIND_AUDIO
		case strings.HasPrefix(mimeType, "video/"):
			return eventpb.StreamKind_STREAM_KIND_VIDEO
		}
	}
	return eventpb.StreamKind_STREAM_KIND_UNSPECIFIED
}

func peerStreamChunkMIMEType(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return ""
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok && blob != nil {
		return blob.MIMEType
	}
	return ""
}

func peerStreamStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isOpusBlob(blob *genx.Blob) bool {
	if blob == nil {
		return false
	}
	return peerStreamBaseMIME(blob.MIMEType) == "audio/opus"
}

func peerStreamBaseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func opusPacketChunk(payload []byte) (*genx.MessageChunk, bool) {
	if len(payload) == 0 {
		return nil, false
	}
	return &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: append([]byte(nil), payload...)},
		Ctrl: &genx.StreamCtrl{StreamID: "audio"},
	}, true
}
