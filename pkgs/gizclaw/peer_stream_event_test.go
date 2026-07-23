package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"google.golang.org/protobuf/proto"
)

func TestPeerStreamEventFrameRoundTrip(t *testing.T) {
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
		Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{
			StreamId: "s1",
			Text:     "hello",
		}},
	}
	var buf bytes.Buffer
	if err := writePeerStreamEvent(&buf, event); err != nil {
		t.Fatalf("writePeerStreamEvent() error = %v", err)
	}
	got, err := readPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("readPeerStreamEvent() error = %v", err)
	}
	if !proto.Equal(got, event) {
		t.Fatalf("round trip event = %+v, want %+v", got, event)
	}
}

func TestPeerStreamEventRejectsJSONFrame(t *testing.T) {
	payload := []byte(`{"v":1,"type":"text.delta","stream_id":"s1","text":"hello"}`)
	var buf bytes.Buffer
	if err := rpcapi.WriteFrame(&buf, rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: payload}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if _, err := readPeerStreamEvent(&buf); err == nil {
		t.Fatal("readPeerStreamEvent() accepted legacy JSON frame")
	}
}

func TestPeerStreamEventBrokerAllowsOneConnectionStream(t *testing.T) {
	broker := newPeerStreamEventBroker()
	first := &bytes.Buffer{}
	unsubscribe, err := broker.Subscribe(first)
	if err != nil {
		t.Fatalf("Subscribe(first) error = %v", err)
	}
	defer unsubscribe()

	if _, err := broker.Subscribe(&bytes.Buffer{}); !errors.Is(err, errPeerEventStreamAlreadyOpen) {
		t.Fatalf("Subscribe(second) error = %v, want errPeerEventStreamAlreadyOpen", err)
	}
}

func TestPeerStreamEventBrokerNotificationDoesNotWaitForPeerWrite(t *testing.T) {
	writer := &peerStreamBlockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	broker := newPeerStreamEventBroker()
	unsubscribe, err := broker.Subscribe(writer)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	t.Cleanup(func() { close(writer.release) })
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		Payload: &eventpb.PeerEvent_WorkspaceHistoryUpdated{
			WorkspaceHistoryUpdated: &eventpb.WorkspaceHistoryUpdated{
				WorkspaceName: "workspace-a",
				WorkspaceKind: eventpb.WorkspaceKind_WORKSPACE_KIND_WORKFLOW,
			},
		},
	}

	returned := make(chan error, 1)
	go func() { returned <- broker.Notify(event) }()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Notify() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Notify() blocked on peer write")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("subscriber writer did not receive notification")
	}
}

func TestPeerStreamEventBrokerReliableBroadcastWaitsForQueueCapacity(t *testing.T) {
	writer := &peerStreamBlockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	broker := newPeerStreamEventBroker()
	unsubscribe, err := broker.Subscribe(writer)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		Payload: &eventpb.PeerEvent_WorkspaceHistoryUpdated{
			WorkspaceHistoryUpdated: &eventpb.WorkspaceHistoryUpdated{
				WorkspaceName: "workspace-a",
				WorkspaceKind: eventpb.WorkspaceKind_WORKSPACE_KIND_WORKFLOW,
			},
		},
	}
	if err := broker.Notify(event); err != nil {
		t.Fatalf("Notify(first) error = %v", err)
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("subscriber writer did not start")
	}
	for index := range peerStreamEventQueueSize {
		if err := broker.Notify(event); err != nil {
			t.Fatalf("Notify(%d) error = %v", index, err)
		}
	}

	returned := make(chan error, 1)
	go func() { returned <- broker.Broadcast(event) }()
	select {
	case err := <-returned:
		t.Fatalf("Broadcast() returned before queue capacity was available: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(writer.release)
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Broadcast() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Broadcast() did not resume after queue capacity became available")
	}
}

type peerStreamBlockingWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *peerStreamBlockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

