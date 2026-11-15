package opusmedia

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
)

func TestPCMEncoderKeepsContinuousFramesAndPadsOnlyTail(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	e, err := NewPCMEncoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if packets, err := e.Push(make([]byte, 320)); err != nil || len(packets) != 0 {
		t.Fatalf("partial=%d,%v", len(packets), err)
	}
	packets, err := e.Push(make([]byte, 640))
	if err != nil || len(packets) != 1 {
		t.Fatalf("full=%d,%v", len(packets), err)
	}
	tail, err := e.Flush()
	if err != nil || len(tail) != 1 {
		t.Fatalf("tail=%d,%v", len(tail), err)
	}
	decoder, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	for _, encoded := range append(packets, tail...) {
		pcm, err := decoder.Decode(encoded, 320, false)
		if err != nil || len(pcm) != 320 || packet.OpusDuration48(encoded) != 960 {
			t.Fatalf("invalid encoded 20 ms frame: %d,%v", len(pcm), err)
		}
	}
	if packets, err := e.Flush(); err != nil || len(packets) != 0 {
		t.Fatal("repeated flush duplicated audio")
	}
	if _, err := e.Push([]byte{1}); err == nil {
		t.Fatal("accepted a partial PCM16 sample")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Push(make([]byte, 640)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed encoder=%v", err)
	}
}

func TestPCMEncoderReusesScratchAndRetainedPacketsOwnStorage(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	encoder, err := NewPCMEncoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	reference, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Close()
	var retained, expected [][]byte
	var sampleScratch *int16
	var outputScratch *byte
	for frame := range 6 {
		pcm := make([]int16, 320)
		data := make([]byte, len(pcm)*2)
		for i := range pcm {
			pcm[i] = int16((i + frame*320) * 97 % 24000)
			binary.LittleEndian.PutUint16(data[i*2:], uint16(pcm[i]))
		}
		packets, err := encoder.Push(data)
		want, refErr := reference.Encode(pcm, 320)
		if err != nil || refErr != nil || len(packets) != 1 || !bytes.Equal(packets[0], want) {
			t.Fatalf("frame %d: packets=%d err=%v reference=%v", frame, len(packets), err, refErr)
		}
		retained, expected = append(retained, packets[0]), append(expected, want)
		if len(encoder.encoded) != opus.DefaultMaxPacketSize || cap(packets[0]) >= opus.DefaultMaxPacketSize {
			t.Fatal("native capacity changed or retained packet kept full native scratch")
		}
		if frame == 0 {
			sampleScratch, outputScratch = &encoder.samples[0], &encoder.encoded[0]
		} else if sampleScratch != &encoder.samples[0] || outputScratch != &encoder.encoded[0] {
			t.Fatal("encoder replaced per-frame sample or native output scratch")
		}
		for i := range retained {
			if !bytes.Equal(retained[i], expected[i]) {
				t.Fatalf("later encode overwrote retained packet %d", i)
			}
		}
	}
	retained[0][0] ^= 0xff
	for i := 1; i < len(retained); i++ {
		if !bytes.Equal(retained[i], expected[i]) {
			t.Fatal("returned packets alias one another")
		}
	}
	data := make([]byte, 640)
	allocations := testing.AllocsPerRun(10, func() {
		packets, err := encoder.Push(data)
		if err != nil || len(packets) != 1 {
			t.Fatalf("warm encoder=%d,%v", len(packets), err)
		}
	})
	if allocations > 2 {
		t.Fatalf("warm frame allocated scratch: %.0f allocations, want only result slice and owned packet", allocations)
	}
}
