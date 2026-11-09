package opusmedia

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream"
	"github.com/pion/rtp"
)

func mediaPackets(t *testing.T, channels int) [][]byte {
	t.Helper()
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	encoder, err := opus.NewEncoder(16000, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	var packets [][]byte
	for frame := range 3 {
		samples := make([]int16, 320*channels)
		for i := range samples {
			samples[i] = int16((i + frame*320) * 97 % 24000)
		}
		data, err := encoder.Encode(samples, 320)
		if err != nil {
			t.Fatal(err)
		}
		packets = append(packets, data)
	}
	return packets
}

func mediaInfo(source string, seq uint16, timestamp uint32) *packet.OpusInfo {
	return &packet.OpusInfo{SourceID: source, Header: rtp.Header{Version: 2, SequenceNumber: seq, Timestamp: timestamp, SSRC: 42, CSRC: []uint32{7}}, Duration48: 960}
}

func mediaReceiver(t *testing.T, channels int) *Receiver {
	t.Helper()
	r, err := NewReceiver(Config{SampleRate: 16000, Channels: channels, Jitter: rtpstream.Config{Wait: 40 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func pcmBytes(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}

func TestReceiverOrdersAndOwnsPacketProvenance(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	later := append([]byte(nil), packets[1]...)
	info := mediaInfo("s", 2, 10960)
	if frames, err := r.Push(later, info, now); err != nil || len(frames) != 0 {
		t.Fatalf("priming=%v,%v", frames, err)
	}
	if frames, err := r.Push(packets[0], mediaInfo("s", 1, 10000), now); err != nil || len(frames) != 0 {
		t.Fatalf("reorder=%v,%v", frames, err)
	}
	later[0] ^= 0xff
	info.Header.CSRC[0] = 99
	if due, ok := r.Deadline(); !ok || !due.Equal(now.Add(40*time.Millisecond)) {
		t.Fatalf("deadline=%v,%t", due, ok)
	}
	frames, err := r.Pop(now.Add(40 * time.Millisecond))
	if err != nil || len(frames) != 2 {
		t.Fatalf("released=%d,%v", len(frames), err)
	}
	reference, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Close()
	for i, frame := range frames {
		want, err := reference.Decode(packets[i], 1920, false)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Concealed || !bytes.Equal(frame.PCM, pcmBytes(want)) || !bytes.Equal(frame.Original, packets[i]) || frame.Info.Header.SequenceNumber != uint16(i+1) || frame.Info.Header.CSRC[0] != 7 {
			t.Fatalf("incorrect PCM/provenance for packet %d", i)
		}
	}
	wantOriginal, wantPCM := bytes.Clone(frames[0].Original), bytes.Clone(frames[0].PCM)
	frames[1].Info.SourceID = "caller-mutated"
	frames[1].Info.Header.Timestamp = 0
	next, err := r.Push(packets[2], mediaInfo("s", 3, 11920), now.Add(40*time.Millisecond))
	if err != nil || len(next) != 1 || next[0].Info.Discontinuity || next[0].Info.SourceID != "s" {
		t.Fatalf("returned metadata changed receiver state: frames=%d error=%v", len(next), err)
	}
	r.Discard()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frames[0].Original, wantOriginal) || !bytes.Equal(frames[0].PCM, wantPCM) || frames[0].Info.Header.CSRC[0] != 7 {
		t.Fatal("later media or cleanup changed retained timed output")
	}
}

func TestReceiverGapUsesSameDecoderAndNeverInventsOpus(t *testing.T) {
	for _, channels := range []int{1, 2} {
		packets := mediaPackets(t, channels)
		r := mediaReceiver(t, channels)
		now := time.Unix(100, 0)
		if _, err := r.Push(packets[0], mediaInfo("s", 10, 10000), now); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Pop(now.Add(40 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if frames, err := r.Push(packets[1], mediaInfo("s", 12, 12880), now.Add(40*time.Millisecond)); err != nil || len(frames) != 0 {
			t.Fatalf("early release=%d,%v", len(frames), err)
		}
		frames, err := r.Pop(now.Add(80 * time.Millisecond))
		if err != nil || len(frames) != 2 {
			t.Fatalf("gap release=%d,%v", len(frames), err)
		}
		gap := frames[0]
		if !gap.Concealed || len(gap.Original) != 0 || gap.Info.Duration48 != 1920 || gap.Info.Header.Timestamp != 10960 || len(gap.PCM) != 640*channels*2 {
			t.Fatalf("inexact or encoded gap: %+v", gap.Info)
		}
		reference, err := opus.NewDecoder(16000, channels)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reference.Decode(packets[0], 1920, false); err != nil {
			t.Fatal(err)
		}
		wantGap, err := reference.Decode(nil, 640, false)
		if err != nil {
			t.Fatal(err)
		}
		wantNext, err := reference.Decode(packets[1], 1920, false)
		_ = reference.Close()
		if err != nil || !bytes.Equal(gap.PCM, pcmBytes(wantGap)) || !bytes.Equal(frames[1].PCM, pcmBytes(wantNext)) || !bytes.Equal(frames[1].Original, packets[1]) {
			t.Fatal("PLC and real packet did not share continuous decoder history")
		}
	}
}

func TestReceiverDiscardResetsPCMAndRetainsSequenceFence(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	if _, err := r.Push(packets[0], mediaInfo("old", 10, 10000), now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Pop(now.Add(40 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(packets[1], mediaInfo("old", 12, 11920), now); err != nil {
		t.Fatal(err)
	}
	r.Discard()
	for _, seq := range []uint16{10, 12} {
		if frames, err := r.Push(packets[0], mediaInfo("old", seq, 10000), now); err != nil || len(frames) != 0 {
			t.Fatalf("old packet %d re-entered after BOS", seq)
		}
	}
	frames, err := r.Push(packets[2], mediaInfo("old", 13, 12880), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) == 0 {
		frames, err = r.Flush()
	}
	if err != nil || len(frames) != 1 || frames[0].Concealed {
		t.Fatalf("BOS produced a gap: %d,%v", len(frames), err)
	}
	fresh, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	want, err := fresh.Decode(packets[2], 1920, false)
	if err != nil || !bytes.Equal(frames[0].PCM, pcmBytes(want)) {
		t.Fatal("BOS retained old native decoder history")
	}
	if _, err := r.Push(packets[0], mediaInfo("new", 1, 90000), now); err != nil {
		t.Fatal(err)
	}
	if frames, err := r.Push(packets[0], mediaInfo("old", 14, 13840), now); err != nil || len(frames) != 0 {
		t.Fatal("retired source re-entered")
	}
	frames, err = r.Flush()
	if err != nil || len(frames) != 1 || frames[0].Info.SourceID != "new" {
		t.Fatal("source replacement did not discard old pending media")
	}
}

func TestReceiverUntimedAudioAndExplicitGap(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	untimed := bytes.Clone(packets[0])
	frames, err := r.Push(untimed, nil, now)
	if err != nil || len(frames) != 1 || len(frames[0].PCM) != 640 {
		t.Fatal("untimed audio acquired a jitter wait")
	}
	retained := frames[0]
	wantPCM := bytes.Clone(retained.PCM)
	clear(untimed)
	if !bytes.Equal(retained.Original, packets[0]) {
		t.Fatal("untimed output retained borrowed packet storage")
	}
	if _, pending := r.Deadline(); pending {
		t.Fatal("untimed audio created a deadline")
	}
	if frames, err := r.Push(nil, nil, now); err != nil || len(frames) != 0 {
		t.Fatal("empty boundary generated PCM")
	}
	gap := mediaInfo("s", 2, 10960)
	gap.Missing = true
	frames, err = r.Push(nil, gap, now)
	if err != nil || len(frames) != 1 || !frames[0].Concealed || len(frames[0].PCM) != 640 || len(frames[0].Original) != 0 {
		t.Fatal("explicit gap was swallowed")
	}
	gap.Header.CSRC[0] = 99
	gap.Header.Timestamp = 1
	if frames[0].Info.Header.CSRC[0] != 7 || frames[0].Info.Header.Timestamp != 10960 {
		t.Fatal("explicit gap retained borrowed metadata")
	}
	// Returned metadata also belongs to the caller, independently of the
	// receiver's retained explicit-gap fence.
	frames[0].Info.SourceID = "caller-mutated"
	frames[0].Info.Header.Timestamp = 0
	if frames, err := r.Push(packets[0], mediaInfo("s", 2, 10960), now); err != nil || len(frames) != 0 {
		t.Fatal("concealed packet replayed")
	}
	if _, err := r.Push(packets[1], mediaInfo("s", 3, 11920), now); err != nil {
		t.Fatal(err)
	}
	frames, err = r.Flush()
	if err != nil || len(frames) != 1 || frames[0].Concealed {
		t.Fatal("explicit gap inferred twice")
	}
	r.Discard()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained.Original, packets[0]) || !bytes.Equal(retained.PCM, wantPCM) {
		t.Fatal("later media or cleanup changed retained untimed output")
	}
}

func TestReceiverCorruptPacketUsesKnownDurationPLCAndRemainsUsable(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	if _, err := r.Push(packets[0], mediaInfo("s", 1, 10000), now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	// Two 20 ms frames with a truncated padding field: the duration is known,
	// but the encoded body is invalid and must never reach recorded originals.
	bad := mediaInfo("s", 2, 10960)
	bad.Duration48 = 1920
	frames, err := r.Push([]byte{0x9b, 0x42}, bad, now)
	if err != nil || len(frames) != 1 || !frames[0].Concealed || len(frames[0].Original) != 0 || len(frames[0].PCM) != 1280 {
		t.Fatalf("corrupt known-duration packet: frames=%d err=%v", len(frames), err)
	}
	reference, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Close()
	if _, err := reference.Decode(packets[0], 1920, false); err != nil {
		t.Fatal(err)
	}
	wantGap, err := reference.Decode(nil, 640, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frames[0].PCM, pcmBytes(wantGap)) {
		t.Fatal("corrupt packet concealment used another decoder")
	}
	frames, err = r.Push(packets[1], mediaInfo("s", 3, 12880), now)
	wantNext, decodeErr := reference.Decode(packets[1], 1920, false)
	if err != nil || decodeErr != nil || len(frames) != 1 || !bytes.Equal(frames[0].PCM, pcmBytes(wantNext)) {
		t.Fatal("corrupt packet terminated or poisoned the next media frame")
	}
	if r.InvalidPackets() != 1 || r.LastDecodeError() == nil {
		t.Fatal("discarded media was not observable")
	}
}

func TestReceiverCorruptUnknownDurationIsDroppedWithoutClosing(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	frames, err := r.Push([]byte{0xff}, nil, now)
	if err != nil || len(frames) != 0 || r.InvalidPackets() != 1 || r.LastDecodeError() == nil {
		t.Fatalf("unknown corrupt packet: frames=%d err=%v invalid=%d", len(frames), err, r.InvalidPackets())
	}
	frames, err = r.Push(packets[0], nil, now)
	if err != nil || len(frames) != 1 || frames[0].Concealed {
		t.Fatal("receiver did not survive a corrupt packet")
	}
}

func TestReceiverFlushCloseAndValidation(t *testing.T) {
	packets := mediaPackets(t, 1)
	if _, err := NewReceiver(Config{SampleRate: 44100}); err == nil {
		t.Fatal("invalid sample rate accepted")
	}
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	if _, err := r.Push(packets[0], mediaInfo("s", 1, 10000), now); err != nil {
		t.Fatal(err)
	}
	frames, err := r.Flush()
	if err != nil || len(frames) != 1 {
		t.Fatal("EOS flush lost pending audio")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(packets[0], nil, now); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed push=%v", err)
	}
	if _, ok := r.Deadline(); ok {
		t.Fatal("closed receiver retained a deadline")
	}
}

func TestReceiverCodecFreeEmptyControls(t *testing.T) {
	r, err := NewReceiver(Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	if frames, err := r.Push(nil, nil, now); err != nil || len(frames) != 0 {
		t.Fatalf("empty push=%d,%v", len(frames), err)
	}
	if frames, err := r.Push(nil, &packet.OpusInfo{SourceID: "s", Discontinuity: true}, now); err != nil || len(frames) != 0 {
		t.Fatalf("empty discontinuity=%d,%v", len(frames), err)
	}
	if frames, err := r.Pop(now); err != nil || len(frames) != 0 {
		t.Fatalf("empty pop=%d,%v", len(frames), err)
	}
	if frames, err := r.Flush(); err != nil || len(frames) != 0 {
		t.Fatalf("empty flush=%d,%v", len(frames), err)
	}
	r.Discard()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	for _, config := range []Config{{SampleRate: 44100}, {SampleRate: -1}, {Channels: 3}, {Channels: -1}} {
		if _, err := NewReceiver(config); err == nil {
			t.Fatalf("accepted invalid receiver format %+v", config)
		}
	}
}

func TestReceiverUnavailableStartup(t *testing.T) {
	if opus.IsRuntimeSupported() {
		t.Skip("requires unavailable native runtime")
	}
	r, err := NewReceiver(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.Push([]byte{0xf8, 0xff, 0xfe}, nil, time.Now()); !errors.Is(err, codecconv.ErrOpusDecoderInit) {
		t.Fatalf("real codec startup failure was swallowed: %v", err)
	}
	if r.InvalidPackets() != 0 || r.LastDecodeError() != nil {
		t.Fatal("native startup failure was classified as corrupt media")
	}
}

func TestReceiverCorruptionCannotRenewConcealmentBudget(t *testing.T) {
	packets := mediaPackets(t, 1)
	for _, unknown := range []bool{false, true} {
		name := "known_duration"
		if unknown {
			name = "unknown_duration_after_exhaustion"
		}
		t.Run(name, func(t *testing.T) {
			r := mediaReceiver(t, 1)
			now := time.Unix(100, 0)
			if _, err := r.Push(packets[0], mediaInfo("s", 1, 10000), now); err != nil {
				t.Fatal(err)
			}
			if frames, err := r.Pop(now.Add(40 * time.Millisecond)); err != nil || len(frames) != 1 {
				t.Fatalf("seed=%d,%v", len(frames), err)
			}
			timestamp := uint32(10960)
			concealedBytes := 0
			for k := 1; k <= 39; k++ {
				payload := []byte{0x9b, 0x42}
				if unknown && k == 13 {
					payload = []byte{0xff}
				}
				info := mediaInfo("s", uint16(k+1), timestamp)
				info.Duration48 = packet.OpusDuration48(payload)
				timestamp += info.Duration48
				frames, err := r.Push(payload, info, now.Add(time.Duration(k)*40*time.Millisecond))
				want := 0
				if k <= 12 {
					want = 1
				}
				if err != nil || len(frames) != want {
					t.Fatalf("corrupt packet %d: frames=%d want=%d err=%v", k, len(frames), want, err)
				}
				for _, frame := range frames {
					if !frame.Concealed || len(frame.Original) != 0 || len(frame.PCM) != 1280 || frame.Info.Discontinuity || frame.Info.SourceID != "s" {
						t.Fatalf("invalid corrupt-packet concealment: %+v", frame.Info)
					}
					concealedBytes += len(frame.PCM)
				}
			}
			if concealedBytes != 15360 || r.InvalidPackets() != 39 || r.LastDecodeError() == nil {
				t.Fatalf("consecutive budget escaped: PCM bytes=%d invalid=%d", concealedBytes, r.InvalidPackets())
			}
			if frames, err := r.Push(packets[1], mediaInfo("s", 41, timestamp), now.Add(2*time.Second)); err != nil || len(frames) != 1 || frames[0].Concealed {
				t.Fatalf("real packet did not resume decoding: %d,%v", len(frames), err)
			}
			bad := mediaInfo("s", 42, timestamp+960)
			bad.Duration48 = 1920
			if frames, err := r.Push([]byte{0x9b, 0x42}, bad, now.Add(2*time.Second)); err != nil || len(frames) != 1 || !frames[0].Concealed {
				t.Fatalf("successful real decode did not renew PLC budget: %d,%v", len(frames), err)
			}
		})
	}
}

func TestReceiverExplicitDiscontinuityResetsSequenceFence(t *testing.T) {
	packets := mediaPackets(t, 1)
	r := mediaReceiver(t, 1)
	now := time.Unix(100, 0)
	if _, err := r.Push(packets[0], mediaInfo("s", 100, 10000), now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Pop(now.Add(40 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	info := mediaInfo("s", 1, 0)
	info.Discontinuity = true
	if _, err := r.Push(packets[1], info, now); err != nil {
		t.Fatal(err)
	}
	frames, err := r.Pop(now.Add(40 * time.Millisecond))
	if err != nil || len(frames) != 1 || frames[0].Concealed || frames[0].Info.Header.SequenceNumber != 1 {
		t.Fatalf("explicit restart retained the old sequence fence: %d,%v", len(frames), err)
	}
}
