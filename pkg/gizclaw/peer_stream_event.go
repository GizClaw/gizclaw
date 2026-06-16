package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

const peerStreamEventVersion = 1

type peerStreamEventBroker struct {
	mu      sync.RWMutex
	writers map[io.Writer]*sync.Mutex
}

func newPeerStreamEventBroker() *peerStreamEventBroker {
	return &peerStreamEventBroker{writers: make(map[io.Writer]*sync.Mutex)}
}

func (b *peerStreamEventBroker) Subscribe(w io.Writer) func() {
	if b == nil || w == nil {
		return func() {}
	}
	b.mu.Lock()
	if b.writers == nil {
		b.writers = make(map[io.Writer]*sync.Mutex)
	}
	b.writers[w] = &sync.Mutex{}
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.writers, w)
			b.mu.Unlock()
		})
	}
}

func (b *peerStreamEventBroker) Broadcast(event apitypes.PeerStreamEvent) error {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	writers := make(map[io.Writer]*sync.Mutex, len(b.writers))
	for w, mu := range b.writers {
		writers[w] = mu
	}
	b.mu.RUnlock()
	var errs error
	for w, mu := range writers {
		mu.Lock()
		err := writePeerStreamEvent(w, event)
		mu.Unlock()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

type peerAgentOutput struct {
	Events *peerStreamEventBroker
	Tracks agenthost.AudioTrackCreator
	Conn   peerDirectPacketWriter
	Now    func() time.Time
}

type peerDirectPacketWriter interface {
	Write(byte, []byte) (int, error)
}

func (o peerAgentOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) error {
	if output == nil {
		return fmt.Errorf("gizclaw: agent output stream is required")
	}
	var pcmTrack pcm.Track
	var pcmCtrl *pcm.TrackCtrl
	defer func() {
		if pcmCtrl != nil {
			_ = pcmCtrl.Close()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := output.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				return nil
			}
			return err
		}
		if chunk == nil {
			continue
		}
		for _, event := range peerStreamEventsFromChunk(chunk) {
			if err := o.Events.Broadcast(event); err != nil {
				return err
			}
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if ok && chunk.IsEndOfStream() && isPCMBlob(blob) && pcmCtrl != nil {
			_ = pcmCtrl.Close()
			pcmCtrl = nil
			pcmTrack = nil
			continue
		}
		if !ok || len(blob.Data) == 0 {
			continue
		}
		switch {
		case isOpusBlob(blob):
			if err := o.writeStampedOpus(chunk, blob.Data); err != nil {
				return err
			}
		case isPCMBlob(blob):
			if pcmTrack == nil {
				if o.Tracks == nil {
					return fmt.Errorf("gizclaw: audio track creator is required")
				}
				var err error
				pcmTrack, pcmCtrl, err = o.Tracks.CreateAudioTrack(pcm.WithTrackLabel("agent"))
				if err != nil {
					return err
				}
			}
			if err := pcmTrack.Write(pcm.L16Mono16K.DataChunk(blob.Data)); err != nil {
				return err
			}
		}
	}
}

func (o peerAgentOutput) writeStampedOpus(chunk *genx.MessageChunk, frame []byte) error {
	if o.Conn == nil {
		return fmt.Errorf("gizclaw: peer conn is required for opus output")
	}
	timestamp := uint64(o.now().UnixMilli())
	if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Timestamp > 0 {
		timestamp = uint64(chunk.Ctrl.Timestamp)
	}
	payload := stampedopus.Pack(timestamp, frame)
	_, err := o.Conn.Write(ProtocolStampedOpus, payload)
	return err
}

func (o peerAgentOutput) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

func readPeerStreamEvent(r io.Reader) (apitypes.PeerStreamEvent, error) {
	frame, err := rpcapi.ReadFrame(r)
	if err != nil {
		return apitypes.PeerStreamEvent{}, err
	}
	if frame.Type == rpcapi.FrameTypeEOS {
		return apitypes.PeerStreamEvent{}, io.EOF
	}
	var event apitypes.PeerStreamEvent
	if err := rpcapi.DecodeJSONFrame(frame, &event); err != nil {
		return apitypes.PeerStreamEvent{}, fmt.Errorf("gizclaw: decode peer stream event: %w", err)
	}
	return event, nil
}

func writePeerStreamEvent(w io.Writer, event apitypes.PeerStreamEvent) error {
	if event.V == 0 {
		event.V = peerStreamEventVersion
	}
	frame, err := rpcapi.NewJSONFrame(event)
	if err != nil {
		return err
	}
	return rpcapi.WriteFrame(w, frame)
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
		return &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: ctrl}, nil
	case apitypes.PeerStreamEventTypeTextDone:
		ctrl.EndOfStream = true
		text := ""
		if event.Text != nil {
			text = *event.Text
		}
		return &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: ctrl}, nil
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

func isPCMBlob(blob *genx.Blob) bool {
	if blob == nil {
		return false
	}
	mimeType := peerStreamBaseMIME(blob.MIMEType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
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

func pushAgentChunk(ctx context.Context, source *agenthost.PushSource, chunk *genx.MessageChunk) error {
	if source == nil || chunk == nil {
		return nil
	}
	err := source.Push(ctx, chunk)
	if errors.Is(err, agenthost.ErrNoActiveInput) {
		return nil
	}
	return err
}
