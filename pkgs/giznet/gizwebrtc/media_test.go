package gizwebrtc

import (
	"bytes"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

func TestWriteOpusUnpacksStampedFrameAndUsesTOCDuration(t *testing.T) {
	writer := &fakeSampleWriter{}
	conn := &Conn{audioTrack: writer}
	frame := []byte{0x00, 0xaa, 0xbb}
	payload := stampedopus.Pack(1234, frame)

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
	if !bytes.Equal(writer.samples[0].Data, frame) {
		t.Fatalf("sample data = %v, want %v", writer.samples[0].Data, frame)
	}
	if writer.samples[0].Duration != 10*time.Millisecond {
		t.Fatalf("sample duration = %v, want 10ms", writer.samples[0].Duration)
	}
}

func TestWriteOpusRejectsInvalidStampedFrame(t *testing.T) {
	writer := &fakeSampleWriter{}
	conn := &Conn{audioTrack: writer}

	if _, err := conn.writeOpus([]byte{0x99}); err == nil {
		t.Fatal("writeOpus invalid frame error = nil")
	}
	if len(writer.samples) != 0 {
		t.Fatalf("samples written = %d, want 0", len(writer.samples))
	}
}

func TestRemoteOpusFrameRoutesThroughConnReadAsStampedOpus(t *testing.T) {
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
	if protocol != giznet.ProtocolStampedOpusPacket {
		t.Fatalf("protocol = %d, want %d", protocol, giznet.ProtocolStampedOpusPacket)
	}
	ts, gotFrame, ok := stampedopus.Unpack(buf[:n])
	if !ok {
		t.Fatalf("stamped opus unpack failed for %v", buf[:n])
	}
	if ts == 0 {
		t.Fatal("timestamp = 0, want current UnixMilli timestamp")
	}
	if !bytes.Equal(gotFrame, frame) {
		t.Fatalf("frame = %v, want %v", gotFrame, frame)
	}
}

type fakeSampleWriter struct {
	samples []media.Sample
}

func (f *fakeSampleWriter) WriteSample(sample media.Sample) error {
	f.samples = append(f.samples, sample)
	return nil
}
