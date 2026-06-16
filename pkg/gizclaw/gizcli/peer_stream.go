package gizcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

type PeerStream struct {
	events      io.ReadWriteCloser
	packets     <-chan []byte
	unsubscribe func()
	conn        peerPacketWriter
	now         func() time.Time

	out  chan *genx.MessageChunk
	done chan struct{}
	once sync.Once
	mu   sync.Mutex
	err  error
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
	packets, unsubscribe := c.subscribePeerPackets(ProtocolStampedOpus, buffer)
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
	timestamp := uint64(s.nowTime().UnixMilli())
	if chunk.Ctrl != nil && chunk.Ctrl.Timestamp > 0 {
		timestamp = uint64(chunk.Ctrl.Timestamp)
	}
	_, err := s.conn.Write(ProtocolStampedOpus, stampedopus.Pack(timestamp, blob.Data))
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
		if err := s.pushOutput(chunk); err != nil {
			_ = s.CloseWithError(err)
			return
		}
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
			chunk, ok := stampedOpusChunk(payload)
			if !ok {
				continue
			}
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

func (s *PeerStream) closeErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	return io.EOF
}

func (s *PeerStream) nowTime() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
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
		return &genx.MessageChunk{Ctrl: ctrl}, nil
	case apitypes.PeerStreamEventTypeEos:
		ctrl.EndOfStream = true
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

func stampedOpusChunk(payload []byte) (*genx.MessageChunk, bool) {
	timestamp, frame, ok := stampedopus.Unpack(payload)
	if !ok || len(frame) == 0 {
		return nil, false
	}
	return &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/opus", Data: frame},
		Ctrl: &genx.StreamCtrl{StreamID: "audio", Timestamp: int64(timestamp)},
	}, true
}
