package codecconv

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
)

func TestOpusPacketDecoderPLCMatchesContinuousDecoder(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	for _, rate := range []int{8000, 12000, 16000, 24000, 48000} {
		for _, channels := range []int{1, 2} {
			t.Run(fmt.Sprintf("%dHz/%dch", rate, channels), func(t *testing.T) {
				packets := buildOpusPackets(t, rate, channels, 2)
				decoder, err := NewOpusPacketDecoder(rate, channels)
				if err != nil {
					t.Fatal(err)
				}
				defer decoder.Close()
				reference, err := opus.NewDecoder(rate, channels)
				if err != nil {
					t.Fatal(err)
				}
				defer reference.Close()
				info := &packet.OpusInfo{SourceID: "source", Header: rtp.Header{SequenceNumber: 41, Timestamp: 9000}}
				first, err := decoder.Decode(packets[0], info)
				if err != nil {
					t.Fatal(err)
				}
				wantFirst, err := reference.Decode(packets[0], rate*3/25, false)
				if err != nil || !slices.Equal(first, wantFirst) {
					t.Fatalf("first packet differs: %v", err)
				}
				// 72.5 ms requires legal 60 + 10 + 2.5 ms PLC calls. A maximum
				// decode followed by PCM trimming would corrupt the next packet.
				info.Missing, info.Duration48 = true, 3480
				concealed, err := decoder.Decode(nil, info)
				if err != nil {
					t.Fatal(err)
				}
				var want []int16
				for _, ticks := range []int{2880, 480, 120} {
					pcm, err := reference.Decode(nil, ticks*rate/48000, false)
					if err != nil {
						t.Fatal(err)
					}
					want = append(want, pcm...)
				}
				if len(concealed) != rate*29/400*channels || !slices.Equal(concealed, want) {
					t.Fatalf("PLC samples=%d, want %d matching the same decoder", len(concealed), len(want))
				}
				info.Missing = false
				// Sequence and timestamp gaps do not trigger a second jitter buffer.
				info.Header.SequenceNumber, info.Header.Timestamp = 17, 1
				got, err := decoder.Decode(packets[1], info)
				want, refErr := reference.Decode(packets[1], rate*3/25, false)
				if err != nil || refErr != nil || !slices.Equal(got, want) {
					t.Fatalf("decoder history after PLC differs: %v, %v", err, refErr)
				}
			})
		}
	}
}