func TestPeerStreamEventChunkMapping(t *testing.T) {
	const (
		label     = "mic"
		streamID  = "s1"
		text      = "hello"
		timestamp = int64(123)
	)
	event := &eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
		Payload: &eventpb.PeerEvent_TextDelta{TextDelta: &eventpb.TextDelta{
			StreamId:        streamID,
			Label:           label,
			Text:            text,
			TimestampUnixMs: timestamp,
		}},
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
	if got.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA || got.Text() != text || got.StreamID() != streamID {
		t.Fatalf("event from chunk = %+v", got)
	}
	if got.Label() != label {
		t.Fatalf("event label = %q, want %q", got.Label(), label)
	}

	errorMessage := "mime changed"
	eos, err := peerStreamEventToChunk(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Error:    &eventpb.EventError{Code: "MIME_CHANGED", Message: errorMessage},
		}},
	})
	if err != nil {
		t.Fatalf("eos peerStreamEventToChunk() error = %v", err)
	}
	if !eos.IsEndOfStream() || eos.Ctrl.Error != errorMessage {
		t.Fatalf("eos chunk = %#v, want end of stream", eos)
	}
	events = peerStreamEventsFromChunk(eos)
	if len(events) != 1 || events[0].StreamError().GetMessage() != errorMessage {
		t.Fatalf("event error = %+v, want %q", events, errorMessage)
	}

	mimeType := "audio/opus"
	audioEOS, err := peerStreamEventToChunk(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_EOS,
		Payload: &eventpb.PeerEvent_Eos{Eos: &eventpb.StreamEnd{
			StreamId: streamID,
			Kind:     eventpb.StreamKind_STREAM_KIND_AUDIO,
			MimeType: mimeType,
		}},
	})
	if err != nil {
		t.Fatalf("audio eos peerStreamEventToChunk() error = %v", err)
	}
	blob, ok := audioEOS.Part.(*genx.Blob)
	if !ok || blob.MIMEType != mimeType || len(blob.Data) != 0 || !audioEOS.IsEndOfStream() {
		t.Fatalf("audio eos chunk = %#v, want empty audio EOS blob", audioEOS)
	}

	lastUpdated := time.Date(2026, 6, 22, 12, 0, 0, 123000000, time.UTC)
	_, err = peerStreamEventToChunk(&eventpb.PeerEvent{
		Version: eventpb.Version,
		Type:    eventpb.PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
		Payload: &eventpb.PeerEvent_WorkspaceHistoryUpdated{WorkspaceHistoryUpdated: &eventpb.WorkspaceHistoryUpdated{
			WorkspaceName:       "demo",
			WorkspaceKind:       eventpb.WorkspaceKind_WORKSPACE_KIND_WORKFLOW,
			LastUpdatedAtUnixMs: lastUpdated.UnixMilli(),
		}},
	})
	if err == nil {
		t.Fatal("resource invalidation was accepted as agent input")
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
	tracks := &peerStreamFakeTracks{createdCh: make(chan struct{}, 1)}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: "answer"},
		},
	}, doneErr: genx.ErrDone}
	done := make(chan error, 1)
	go func() {
		done <- (peerAgentOutput{Events: newPeerStreamEventBroker(), Tracks: tracks}).ConsumeAgentOutput(context.Background(), output)
	}()
	<-tracks.createdCh
	if err := waitPeerAgentOutputDrain(t, tracks.mixer, done); err != nil {
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
	tracks := &peerStreamFakeTracks{createdCh: make(chan struct{}, 1)}
	output := &peerStreamSliceStream{chunks: []*genx.MessageChunk{
		{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000; channels=1", Data: []byte{1, 0}}},
		{Part: &genx.Blob{MIMEType: "audio/L16; rate=16000; channels=1", Data: []byte{2, 0}}},
	}, doneErr: genx.ErrDone}
	done := make(chan error, 1)
	go func() {
		done <- (peerAgentOutput{Tracks: tracks}).ConsumeAgentOutput(context.Background(), output)
	}()
	<-tracks.createdCh
	if err := waitPeerAgentOutputDrain(t, tracks.mixer, done); err != nil {
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
	created   int
	createdCh chan struct{}
	track     *peerStreamFakeTrack
	mixer     *pcm.Mixer
}

func (t *peerStreamFakeTracks) CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	t.created++
	t.track = &peerStreamFakeTrack{}
	if t.mixer == nil {
		t.mixer = pcm.NewMixer(pcm.L16Mono16K)
	}
	_, ctrl, err := t.mixer.CreateTrack()
	if t.createdCh != nil {
		select {
		case t.createdCh <- struct{}{}:
		default:
		}
	}
	return t.track, ctrl, err
}

func waitPeerAgentOutputDrain(t *testing.T, mixer *pcm.Mixer, done <-chan error) error {
	t.Helper()
	buffer := make([]byte, mixer.Output().BytesInDuration(60*time.Millisecond))
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, err := mixer.Read(buffer); err != nil {
				return
			}
		}
	}()
	select {
	case err := <-done:
		_ = mixer.Close()
		<-readDone
		return err
	case <-time.After(time.Second):
		_ = mixer.Close()
		<-readDone
		t.Fatal("audio output did not finish after mixer drain")
		return nil
	}
}

type peerStreamFakeTrack struct {
	chunks []pcm.Chunk
}

func (t *peerStreamFakeTrack) Write(chunk pcm.Chunk) error {
	t.chunks = append(t.chunks, chunk)
	return nil
}
