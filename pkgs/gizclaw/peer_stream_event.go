package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"google.golang.org/protobuf/proto"
)

const peerStreamEventHistoryUpdatedLabel = "workspace.history.updated"

const peerStreamEventQueueSize = 256

var (
	errPeerEventStreamAlreadyOpen = errors.New("gizclaw: peer event stream already open")
	errPeerEventStreamClosed      = errors.New("gizclaw: peer event stream closed")
	errPeerEventQueueFull         = errors.New("gizclaw: peer event stream queue full")
)

type peerStreamEventBroker struct {
	mu         sync.Mutex
	subscriber *peerStreamEventSubscriber
}

func newPeerStreamEventBroker() *peerStreamEventBroker {
	return &peerStreamEventBroker{}
}

type peerStreamEventWrite struct {
	event  *eventpb.PeerEvent
	result chan error
}

type peerStreamEventSubscriber struct {
	writer io.Writer
	queue  chan peerStreamEventWrite
	done   chan struct{}
	once   sync.Once
}

func (b *peerStreamEventBroker) Subscribe(w io.Writer) (func(), error) {
	if b == nil || w == nil {
		return func() {}, errPeerEventStreamClosed
	}
	b.mu.Lock()
	if b.subscriber != nil {
		b.mu.Unlock()
		return func() {}, errPeerEventStreamAlreadyOpen
	}
	subscriber := &peerStreamEventSubscriber{
		writer: w,
		queue:  make(chan peerStreamEventWrite, peerStreamEventQueueSize),
		done:   make(chan struct{}),
	}
	b.subscriber = subscriber
	b.mu.Unlock()
	go b.serveSubscriber(subscriber)
	var once sync.Once
	return func() {
		once.Do(func() {
			b.removeSubscriber(subscriber)
		})
	}, nil
}

// Broadcast writes an ordered event and waits until the current stream accepts
// it. Agent output uses this path so transport failures stop that output.
func (b *peerStreamEventBroker) Broadcast(event *eventpb.PeerEvent) error {
	return b.publish(event, true)
}

// Notify queues a best-effort invalidation without waiting on peer I/O.
// Committed domain mutations must not be blocked by a stalled client.
func (b *peerStreamEventBroker) Notify(event *eventpb.PeerEvent) error {
	return b.publish(event, false)
}

func (b *peerStreamEventBroker) publish(event *eventpb.PeerEvent, wait bool) error {
	if b == nil || event == nil {
		return nil
	}
	b.mu.Lock()
	subscriber := b.subscriber
	b.mu.Unlock()
	if subscriber == nil {
		return nil
	}
	write := peerStreamEventWrite{event: proto.Clone(event).(*eventpb.PeerEvent)}
	if wait {
		write.result = make(chan error, 1)
	}
	if wait {
		select {
		case <-subscriber.done:
			return errPeerEventStreamClosed
		case subscriber.queue <- write:
		}
	} else {
		select {
		case <-subscriber.done:
			return errPeerEventStreamClosed
		case subscriber.queue <- write:
		default:
			return errPeerEventQueueFull
		}
	}
	if write.result == nil {
		return nil
	}
	select {
	case err := <-write.result:
		return err
	case <-subscriber.done:
		select {
		case err := <-write.result:
			return err
		default:
			return errPeerEventStreamClosed
		}
	}
}

func (b *peerStreamEventBroker) serveSubscriber(subscriber *peerStreamEventSubscriber) {
	for {
		select {
		case <-subscriber.done:
			return
		case write := <-subscriber.queue:
			err := writePeerStreamEvent(subscriber.writer, write.event)
			if write.result != nil {
				write.result <- err
			}
			if err != nil {
				b.removeSubscriber(subscriber)
				if closer, ok := subscriber.writer.(io.Closer); ok {
					_ = closer.Close()
				}
				return
			}
		}
	}
}

func (b *peerStreamEventBroker) removeSubscriber(subscriber *peerStreamEventSubscriber) {
	if b == nil || subscriber == nil {
		return
	}
	b.mu.Lock()
	if b.subscriber == subscriber {
		b.subscriber = nil
	}
	subscriber.once.Do(func() { close(subscriber.done) })
	b.mu.Unlock()
}

type peerAgentOutput struct {
	Events *peerStreamEventBroker
	Tracks agenthost.AudioTrackCreator
}

func (o peerAgentOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) error {
	return (agenthost.MixerOutput{
		Tracks:            o.Tracks,
		WaitForAudioDrain: true,
		Observe: func(chunk *genx.MessageChunk) error {
			for _, event := range peerStreamEventsFromChunk(chunk) {
				if err := o.Events.Broadcast(event); err != nil {
					return err
				}
			}
			return nil
		},
	}).ConsumeAgentOutput(ctx, output)
}

func readPeerStreamEvent(r io.Reader) (*eventpb.PeerEvent, error) {
	frame, err := rpcapi.ReadFrame(r)
	if err != nil {
		return nil, err
	}
	if frame.Type == rpcapi.FrameTypeEOS {
		return nil, io.EOF
	}
	if frame.Type != rpcapi.FrameTypeBinary {
		return nil, fmt.Errorf("gizclaw: expected peer stream event binary frame, got type %d", frame.Type)
	}
	event := &eventpb.PeerEvent{}
	if err := proto.Unmarshal(frame.Payload, event); err != nil {
		return nil, fmt.Errorf("gizclaw: decode peer stream event: %w", err)
	}
	if err := event.Validate(); err != nil {
		return nil, fmt.Errorf("gizclaw: validate peer stream event: %w", err)
	}
	return event, nil
}

func writePeerStreamEvent(w io.Writer, event *eventpb.PeerEvent) error {
	if event == nil {
		return fmt.Errorf("gizclaw: peer stream event is nil")
	}
	if event.Version == 0 {
		event = proto.Clone(event).(*eventpb.PeerEvent)
		event.Version = eventpb.Version
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("gizclaw: validate peer stream event: %w", err)
	}
	data, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("gizclaw: encode peer stream event: %w", err)
	}
	return rpcapi.WriteFrame(w, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data})
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
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		return &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(event.Text()), Ctrl: ctrl}, nil
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		ctrl.EndOfStream = true
		return &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(event.Text()), Ctrl: ctrl}, nil
	default:
		return nil, fmt.Errorf("gizclaw: unsupported agent-input peer event type %s", event.Type)
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
	if chunk.Ctrl != nil && chunk.Ctrl.Label == peerStreamEventHistoryUpdatedLabel {
		return out
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
				MimeType:        chunkMIMEType(chunk),
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
				MimeType:        chunkMIMEType(chunk),
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
				Text:            stringValue(text),
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
				Text:            stringValue(text),
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

func chunkMIMEType(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return ""
	}
	if blob, ok := chunk.Part.(*genx.Blob); ok && blob != nil {
		return blob.MIMEType
	}
	return ""
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func isOpusBlob(blob *genx.Blob) bool {
	if blob == nil {
		return false
	}
	mimeType := strings.ToLower(strings.TrimSpace(blob.MIMEType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType == "audio/opus"
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

type agentChunkPusher interface {
	Push(context.Context, *genx.MessageChunk) error
}

func pushAgentChunk(ctx context.Context, source agentChunkPusher, chunk *genx.MessageChunk) error {
	if source == nil || chunk == nil {
		return nil
	}
	err := source.Push(ctx, chunk)
	if errors.Is(err, agenthost.ErrNoActiveInput) {
		return nil
	}
	return err
}