func TestOpusPacketDecoderResetSourcesAndBoundaries(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	packets := buildOpusPackets(t, 16000, 1, 2)
	for _, reset := range []string{"source", "discontinuity", "BOS"} {
		t.Run(reset, func(t *testing.T) {
			decoder, err := NewOpusPacketDecoder(16000, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer decoder.Close()
			info := &packet.OpusInfo{SourceID: "old"}
			if _, err := decoder.Decode(packets[0], info); err != nil {
				t.Fatal(err)
			}
			old := decoder.decoder
			if pcm, err := decoder.Decode(nil, nil); err != nil || len(pcm) != 0 || decoder.decoder != old {
				t.Fatalf("unmarked empty boundary changed decoder: %v", err)
			}
			switch reset {
			case "source":
				info.SourceID = "new"
			case "discontinuity":
				if pcm, err := decoder.Decode(nil, &packet.OpusInfo{SourceID: "old", Discontinuity: true}); err != nil || len(pcm) != 0 {
					t.Fatalf("discontinuity synthesized audio: %v", err)
				}
			case "BOS":
				if err := decoder.Reset(); err != nil {
					t.Fatal(err)
				}
			}
			got, err := decoder.Decode(packets[1], info)
			fresh, newErr := opus.NewDecoder(16000, 1)
			if newErr != nil {
				t.Fatal(newErr)
			}
			defer fresh.Close()
			want, refErr := fresh.Decode(packets[1], 1920, false)
			if err != nil || refErr != nil || !slices.Equal(got, want) || decoder.decoder == old {
				t.Fatalf("old source contaminated replacement: %v, %v", err, refErr)
			}
		})
	}
}

func TestOpusPacketDecoderRejectsUnknownAndExcessiveGaps(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	decoder, err := NewOpusPacketDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	for _, ticks := range []uint32{0, 1, 24120, ^uint32(0)} {
		if _, err := decoder.Decode(nil, &packet.OpusInfo{Missing: true, Duration48: ticks}); err == nil {
			t.Fatalf("accepted invalid gap %d", ticks)
		}
	}
	if _, err := decoder.Decode([]byte{1}, &packet.OpusInfo{Missing: true, Duration48: 960}); err == nil {
		t.Fatal("accepted payload on missing packet")
	}
	if _, err := decoder.Decode(nil, &packet.OpusInfo{Missing: true, Duration48: 24000}); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.Decode(nil, &packet.OpusInfo{Missing: true, Duration48: 120}); err == nil {
		t.Fatal("accepted more than 500 ms of consecutive PLC")
	}
	if err := decoder.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.Decode([]byte{1}, nil); err == nil {
		t.Fatal("accepted decoding after Close")
	}
}

func TestOpusPacketDecoderCodecFreeLifecycle(t *testing.T) {
	decoder, err := NewOpusPacketDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range []*packet.OpusInfo{nil, {SourceID: "s"}, {SourceID: "s", Discontinuity: true}} {
		if pcm, err := decoder.Decode(nil, info); err != nil || len(pcm) != 0 || decoder.decoder != nil {
			t.Fatalf("empty control started codec: %d,%v", len(pcm), err)
		}
	}
	for _, data := range [][]byte{{0x9b}, {0x9b, 0x40}, {0x9b, 0x3f}} {
		if _, err := decoder.Decode(data, nil); err == nil || decoder.decoder != nil || errors.Is(err, ErrOpusDecoderInit) {
			t.Fatalf("invalid packet duration started codec: %x %v", data, err)
		}
	}
	if err := decoder.ResetDecoder(); err != nil || decoder.decoder != nil {
		t.Fatalf("idle recovery started codec: %v", err)
	}
	if err := decoder.Reset(); err != nil || decoder.decoder != nil {
		t.Fatalf("idle boundary started codec: %v", err)
	}
	if err := decoder.Close(); err != nil || decoder.decoder != nil {
		t.Fatalf("idle close started codec: %v", err)
	}
	for _, format := range [][2]int{{44100, 1}, {0, 1}, {16000, 0}, {16000, 3}} {
		if _, err := NewOpusPacketDecoder(format[0], format[1]); err == nil {
			t.Fatalf("accepted invalid PCM format %v", format)
		}
	}
}

func TestOpusPacketDecoderUnavailableStartup(t *testing.T) {
	if opus.IsRuntimeSupported() {
		t.Skip("requires unavailable native runtime")
	}
	decoder, err := NewOpusPacketDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	if _, err := decoder.Decode([]byte{0xf8, 0xff, 0xfe}, nil); !errors.Is(err, ErrOpusDecoderInit) {
		t.Fatalf("real media startup error lost: %v", err)
	}
	if _, err := decoder.Decode(nil, &packet.OpusInfo{Missing: true, Duration48: 960}); !errors.Is(err, ErrOpusDecoderInit) {
		t.Fatalf("PLC startup error lost: %v", err)
	}
}

func TestOpusPacketDecoderUsesEncodedPacketCapacity(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	for _, rate := range []int{8000, 12000, 16000, 24000, 48000} {
		for _, channels := range []int{1, 2} {
			packets := buildOpusPackets(t, rate, channels, 2)
			decoder, err := NewOpusPacketDecoder(rate, channels)
			if err != nil {
				t.Fatal(err)
			}
			for _, data := range packets {
				// Metadata cannot inflate native output capacity beyond the TOC.
				pcm, err := decoder.Decode(data, &packet.OpusInfo{Duration48: 5760})
				if err != nil || len(pcm) != rate/50*channels || cap(pcm) != len(pcm) {
					t.Fatalf("%d Hz/%d channels: len=%d cap=%d err=%v", rate, channels, len(pcm), cap(pcm), err)
				}
			}
			_ = decoder.Close()
		}
	}
	decoder, err := NewOpusPacketDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	// Legal code-3 packets with zero-byte compressed frames cover durations
	// that are valid decode capacities but not individual encoder frame sizes.
	for _, data := range [][]byte{{0x83, 3}, {0x9b, 4}, {0x9b, 5}, {0x9b, 6}} {
		pcm, err := decoder.Decode(data, nil)
		want := int(packet.OpusDuration48(data)) * 16000 / 48000
		if err != nil || len(pcm) != want || cap(pcm) != want {
			t.Fatalf("packet %x: len=%d cap=%d want=%d err=%v", data, len(pcm), cap(pcm), want, err)
		}
	}
}

func TestOpusPacketDecoderRecoveryPreservesSourceBudget(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}
	packets := buildOpusPackets(t, 16000, 1, 2)
	decoder, err := NewOpusPacketDecoder(16000, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	info := &packet.OpusInfo{SourceID: "s"}
	if _, err := decoder.Decode(packets[0], info); err != nil {
		t.Fatal(err)
	}
	gap := &packet.OpusInfo{SourceID: "s", Missing: true, Duration48: 24000}
	if _, err := decoder.Decode(nil, gap); err != nil {
		t.Fatal(err)
	}
	if err := decoder.ResetDecoder(); err != nil || decoder.decoder != nil || !decoder.haveSource || decoder.sourceID != "s" || decoder.missing48 != 24000 {
		t.Fatalf("recovery discarded source/budget: %+v, %v", decoder, err)
	}
	gap.Duration48 = 120
	if _, err := decoder.Decode(nil, gap); err == nil || decoder.decoder != nil {
		t.Fatalf("exhausted PLC restarted a codec: %v", err)
	}
	if _, err := decoder.Decode([]byte{0xff}, info); err == nil || decoder.missing48 != 24000 {
		t.Fatalf("unknown corruption cleared budget: %v", err)
	}
	if _, err := decoder.Decode(packets[1], info); err != nil || decoder.missing48 != 0 {
		t.Fatalf("successful real decode did not renew budget: %v", err)
	}
	if _, err := decoder.Decode(nil, gap); err != nil {
		t.Fatalf("good real packet did not restore PLC: %v", err)
	}
	if err := decoder.Reset(); err != nil || decoder.decoder != nil || decoder.haveSource || decoder.missing48 != 0 {
		t.Fatalf("explicit boundary did not clear state: %+v, %v", decoder, err)
	}
}
