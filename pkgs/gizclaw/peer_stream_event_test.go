package gizclaw

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
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

func TestPeerStreamEventReadsJSONFrame(t *testing.T) {
	payload := []byte(`{"v":1,"type":"text.delta","stream_id":"s1","text":"hello"}`)
	var buf bytes.Buffer
	if err := rpcapi.WriteFrame(&buf, rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: payload}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	got, err := readPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("readPeerStreamEvent() error = %v", err)
	}
	if got.Type != apitypes.PeerStreamEventTypeTextDelta || got.Text == nil || *got.Text != "hello" {
		t.Fatalf("readPeerStreamEvent() = %+v", got)
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

	mimeType := "audio/opus"
	audioEOS, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{V: 1, Type: apitypes.PeerStreamEventTypeEos, StreamId: &streamID, MimeType: &mimeType})
	if err != nil {
		t.Fatalf("audio eos peerStreamEventToChunk() error = %v", err)
	}
	blob, ok := audioEOS.Part.(*genx.Blob)
	if !ok || blob.MIMEType != mimeType || len(blob.Data) != 0 || !audioEOS.IsEndOfStream() {
		t.Fatalf("audio eos chunk = %#v, want empty audio EOS blob", audioEOS)
	}

	lastUpdated := time.Date(2026, 6, 22, 12, 0, 0, 123000000, time.UTC)
	historyUpdated, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{
		V:             1,
		Type:          apitypes.PeerStreamEventTypeWorkspaceHistoryUpdated,
		LastUpdatedAt: &lastUpdated,
	})
	if err != nil {
		t.Fatalf("history updated peerStreamEventToChunk() error = %v", err)
	}
	if historyUpdated.Ctrl == nil || historyUpdated.Ctrl.Label != peerStreamEventHistoryUpdatedLabel || historyUpdated.Ctrl.Timestamp != lastUpdated.UnixMilli() {
		t.Fatalf("history updated chunk = %#v", historyUpdated)
	}
	events = peerStreamEventsFromChunk(historyUpdated)
	if len(events) != 1 || events[0].Type != apitypes.PeerStreamEventTypeWorkspaceHistoryUpdated {
		t.Fatalf("history updated events = %+v", events)
	}
	if events[0].LastUpdatedAt == nil || !events[0].LastUpdatedAt.Equal(time.UnixMilli(lastUpdated.UnixMilli()).UTC()) {
		t.Fatalf("history updated last_updated_at = %#v", events[0].LastUpdatedAt)
	}

	historyUpdatedFromTimestamp, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{
		V:         1,
		Type:      apitypes.PeerStreamEventTypeWorkspaceHistoryUpdated,
		Timestamp: &timestamp,
	})
	if err != nil {
		t.Fatalf("history updated timestamp peerStreamEventToChunk() error = %v", err)
	}
	if historyUpdatedFromTimestamp.Ctrl == nil || historyUpdatedFromTimestamp.Ctrl.Timestamp != timestamp {
		t.Fatalf("history updated timestamp chunk = %#v, want timestamp %d", historyUpdatedFromTimestamp, timestamp)
	}
}

func TestPeerAgentOutputDecodesOpusIntoPCMTrack(t *testing.T) {
	const frameSize = 960
	encoder, err := opus.NewEncoder(48000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer encoder.Close()
	packet, err := encoder.Encode(make([]int16, frameSize), frameSize)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	tracks := &peerStreamFakeTracks{}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: "answer"},
		},
	}, doneErr: genx.ErrDone}
	err = (peerAgentOutput{Events: newPeerStreamEventBroker(), Tracks: tracks}).ConsumeAgentOutput(context.Background(), output)
	if err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
	if tracks.created != 1 || len(tracks.track.chunks) != 1 {
		t.Fatalf("tracks created=%d chunks=%d, want 1/1", tracks.created, len(tracks.track.chunks))
	}
	if got := tracks.track.chunks[0].Format(); got != pcm.L16Mono48K {
		t.Fatalf("decoded format = %v, want %v", got, pcm.L16Mono48K)
	}
}

func TestPeerAgentOutputRejectsMalformedOgg(t *testing.T) {
	tracks := &peerStreamFakeTracks{}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{
			Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")},
			Ctrl: &genx.StreamCtrl{StreamID: "answer", EndOfStream: true},
		},
	}, doneErr: genx.ErrDone}
	err := (peerAgentOutput{Events: newPeerStreamEventBroker(), Tracks: tracks}).ConsumeAgentOutput(context.Background(), output)
	if err == nil || !strings.Contains(err.Error(), "stream_id=\"answer\"") {
		t.Fatalf("ConsumeAgentOutput() error = %v, want contextual Ogg error", err)
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
	mixer   *pcm.Mixer
}

func (t *peerStreamFakeTracks) CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	t.created++
	t.track = &peerStreamFakeTrack{}
	if t.mixer == nil {
		t.mixer = pcm.NewMixer(pcm.L16Mono16K)
	}
	_, ctrl, err := t.mixer.CreateTrack()
	return t.track, ctrl, err
}

type peerStreamFakeTrack struct {
	chunks []pcm.Chunk
}

func (t *peerStreamFakeTrack) Write(chunk pcm.Chunk) error {
	t.chunks = append(t.chunks, chunk)
	return nil
}
