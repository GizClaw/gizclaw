package ogg

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestPacketDecoderConsumesArbitraryChunks(t *testing.T) {
	var stream bytes.Buffer
	writer, err := NewStreamWriter(&stream, 42)
	if err != nil {
		t.Fatalf("NewStreamWriter() error = %v", err)
	}
	want := [][]byte{
		[]byte("OpusHead-packet"),
		bytes.Repeat([]byte{0x5a}, 600),
		[]byte("audio"),
	}
	for i, packet := range want {
		if _, err := writer.WritePacket(packet, uint64(i+1), i == len(want)-1); err != nil {
			t.Fatalf("WritePacket(%d) error = %v", i, err)
		}
	}

	decoder := &PacketDecoder{}
	var got [][]byte
	raw := stream.Bytes()
	for len(raw) > 0 {
		n := min(7, len(raw))
		packets, err := decoder.Write(raw[:n])
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		for _, packet := range packets {
			got = append(got, packet.Data)
		}
		raw = raw[n:]
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded packets differ: got lengths=%v want lengths=%v", packetLengths(got), packetLengths(want))
	}
}

func TestPacketDecoderCloseRejectsTruncatedInput(t *testing.T) {
	decoder := &PacketDecoder{}
	if _, err := decoder.Write([]byte("OggS")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := decoder.Close(); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("Close() error = %v, want truncated", err)
	}
}

func packetLengths(packets [][]byte) []int {
	lengths := make([]int, len(packets))
	for i := range packets {
		lengths[i] = len(packets[i])
	}
	return lengths
}
