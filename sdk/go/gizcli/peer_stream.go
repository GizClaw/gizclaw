package gizcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
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
		events:      eventStream,
		packets:     packets,
		unsubscribe: unsubscribe,
		conn:        c.PeerConn(),
		out:         make(chan *genx.MessageChunk, buffer),
		done:        make(chan struct{}),
	}
	go stream.readEvents()
	go stream.readPackets()
	return stream, nil
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

func peerStreamEventToChunk(event apitypes.PeerStreamEvent) (*genx.MessageChunk, error) {
	ctrl := &genx.StreamCtrl{}
	if event.StreamId != nil {
		ctrl.StreamID = *event.StreamId
	}
	if event.Label != nil {
		ctrl.Label = *event.Label
	}
	if event.Error != nil {
		ctrl.Error = *event.Error
	}
	if event.Timestamp != nil {
		ctrl.Timestamp = *event.Timestamp
	}
	switch event.Type {
	case apitypes.PeerStreamEventTypeBos:
		ctrl.BeginOfStream = true
		return peerStreamEventControlChunk(ctrl, event), nil
	case apitypes.PeerStreamEventTypeEos:
		ctrl.EndOfStream = true
		return peerStreamEventControlChunk(ctrl, event), nil
	case apitypes.PeerStreamEventTypeWorkspaceHistoryUpdated:
		ctrl.Label = "workspace.history.updated"
		if event.LastUpdatedAt != nil {
			ctrl.Timestamp = event.LastUpdatedAt.UTC().UnixMilli()
		} else if event.Timestamp != nil {
			ctrl.Timestamp = *event.Timestamp
		}
		return &genx.MessageChunk{Ctrl: ctrl}, nil
	case apitypes.PeerStreamEventTypeTextDelta:
		text := ""
		if event.Text != nil {
			text = *event.Text
		}
		return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text), Ctrl: ctrl}, nil
	case apitypes.PeerStreamEventTypeTextDone:
		ctrl.EndOfStream = true
		text := ""
		if event.Text != nil {
			text = *event.Text
		}
		return &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text), Ctrl: ctrl}, nil
	default:
		return nil, fmt.Errorf("gizclaw: unsupported peer stream event type %q", event.Type)
	}
}

func peerStreamEventControlChunk(ctrl *genx.StreamCtrl, event apitypes.PeerStreamEvent) *genx.MessageChunk {
	chunk := &genx.MessageChunk{Ctrl: ctrl}
	if blob := peerStreamEventBlobPart(event); blob != nil {
		chunk.Part = blob
	}
	return chunk
}

func peerStreamEventBlobPart(event apitypes.PeerStreamEvent) *genx.Blob {
	mimeType := ""
	if event.MimeType != nil {
		mimeType = strings.TrimSpace(*event.MimeType)
	}
	if mimeType == "" && event.Kind != nil && *event.Kind == apitypes.PeerStreamKindAudio {
		mimeType = "audio/opus"
	}
	if mimeType == "" {
		return nil
	}
	return &genx.Blob{MIMEType: mimeType}
}

func peerStreamEventsFromChunk(chunk *genx.MessageChunk) []apitypes.PeerStreamEvent {
	if chunk == nil {
		return nil
	}
	var out []apitypes.PeerStreamEvent
	if chunk.IsBeginOfStream() {
		out = append(out, peerStreamEventFromChunk(chunk, apitypes.PeerStreamEventTypeBos, nil))
	}
	if text, ok := chunk.Part.(genx.Text); ok {
		value := string(text)
		eventType := apitypes.PeerStreamEventTypeTextDelta
		if chunk.IsEndOfStream() {
			eventType = apitypes.PeerStreamEventTypeTextDone
		}
		out = append(out, peerStreamEventFromChunk(chunk, eventType, &value))
		return out
	}
	if chunk.IsEndOfStream() {
		out = append(out, peerStreamEventFromChunk(chunk, apitypes.PeerStreamEventTypeEos, nil))
	}
	return out
}

func peerStreamEventFromChunk(chunk *genx.MessageChunk, eventType apitypes.PeerStreamEventType, text *string) apitypes.PeerStreamEvent {
	event := apitypes.PeerStreamEvent{
		V:    peerStreamEventVersion,
		Type: eventType,
		Text: text,
	}
	if chunk == nil || chunk.Ctrl == nil {
		return event
	}
	if chunk.Ctrl.StreamID != "" {
		event.StreamId = &chunk.Ctrl.StreamID
	}
	if chunk.Ctrl.Label != "" {
		event.Label = &chunk.Ctrl.Label
	}
	if chunk.Ctrl.Error != "" {
		event.Error = &chunk.Ctrl.Error
	}
	if chunk.Ctrl.Timestamp != 0 {
		event.Timestamp = &chunk.Ctrl.Timestamp
	}
	if kind := peerStreamKindFromChunk(chunk); kind != nil {
		event.Kind = kind
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType != "" {
		event.MimeType = &blob.MIMEType
	}
	return event
}

func peerStreamKindFromChunk(chunk *genx.MessageChunk) *apitypes.PeerStreamKind {
	if chunk == nil {
		return nil
	}
	if _, ok := chunk.Part.(genx.Text); ok {
		kind := apitypes.PeerStreamKindText
		return &kind
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok {
		mimeType := strings.ToLower(strings.TrimSpace(blob.MIMEType))
		switch {
		case strings.HasPrefix(mimeType, "audio/"):
			kind := apitypes.PeerStreamKindAudio
			return &kind
		case strings.HasPrefix(mimeType, "video/"):
			kind := apitypes.PeerStreamKindVideo
			return &kind
		}
	}
	return nil
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
