package gizcli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestDialPeerEventStreamValidation(t *testing.T) {
	var nilClient *Client
	if _, err := nilClient.DialPeerEventStream(); err == nil || !strings.Contains(err.Error(), "nil client") {
		t.Fatalf("nil DialPeerEventStream() error = %v", err)
	}
	if _, err := (&Client{}).DialPeerEventStream(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("unconnected DialPeerEventStream() error = %v", err)
	}
}

func TestPeerStreamEventHelpers(t *testing.T) {
	text := "hello"
	event := apitypes.PeerStreamEvent{
		Type: apitypes.PeerStreamEventTypeTextDelta,
		Text: &text,
	}
	var buf bytes.Buffer
	if err := WritePeerStreamEvent(&buf, event); err != nil {
		t.Fatalf("WritePeerStreamEvent() error = %v", err)
	}
	got, err := ReadPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent() error = %v", err)
	}
	if got.V != 1 || got.Type != event.Type || got.Text == nil || *got.Text != text {
		t.Fatalf("event = %+v", got)
	}
	buf.Reset()
	if err := rpcapi.WriteFrame(&buf, rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: []byte(`{"v":1,"type":"text.delta","text":"hello"}`)}); err != nil {
		t.Fatalf("WriteFrame(JSON) error = %v", err)
	}
	got, err = ReadPeerStreamEvent(&buf)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent(JSON) error = %v", err)
	}
	if got.V != 1 || got.Type != event.Type || got.Text == nil || *got.Text != text {
		t.Fatalf("json event = %+v", got)
	}
	if _, err := ReadPeerStreamEvent(bytes.NewBufferString("bad")); err == nil {
		t.Fatal("ReadPeerStreamEvent() succeeded for bad frame")
	}
}

func TestPeerStreamPushWritesEventsAndOpus(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
	stream := &PeerStream{
		events: clientSide,
		conn:   writer,
		out:    make(chan *genx.MessageChunk, 1),
		done:   make(chan struct{}),
	}
	defer stream.Close()

	streamID := "s1"
	label := "mic"
	pushErr := make(chan error, 1)
	go func() {
		pushErr <- stream.Push(context.Background(), &genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0x01, 0x02, 0x03}},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: true},
		})
	}()
	event, err := ReadPeerStreamEvent(serverSide)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent() error = %v", err)
	}
	if err := <-pushErr; err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if event.Type != apitypes.PeerStreamEventTypeBos || event.StreamId == nil || *event.StreamId != streamID || event.Label == nil || *event.Label != label {
		t.Fatalf("event = %+v, want BOS with stream metadata", event)
	}
	select {
	case payload := <-writer.ch:
		if !bytes.Equal(payload, []byte{0x01, 0x02, 0x03}) {
			t.Fatalf("packet frame=%x", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for opus packet")
	}

	pushErr = make(chan error, 1)
	errorMessage := "mime changed"
	go func() {
		pushErr <- stream.Push(context.Background(), &genx.MessageChunk{
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true, Error: errorMessage},
		})
	}()
	event, err = ReadPeerStreamEvent(serverSide)
	if err != nil {
		t.Fatalf("ReadPeerStreamEvent(EOS) error = %v", err)
	}
	if err := <-pushErr; err != nil {
		t.Fatalf("Push(EOS) error = %v", err)
	}
	if event.Type != apitypes.PeerStreamEventTypeEos || event.StreamId == nil || *event.StreamId != streamID || event.Error == nil || *event.Error != errorMessage {
		t.Fatalf("event = %+v, want EOS", event)
	}
}

func TestPeerStreamEventToChunkPreservesAudioEOSMIME(t *testing.T) {
	streamID := "s1"
	mimeType := "audio/opus"
	chunk, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{
		Type:     apitypes.PeerStreamEventTypeEos,
		StreamId: &streamID,
		MimeType: &mimeType,
	})
	if err != nil {
		t.Fatalf("peerStreamEventToChunk() error = %v", err)
	}
	if chunk.Ctrl == nil || !chunk.Ctrl.EndOfStream || chunk.Ctrl.StreamID != streamID {
		t.Fatalf("chunk ctrl = %#v, want EOS stream", chunk.Ctrl)
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob.MIMEType != mimeType || len(blob.Data) != 0 {
		t.Fatalf("chunk part = %#v, want empty audio blob", chunk.Part)
	}
}

func TestPeerStreamEventToChunkAcceptsWorkspaceHistoryUpdated(t *testing.T) {
	lastUpdated := time.Date(2026, 6, 22, 12, 0, 0, 123000000, time.UTC)
	chunk, err := peerStreamEventToChunk(apitypes.PeerStreamEvent{
		Type:          apitypes.PeerStreamEventTypeWorkspaceHistoryUpdated,
		LastUpdatedAt: &lastUpdated,
	})
	if err != nil {
		t.Fatalf("peerStreamEventToChunk() error = %v", err)
	}
	if chunk.Ctrl == nil || chunk.Ctrl.Label != "workspace.history.updated" || chunk.Ctrl.Timestamp != lastUpdated.UnixMilli() {
		t.Fatalf("chunk ctrl = %#v, want workspace history update", chunk.Ctrl)
	}
	if chunk.Part != nil {
		t.Fatalf("chunk part = %#v, want nil", chunk.Part)
	}
}

