package gizcli

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
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
		now:    func() time.Time { return time.UnixMilli(123).UTC() },
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
		timestamp, frame, ok := stampedopus.Unpack(payload)
		if !ok || timestamp != 123 || !bytes.Equal(frame, []byte{0x01, 0x02, 0x03}) {
			t.Fatalf("packet timestamp=%d frame=%x ok=%v", timestamp, frame, ok)
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
		t.Fatalf("audio/ogg was written as direct stamped opus: %x", payload)
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

	packets <- stampedopus.Pack(456, []byte{0x04, 0x05})
	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next(packet) error = %v", err)
	}
	blob := chunk.Part.(*genx.Blob)
	if blob.MIMEType != "audio/opus" || !bytes.Equal(blob.Data, []byte{0x04, 0x05}) || chunk.Ctrl.Timestamp != 456 {
		t.Fatalf("packet chunk = %#v", chunk)
	}
}

type recordingPeerPacketWriter struct {
	ch chan []byte
}

func (w *recordingPeerPacketWriter) Write(protocol byte, payload []byte) (int, error) {
	if protocol != ProtocolStampedOpus {
		return 0, nil
	}
	w.ch <- append([]byte(nil), payload...)
	return len(payload), nil
}
