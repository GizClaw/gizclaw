package codecconv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
)

func opusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

func opusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}

func buildPacketStream(t *testing.T, packets ...[]byte) []byte {
	t.Helper()

	var out bytes.Buffer
	sw, err := ogg.NewStreamWriter(&out, 66)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	for i, packet := range packets {
		if _, err := sw.WritePacket(packet, uint64(i), i == len(packets)-1); err != nil {
			t.Fatalf("WritePacket %d: %v", i, err)
		}
	}
	return out.Bytes()
}

func buildAudioFrame(frameSize, channels int) []int16 {
	frame := make([]int16, frameSize*channels)
	for i := range frame {
		frame[i] = int16((i * 113) % 30000)
	}
	return frame
}

func buildOGGOpusStream(t *testing.T, sampleRate, channels int, frame []int16) []byte {
	t.Helper()

	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}

	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	frameSize := len(frame) / channels
	packet, err := enc.Encode(frame, frameSize)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return buildPacketStream(t, opusHeadPacket(sampleRate, channels), opusTagsPacket("codecconv-test"), packet)
}

func TestOggToPCM(t *testing.T) {
	raw := buildOGGOpusStream(t, 16000, 1, buildAudioFrame(320, 1))

	var out bytes.Buffer
	n, err := OggToPCM(&out, bytes.NewReader(raw), opus.SampleRate16K)
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("bytes written = %d", n)
	}
	if len(out.Bytes()) == 0 {
		t.Fatal("expected decoded pcm output")
	}
}

func TestOggToPCMResamples(t *testing.T) {
	raw := buildOGGOpusStream(t, 48000, 1, buildAudioFrame(960, 1))

	var out bytes.Buffer
	n, err := OggToPCM(&out, bytes.NewReader(raw), opus.SampleRate16K)
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("bytes written = %d", n)
	}
	if len(out.Bytes()) == 0 {
		t.Fatal("expected resampled pcm output")
	}
}

func TestOpusHeadPacketErrors(t *testing.T) {
	if _, _, err := ParseOpusHeadPacket([]byte("bad")); err == nil {
		t.Fatal("expected non-head error")
	}
	short := opusHeadPacket(16000, 1)[:10]
	if _, _, err := ParseOpusHeadPacket(short); err == nil {
		t.Fatal("expected short packet error")
	}
	packet := opusHeadPacket(0, 1)
	if _, _, err := ParseOpusHeadPacket(packet); err == nil {
		t.Fatal("expected invalid sample rate error")
	}
	packet = opusHeadPacket(16000, 3)
	if _, _, err := ParseOpusHeadPacket(packet); err == nil {
		t.Fatal("expected invalid channels error")
	}
	if !IsOpusHeadPacket(opusHeadPacket(16000, 1)) {
		t.Fatal("expected head packet to be detected")
	}
	if !IsOpusTagsPacket(opusTagsPacket("vendor")) {
		t.Fatal("expected tags packet to be detected")
	}
}

func TestOggToPCMErrors(t *testing.T) {
	if _, err := OggToPCM(io.Discard, bytes.NewReader(nil), opus.OpusSampleRate(0)); err == nil || !strings.Contains(err.Error(), "unsupported sample rate") {
		t.Fatalf("expected invalid sample rate error, got %v", err)
	}

	if _, err := OggToPCM(io.Discard, bytes.NewReader([]byte("bad")), opus.SampleRate16K); err == nil || !strings.Contains(err.Error(), "read ogg packets") {
		t.Fatalf("expected read packets error, got %v", err)
	}

	if _, err := OggToPCM(io.Discard, bytes.NewReader(nil), opus.SampleRate16K); err == nil || !strings.Contains(err.Error(), "empty ogg packet stream") {
		t.Fatalf("expected empty packet error, got %v", err)
	}

	headOnly := buildPacketStream(t, opusHeadPacket(16000, 1), opusTagsPacket("vendor"))
	if _, err := OggToPCM(io.Discard, bytes.NewReader(headOnly), opus.SampleRate16K); err == nil || !strings.Contains(err.Error(), "no opus audio packets") {
		t.Fatalf("expected no audio packets error, got %v", err)
	}

	badHead := buildPacketStream(t, []byte("OpusHead"))
	if _, err := OggToPCM(io.Discard, bytes.NewReader(badHead), opus.SampleRate16K); err == nil || !strings.Contains(err.Error(), "parse opus head packet") {
		t.Fatalf("expected opus head parse error, got %v", err)
	}
}

