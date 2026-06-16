package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func TestPeerStreamEventFrameRoundTrip(t *testing.T) {
	text := "hello"
	streamID := "s1"
	event := apitypes.PeerStreamEvent{
		V:        1,
		Type:     apitypes.PeerStreamEventTypeTextDelta,
		StreamId: &streamID,
		Text:     &text,
	}
	var buf bytes.Buffer
	if err := writePeerStreamEvent(&buf, event); err != nil {
		t.Fatalf("writePeerStreamEvent() error = %v", err)
	}
	got, err := readPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("readPeerStreamEvent() error = %v", err)
	}
	if got.Type != event.Type || got.StreamId == nil || *got.StreamId != streamID || got.Text == nil || *got.Text != text {
		t.Fatalf("round trip event = %+v, want %+v", got, event)
	}
}

func TestPeerStreamEventChunkMapping(t *testing.T) {
	label := "mic"
	streamID := "s1"
	text := "hello"
	errorMessage := "mime changed"
	timestamp := int64(123)
	event := apitypes.PeerStreamEvent{
		V:         1,
		Type:      apitypes.PeerStreamEventTypeTextDelta,
		StreamId:  &streamID,
		Label:     &label,
		Text:      &text,
		Timestamp: &timestamp,
	}
	chunk, err := peerStreamEventToChunk(event)
	if err != nil {
		t.Fatalf("peerStreamEventToChunk() error = %v", err)
	}
	if chunk.Role != genx.RoleUser || string(chunk.Part.(genx.Text)) != text || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != label || chunk.Ctrl.Timestamp != timestamp {
		t.Fatalf("chunk = %#v, want mapped text event", chunk)
	}
	events := peerStreamEventsFromChunk(chunk)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	got := events[0]
	if got.Type != apitypes.PeerStreamEventTypeTextDelta || got.Text == nil || *got.Text != text || got.StreamId == nil || *got.StreamId != streamID {
		t.Fatalf("event from chunk = %+v", got)
	}
	if got.Label == nil || *got.Label != label {
		t.Fatalf("event label = %#v, want %q", got.Label, label)
	}

	eos, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{V: 1, Type: apitypes.PeerStreamEventTypeEos, StreamId: &streamID, Error: &errorMessage})
	if err != nil {
		t.Fatalf("eos peerStreamEventToChunk() error = %v", err)
	}
	if !eos.IsEndOfStream() || eos.Ctrl.Error != errorMessage {
		t.Fatalf("eos chunk = %#v, want end of stream", eos)
	}
	events = peerStreamEventsFromChunk(eos)
	if len(events) != 1 || events[0].Error == nil || *events[0].Error != errorMessage {
		t.Fatalf("event error = %+v, want %q", events, errorMessage)
	}
}

func TestPeerAgentOutputWritesStampedOpus(t *testing.T) {
	writer := &recordingDirectPackets{ch: make(chan []byte, 1)}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01, 0x02, 0x03}},
			Ctrl: &genx.StreamCtrl{Timestamp: 123},
		},
	}, doneErr: genx.ErrDone}
	err := (peerAgentOutput{Events: newPeerStreamEventBroker(), Conn: writer}).ConsumeAgentOutput(context.Background(), output)
	if err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
	select {
	case payload := <-writer.ch:
		timestamp, frame, ok := stampedopus.Unpack(payload)
		if !ok {
			t.Fatalf("output payload is not stamped opus: %x", payload)
		}
		if timestamp != 123 || !bytes.Equal(frame, []byte{0x01, 0x02, 0x03}) {
			t.Fatalf("output opus timestamp=%d frame=%x", timestamp, frame)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for opus output")
	}
}

func TestPeerAgentOutputSkipsOggDirectPacket(t *testing.T) {
	writer := &recordingDirectPackets{ch: make(chan []byte, 1)}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{
			Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")},
		},
	}, doneErr: genx.ErrDone}
	err := (peerAgentOutput{Events: newPeerStreamEventBroker(), Conn: writer}).ConsumeAgentOutput(context.Background(), output)
	if err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
	select {
	case payload := <-writer.ch:
		t.Fatalf("audio/ogg was written as direct stamped opus: %x", payload)
	default:
	}
}

func TestPeerAgentOutputReusesPCMTrack(t *testing.T) {
	tracks := &peerStreamFakeTracks{}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000; channels=1", Data: []byte{1, 0}}},
		{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000; channels=1", Data: []byte{2, 0}}},
	}, doneErr: genx.ErrDone}
	err := (peerAgentOutput{Tracks: tracks}).ConsumeAgentOutput(context.Background(), output)
	if err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
	if tracks.created != 1 {
		t.Fatalf("audio tracks created = %d, want 1", tracks.created)
	}
	if len(tracks.track.chunks) != 2 {
		t.Fatalf("track chunks = %d, want 2", len(tracks.track.chunks))
	}
}

type peerStreamSliceStream struct {
	chunks  []*genx.MessageChunk
	doneErr error
}

func (s *peerStreamSliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		if s.doneErr != nil {
			return nil, s.doneErr
		}
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*peerStreamSliceStream) Close() error {
	return nil
}

func (*peerStreamSliceStream) CloseWithError(error) error {
	return nil
}

type peerStreamFakeTracks struct {
	created int
	track   *peerStreamFakeTrack
}

func (t *peerStreamFakeTracks) CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	t.created++
	t.track = &peerStreamFakeTrack{}
	return t.track, nil, nil
}

type peerStreamFakeTrack struct {
	chunks []pcm.Chunk
}

func (t *peerStreamFakeTrack) Write(chunk pcm.Chunk) error {
	t.chunks = append(t.chunks, chunk)
	return nil
}

type recordingDirectPackets struct {
	ch chan []byte
}

func (w *recordingDirectPackets) Write(protocol byte, payload []byte) (int, error) {
	if protocol != ProtocolStampedOpus {
		return 0, errors.New("unexpected protocol")
	}
	w.ch <- append([]byte(nil), payload...)
	return len(payload), nil
}
