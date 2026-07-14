package gizwebrtc

import (
	"bytes"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func TestWriteOpusUsesRawFrameAndTOCDuration(t *testing.T) {
	writer := &fakeSampleWriter{}
	conn := &Conn{audioTrack: writer}
	payload := []byte{0x00, 0xaa, 0xbb}

	n, err := conn.writeOpus(payload)
	if err != nil {
		t.Fatalf("writeOpus error = %v", err)
	}
	if n != len(payload) {
		t.Fatalf("writeOpus n = %d, want %d", n, len(payload))
	}
	if len(writer.samples) != 1 {
		t.Fatalf("samples written = %d, want 1", len(writer.samples))
	}
	if !bytes.Equal(writer.samples[0].Data, payload) {
		t.Fatalf("sample data = %v, want %v", writer.samples[0].Data, payload)
	}
	if writer.samples[0].Duration != 10*time.Millisecond {
		t.Fatalf("sample duration = %v, want 10ms", writer.samples[0].Duration)
	}
}

func TestWriteOpusRejectsEmptyFrame(t *testing.T) {
	writer := &fakeSampleWriter{}
	conn := &Conn{audioTrack: writer}

	if _, err := conn.writeOpus(nil); err == nil {
		t.Fatal("writeOpus empty frame error = nil")
	}
	if len(writer.samples) != 0 {
		t.Fatalf("samples written = %d, want 0", len(writer.samples))
	}
}

func TestRemoteOpusFrameRoutesThroughConnReadAsRawOpus(t *testing.T) {
	conn := &Conn{
		pc:      &webrtc.PeerConnection{},
		readCh:  make(chan directPacket, 1),
		closeCh: make(chan struct{}),
	}
	frame := []byte{0x00, 0x10, 0x20}

	conn.enqueueRemoteOpusFrame(frame)

	buf := make([]byte, 64)
	protocol, n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	if protocol != giznet.ProtocolOpusPacket {
		t.Fatalf("protocol = %d, want %d", protocol, giznet.ProtocolOpusPacket)
	}
	if !bytes.Equal(buf[:n], frame) {
		t.Fatalf("frame = %v, want %v", buf[:n], frame)
	}
}

type fakeSampleWriter struct {
	samples []media.Sample
}

func (f *fakeSampleWriter) WriteSample(sample media.Sample) error {
	f.samples = append(f.samples, sample)
	return nil
}