func TestOggToPCMWriteError(t *testing.T) {
	raw := buildOGGOpusStream(t, 16000, 1, buildAudioFrame(320, 1))

	if _, err := OggToPCM(failWriter{}, bytes.NewReader(raw), opus.SampleRate16K); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestPCMToOggOpusEncoderBuffersAndPads(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	pcmFrame := buildAudioFrame(320, 1)
	pcmBytes := int16ToBytes(pcmFrame)

	var out bytes.Buffer
	enc, err := NewPCMToOggOpusEncoder(&out, 16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatalf("NewPCMToOggOpusEncoder: %v", err)
	}
	if n, err := enc.Write(pcmBytes[:300]); err != nil || n != 300 {
		t.Fatalf("Write first = %d/%v", n, err)
	}
	if n, err := enc.Write(pcmBytes[300:]); err != nil || n != len(pcmBytes)-300 {
		t.Fatalf("Write second = %d/%v", n, err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	packets, err := ogg.ReadAllPackets(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 3 || !IsOpusHeadPacket(packets[0].Data) || !IsOpusTagsPacket(packets[1].Data) || len(packets[2].Data) == 0 {
		t.Fatalf("packets = %+v", packets)
	}
}

func TestPCMToOggOpusEncoderErrors(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	if _, err := NewPCMToOggOpusEncoder(nil, 16000, 1, opus.ApplicationVoIP); err == nil || !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("NewPCMToOggOpusEncoder(nil) error = %v", err)
	}

	var out bytes.Buffer
	enc, err := NewPCMToOggOpusEncoder(&out, 16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatalf("NewPCMToOggOpusEncoder: %v", err)
	}
	if n, err := enc.Write(nil); err != nil || n != 0 {
		t.Fatalf("Write(nil) = %d/%v, want 0/nil", n, err)
	}
	if err := enc.Close(); err == nil || !strings.Contains(err.Error(), "no opus frames") {
		t.Fatalf("Close empty error = %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close after closed = %v", err)
	}
	if _, err := enc.Write([]byte{1, 0}); err == nil || !strings.Contains(err.Error(), "encoder is nil") {
		t.Fatalf("Write after Close error = %v", err)
	}
}

func TestOpusPacketsToOggAndOggOpusPackets(t *testing.T) {
	var out bytes.Buffer
	if err := OpusPacketsToOgg(&out, 48000, 1, [][]byte{{1, 2, 3}, nil, {4, 5}}); err != nil {
		t.Fatalf("OpusPacketsToOgg: %v", err)
	}
	var got [][]byte
	for packet, err := range OggOpusPackets(bytes.NewReader(out.Bytes())) {
		if err != nil {
			t.Fatalf("OggOpusPackets: %v", err)
		}
		got = append(got, packet)
	}
	if len(got) != 2 || !bytes.Equal(got[0], []byte{1, 2, 3}) || !bytes.Equal(got[1], []byte{4, 5}) {
		t.Fatalf("packets = %#v", got)
	}
}

func TestOpusPacketsToOggRejectsInvalidParameters(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sampleRate int
		channels   int
		packets    [][]byte
		want       string
	}{
		{name: "sample rate", sampleRate: 44100, channels: 1, packets: [][]byte{{1}}, want: "unsupported sample rate"},
		{name: "channels", sampleRate: 48000, channels: 3, packets: [][]byte{{1}}, want: "invalid opus channels"},
		{name: "packets", sampleRate: 48000, channels: 1, packets: [][]byte{nil}, want: "no opus packets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := OpusPacketsToOgg(&out, tc.sampleRate, tc.channels, tc.packets)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("OpusPacketsToOgg() error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("OpusPacketsToOgg wrote %d bytes on validation failure", out.Len())
			}
		})
	}
}

func TestOggOpusPacketsPropagatesReadErrors(t *testing.T) {
	for _, err := range OggOpusPackets(strings.NewReader("bad")) {
		if err == nil {
			t.Fatal("OggOpusPackets error = nil")
		}
		return
	}
	t.Fatal("OggOpusPackets produced no result")
}

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}
