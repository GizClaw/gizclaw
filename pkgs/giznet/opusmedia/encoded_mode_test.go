package opusmedia

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestPacketReaderEncodedModeOrdersOriginalsWithoutDecoder(t *testing.T) {
	input := make(chan rawMediaPacket, 4)
	original := []byte{0xf8, 0xff, 0xfe}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, original, mediaInfo("a", 12, 1920)}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, original, mediaInfo("a", 10, 0)}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, original, mediaInfo("a", 12, 1920)}
	input <- rawMediaPacket{0x70, []byte("control"), nil}
	r, err := NewPacketReader(channelReader(input, nil), Config{OutputMode: OutputEncoded, Jitter: rtpstream.Config{Wait: time.Hour}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	buf := make([]byte, 65536)
	protocol, n, frame, err := r.Read(ctx, buf)
	if err != nil || protocol != 0x70 || frame != nil || string(buf[:n]) != "control" {
		t.Fatalf("encoded jitter blocked unrelated packet: %x,%d,%+v,%v", protocol, n, frame, err)
	}
	close(input) // Transport EOF flushes the bounded retained originals.
	for _, seq := range []uint16{10, 12} {
		protocol, n, frame, err := r.Read(ctx, buf)
		if err != nil || protocol != giznet.ProtocolOpusPacket || frame == nil {
			t.Fatalf("encoded read=%x,%d,%+v,%v", protocol, n, frame, err)
		}
		if frame.Info.Header.SequenceNumber != seq || !bytes.Equal(frame.Original, original) || !bytes.Equal(buf[:n], original) || len(frame.PCM) != 0 || frame.Concealed {
			t.Fatalf("encoded original changed or synthesized: %+v", frame)
		}
	}
	if _, _, frame, err := r.Read(ctx, buf); frame != nil || err == nil {
		t.Fatalf("duplicate or gap became an encoded packet: %+v,%v", frame, err)
	}
}

func TestPacketReaderOutputModeSwitchDiscardsPendingBeforeFence(t *testing.T) {
	input := make(chan rawMediaPacket, 1)
	entered := make(chan struct{}, 8)
	original := []byte{0xf8, 0xff, 0xfe}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, original, mediaInfo("a", 10, 0)}
	r, err := NewPacketReader(channelReader(input, entered), Config{Jitter: rtpstream.Config{Wait: time.Hour}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	frames := make(chan *Frame, 4)
	done := make(chan error, 1)
	go func() {
		defer r.Close()
		buf := make([]byte, 65536)
		for {
			_, _, frame, err := r.Read(ctx, buf)
			if err != nil {
				done <- err
				return
			}
			select {
			case frames <- frame:
			case <-ctx.Done():
				done <- ctx.Err()
				return
			}
		}
	}()
	waitRead := func() {
		t.Helper()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	waitRead()
	waitRead() // PCM packet is pending; native decoding has not started.
	if err := r.SetOutputMode(ctx, OutputEncoded); err != nil || r.OutputMode() != OutputEncoded {
		t.Fatalf("switch to encoded=%v, mode=%d", err, r.OutputMode())
	}
	waitRead()
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, original, mediaInfo("a", 11, 960)}
	waitRead()
	// Re-selecting the current mode must not discard an active encoded route.
	if err := r.SetOutputMode(ctx, OutputEncoded); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-frames:
		if frame == nil || frame.Info.Header.SequenceNumber != 11 || len(frame.PCM) != 0 || !bytes.Equal(frame.Original, original) {
			t.Fatalf("mode fence retained old media or lost new packet: %+v", frame)
		}
	case <-ctx.Done():
		t.Fatal("encoded route was not dispatched before flush completed")
	}
	if err := r.SetOutputMode(ctx, OutputPCM); err != nil || r.OutputMode() != OutputPCM {
		t.Fatalf("switch back to PCM=%v, mode=%d", err, r.OutputMode())
	}
	if err := r.SetOutputMode(ctx, OutputMode(99)); err == nil {
		t.Fatal("accepted invalid output mode")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("mode-switch packet pump did not stop")
	}
	if _, err := NewReceiver(Config{OutputMode: OutputMode(99)}); err == nil {
		t.Fatal("accepted invalid initial output mode")
	}
}