func TestPeerStreamPushSkipsNilAndOggDirectPacket(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()
	writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
	stream := &PeerStream{
		events: clientSide,
		conn:   writer,
		out:    make(chan *genx.MessageChunk, 1),
		done:   make(chan struct{}),
	}
	defer stream.Close()

	if err := stream.Push(context.Background(), nil); err != nil {
		t.Fatalf("Push(nil) error = %v", err)
	}
	if err := stream.Push(context.Background(), &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: []byte("OggS")},
	}); err != nil {
		t.Fatalf("Push(audio/ogg) error = %v", err)
	}
	select {
	case payload := <-writer.ch:
		t.Fatalf("audio/ogg was written as direct opus: %x", payload)
	default:
	}
}

func TestPeerStreamNextReadsEventsAndOpus(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer serverSide.Close()
	packets := make(chan []byte, 1)
	stream := &PeerStream{
		events:  clientSide,
		packets: packets,
		out:     make(chan *genx.MessageChunk, 2),
		done:    make(chan struct{}),
	}
	defer stream.Close()
	go stream.readEvents()
	go stream.readPackets()

	text := "hello"
	if err := WritePeerStreamEvent(serverSide, apitypes.PeerStreamEvent{Type: apitypes.PeerStreamEventTypeTextDelta, Text: &text}); err != nil {
		t.Fatalf("WritePeerStreamEvent() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next(event) error = %v", err)
	}
	if string(chunk.Part.(genx.Text)) != text {
		t.Fatalf("event chunk text = %q, want %q", chunk.Part, text)
	}

	errorMessage := "mime changed"
	if err := WritePeerStreamEvent(serverSide, apitypes.PeerStreamEvent{Type: apitypes.PeerStreamEventTypeEos, Error: &errorMessage}); err != nil {
		t.Fatalf("WritePeerStreamEvent(EOS) error = %v", err)
	}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(EOS) error = %v", err)
	}
	if !chunk.IsEndOfStream() || chunk.Ctrl.Error != errorMessage {
		t.Fatalf("EOS chunk = %#v, want error %q", chunk, errorMessage)
	}

	streamID := "history-replay-1"
	label := "transcript"
	audioKind := apitypes.PeerStreamKindAudio
	mimeType := "audio/opus"
	if err := WritePeerStreamEvent(serverSide, apitypes.PeerStreamEvent{
		Type:     apitypes.PeerStreamEventTypeBos,
		StreamId: &streamID,
		Label:    &label,
		Kind:     &audioKind,
		MimeType: &mimeType,
	}); err != nil {
		t.Fatalf("WritePeerStreamEvent(audio BOS) error = %v", err)
	}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(audio BOS) error = %v", err)
	}
	if !chunk.IsBeginOfStream() || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != label {
		t.Fatalf("audio BOS chunk = %#v", chunk)
	}

	packets <- []byte{0x04, 0x05}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(packet) error = %v", err)
	}
	blob := chunk.Part.(*genx.Blob)
	if blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{0x04, 0x05}) || chunk.Ctrl.Timestamp != 0 || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != label {
		t.Fatalf("packet chunk = %#v", chunk)
	}

	if err := WritePeerStreamEvent(serverSide, apitypes.PeerStreamEvent{
		Type:     apitypes.PeerStreamEventTypeEos,
		StreamId: &streamID,
		Label:    &label,
		Kind:     &audioKind,
		MimeType: &mimeType,
	}); err != nil {
		t.Fatalf("WritePeerStreamEvent(audio EOS) error = %v", err)
	}
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(audio EOS) error = %v", err)
	}
	if !chunk.IsEndOfStream() || chunk.Ctrl.StreamID != streamID || chunk.Ctrl.Label != label {
		t.Fatalf("audio EOS chunk = %#v", chunk)
	}
}

func TestClientTransformBridgesInputToPeerStream(t *testing.T) {
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	peer := &PeerStream{
		out:  make(chan *genx.MessageChunk, 2),
		done: make(chan struct{}),
	}
	var pushed []*genx.MessageChunk
	peer.push = func(_ context.Context, chunk *genx.MessageChunk) error {
		pushed = append(pushed, chunk.Clone())
		if text, ok := chunk.Part.(genx.Text); ok {
			peer.out <- &genx.MessageChunk{Part: genx.Text("echo:" + string(text))}
		}
		return nil
	}
	client := &Client{
		openPeerStream: func(int) (*PeerStream, error) {
			return peer, nil
		},
	}
	output, err := client.Transform(context.Background(), input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Add(&genx.MessageChunk{Part: genx.Text("hello")}); err != nil {
		t.Fatalf("input Add() error = %v", err)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input Done() error = %v", err)
	}
	chunk, err := output.Next()
	if err != nil {
		t.Fatalf("output Next() error = %v", err)
	}
	if got := string(chunk.Part.(genx.Text)); got != "echo:hello" {
		t.Fatalf("output text = %q", got)
	}
	deadline := time.After(time.Second)
	for len(pushed) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pushed input")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := string(pushed[0].Part.(genx.Text)); got != "hello" {
		t.Fatalf("pushed text = %q", got)
	}
}

type recordingPeerPacketWriter struct {
	ch chan []byte
}

func (w *recordingPeerPacketWriter) Write(protocol byte, payload []byte) (int, error) {
	if protocol != giznet.ProtocolOpusPacket {
		return 0, nil
	}
	w.ch <- append([]byte(nil), payload...)
	return len(payload), nil
}
