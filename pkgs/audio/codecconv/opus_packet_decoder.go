package codecconv

import (
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
)

// ErrOpusDecoderInit identifies a native decoder startup failure, rather than a
// corrupt packet. Decoder startup is deferred until actual media needs it.
var ErrOpusDecoderInit = errors.New("opus packet decoder initialization")

// OpusPacketDecoder decodes already ordered raw Opus packets and explicit gaps.
// One consumer owns it; Decode, Reset and Close must not run concurrently. It
// buffers no packets and adds no playout delay. Transport owns jitter ordering.
type OpusPacketDecoder struct {
	decoder    *opus.Decoder
	sampleRate int
	channels   int
	sourceID   string
	haveSource bool
	missing48  uint32
	closed     bool
}

// NewOpusPacketDecoder validates the PCM format without starting a native codec.
// The first real packet or valid concealment request creates the decoder.
func NewOpusPacketDecoder(sampleRate, channels int) (*OpusPacketDecoder, error) {
	if err := opus.OpusSampleRate(sampleRate).Validate(); err != nil {
		return nil, err
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("opus: invalid channel count %d (allowed: 1 or 2)", channels)
	}
	return &OpusPacketDecoder{sampleRate: sampleRate, channels: channels}, nil
}

// Decode returns interleaved PCM. Empty data without Missing is a boundary, not
// a concealment request. Missing durations use the fixed 48 kHz RTP clock and
// must be multiples of 2.5 ms. Consecutive concealment is bounded to 500 ms;
// larger or unknown jumps must instead be marked Discontinuity by the producer.
// Source changes and Discontinuity discard the preceding decoder history.
func (d *OpusPacketDecoder) Decode(data []byte, info *packet.OpusInfo) ([]int16, error) {
	if d == nil || d.closed {
		return nil, fmt.Errorf("opus packet decoder is closed")
	}
	if info != nil && info.Missing {
		if len(data) != 0 || info.Discontinuity {
			return nil, fmt.Errorf("opus missing packet requires empty data and no discontinuity")
		}
		if info.Duration48 == 0 || info.Duration48%120 != 0 || info.Duration48 > packet.OpusClockRate/2 {
			return nil, fmt.Errorf("opus missing duration %d is unknown, invalid, or exceeds 500 ms", info.Duration48)
		}
	}
	if info != nil {
		if info.Discontinuity || (d.haveSource && d.sourceID != info.SourceID) {
			if err := d.Reset(); err != nil {
				return nil, err
			}
		}
		d.sourceID, d.haveSource = info.SourceID, true
	}
	if len(data) == 0 && (info == nil || !info.Missing) {
		return nil, nil
	}
	missing := info != nil && info.Missing
	var frameSize int
	if missing {
		if d.missing48+info.Duration48 > packet.OpusClockRate/2 {
			return nil, fmt.Errorf("opus consecutive missing duration exceeds 500 ms; discontinuity required")
		}
	} else {
		// Derive capacity from the encoded packet, never from untrusted timing
		// metadata. Code-3 packets must supply their nonzero frame count; the
		// duration helper's compatibility fallback is not a known duration.
		if data[0]&3 == 3 && (len(data) < 2 || data[1]&0x3f == 0) {
			return nil, fmt.Errorf("opus packet has no frame count")
		}
		ticks := packet.OpusDuration48(data)
		if ticks == 0 || ticks > 5760 || ticks%120 != 0 {
			return nil, fmt.Errorf("opus packet duration %d is invalid or exceeds 120 ms", ticks)
		}
		frameSize = int(ticks) * d.sampleRate / packet.OpusClockRate
	}
	if d.decoder == nil {
		decoder, err := opus.NewDecoder(d.sampleRate, d.channels)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrOpusDecoderInit, err)
		}
		d.decoder = decoder
	}
	if !missing {
		samples, err := d.decoder.Decode(data, frameSize, false)
		if err != nil {
			return nil, err
		}
		d.missing48 = 0
		return samples, nil
	}
	// libopus requires exact PLC frame sizes. Split the gap into legal durations
	// rather than decoding one maximum-size frame and trimming its PCM afterward.
	remaining := info.Duration48
	samples := make([]int16, 0, int(remaining)*d.sampleRate/packet.OpusClockRate*d.channels)
	for _, ticks := range [...]uint32{2880, 1920, 960, 480, 240, 120} {
		for remaining >= ticks {
			frameSize := int(ticks) * d.sampleRate / packet.OpusClockRate
			frame, err := d.decoder.Decode(nil, frameSize, false)
			if err != nil {
				return nil, err
			}
			if len(frame) != frameSize*d.channels {
				return nil, fmt.Errorf("opus PLC returned %d samples, want %d", len(frame), frameSize*d.channels)
			}
			samples = append(samples, frame...)
			remaining -= ticks
		}
	}
	d.missing48 += info.Duration48
	return samples, nil
}

// ResetDecoder releases native codec history after a decode failure. It keeps
// source identity and the consecutive concealment budget: corruption is not a
// stream boundary. The next media packet recreates the native decoder.
func (d *OpusPacketDecoder) ResetDecoder() error {
	if d == nil || d.closed || d.decoder == nil {
		return nil
	}
	err := d.decoder.Close()
	d.decoder = nil
	return err
}

// Reset releases the current source's decoder history and concealment budget.
// The next packet creates a fresh decoder; a boundary does not synthesize PCM.
func (d *OpusPacketDecoder) Reset() error {
	if d == nil || d.closed {
		return nil
	}
	err := d.ResetDecoder()
	d.sourceID, d.haveSource, d.missing48 = "", false, 0
	return err
}

// Close releases the native decoder and prevents further decoding.
func (d *OpusPacketDecoder) Close() error {
	if d == nil || d.closed {
		return nil
	}
	err := d.Reset()
	d.closed = true
	return err
}
