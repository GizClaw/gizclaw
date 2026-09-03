package sfu

import (
	"errors"
	"testing"
	"time"
)

func TestOpusPacketDuration(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
		want   time.Duration
		err    error
	}{
		{name: "empty", err: ErrInvalidOpusPacket},
		{name: "silk nb 10ms", packet: []byte{0x00}, want: 10 * time.Millisecond},
		{name: "silk nb 60ms", packet: []byte{0x18}, want: 60 * time.Millisecond},
		{name: "silk wb 20ms code 0", packet: []byte{0x48}, want: 20 * time.Millisecond},
		{name: "silk wb 20ms code 1 two frames", packet: []byte{0x49}, want: 40 * time.Millisecond},
		{name: "hybrid swb 10ms", packet: []byte{0x60}, want: 10 * time.Millisecond},
		{name: "hybrid fb 20ms", packet: []byte{0x78}, want: 20 * time.Millisecond},
		{name: "celt fb 2.5ms", packet: []byte{0xE0}, want: 2500 * time.Microsecond},
		{name: "celt fb 20ms code 2", packet: []byte{0xFA}, want: 40 * time.Millisecond},
		{name: "celt fb 20ms code 3 three frames", packet: []byte{0xFB, 0x03}, want: 60 * time.Millisecond},
		{name: "code 3 missing frame count", packet: []byte{0xFB}, err: ErrInvalidOpusPacket},
		{name: "code 3 zero frames", packet: []byte{0xFB, 0x80}, err: ErrInvalidOpusPacket},
		{name: "code 3 exceeds 120ms", packet: []byte{0x1B, 0x03}, err: ErrInvalidOpusPacket},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := OpusPacketDuration(test.packet)
			if !errors.Is(err, test.err) {
				t.Fatalf("OpusPacketDuration(%x) error = %v, want %v", test.packet, err, test.err)
			}
			if got != test.want {
				t.Fatalf("OpusPacketDuration(%x) = %s, want %s", test.packet, got, test.want)
			}
		})
	}
}
