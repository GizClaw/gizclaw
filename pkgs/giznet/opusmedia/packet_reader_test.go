package opusmedia

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type rawMediaPacket struct {
	protocol byte
	payload  []byte
	info     *packet.OpusInfo
}

func channelReader(input <-chan rawMediaPacket, entered chan<- struct{}) RawReader {
	return func(ctx context.Context, dst []byte) (byte, int, *packet.OpusInfo, error) {
		if entered != nil {
			select {
			case entered <- struct{}{}:
			case <-ctx.Done():
				return 0, 0, nil, ctx.Err()
			}
		}
		select {
		case <-ctx.Done():
			return 0, 0, nil, ctx.Err()
		case p, ok := <-input:
			if !ok {
				return 0, 0, nil, io.EOF
			}
			return p.protocol, copy(dst, p.payload), p.info, nil
		}
	}
}

func TestPacketReaderDeadlineRestoresReorderAndLoss(t *testing.T) {
	packets := mediaPackets(t, 1)
	input := make(chan rawMediaPacket, 2)
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[2], mediaInfo("a", 12, 1920)}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[0], mediaInfo("a", 10, 0)}
	r, err := NewPacketReader(channelReader(input, nil), Config{Jitter: rtpstream.Config{Wait: time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	buf := make([]byte, 65536)
	for i := range 3 {
		protocol, n, frame, err := r.Read(ctx, buf)
		if err != nil || protocol != giznet.ProtocolOpusPacket || frame == nil {
			t.Fatalf("frame %d: %d %+v %v", i, protocol, frame, err)
		}
		if frame.Info.Header.SequenceNumber != uint16(10+i) || len(frame.PCM) != 640 || frame.Concealed != (i == 1) {
			t.Fatalf("incorrect timed frame %d: %+v", i, frame)
		}
		if i == 1 && (n != 0 || len(frame.Original) != 0) {
			t.Fatal("PLC invented an encoded packet")
		}
		if i != 1 && !bytes.Equal(buf[:n], packets[i]) {
			t.Fatal("original packet changed")
		}
	}
}

func TestPacketReaderFlushInterruptsReadAndWaitsForDispatch(t *testing.T) {
	packets := mediaPackets(t, 1)
	input := make(chan rawMediaPacket, 1)
	entered := make(chan struct{}, 4)
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[0], mediaInfo("a", 10, 0)}
	r, err := NewPacketReader(channelReader(input, entered), Config{Jitter: rtpstream.Config{Wait: time.Hour}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	frames := make(chan *Frame, 2)
	dispatch := make(chan struct{})
	ownerDone := make(chan error, 1)
	go func() {
		defer r.Close()
		buf := make([]byte, 65536)
		for {
			_, _, frame, err := r.Read(ctx, buf)
			if err != nil {
				ownerDone <- err
				return
			}
			frames <- frame
			select {
			case <-dispatch:
			case <-ctx.Done():
				ownerDone <- ctx.Err()
				return
			}
		}
	}()
	// The second raw read proves the first packet is buffered in jitter.
	for range 2 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	flushed := make(chan error, 1)
	go func() { flushed <- r.Flush(ctx) }()
	select {
	case frame := <-frames:
		if frame == nil || !bytes.Equal(frame.Original, packets[0]) {
			t.Fatal("flush lost the buffered packet")
		}
	case <-ctx.Done():
		t.Fatal("control could not interrupt the idle raw read")
	}
	select {
	case err := <-flushed:
		t.Fatalf("flush overtook dispatch: %v", err)
	default:
	}
	select {
	case <-entered:
		t.Fatal("flush read future transport packets while ending the owned media epoch")
	default:
	}
	close(dispatch)
	select {
	case err := <-flushed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("flush did not finish after dispatch")
	}
	// A new epoch's silent interval must not become a synthesized gap.
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[1], mediaInfo("a", 11, 48000)}
	select {
	case frame := <-frames:
		if frame.Concealed || frame.Info.Header.SequenceNumber != 11 {
			t.Fatal("boundary silence became loss")
		}
	case <-ctx.Done():
		t.Fatal("new epoch was blocked")
	}
	cancel()
	select {
	case <-ownerDone:
	case <-time.After(time.Second):
		t.Fatal("packet owner leaked on cancellation")
	}
}

func TestPacketReaderDoesNotHoldOtherPacketsOrEncodedOpus(t *testing.T) {
	packets := mediaPackets(t, 1)
	input := make(chan rawMediaPacket, 3)
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[0], mediaInfo("a", 1, 0)}
	input <- rawMediaPacket{0x70, []byte("telemetry"), nil}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, []byte("legacy payload"), nil}
	r, err := NewPacketReader(channelReader(input, nil), Config{OutputMode: OutputEncoded, Jitter: rtpstream.Config{Wait: time.Hour}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	buf := make([]byte, 65536)
	protocol, n, frame, err := r.Read(ctx, buf)
	if err != nil || protocol != 0x70 || frame != nil || string(buf[:n]) != "telemetry" {
		t.Fatalf("custom packet blocked: %x %v", protocol, err)
	}
	// Untimed media flushes already-held packets without waiting for their
	// reorder deadline, then follows them in the same encoded output mode.
	for _, want := range [][]byte{packets[0], []byte("legacy payload")} {
		protocol, n, frame, err = r.Read(ctx, buf)
		if err != nil || protocol != giznet.ProtocolOpusPacket || frame == nil || len(frame.PCM) != 0 || !bytes.Equal(buf[:n], want) || !bytes.Equal(frame.Original, want) {
			t.Fatalf("encoded packet changed or waited: %x %v", protocol, err)
		}
	}
}

func TestPacketReaderUntimedOpusUsesSelectedOutputMode(t *testing.T) {
	for _, mode := range []OutputMode{OutputPCM, OutputEncoded} {
		if mode == OutputPCM && !opus.IsRuntimeSupported() {
			continue
		}
		payload := []byte{0xf8, 0xff, 0xfe}
		input := make(chan rawMediaPacket, 1)
		input <- rawMediaPacket{giznet.ProtocolOpusPacket, payload, nil}
		r, err := NewPacketReader(channelReader(input, nil), Config{OutputMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		buf := make([]byte, 65536)
		protocol, n, frame, err := r.Read(ctx, buf)
		cancel()
		_ = r.Close()
		if err != nil || protocol != giznet.ProtocolOpusPacket || frame == nil || frame.Info != nil || frame.Concealed || !bytes.Equal(frame.Original, payload) || !bytes.Equal(buf[:n], payload) {
			t.Fatalf("untimed media did not follow mode %d: frame=%+v error=%v", mode, frame, err)
		}
		if mode == OutputPCM && len(frame.PCM) != 640 || mode == OutputEncoded && len(frame.PCM) != 0 {
			t.Fatalf("mode %d produced %d PCM bytes", mode, len(frame.PCM))
		}
	}
}

func TestPacketReaderCancellationLeavesReaderUsable(t *testing.T) {
	input := make(chan rawMediaPacket, 1)
	entered := make(chan struct{}, 1)
	r, err := NewPacketReader(channelReader(input, entered), Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	readCtx, stopRead := context.WithCancel(ctx)
	defer stopRead()
	finished := make(chan error, 1)
	go func() {
		_, _, _, err := r.Read(readCtx, make([]byte, 65536))
		finished <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("packet read did not start")
	}
	stopRead()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled packet read = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("packet read ignored cancellation")
	}
	input <- rawMediaPacket{protocol: 0x70, payload: []byte("telemetry")}
	buf := make([]byte, 65536)
	protocol, n, frame, err := r.Read(ctx, buf)
	if err != nil || protocol != 0x70 || frame != nil || string(buf[:n]) != "telemetry" {
		t.Fatalf("read after cancellation = %x %d %v", protocol, n, err)
	}
}

func TestPacketReaderCloseUnblocksControlWaiter(t *testing.T) {
	r, err := NewPacketReader(channelReader(make(chan rawMediaPacket), nil), Config{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	flushed := make(chan error, 1)
	go func() { flushed <- r.Flush(ctx) }()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-flushed:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("control error = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("control waiter leaked after close")
	}
}

func TestPacketReaderCodecFreeControlsAndBufferBorrowing(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"empty", 0}, {"short", 3}, {"exact_payload", 4}, {"almost_full", 65535}, {"full", 65536}, {"larger", 65537},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := make([]byte, tc.size)
			raw := func(_ context.Context, input []byte) (byte, int, *packet.OpusInfo, error) {
				if len(input) != 65536 {
					t.Fatalf("transport buffer size=%d", len(input))
				}
				if len(dst) >= 65536 && &input[0] != &dst[0] {
					t.Fatal("full caller buffer was not borrowed")
				}
				return 0x70, copy(input, "data"), nil, nil
			}
			r, err := NewPacketReader(raw, Config{})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if r.scratch != nil {
				t.Fatal("constructor eagerly allocated transport scratch")
			}
			for _, discard := range []bool{false, true} {
				fence := make(chan error, 1)
				r.apply(readCommand{discard: discard, fence: fence})
				protocol, n, frame, err := r.Read(t.Context(), dst)
				if len(dst) < 4 {
					if !errors.Is(err, io.ErrShortBuffer) {
						t.Fatalf("short destination error=%v", err)
					}
				} else if err != nil || protocol != 0x70 || frame != nil || n != 4 || string(dst[:n]) != "data" {
					t.Fatalf("nonmedia packet=%x,%d,%v", protocol, n, err)
				}
				if err := <-fence; err != nil {
					t.Fatalf("empty control boundary=%v", err)
				}
			}
			if len(dst) >= 65536 && r.scratch != nil {
				t.Fatal("full caller buffer acquired a duplicate transport buffer")
			}
			if len(dst) < 65536 && len(r.scratch) != 65536 {
				t.Fatal("small destination lost its full transport scratch")
			}
		})
	}
	if _, err := NewPacketReader(nil, Config{}); err == nil {
		t.Fatal("accepted nil raw reader")
	}
}

func TestPacketReaderUnavailableStartup(t *testing.T) {
	if opus.IsRuntimeSupported() {
		t.Skip("requires unavailable native runtime")
	}
	input := make(chan rawMediaPacket, 1)
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, []byte{0xf8, 0xff, 0xfe}, mediaInfo("s", 1, 0)}
	close(input)
	r, err := NewPacketReader(channelReader(input, nil), Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, _, _, err := r.Read(t.Context(), make([]byte, 65536)); !errors.Is(err, codecconv.ErrOpusDecoderInit) {
		t.Fatalf("packet pump swallowed native startup failure: %v", err)
	}
}

func TestPacketReaderBorrowedMediaFramesOwnStorage(t *testing.T) {
	packets := mediaPackets(t, 1)
	input := make(chan rawMediaPacket, 2)
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[0], mediaInfo("s", 1, 0)}
	input <- rawMediaPacket{giznet.ProtocolOpusPacket, packets[1], mediaInfo("s", 2, 960)}
	close(input)
	r, err := NewPacketReader(channelReader(input, nil), Config{Jitter: rtpstream.Config{Wait: time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	dst := make([]byte, 65536)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, first, err := r.Read(ctx, dst)
	if err != nil || first == nil {
		t.Fatalf("first media frame: %v", err)
	}
	original, pcm := bytes.Clone(first.Original), bytes.Clone(first.PCM)
	for i := range dst {
		dst[i] = 0xa5
	}
	_, _, second, err := r.Read(ctx, dst)
	if err != nil || second == nil || !bytes.Equal(second.Original, packets[1]) {
		t.Fatalf("second media frame: %v", err)
	}
	if !bytes.Equal(first.Original, original) || !bytes.Equal(first.PCM, pcm) || !bytes.Equal(first.Original, packets[0]) {
		t.Fatal("retained frame aliased borrowed transport storage")
	}
	if r.scratch != nil {
		t.Fatal("media read eagerly allocated duplicate transport scratch")
	}
}
